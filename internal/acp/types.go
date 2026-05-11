package acp

import "encoding/json"

// JSON-RPC method names.
const (
	MethodInitialize  = "initialize"
	MethodSessionNew  = "session/new"
	MethodSessionPrompt = "session/prompt"
	MethodNotifyUpdate = "session/update"
)

// InitializeParams is the client initialize request body.
type InitializeParams struct {
	ProtocolVersion   string          `json:"protocolVersion"`
	ClientCapabilities json.RawMessage `json:"clientCapabilities"`
	ClientInfo        *Implementation `json:"clientInfo,omitempty"`
}

// InitializeResult is returned by initialize.
type InitializeResult struct {
	ProtocolVersion   string          `json:"protocolVersion"`
	AgentCapabilities json.RawMessage `json:"agentCapabilities"`
	AgentInfo         *Implementation `json:"agentInfo,omitempty"`
}

// Implementation identifies client/agent build.
type Implementation struct {
	Name    string `json:"name,omitempty"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// NewSessionParams creates a session rooted at cwd.
type NewSessionParams struct {
	Cwd        string          `json:"cwd"`
	McpServers json.RawMessage `json:"mcpServers"`
}

// NewSessionResult contains sessionId.
type NewSessionResult struct {
	SessionID string `json:"sessionId"`
}

// PromptParams sends user content blocks.
type PromptParams struct {
	SessionID string          `json:"sessionId"`
	Prompt    json.RawMessage `json:"prompt"`
}

// PromptResult is returned when a prompt turn completes.
type PromptResult struct {
	StopReason string `json:"stopReason"`
}

// RPCRequest is a JSON-RPC 2.0 request.
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCResponse is a JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// TextBlock builds a minimal prompt array with one text block.
func TextBlock(text string) json.RawMessage {
	type textblk struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	arr := []textblk{{Type: "text", Text: text}}
	b, _ := json.Marshal(arr)
	return b
}
