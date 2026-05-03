package core

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// NetworkApprovalProvider asks the user or a surface whether a non-allowlisted
// outbound request should be allowed in per-request mode.
type NetworkApprovalProvider interface {
	ApproveNetworkRequest(context.Context, contracts.NetworkRequest) (bool, string, error)
}

// NetworkSandboxConfig controls network access for sandboxed agent actions.
type NetworkSandboxConfig struct {
	Mode         contracts.NetworkMode
	Allowlist    []string
	PortForwards []contracts.PortForward
	Approval     NetworkApprovalProvider
	Now          func() time.Time
}

// NetworkSandbox evaluates outbound and inbound network requests and records
// the DNS/traffic audit trail required by the security policy.
type NetworkSandbox struct {
	config NetworkSandboxConfig
	events []contracts.NetworkEvent
}

// DefaultNetworkSandboxConfig returns Conduit's restricted-by-default network
// policy from PRD section 15.3.
func DefaultNetworkSandboxConfig() NetworkSandboxConfig {
	return NetworkSandboxConfig{
		Mode: contracts.NetworkModeRestricted,
		Allowlist: []string{
			"pypi.org",
			"files.pythonhosted.org",
			"registry.npmjs.org",
			"npmjs.com",
			"yarnpkg.com",
			"proxy.golang.org",
			"sum.golang.org",
			"crates.io",
			"index.crates.io",
			"static.crates.io",
			"github.com",
			"api.github.com",
			"anthropic.com",
			"api.anthropic.com",
			"openai.com",
			"api.openai.com",
			"ollama.com",
			"openrouter.ai",
		},
	}
}

// NewNetworkSandbox creates a network policy evaluator.
func NewNetworkSandbox(config NetworkSandboxConfig) *NetworkSandbox {
	if config.Mode == "" {
		config.Mode = contracts.NetworkModeRestricted
	}
	if len(config.Allowlist) == 0 {
		config.Allowlist = DefaultNetworkSandboxConfig().Allowlist
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	return &NetworkSandbox{config: config}
}

// CheckOutbound evaluates an outbound connection request.
func (s *NetworkSandbox) CheckOutbound(ctx context.Context, req contracts.NetworkRequest) contracts.NetworkDecision {
	req.Direction = contracts.NetworkDirectionOutbound
	req.Host = normalizeNetworkHost(req.Host, req.URL)
	decision := s.outboundDecision(ctx, req)
	s.logDNS(req)
	s.logTraffic(req, decision)
	return decision
}

// CheckInbound evaluates an inbound request against explicit port forwards.
func (s *NetworkSandbox) CheckInbound(req contracts.NetworkRequest) contracts.NetworkDecision {
	req.Direction = contracts.NetworkDirectionInbound
	req.Host = normalizeNetworkHost(req.Host, req.URL)
	decision := s.inboundDecision(req)
	s.logTraffic(req, decision)
	return decision
}

// Events returns a copy of the network audit log.
func (s *NetworkSandbox) Events() []contracts.NetworkEvent {
	return append([]contracts.NetworkEvent(nil), s.events...)
}

// NetworkSandbox returns the engine-owned network policy evaluator.
func (e *Engine) NetworkSandbox() *NetworkSandbox {
	return e.network
}

func (s *NetworkSandbox) outboundDecision(ctx context.Context, req contracts.NetworkRequest) contracts.NetworkDecision {
	base := contracts.NetworkDecision{Mode: s.config.Mode, Host: req.Host, Port: req.Port}
	if req.Host == "" {
		base.Reason = "network request requires a host"
		return base
	}

	switch s.config.Mode {
	case contracts.NetworkModeOffline:
		base.Reason = "offline mode blocks outbound network"
		return base
	case contracts.NetworkModeOpen:
		base.Allowed = true
		base.Reason = "open mode allows outbound network"
		return base
	case contracts.NetworkModeRestricted:
		if s.allowlisted(req.Host) {
			base.Allowed = true
			base.Reason = "host matches network allowlist"
			return base
		}
		base.Reason = "host is not in the restricted-mode allowlist"
		return base
	case contracts.NetworkModePerRequest:
		if s.allowlisted(req.Host) {
			base.Allowed = true
			base.Reason = "host matches network allowlist"
			return base
		}
		if s.config.Approval == nil {
			base.Reason = "per-request mode requires an approval provider"
			return base
		}
		allowed, reason, err := s.config.Approval.ApproveNetworkRequest(ctx, req)
		if err != nil {
			base.Reason = fmt.Sprintf("network approval failed: %v", err)
			return base
		}
		base.Allowed = allowed
		base.Reason = strings.TrimSpace(reason)
		if base.Reason == "" {
			if allowed {
				base.Reason = "approved by per-request policy"
			} else {
				base.Reason = "denied by per-request policy"
			}
		}
		return base
	default:
		base.Reason = fmt.Sprintf("unknown network mode %q", s.config.Mode)
		return base
	}
}

func (s *NetworkSandbox) inboundDecision(req contracts.NetworkRequest) contracts.NetworkDecision {
	decision := contracts.NetworkDecision{
		Allowed: false,
		Reason:  "inbound network is blocked unless a port forward matches",
		Mode:    s.config.Mode,
		Host:    req.Host,
		Port:    req.Port,
	}
	for _, forward := range s.config.PortForwards {
		if forward.ListenPort == req.Port && protocolMatches(forward.Protocol, req.Protocol) {
			decision.Allowed = true
			decision.Reason = "explicit port forward allows inbound network"
			return decision
		}
	}
	return decision
}

func (s *NetworkSandbox) allowlisted(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for _, entry := range s.config.Allowlist {
		entry = strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(entry), "."), "*.")
		if host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}

func (s *NetworkSandbox) logDNS(req contracts.NetworkRequest) {
	if req.Host == "" || net.ParseIP(req.Host) != nil {
		return
	}
	if slices.ContainsFunc(s.events, func(event contracts.NetworkEvent) bool {
		return event.Kind == "dns" && event.Host == req.Host
	}) {
		return
	}
	s.events = append(s.events, contracts.NetworkEvent{
		At:        s.config.Now(),
		Kind:      "dns",
		Direction: req.Direction,
		Host:      req.Host,
		Protocol:  req.Protocol,
		Allowed:   true,
		Reason:    "dns lookup recorded",
	})
}

func (s *NetworkSandbox) logTraffic(req contracts.NetworkRequest, decision contracts.NetworkDecision) {
	s.events = append(s.events, contracts.NetworkEvent{
		At:        s.config.Now(),
		Kind:      "traffic",
		Direction: req.Direction,
		Host:      req.Host,
		Port:      req.Port,
		Protocol:  req.Protocol,
		Allowed:   decision.Allowed,
		Reason:    decision.Reason,
	})
}

func normalizeNetworkHost(host, rawURL string) string {
	if host == "" && rawURL != "" {
		parsed, err := url.Parse(rawURL)
		if err == nil {
			host = parsed.Hostname()
		}
	}
	host = strings.TrimSpace(strings.ToLower(host))
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return strings.TrimSuffix(host, ".")
}

func protocolMatches(forwardProtocol, requestProtocol string) bool {
	forwardProtocol = strings.ToLower(strings.TrimSpace(forwardProtocol))
	requestProtocol = strings.ToLower(strings.TrimSpace(requestProtocol))
	return forwardProtocol == "" || requestProtocol == "" || forwardProtocol == requestProtocol
}
