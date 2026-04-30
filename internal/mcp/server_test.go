package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jabreeflor/conduit/internal/mcp"
)

func TestServerInitialize(t *testing.T) {
	srv := mcp.NewServer("test-conduit", "1.0")

	idRaw := json.RawMessage(`1`)
	msg := mcp.Message{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      &idRaw,
		Method:  mcp.MethodInitialize,
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","clientInfo":{"name":"test","version":"0"},"capabilities":{}}`),
	}

	resp, err := srv.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result mcp.InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ServerInfo.Name != "test-conduit" {
		t.Errorf("server name = %q, want %q", result.ServerInfo.Name, "test-conduit")
	}
	if result.ProtocolVersion != mcp.Version {
		t.Errorf("protocol version = %q, want %q", result.ProtocolVersion, mcp.Version)
	}
}

func TestServerToolsListAndCall(t *testing.T) {
	srv := mcp.NewServer("conduit", "dev")
	srv.RegisterTool(mcp.ToolDef{
		Name:        "echo",
		Description: "Echoes input back.",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, args map[string]any) ([]mcp.Content, error) {
		text, _ := args["text"].(string)
		return []mcp.Content{{Type: mcp.ContentTypeText, Text: text}}, nil
	})

	t.Run("tools/list", func(t *testing.T) {
		idRaw := json.RawMessage(`2`)
		msg := mcp.Message{
			JSONRPC: mcp.JSONRPCVersion,
			ID:      &idRaw,
			Method:  mcp.MethodToolsList,
		}
		resp, err := srv.Handle(context.Background(), msg)
		if err != nil || resp.Error != nil {
			t.Fatalf("tools/list failed: err=%v rpc=%v", err, resp.Error)
		}
		var result mcp.ToolsListResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Tools) != 1 || result.Tools[0].Name != "echo" {
			t.Errorf("unexpected tools: %+v", result.Tools)
		}
	})

	t.Run("tools/call", func(t *testing.T) {
		idRaw := json.RawMessage(`3`)
		params, _ := json.Marshal(mcp.ToolCallParams{
			Name:      "echo",
			Arguments: map[string]any{"text": "hello"},
		})
		msg := mcp.Message{
			JSONRPC: mcp.JSONRPCVersion,
			ID:      &idRaw,
			Method:  mcp.MethodToolsCall,
			Params:  params,
		}
		resp, err := srv.Handle(context.Background(), msg)
		if err != nil || resp.Error != nil {
			t.Fatalf("tools/call failed: err=%v rpc=%v", err, resp.Error)
		}
		var result mcp.ToolCallResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatal(err)
		}
		if result.IsError || len(result.Content) == 0 || result.Content[0].Text != "hello" {
			t.Errorf("unexpected result: %+v", result)
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		idRaw := json.RawMessage(`4`)
		params, _ := json.Marshal(mcp.ToolCallParams{Name: "nope"})
		msg := mcp.Message{
			JSONRPC: mcp.JSONRPCVersion,
			ID:      &idRaw,
			Method:  mcp.MethodToolsCall,
			Params:  params,
		}
		resp, _ := srv.Handle(context.Background(), msg)
		if resp.Error == nil {
			t.Error("expected RPC error for unknown tool")
		}
	})
}

func TestServerUnknownMethod(t *testing.T) {
	srv := mcp.NewServer("conduit", "dev")
	idRaw := json.RawMessage(`5`)
	msg := mcp.Message{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      &idRaw,
		Method:  "unknown/method",
	}
	resp, _ := srv.Handle(context.Background(), msg)
	if resp.Error == nil || resp.Error.Code != mcp.CodeMethodNotFound {
		t.Errorf("expected method-not-found error, got %+v", resp.Error)
	}
}
