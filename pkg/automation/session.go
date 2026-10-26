package automation

import "time"

// Lineage identifies the exact stored parent revision a session was forked from.
type Lineage struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

// SessionSummary describes a stored session.
type SessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Revision is the sha256 of the stored file bytes.
	Revision string `json:"revision"`
	// Messages counts stored non-system messages.
	Messages int      `json:"messages"`
	Frozen   bool     `json:"frozen"`
	Parent   *Lineage `json:"parent"`
}

// SessionList is the output of `aura session list --json`.
type SessionList struct {
	Sessions []SessionSummary `json:"sessions"`
	Warnings []string         `json:"warnings"`
}

// ForkResult is the output of `aura session fork --json`.
type ForkResult struct {
	Session SessionSummary `json:"session"`
	Parent  SessionSummary `json:"parent"`
	Created bool           `json:"created"`
}

// FreezeResult is the output of `aura session freeze --json`.
type FreezeResult struct {
	Session SessionSummary `json:"session"`
	Changed bool           `json:"changed"`
}

// ImportResult is the output of `aura session import --json`.
type ImportResult struct {
	Session      SessionSummary `json:"session"`
	Created      bool           `json:"created"`
	FrozenNow    bool           `json:"frozen_now"`
	SourceSHA256 string         `json:"source_sha256"`
}

// ExportResult is the output of `aura session export --json`.
type ExportResult struct {
	Session SessionSummary `json:"session"`
	Path    string         `json:"path"`
	SHA256  string         `json:"sha256"`
	Written bool           `json:"written"`
}

// SessionMeta is the live selection of a served session.
type SessionMeta struct {
	Agent    string `json:"agent"`
	Mode     string `json:"mode"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Think    string `json:"think"`
	Sandbox  bool   `json:"sandbox"`
}

// SessionStatus is the live context state of a served session.
type SessionStatus struct {
	Iteration  int `json:"iteration"`
	TokensUsed int `json:"tokens_used"`
	TokensMax  int `json:"tokens_max"`
}

// SessionInfo is the response of GET /v1/session: the stored summary plus live state.
type SessionInfo struct {
	SessionSummary

	PersistedSeq int64         `json:"persisted_seq"`
	Meta         SessionMeta   `json:"meta"`
	Status       SessionStatus `json:"status"`
}

// Messages is the response of GET /v1/session/messages.
type Messages struct {
	Messages []map[string]any `json:"messages"`
	Total    int              `json:"total"`
}
