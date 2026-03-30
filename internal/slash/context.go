package slash

import (
	"context"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/conversation"

	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/snapshot"
	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// Context is the interface that slash commands use to interact with the assistant.
// It exposes only the state and actions that commands actually need,
// decoupling the slash system from the assistant's internal structure.
type Context interface {
	// Agent/mode/model switching
	SwitchAgent(name, reason string) error
	SwitchMode(mode string) error
	SwitchModel(ctx context.Context, provider, model string) error
	SetThink(t thinking.Value) error
	SetSandbox(enabled bool) error
	ResizeContext(size int) error
	SetAuto(val bool)
	ResetTokens()

	// Heavy operations
	Compact(ctx context.Context, force bool) error
	GenerateTitle(ctx context.Context) (string, error)
	ProcessInput(ctx context.Context, input string) error
	Reload(ctx context.Context) error
	ResumeSession(ctx context.Context, sess *session.Session) []string

	// Read-only state
	Resolved() config.Resolved
	Status() ui.Status
	DisplayHints() ui.DisplayHints
	SandboxDisplay() string
	SystemPrompt() string
	ToolNames() []string
	LoadedTools() []string
	SessionMeta() session.Meta
	ResolvedModel() model.Model

	// Sub-object access
	ToolPolicy() *config.ToolPolicy
	Cfg() config.Config
	Paths() config.Paths
	Runtime() *config.Runtime
	Builder() *conversation.Builder
	SessionManager() *session.Manager
	TodoList() *todo.List
	SessionStats() *stats.Stats
	InjectorRegistry() *injector.Registry
	MCPSessions() []*mcp.Session
	RegisterMCPSession(s *mcp.Session) error
	SnapshotManager() *snapshot.Manager
	EventChan() chan<- ui.Event

	// Lifecycle
	RequestExit()

	// Simple state
	SetVerbose(val bool)
	SetDone(val bool) error
	ReadBeforePolicy() tool.ReadBeforePolicy
	SetReadBeforePolicy(tool.ReadBeforePolicy) error

	// Template variables
	TemplateVars() map[string]string

	// Plugin display
	PluginSummary() string

	// Caching
	ModelListCache() []ProviderModels
	CacheModelList(v []ProviderModels)
	ClearModelListCache()
}

