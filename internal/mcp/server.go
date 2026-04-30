package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolHandler is a function that executes one MCP tool call.
type ToolHandler func(ctx context.Context, args map[string]any) ([]Content, error)

// registeredTool pairs a descriptor with its handler.
type registeredTool struct {
	def     ToolDef
	handler ToolHandler
}

// Server is a stateful MCP server that exposes registered tools over any
// Transport. It is safe to use from multiple goroutines once initialized.
type Server struct {
	info  Info
	tools map[string]registeredTool
}

// NewServer creates an MCP server with the given identity metadata.
func NewServer(name, version string) *Server {
	return &Server{
		info:  Info{Name: name, Version: version},
		tools: make(map[string]registeredTool),
	}
}

// RegisterTool adds a tool to the server's tool list.
// Registering a tool with a name that already exists replaces it.
func (s *Server) RegisterTool(def ToolDef, handler ToolHandler) {
	s.tools[def.Name] = registeredTool{def: def, handler: handler}
}

// Serve reads messages from t and dispatches them until ctx is cancelled or
// the transport returns io.EOF.
func (s *Server) Serve(ctx context.Context, t Transport) error {
	defer t.Close()
	for {
		msg, err := t.Receive(ctx)
		if err != nil {
			return err
		}
		if msg.IsNotification() {
			// Notifications have no response — handle fire-and-forget.
			continue
		}
		resp, err := s.Handle(ctx, msg)
		if err != nil {
			resp = ErrorResponse(msg.ID, CodeInternalError, err.Error())
		}
		if err := t.Send(ctx, resp); err != nil {
			return err
		}
	}
}

// Handle dispatches one request message and returns the response. It is
// exported so the HTTP transport can call it per-POST without a long-lived
// Serve loop.
func (s *Server) Handle(ctx context.Context, msg Message) (Message, error) {
	switch msg.Method {
	case MethodInitialize:
		return s.handleInitialize(msg)
	case MethodToolsList:
		return s.handleToolsList(msg)
	case MethodToolsCall:
		return s.handleToolsCall(ctx, msg)
	case MethodPing:
		return OKResponse(msg.ID, struct{}{})
	default:
		return ErrorResponse(msg.ID, CodeMethodNotFound, fmt.Sprintf("method not found: %s", msg.Method)), nil
	}
}

func (s *Server) handleInitialize(msg Message) (Message, error) {
	var params InitializeParams
	if msg.Params != nil {
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return ErrorResponse(msg.ID, CodeInvalidParams, "invalid initialize params"), nil
		}
	}

	result := InitializeResult{
		ProtocolVersion: Version,
		ServerInfo:      s.info,
		Capabilities: ServerCaps{
			Tools: &ToolsCap{ListChanged: false},
		},
	}
	return OKResponse(msg.ID, result)
}

func (s *Server) handleToolsList(msg Message) (Message, error) {
	defs := make([]ToolDef, 0, len(s.tools))
	for _, rt := range s.tools {
		defs = append(defs, rt.def)
	}
	return OKResponse(msg.ID, ToolsListResult{Tools: defs})
}

func (s *Server) handleToolsCall(ctx context.Context, msg Message) (Message, error) {
	var params ToolCallParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return ErrorResponse(msg.ID, CodeInvalidParams, "invalid tools/call params"), nil
	}

	rt, ok := s.tools[params.Name]
	if !ok {
		return ErrorResponse(msg.ID, CodeMethodNotFound, fmt.Sprintf("tool not found: %s", params.Name)), nil
	}

	content, err := rt.handler(ctx, params.Arguments)
	if err != nil {
		result := ToolCallResult{
			Content: []Content{{Type: ContentTypeText, Text: err.Error()}},
			IsError: true,
		}
		return OKResponse(msg.ID, result)
	}

	return OKResponse(msg.ID, ToolCallResult{Content: content})
}
