package router

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/cache"
	"github.com/jabreeflor/conduit/internal/contextassembler"
	conduiterrors "github.com/jabreeflor/conduit/internal/errors"
)

// Provider is the narrow adapter every model integration implements.
type Provider interface {
	Name() string
	Infer(context.Context, Request) (Response, error)
}

// Request is the unified inference API consumed by the rest of the engine.
type Request struct {
	SessionID    string
	CheckpointID string
	TaskType     TaskType
	Feature      string
	Plugin       string
	Inputs       []Input
	Prompt       string
}

// Input is an inference request payload.
type Input struct {
	Type      InputType
	Ref       string
	Text      string
	Data      string // base64-encoded content for image and PDF inputs
	MediaType string // MIME type, e.g. "image/png", "application/pdf"
}

// Response is the normalized provider result returned by the router.
type Response struct {
	Provider     string
	Model        string
	Text         string
	Usage        Usage
	CheckpointID string
}

// Usage records token and cost data for one completed inference.
type Usage struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// Checkpoint identifies the most recent resumable state for a session.
type Checkpoint struct {
	SessionID string
	ID        string
}

// CheckpointStore supplies resume points after provider failures.
type CheckpointStore interface {
	LastCheckpoint(context.Context, string) (Checkpoint, error)
}

// UsageSink receives per-provider accounting after successful calls.
type UsageSink interface {
	RecordUsage(context.Context, UsageRecord) error
}

// UsageRecord is a durable accounting event.
type UsageRecord struct {
	SessionID    string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	TTFT         time.Duration
	TotalLatency time.Duration
	Status       string
	ErrorType    string
	Feature      string
	Plugin       string
	RecordedAt   time.Time
}

// FailoverSink receives provider failure events before the router tries the
// next candidate.
type FailoverSink interface {
	RecordFailover(context.Context, FailoverEvent) error
}

// ContextAssembler optimizes raw request context before provider inference.
type ContextAssembler interface {
	Assemble(contextassembler.Request) contextassembler.Result
}

// OptimizationSink receives context assembler transparency reports.
type OptimizationSink interface {
	RecordContextOptimization(context.Context, contextassembler.Summary) error
}

// FailoverEvent describes a failed attempt and the checkpoint used to resume.
type FailoverEvent struct {
	SessionID    string
	FromProvider string
	ToProvider   string
	CheckpointID string
	Err          error
	RecordedAt   time.Time
}

// Router owns provider selection, failover, and accounting.
type Router struct {
	cfg           Config
	providers     map[string]Provider
	metadata      map[string]ProviderConfig
	checkpoints   CheckpointStore
	usage         UsageSink
	failovers     FailoverSink
	responses     *cache.ResponseCache[Response]
	assembler     ContextAssembler
	optimizations OptimizationSink
	now           func() time.Time
}

// Option customizes a router.
type Option func(*Router)

// WithCheckpointStore configures the store used for failover resume.
func WithCheckpointStore(store CheckpointStore) Option {
	return func(r *Router) {
		r.checkpoints = store
	}
}

// WithUsageSink configures usage accounting.
func WithUsageSink(sink UsageSink) Option {
	return func(r *Router) {
		r.usage = sink
	}
}

// WithFailoverSink configures failover event accounting.
func WithFailoverSink(sink FailoverSink) Option {
	return func(r *Router) {
		r.failovers = sink
	}
}

// WithResponseCache enables exact-match response caching for identical
// inference requests.
func WithResponseCache(c *cache.ResponseCache[Response]) Option {
	return func(r *Router) {
		r.responses = c
	}
}

// WithContextAssembler configures the first-class pre-call context optimizer.
func WithContextAssembler(assembler ContextAssembler) Option {
	return func(r *Router) {
		r.assembler = assembler
	}
}

// WithOptimizationSink records context assembler transparency events.
func WithOptimizationSink(sink OptimizationSink) Option {
	return func(r *Router) {
		r.optimizations = sink
	}
}

// New creates a model router from config and provider adapters.
func New(cfg Config, providers []Provider, opts ...Option) (*Router, error) {
	if cfg.Models.Primary == "" {
		return nil, errors.New("router requires a primary model")
	}

	r := &Router{
		cfg:       cfg,
		providers: make(map[string]Provider, len(providers)),
		metadata:  providerConfigs(cfg),
		now:       func() time.Time { return time.Now().UTC() },
	}
	for _, provider := range providers {
		r.providers[provider.Name()] = provider
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

// Infer routes a request through the preferred provider and configured
// fallbacks, returning the first successful response.
func (r *Router) Infer(ctx context.Context, req Request) (Response, error) {
	req = r.assembleContext(ctx, req)
	chain := r.route(req)
	if len(chain) == 0 {
		return Response{}, errors.New("no providers configured")
	}
	cacheKey, cacheable := r.responseCacheKey(req, chain)
	if cacheable {
		if resp, ok := r.responses.Get(cacheKey); ok {
			return resp, nil
		}
	}

	var failures []error
	for i, name := range chain {
		provider, ok := r.providers[name]
		if !ok {
			failures = append(failures, fmt.Errorf("%s: provider adapter unavailable", name))
			continue
		}

		attemptReq := req
		if i > 0 && r.checkpoints != nil && req.SessionID != "" {
			checkpoint, err := r.checkpoints.LastCheckpoint(ctx, req.SessionID)
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: load checkpoint: %w", name, err))
				continue
			}
			attemptReq.CheckpointID = checkpoint.ID
		}

		attemptCtx, cancel := r.contextForProvider(ctx, name)
		startedAt := r.now()
		resp, err := provider.Infer(attemptCtx, attemptReq)
		totalLatency := r.now().Sub(startedAt)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
			r.recordUsageFailure(ctx, req, name, totalLatency, err)
			r.recordFailover(ctx, req.SessionID, name, nextProvider(chain, i), attemptReq.CheckpointID, err)
			continue
		}

		resp.Provider = name
		if resp.Model == "" {
			resp.Model = r.metadata[name].Model
		}
		if resp.CheckpointID == "" {
			resp.CheckpointID = attemptReq.CheckpointID
		}
		resp.Usage.CostUSD = r.costFor(name, resp.Usage)
		r.recordUsage(ctx, req, resp, totalLatency)
		if cacheable {
			r.responses.Put(cacheKey, resp)
		}
		return resp, nil
	}

	return Response{}, fmt.Errorf("all providers failed: %w", errors.Join(failures...))
}

func (r *Router) assembleContext(ctx context.Context, req Request) Request {
	if r.assembler == nil {
		return req
	}
	result := r.assembler.Assemble(contextassembler.Request{
		Query: req.Prompt,
		Items: routerContextItems(req),
	})
	if result.Prompt == "" {
		return req
	}
	req.Prompt = result.Prompt
	if r.optimizations != nil {
		_ = r.optimizations.RecordContextOptimization(ctx, result.Summary)
	}
	return req
}

func routerContextItems(req Request) []contextassembler.Item {
	items := []contextassembler.Item{{
		ID:       "prompt",
		Category: contextassembler.CategoryRecent,
		Content:  req.Prompt,
		Pinned:   true,
	}}
	for i, input := range req.Inputs {
		item := contextassembler.Item{
			ID:       fmt.Sprintf("input-%d", i),
			Category: contextassembler.CategoryMultimodal,
			Path:     input.Ref,
			Metadata: map[string]string{
				"type":       string(input.Type),
				"media_type": input.MediaType,
			},
		}
		switch {
		case input.Text != "":
			item.Content = input.Text
		case input.Ref != "":
			item.Content = input.Ref
		case input.Data != "":
			item.Content = fmt.Sprintf("[%s data omitted from text prompt]", input.MediaType)
		default:
			item.Content = fmt.Sprintf("[%s input]", input.Type)
		}
		items = append(items, item)
	}
	return items
}

func (r *Router) responseCacheKey(req Request, chain []string) (string, bool) {
	if r.responses == nil {
		return "", false
	}
	key, err := cache.Key("router-response", struct {
		Request Request
		Chain   []string
	}{Request: req, Chain: chain})
	if err != nil {
		return "", false
	}
	return key, true
}

func (r *Router) contextForProvider(ctx context.Context, name string) (context.Context, context.CancelFunc) {
	timeout := r.metadata[name].Timeout
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func (r *Router) route(req Request) []string {
	var chain []string
	seen := map[string]bool{}

	add := func(name string) {
		name = r.providerName(name)
		if name == "" || seen[name] {
			return
		}
		if r.requestNeeds(req, CapabilityVision) && !r.providerHas(name, CapabilityVision) {
			return
		}
		seen[name] = true
		chain = append(chain, name)
	}

	for _, rule := range r.cfg.Models.RoutingRules {
		if rule.Prefer == "" {
			continue
		}
		if rule.TaskType != "" && rule.TaskType != req.TaskType {
			continue
		}
		if rule.InputType != "" && !requestHasInput(req, rule.InputType) {
			continue
		}
		if rule.RequireCapability != "" && !r.providerHas(rule.Prefer, rule.RequireCapability) {
			continue
		}
		add(rule.Prefer)
	}

	if req.TaskType == TaskBrowser && r.cfg.Models.ComputerUse != "" {
		add(r.cfg.Models.ComputerUse)
	}
	add(r.cfg.Models.Primary)
	for _, fallback := range r.cfg.Models.Fallbacks {
		add(fallback)
	}

	return chain
}

func (r *Router) providerHas(name string, capability Capability) bool {
	name = r.providerName(name)
	for _, candidate := range r.metadata[name].Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

func (r *Router) providerName(name string) string {
	if meta, ok := r.metadata[name]; ok && meta.Name != "" {
		return meta.Name
	}
	return name
}

func (r *Router) requestNeeds(req Request, capability Capability) bool {
	if capability != CapabilityVision {
		return false
	}
	return requestHasInput(req, InputImage) || requestHasInput(req, InputPDF)
}

func requestHasInput(req Request, inputType InputType) bool {
	for _, input := range req.Inputs {
		if input.Type == inputType {
			return true
		}
	}
	return false
}

func (r *Router) costFor(provider string, usage Usage) float64 {
	meta := r.metadata[provider]
	if usage.CostUSD > 0 {
		return usage.CostUSD
	}
	return (float64(usage.InputTokens)/1000)*meta.InputCostPer1KUSD +
		(float64(usage.OutputTokens)/1000)*meta.OutputCostPer1KUSD
}

func (r *Router) recordUsage(ctx context.Context, req Request, resp Response, totalLatency time.Duration) {
	if r.usage == nil {
		return
	}
	_ = r.usage.RecordUsage(ctx, UsageRecord{
		SessionID:    req.SessionID,
		Provider:     resp.Provider,
		Model:        resp.Model,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CostUSD:      resp.Usage.CostUSD,
		TotalLatency: totalLatency,
		Status:       "success",
		Feature:      req.Feature,
		Plugin:       req.Plugin,
		RecordedAt:   r.now(),
	})
}

func (r *Router) recordUsageFailure(ctx context.Context, req Request, provider string, totalLatency time.Duration, err error) {
	if r.usage == nil {
		return
	}
	_ = r.usage.RecordUsage(ctx, UsageRecord{
		SessionID:    req.SessionID,
		Provider:     provider,
		Model:        r.metadata[provider].Model,
		TotalLatency: totalLatency,
		Status:       "error",
		ErrorType:    errorType(err),
		Feature:      req.Feature,
		Plugin:       req.Plugin,
		RecordedAt:   r.now(),
	})
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return string(conduiterrors.Classify(err))
}

func (r *Router) recordFailover(ctx context.Context, sessionID, from, to, checkpointID string, err error) {
	if r.failovers == nil || to == "" {
		return
	}
	_ = r.failovers.RecordFailover(ctx, FailoverEvent{
		SessionID:    sessionID,
		FromProvider: from,
		ToProvider:   to,
		CheckpointID: checkpointID,
		Err:          err,
		RecordedAt:   r.now(),
	})
}

func nextProvider(chain []string, index int) string {
	if index+1 >= len(chain) {
		return ""
	}
	return chain[index+1]
}

// MemoryUsageSink is a concurrency-safe in-memory UsageSink for tests and
// early surfaces that have not attached durable storage yet.
type MemoryUsageSink struct {
	mu      sync.Mutex
	Records []UsageRecord
}

// RecordUsage stores a usage event.
func (s *MemoryUsageSink) RecordUsage(_ context.Context, record UsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Records = append(s.Records, record)
	return nil
}

// MemoryFailoverSink is a concurrency-safe in-memory FailoverSink.
type MemoryFailoverSink struct {
	mu     sync.Mutex
	Events []FailoverEvent
}

// RecordFailover stores a failover event.
func (s *MemoryFailoverSink) RecordFailover(_ context.Context, event FailoverEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, event)
	return nil
}
