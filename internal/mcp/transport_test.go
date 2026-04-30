package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/mcp"
)

func TestStdioTransportRoundtrip(t *testing.T) {
	idRaw := json.RawMessage(`42`)
	msg := mcp.Message{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      &idRaw,
		Method:  mcp.MethodPing,
	}

	var buf bytes.Buffer
	tr := mcp.NewStdioTransport(strings.NewReader(""), &buf)
	if err := tr.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	line := buf.String()
	if !strings.Contains(line, `"ping"`) {
		t.Errorf("encoded message missing method: %s", line)
	}
	if !strings.Contains(line, `"42"`) && !strings.Contains(line, `42`) {
		t.Errorf("encoded message missing id: %s", line)
	}
}

func TestStdioTransportReceive(t *testing.T) {
	idRaw := json.RawMessage(`7`)
	msg := mcp.Message{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      &idRaw,
		Method:  mcp.MethodPing,
	}
	raw, _ := json.Marshal(msg)

	tr := mcp.NewStdioTransport(bytes.NewReader(append(raw, '\n')), &bytes.Buffer{})
	got, err := tr.Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if got.Method != mcp.MethodPing {
		t.Errorf("method = %q, want %q", got.Method, mcp.MethodPing)
	}
}

func TestErrorResponse(t *testing.T) {
	idRaw := json.RawMessage(`1`)
	resp := mcp.ErrorResponse(&idRaw, mcp.CodeMethodNotFound, "no such method")
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != mcp.CodeMethodNotFound {
		t.Errorf("code = %d, want %d", resp.Error.Code, mcp.CodeMethodNotFound)
	}
	if resp.JSONRPC != mcp.JSONRPCVersion {
		t.Errorf("jsonrpc = %q, want %q", resp.JSONRPC, mcp.JSONRPCVersion)
	}
}
