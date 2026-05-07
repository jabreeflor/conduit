package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestDefaultNetworkSandboxRestrictsOutboundToAllowlist(t *testing.T) {
	sandbox := NewNetworkSandbox(DefaultNetworkSandboxConfig())

	allowed := sandbox.CheckOutbound(context.Background(), contracts.NetworkRequest{
		URL:      "https://api.github.com/repos/jabreeflor/conduit",
		Port:     443,
		Protocol: "tcp",
	})
	blocked := sandbox.CheckOutbound(context.Background(), contracts.NetworkRequest{
		Host:     "example.net",
		Port:     443,
		Protocol: "tcp",
	})

	if !allowed.Allowed {
		t.Fatalf("github decision = %#v, want allowed", allowed)
	}
	if blocked.Allowed {
		t.Fatalf("example decision = %#v, want blocked", blocked)
	}
	if blocked.Mode != contracts.NetworkModeRestricted {
		t.Fatalf("blocked Mode = %q, want restricted", blocked.Mode)
	}
}

func TestNetworkSandboxPerRequestApprovalForUnknownDomains(t *testing.T) {
	approval := &fakeNetworkApproval{allowed: true, reason: "temporary install approved"}
	sandbox := NewNetworkSandbox(NetworkSandboxConfig{
		Mode:      contracts.NetworkModePerRequest,
		Allowlist: []string{"github.com"},
		Approval:  approval,
	})

	decision := sandbox.CheckOutbound(context.Background(), contracts.NetworkRequest{
		Host: "downloads.example.com",
		Port: 443,
	})

	if !decision.Allowed {
		t.Fatalf("decision = %#v, want allowed", decision)
	}
	if decision.Reason != "temporary install approved" {
		t.Fatalf("Reason = %q, want approval reason", decision.Reason)
	}
	if len(approval.requests) != 1 || approval.requests[0].Host != "downloads.example.com" {
		t.Fatalf("approval requests = %#v, want one normalized host", approval.requests)
	}
}

func TestNetworkSandboxOfflineBlocksEvenAllowlistedOutbound(t *testing.T) {
	sandbox := NewNetworkSandbox(NetworkSandboxConfig{Mode: contracts.NetworkModeOffline})

	decision := sandbox.CheckOutbound(context.Background(), contracts.NetworkRequest{Host: "github.com", Port: 443})

	if decision.Allowed {
		t.Fatalf("decision = %#v, want blocked", decision)
	}
	if decision.Reason != "offline mode blocks outbound network" {
		t.Fatalf("Reason = %q, want offline block reason", decision.Reason)
	}
}

func TestNetworkSandboxInboundRequiresPortForward(t *testing.T) {
	sandbox := NewNetworkSandbox(NetworkSandboxConfig{
		PortForwards: []contracts.PortForward{{
			Name:       "preview",
			ListenPort: 3000,
			TargetHost: "127.0.0.1",
			TargetPort: 3000,
			Protocol:   "tcp",
		}},
	})

	allowed := sandbox.CheckInbound(contracts.NetworkRequest{Port: 3000, Protocol: "tcp"})
	blocked := sandbox.CheckInbound(contracts.NetworkRequest{Port: 3001, Protocol: "tcp"})

	if !allowed.Allowed {
		t.Fatalf("allowed decision = %#v, want allowed", allowed)
	}
	if blocked.Allowed {
		t.Fatalf("blocked decision = %#v, want blocked", blocked)
	}
}

func TestNetworkSandboxRecordsDNSAndTrafficEvents(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	sandbox := NewNetworkSandbox(NetworkSandboxConfig{
		Mode:      contracts.NetworkModeRestricted,
		Allowlist: []string{"github.com"},
		Now:       func() time.Time { return now },
	})

	sandbox.CheckOutbound(context.Background(), contracts.NetworkRequest{Host: "api.github.com", Port: 443, Protocol: "tcp"})
	sandbox.CheckOutbound(context.Background(), contracts.NetworkRequest{Host: "203.0.113.10", Port: 443, Protocol: "tcp"})

	events := sandbox.Events()
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want dns + two traffic events: %#v", len(events), events)
	}
	if events[0].Kind != "dns" || events[0].Host != "api.github.com" {
		t.Fatalf("first event = %#v, want DNS event for api.github.com", events[0])
	}
	if events[1].Kind != "traffic" || !events[1].Allowed {
		t.Fatalf("second event = %#v, want allowed traffic", events[1])
	}
	if events[2].Kind != "traffic" || events[2].Allowed {
		t.Fatalf("third event = %#v, want blocked IP traffic", events[2])
	}
}

func TestEngineExposesDefaultNetworkSandbox(t *testing.T) {
	engine := New("test")

	decision := engine.NetworkSandbox().CheckOutbound(context.Background(), contracts.NetworkRequest{Host: "pypi.org", Port: 443})

	if !decision.Allowed {
		t.Fatalf("decision = %#v, want default allowlist to include pypi.org", decision)
	}
}

func TestNetworkSandboxApprovalErrorsBlockRequest(t *testing.T) {
	sandbox := NewNetworkSandbox(NetworkSandboxConfig{
		Mode:     contracts.NetworkModePerRequest,
		Approval: &fakeNetworkApproval{err: errors.New("surface unavailable")},
	})

	decision := sandbox.CheckOutbound(context.Background(), contracts.NetworkRequest{Host: "example.com"})

	if decision.Allowed {
		t.Fatalf("decision = %#v, want blocked", decision)
	}
	if decision.Reason != "network approval failed: surface unavailable" {
		t.Fatalf("Reason = %q, want approval error", decision.Reason)
	}
}

type fakeNetworkApproval struct {
	allowed  bool
	reason   string
	err      error
	requests []contracts.NetworkRequest
}

func (a *fakeNetworkApproval) ApproveNetworkRequest(_ context.Context, req contracts.NetworkRequest) (bool, string, error) {
	a.requests = append(a.requests, req)
	return a.allowed, a.reason, a.err
}
