package automation

// Ready is the content of the serve ready file: either the server address or the startup error.
type Ready struct {
	API       int    `json:"api"`
	URL       string `json:"url,omitempty"`
	PID       int    `json:"pid,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Version   string `json:"aura_version,omitempty"`
	Commit    string `json:"commit,omitempty"`
	Workdir   string `json:"workdir,omitempty"`
	WriteHome string `json:"write_home,omitempty"`
	Error     *Error `json:"error,omitempty"`
}

// MCPServer describes an MCP server of the served session.
type MCPServer struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // connected or failed
	Required  bool   `json:"required"`
	Sandboxed bool   `json:"sandboxed"`
	Tools     int    `json:"tools"`
	Error     string `json:"error"`
}

// ServerInfo is the response of GET /v1/server.
type ServerInfo struct {
	API          int            `json:"api"`
	Version      string         `json:"version"`
	Commit       string         `json:"commit"`
	PID          int            `json:"pid"`
	SessionID    string         `json:"session_id"`
	Workdir      string         `json:"workdir"`
	WriteHome    string         `json:"write_home"`
	ActiveRun    NullableString `json:"active_run"`
	LastSeq      int64          `json:"last_seq"`
	ShuttingDown bool           `json:"shutting_down"`
	MCP          []MCPServer    `json:"mcp"`
}
