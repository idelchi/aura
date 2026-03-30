package tool_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/tool"
)

// stubTool is a minimal Tool implementation for collection tests.
type stubTool struct {
	tool.Base

	name string
}

func (s *stubTool) Name() string        { return s.name }
func (s *stubTool) Schema() tool.Schema { return tool.Schema{Name: s.name} }
func (s *stubTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "ok", nil
}

func newStub(name string) *stubTool {
	return &stubTool{name: name}
}

// makeSchema builds a Schema with the given required string field for test cases.
func makeSchema(t *testing.T, requiredField string) tool.Schema {
	t.Helper()

	return tool.Schema{
		Name:        "test",
		Description: "test schema",
		Parameters: tool.Parameters{
			Type: "object",
			Properties: map[string]tool.Property{
				requiredField: {Type: "string", Description: "a string param"},
				"count":       {Type: "integer", Description: "an integer param"},
			},
			Required: []string{requiredField},
		},
	}
}

// -----------------------------------------------------------------------------
// Schema.ValidateArgs
// -----------------------------------------------------------------------------

func TestSchema_ValidateArgs(t *testing.T) {
	t.Parallel()

	schema := makeSchema(t, "path")

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid args with required field",
			args:    map[string]any{"path": "/tmp/file.txt"},
			wantErr: false,
		},
		{
			name:    "valid args with required and optional field",
			args:    map[string]any{"path": "/tmp/file.txt", "count": float64(3)},
			wantErr: false,
		},
		{
			name:    "missing required field",
			args:    map[string]any{"count": float64(1)},
			wantErr: true,
			errMsg:  `missing required parameter "path"`,
		},
		{
			name:    "wrong type for string field",
			args:    map[string]any{"path": 42},
			wantErr: true,
			errMsg:  `expected string`,
		},
		{
			name:    "unknown field",
			args:    map[string]any{"path": "/tmp", "unknown": "val"},
			wantErr: true,
			errMsg:  `unknown parameter "unknown"`,
		},
		{
			name:    "empty args on required schema",
			args:    map[string]any{},
			wantErr: true,
			errMsg:  `missing required parameter "path"`,
		},
		{
			name:    "nil args on required schema",
			args:    nil,
			wantErr: true,
			errMsg:  `missing required parameter "path"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := schema.ValidateArgs(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateArgs() = nil, want error containing %q", tt.errMsg)
				}

				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateArgs() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateArgs() = %v, want nil", err)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Tools.Get
// -----------------------------------------------------------------------------

func TestTools_Get(t *testing.T) {
	t.Parallel()

	ts := tool.Tools{newStub("read"), newStub("write")}

	tests := []struct {
		name    string
		lookup  string
		wantErr bool
	}{
		{
			name:    "found",
			lookup:  "read",
			wantErr: false,
		},
		{
			name:    "second tool found",
			lookup:  "write",
			wantErr: false,
		},
		{
			name:    "not found returns ErrToolNotFound",
			lookup:  "delete",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ts.Get(tt.lookup)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Get(%q) = %v, want error", tt.lookup, got)
				}

				if !errors.Is(err, tool.ErrToolNotFound) {
					t.Errorf("Get(%q) error = %v, want errors.Is(err, ErrToolNotFound)", tt.lookup, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Get(%q) = %v, want nil error", tt.lookup, err)
				}

				if got.Name() != tt.lookup {
					t.Errorf("Get(%q).Name() = %q, want %q", tt.lookup, got.Name(), tt.lookup)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Tools.Add
// -----------------------------------------------------------------------------

func TestTools_Add(t *testing.T) {
	t.Parallel()

	t.Run("new tool is appended", func(t *testing.T) {
		t.Parallel()

		var ts tool.Tools
		ts.Add(newStub("alpha"))

		if len(ts) != 1 {
			t.Fatalf("len(Tools) = %d, want 1", len(ts))
		}

		if ts[0].Name() != "alpha" {
			t.Errorf("Tools[0].Name() = %q, want %q", ts[0].Name(), "alpha")
		}
	})

	t.Run("duplicate add is idempotent", func(t *testing.T) {
		t.Parallel()

		var ts tool.Tools
		ts.Add(newStub("beta"))
		ts.Add(newStub("beta"))

		if len(ts) != 1 {
			t.Errorf("len(Tools) = %d, want 1 after duplicate Add", len(ts))
		}
	})
}

// -----------------------------------------------------------------------------
// Tools.Remove
// -----------------------------------------------------------------------------

func TestTools_Remove(t *testing.T) {
	t.Parallel()

	t.Run("existing tool is removed", func(t *testing.T) {
		t.Parallel()

		ts := tool.Tools{newStub("alpha"), newStub("beta"), newStub("gamma")}
		ts.Remove("beta")

		if len(ts) != 2 {
			t.Fatalf("len(Tools) = %d, want 2 after remove", len(ts))
		}

		if ts.Has("beta") {
			t.Error("Tools still contains \"beta\" after Remove")
		}
	})

	t.Run("removing missing tool is no-op", func(t *testing.T) {
		t.Parallel()

		ts := tool.Tools{newStub("alpha")}
		ts.Remove("nonexistent")

		if len(ts) != 1 {
			t.Errorf("len(Tools) = %d, want 1 after no-op Remove", len(ts))
		}
	})

	t.Run("removing from empty collection is no-op", func(t *testing.T) {
		t.Parallel()

		var ts tool.Tools
		ts.Remove("anything") // must not panic

		if len(ts) != 0 {
			t.Errorf("len(Tools) = %d, want 0", len(ts))
		}
	})
}

// -----------------------------------------------------------------------------
// Tools.Has / Names
// -----------------------------------------------------------------------------

func TestTools_Has(t *testing.T) {
	t.Parallel()

	ts := tool.Tools{newStub("alpha"), newStub("beta")}

	tests := []struct {
		name string
		tool string
		want bool
	}{
		{name: "present tool", tool: "alpha", want: true},
		{name: "second present tool", tool: "beta", want: true},
		{name: "absent tool", tool: "gamma", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ts.Has(tt.tool); got != tt.want {
				t.Errorf("Has(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func TestTools_Names(t *testing.T) {
	t.Parallel()

	t.Run("returns names in order", func(t *testing.T) {
		t.Parallel()

		ts := tool.Tools{newStub("alpha"), newStub("beta"), newStub("gamma")}
		names := ts.Names()

		want := []string{"alpha", "beta", "gamma"}
		if len(names) != len(want) {
			t.Fatalf("Names() length = %d, want %d", len(names), len(want))
		}

		for i, n := range want {
			if names[i] != n {
				t.Errorf("Names()[%d] = %q, want %q", i, names[i], n)
			}
		}
	})

	t.Run("empty collection returns empty slice", func(t *testing.T) {
		t.Parallel()

		var ts tool.Tools

		names := ts.Names()

		if len(names) != 0 {
			t.Errorf("Names() = %v, want empty", names)
		}
	})
}

// -----------------------------------------------------------------------------
// Tools.Filtered
// -----------------------------------------------------------------------------

func TestTools_Filtered(t *testing.T) {
	t.Parallel()

	ts := tool.Tools{
		newStub("read_file"),
		newStub("write_file"),
		newStub("mcp__echo__ping"),
		newStub("mcp__echo__pong"),
		newStub("bash"),
	}

	tests := []struct {
		name      string
		include   []string
		exclude   []string
		wantNames []string
	}{
		{
			name:      "empty filters returns all tools",
			include:   nil,
			exclude:   nil,
			wantNames: []string{"read_file", "write_file", "mcp__echo__ping", "mcp__echo__pong", "bash"},
		},
		{
			name:      "include wildcard returns all tools",
			include:   []string{"*"},
			exclude:   nil,
			wantNames: []string{"read_file", "write_file", "mcp__echo__ping", "mcp__echo__pong", "bash"},
		},
		{
			name:      "include exact names",
			include:   []string{"bash", "read_file"},
			exclude:   nil,
			wantNames: []string{"read_file", "bash"},
		},
		{
			name:      "include wildcard pattern",
			include:   []string{"mcp__echo__*"},
			exclude:   nil,
			wantNames: []string{"mcp__echo__ping", "mcp__echo__pong"},
		},
		{
			name:      "exclude exact name",
			include:   nil,
			exclude:   []string{"bash"},
			wantNames: []string{"read_file", "write_file", "mcp__echo__ping", "mcp__echo__pong"},
		},
		{
			name:      "exclude wildcard pattern",
			include:   nil,
			exclude:   []string{"mcp__*"},
			wantNames: []string{"read_file", "write_file", "bash"},
		},
		{
			name:      "include and exclude combined — exclude wins",
			include:   []string{"mcp__echo__*"},
			exclude:   []string{"mcp__echo__ping"},
			wantNames: []string{"mcp__echo__pong"},
		},
		{
			name:      "include non-matching pattern returns empty",
			include:   []string{"nonexistent__*"},
			exclude:   nil,
			wantNames: []string{},
		},
		{
			name:      "exclude all via wildcard returns empty",
			include:   nil,
			exclude:   []string{"*"},
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ts.Filtered(tt.include, tt.exclude)

			if len(got) != len(tt.wantNames) {
				t.Fatalf("Filtered() len = %d, want %d; got names %v", len(got), len(tt.wantNames), got.Names())
			}

			gotSet := make(map[string]bool, len(got))
			for _, tool := range got {
				gotSet[tool.Name()] = true
			}

			for _, wantName := range tt.wantNames {
				if !gotSet[wantName] {
					t.Errorf("Filtered() missing %q; got %v", wantName, got.Names())
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Filter.IsSet
// -----------------------------------------------------------------------------

func TestFilter_IsSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter tool.Filter
		want   bool
	}{
		{
			name:   "empty filter is not set",
			filter: tool.Filter{},
			want:   false,
		},
		{
			name:   "enabled only is set",
			filter: tool.Filter{Enabled: []string{"bash"}},
			want:   true,
		},
		{
			name:   "disabled only is set",
			filter: tool.Filter{Disabled: []string{"bash"}},
			want:   true,
		},
		{
			name:   "both enabled and disabled is set",
			filter: tool.Filter{Enabled: []string{"read"}, Disabled: []string{"write"}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.filter.IsSet(); got != tt.want {
				t.Errorf("IsSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Filter.String
// -----------------------------------------------------------------------------

func TestFilter_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filter   tool.Filter
		contains []string
		absent   []string
	}{
		{
			name:   "empty filter produces empty string",
			filter: tool.Filter{},
			absent: []string{"+", "-"},
		},
		{
			name:     "enabled only",
			filter:   tool.Filter{Enabled: []string{"read", "write"}},
			contains: []string{"+[read,write]"},
			absent:   []string{"-["},
		},
		{
			name:     "disabled only",
			filter:   tool.Filter{Disabled: []string{"bash"}},
			contains: []string{"-[bash]"},
			absent:   []string{"+["},
		},
		{
			name:     "both enabled and disabled",
			filter:   tool.Filter{Enabled: []string{"read"}, Disabled: []string{"write"}},
			contains: []string{"+[read]", "-[write]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.filter.String()

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("String() = %q, want it to contain %q", got, want)
				}
			}

			for _, unwanted := range tt.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("String() = %q, want it NOT to contain %q", got, unwanted)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ValidatePath
// -----------------------------------------------------------------------------

func TestValidatePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid absolute path",
			path:    "/tmp/file.txt",
			wantErr: false,
		},
		{
			name:    "valid relative path",
			path:    "some/dir/file.go",
			wantErr: false,
		},
		{
			name:    "root slash rejected",
			path:    "/",
			wantErr: true,
		},
		{
			name:    "whitespace-padded root rejected",
			path:    "  /  ",
			wantErr: true,
		},
		{
			name:    "non-root path with trailing slash is valid",
			path:    "/tmp/",
			wantErr: false,
		},
		{
			name:    "empty string is valid",
			path:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tool.ValidatePath(tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePath(%q) = nil, want error", tt.path)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePath(%q) = %v, want nil", tt.path, err)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Base.MergeText
// -----------------------------------------------------------------------------

func TestBase_MergeTextEmpty(t *testing.T) {
	t.Parallel()

	var b tool.Base
	b.MergeText(tool.Text{})

	if b.Description() != "" {
		t.Errorf("Description() = %q, want empty", b.Description())
	}

	if b.Usage() != "" {
		t.Errorf("Usage() = %q, want empty", b.Usage())
	}

	if b.Examples() != "" {
		t.Errorf("Examples() = %q, want empty", b.Examples())
	}
}

func TestBase_MergeTextPartial(t *testing.T) {
	t.Parallel()

	var b tool.Base
	b.MergeText(tool.Text{Description: "desc"})

	if b.Description() != "desc" {
		t.Errorf("Description() = %q, want %q", b.Description(), "desc")
	}

	if b.Usage() != "" {
		t.Errorf("Usage() = %q, want empty after partial merge", b.Usage())
	}
}

func TestBase_MergeTextFull(t *testing.T) {
	t.Parallel()

	var b tool.Base
	b.MergeText(tool.Text{Description: "desc", Usage: "use", Examples: "ex"})

	if b.Description() != "desc" {
		t.Errorf("Description() = %q, want %q", b.Description(), "desc")
	}

	if b.Usage() != "use" {
		t.Errorf("Usage() = %q, want %q", b.Usage(), "use")
	}

	if b.Examples() != "ex" {
		t.Errorf("Examples() = %q, want %q", b.Examples(), "ex")
	}
}

func TestBase_MergeTextDoesNotClearExisting(t *testing.T) {
	t.Parallel()

	var b tool.Base
	b.MergeText(tool.Text{Description: "first", Usage: "original", Examples: "sample"})
	b.MergeText(tool.Text{Description: "second"})

	if b.Description() != "second" {
		t.Errorf("Description() = %q, want %q", b.Description(), "second")
	}

	if b.Usage() != "original" {
		t.Errorf("Usage() = %q, want %q (should not be cleared)", b.Usage(), "original")
	}

	if b.Examples() != "sample" {
		t.Errorf("Examples() = %q, want %q (should not be cleared)", b.Examples(), "sample")
	}
}

// -----------------------------------------------------------------------------
// Tools.Schemas
// -----------------------------------------------------------------------------

func TestTools_Schemas(t *testing.T) {
	t.Parallel()

	t.Run("multiple tools returns ordered schemas", func(t *testing.T) {
		t.Parallel()

		ts := tool.Tools{newStub("alpha"), newStub("beta"), newStub("gamma")}
		schemas := ts.Schemas()

		if len(schemas) != 3 {
			t.Fatalf("len(Schemas) = %d, want 3", len(schemas))
		}

		want := []string{"alpha", "beta", "gamma"}
		for i, name := range want {
			if schemas[i].Name != name {
				t.Errorf("Schemas[%d].Name = %q, want %q", i, schemas[i].Name, name)
			}
		}
	})

	t.Run("empty tools returns empty schemas", func(t *testing.T) {
		t.Parallel()

		var ts tool.Tools

		schemas := ts.Schemas()

		if len(schemas) != 0 {
			t.Errorf("len(Schemas) = %d, want 0", len(schemas))
		}
	})
}
