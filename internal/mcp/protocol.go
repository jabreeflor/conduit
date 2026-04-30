// Package mcp implements the Model Context Protocol (MCP) for Conduit.
// It supports STDIO and streaming-HTTP transports, server and client modes,
// auto-discovery of tools, and the @ToolName inline mention syntax.
package mcp

import (
	"encoding/json"
	"fmt"
)

// Version is the MCP protocol version this implementation targets.
const Version = "2024-11-05"

// JSONRPCVersion is the JSON-RPC version used in all messages.
const JSONRPCVersion = "2.0"

// Method constants for the MCP protocol.
const (
	MethodInitialize      = "initialize"
	MethodInitialized     = "notifications/initialized"
	MethodToolsList       = "tools/list"
	MethodToolsCall       = "tools/call"
	MethodResourcesList   = "resources/list"
	MethodResourcesRead   = "resources/read"
	MethodPromptsList     = "prompts/list"
	MethodPromptsGet      = "prompts/get"
	MethodLoggingSetLevel = "logging/setLevel"
	MethodPing            = "ping"
)

// Message is the base JSON-RPC 2.0 envelope.
type Message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

// IsRequest returns true if the message has a method and an ID.
func (m Message) IsRequest() bool { return m.Method != "" && m.ID != nil }

// IsNotification returns true if the message has a method but no ID.
func (m Message) IsNotification() bool { return m.Method != "" && m.ID == nil }

// IsResponse returns true if the message has an ID but no method.
func (m Message) IsResponse() bool { return m.Method == "" && m.ID != nil }

// RPCError is the JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// Standard JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// ErrorResponse builds a JSON-RPC error response for the given request ID.
func ErrorResponse(id *json.RawMessage, code int, msg string) Message {
	return Message{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
}

// OKResponse builds a JSON-RPC success response.
func OKResponse(id *json.RawMessage, result any) (Message, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return Message{}, err
	}
	return Message{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  raw,
	}, nil
}

// InitializeParams are the params sent by an MCP client on first connect.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ClientCaps `json:"capabilities"`
	ClientInfo      Info       `json:"clientInfo"`
}

// InitializeResult is returned by the server in response to initialize.
type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ServerCaps `json:"capabilities"`
	ServerInfo      Info       `json:"serverInfo"`
}

// ClientCaps describes what the connecting client supports.
type ClientCaps struct {
	Roots    *RootsCap    `json:"roots,omitempty"`
	Sampling *SamplingCap `json:"sampling,omitempty"`
}

// ServerCaps describes what this server exposes.
type ServerCaps struct {
	Tools     *ToolsCap     `json:"tools,omitempty"`
	Resources *ResourcesCap `json:"resources,omitempty"`
	Prompts   *PromptsCap   `json:"prompts,omitempty"`
	Logging   *LoggingCap   `json:"logging,omitempty"`
}

// RootsCap signals filesystem root negotiation support.
type RootsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCap signals that the client can handle sampling requests.
type SamplingCap struct{}

// ToolsCap signals that the server exposes callable tools.
type ToolsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCap signals that the server exposes readable resources.
type ResourcesCap struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCap signals that the server exposes prompt templates.
type PromptsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCap signals that the server supports logging level control.
type LoggingCap struct{}

// Info is name+version metadata for client and server identification.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolDef is an MCP tool descriptor as returned in tools/list.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolsListResult is the payload for a tools/list response.
type ToolsListResult struct {
	Tools []ToolDef `json:"tools"`
}

// ToolCallParams are the params for a tools/call request.
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ContentType describes the kind of content in a tool result.
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
)

// Content is one item in a tool call result.
type Content struct {
	Type ContentType `json:"type"`
	Text string      `json:"text,omitempty"`
	Data string      `json:"data,omitempty"`
	MIME string      `json:"mimeType,omitempty"`
}

// ToolCallResult is the payload for a tools/call response.
type ToolCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}
