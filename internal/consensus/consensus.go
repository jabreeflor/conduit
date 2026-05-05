package consensus

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// ConsensusMode represents the strategy for selecting the winning response
type ConsensusMode string

const (
	ModeMajority ConsensusMode = "majority"
	ModeRanked   ConsensusMode = "ranked"
	ModeWeighted ConsensusMode = "weighted"
)

// ModelEndpoint represents a single AI model endpoint
type ModelEndpoint struct {
	Name  string
	URL   string
	Tier  int    // 1=basic, 2=standard, 3=premium
	Model string
}

// DeliberationRequest represents a request for consensus deliberation
type DeliberationRequest struct {
	Prompt   string
	Models   []ModelEndpoint
	Timeout  time.Duration
	Metadata map[string]string
}

// DeliberationResponse represents a single model's response
type DeliberationResponse struct {
	ModelName  string
	Response   string
	Confidence float64
	Duration   time.Duration
	Error      string
}

// Consensus represents the final consensus result
type Consensus struct {
	WinningResponse string
	WinningModel    string
	Agreement       float64
	AllResponses    []DeliberationResponse
	Strategy        string
	Message         string
}

// Engine handles consensus deliberation across multiple models
type Engine struct {
	endpoints []ModelEndpoint
	mode      ConsensusMode
}

// NewEngine creates a new consensus engine
func NewEngine(endpoints []ModelEndpoint, mode ConsensusMode) *Engine {
	return &Engine{
		endpoints: endpoints,
		mode:      mode,
	}
}

// Run executes the consensus deliberation
func (e *Engine) Run(ctx context.Context, req DeliberationRequest) (*Consensus, error) {
	if len(req.Models) == 0 {
		return nil, fmt.Errorf("no models provided for deliberation")
	}

	// Set default timeout if not specified
	if req.Timeout == 0 {
		req.Timeout = 30 * time.Second
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	responses := make([]DeliberationResponse, 0, len(req.Models))
	responseMutex := &sync.Mutex{}
	wg := &sync.WaitGroup{}

	// Fan out to all models concurrently
	for _, endpoint := range req.Models {
		wg.Add(1)
		go func(ep ModelEndpoint) {
			defer wg.Done()
			start := time.Now()
			resp := DeliberationResponse{
				ModelName: ep.Name,
			}

			// Make request to the model endpoint
			response, err := makeRequest(ctx, ep.URL, req.Prompt)
			if err != nil {
				resp.Error = err.Error()
			} else {
				resp.Response = response
				resp.Confidence = estimateConfidence(response)
			}
			resp.Duration = time.Since(start)

			responseMutex.Lock()
			responses = append(responses, resp)
			responseMutex.Unlock()
		}(endpoint)
	}

	wg.Wait()

	// Filter out responses with errors
	validResponses := filterValidResponses(responses)
	if len(validResponses) == 0 {
		return nil, fmt.Errorf("all models returned errors")
	}

	// Pick the winner based on the consensus mode
	winner := e.pickWinner(validResponses)
	if winner == nil {
		return nil, fmt.Errorf("failed to select winning response")
	}

	// Calculate agreement metric
	agreement := calculateAgreement(validResponses)

	return &Consensus{
		WinningResponse: winner.Response,
		WinningModel:    winner.ModelName,
		Agreement:       agreement,
		AllResponses:    responses,
		Strategy:        string(e.mode),
		Message:         fmt.Sprintf("Consensus reached using %s strategy (agreement: %.2f%%)", e.mode, agreement*100),
	}, nil
}

// pickWinner selects the winning response based on the consensus mode
func (e *Engine) pickWinner(responses []DeliberationResponse) *DeliberationResponse {
	switch e.mode {
	case ModeMajority:
		return pickByMajority(responses)
	case ModeRanked:
		return pickByRanked(responses)
	case ModeWeighted:
		return pickByWeighted(responses)
	default:
		return pickByRanked(responses)
	}
}

// pickByMajority selects the most common response
func pickByMajority(responses []DeliberationResponse) *DeliberationResponse {
	if len(responses) == 0 {
		return nil
	}
	if len(responses) == 1 {
		return &responses[0]
	}

	// Count occurrences of each response
	counts := make(map[string]*DeliberationResponse)
	for i := range responses {
		normalized := strings.TrimSpace(strings.ToLower(responses[i].Response))
		if counts[normalized] == nil {
			counts[normalized] = &responses[i]
		}
	}

	// Find the most common response
	maxCount := 0
	var winner *DeliberationResponse
	for _, resp := range counts {
		count := countMatches(responses, resp.Response)
		if count > maxCount {
			maxCount = count
			winner = resp
		}
	}

	return winner
}

// pickByRanked selects the response with highest confidence
func pickByRanked(responses []DeliberationResponse) *DeliberationResponse {
	if len(responses) == 0 {
		return nil
	}

	best := &responses[0]
	for i := 1; i < len(responses); i++ {
		if responses[i].Confidence > best.Confidence {
			best = &responses[i]
		}
	}
	return best
}

// pickByWeighted selects based on confidence weighted by model tier
func pickByWeighted(responses []DeliberationResponse) *DeliberationResponse {
	if len(responses) == 0 {
		return nil
	}

	// Find corresponding tier for each response
	tiers := make(map[string]int)
	for _, endpoint := range getAllEndpoints(responses) {
		tiers[endpoint.Name] = endpoint.Tier
	}

	best := &responses[0]
	bestScore := best.Confidence * float64(getModelTier(best.ModelName, tiers))

	for i := 1; i < len(responses); i++ {
		score := responses[i].Confidence * float64(getModelTier(responses[i].ModelName, tiers))
		if score > bestScore {
			best = &responses[i]
			bestScore = score
		}
	}
	return best
}

// calculateAgreement calculates the agreement metric using Jaccard similarity
func calculateAgreement(responses []DeliberationResponse) float64 {
	if len(responses) <= 1 {
		return 1.0
	}

	totalSimilarity := 0.0
	count := 0

	for i := 0; i < len(responses); i++ {
		for j := i + 1; j < len(responses); j++ {
			totalSimilarity += stringSimilarity(responses[i].Response, responses[j].Response)
			count++
		}
	}

	if count == 0 {
		return 0.0
	}

	return totalSimilarity / float64(count)
}

// stringSimilarity calculates Jaccard similarity between two strings
func stringSimilarity(a, b string) float64 {
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))

	// Build sets
	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}
	setB := make(map[string]bool)
	for _, w := range wordsB {
		setB[w] = true
	}

	// Calculate intersection and union
	intersection := 0
	for word := range setA {
		if setB[word] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// estimateConfidence estimates confidence based on linguistic patterns
func estimateConfidence(response string) float64 {
	confidence := 0.5 // Base confidence

	// Hedging language penalties
	hedges := []string{"maybe", "perhaps", "possibly", "might", "could", "seems", "appears"}
	for _, hedge := range hedges {
		if strings.Contains(strings.ToLower(response), hedge) {
			confidence -= 0.05
		}
	}

	// Definitive language rewards
	definitive := []string{"definitely", "certainly", "clearly", "obviously", "must", "will", "cannot"}
	for _, word := range definitive {
		if strings.Contains(strings.ToLower(response), word) {
			confidence += 0.05
		}
	}

	// Clamp confidence to [0, 1]
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return confidence
}

// filterValidResponses removes responses with errors
func filterValidResponses(responses []DeliberationResponse) []DeliberationResponse {
	valid := make([]DeliberationResponse, 0, len(responses))
	for _, r := range responses {
		if r.Error == "" {
			valid = append(valid, r)
		}
	}
	return valid
}

// Helper functions

func countMatches(responses []DeliberationResponse, response string) int {
	count := 0
	normalized := strings.TrimSpace(strings.ToLower(response))
	for _, r := range responses {
		if strings.TrimSpace(strings.ToLower(r.Response)) == normalized {
			count++
		}
	}
	return count
}

func getAllEndpoints(responses []DeliberationResponse) []ModelEndpoint {
	// This is a simplified helper - in practice, endpoints would be passed through
	endpoints := make([]ModelEndpoint, 0)
	for _, r := range responses {
		endpoints = append(endpoints, ModelEndpoint{Name: r.ModelName, Tier: 2})
	}
	return endpoints
}

func getModelTier(modelName string, tiers map[string]int) int {
	if tier, ok := tiers[modelName]; ok {
		return tier
	}
	return 2 // Default to standard tier
}

// makeRequest is a placeholder for making HTTP requests to model endpoints
func makeRequest(ctx context.Context, url string, prompt string) (string, error) {
	// This would be implemented to make actual HTTP requests
	// For now, return a placeholder
	return "mock response", nil
}
