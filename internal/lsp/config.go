package lsp

// Config holds LSP configuration passed to the Manager.
type Config struct {
	// Servers maps server names to their configuration.
	// Example: "gopls" → {Command: "gopls", Args: ["serve"], FileTypes: ["go", "mod"]}.
	Servers map[string]Server `yaml:"servers"`
}

// Server describes a single LSP server and how to match files to it.
type Server struct {
	// Command is the executable to run (e.g. "gopls", "typescript-language-server").
	Command string `yaml:"command"`
	// Args are command-line arguments (e.g. ["serve"]).
	Args []string `yaml:"args"`
	// FileTypes lists file extensions or language identifiers this server handles.
	// Extensions can include the leading dot or not (e.g. ".go" or "go").
	// Empty means the server handles all file types.
	FileTypes []string `yaml:"file_types"`
	// RootMarkers are filenames that identify the project root (e.g. ["go.mod"]).
	// The server only starts if at least one marker is found in the working directory.
	// Empty means the server applies to any directory.
	RootMarkers []string `yaml:"root_markers"`
	// Settings are LSP workspace settings passed to the server.
	Settings map[string]any `yaml:"settings"`
	// InitOptions are LSP initialization options.
	InitOptions map[string]any `yaml:"init_options"`
	// Timeout is the initialization timeout in seconds. Default: 30.
	Timeout int `yaml:"timeout"`
	// Disabled skips this server during startup. Default: false.
	Disabled bool `yaml:"disabled"`
}

// TimeoutOrDefault returns the configured timeout or 30 seconds.
func (s Server) TimeoutOrDefault() int {
	if s.Timeout > 0 {
		return s.Timeout
	}

	return 30
}
