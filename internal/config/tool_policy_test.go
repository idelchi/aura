package config_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
)

func TestToolPolicyEvaluateDefault(t *testing.T) {
	t.Parallel()

	p := config.ToolPolicy{}

	tools := []string{"Bash", "Write", "Read", "mcp__server__tool"}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()

			got := p.Evaluate(tool, nil)
			if got != config.PolicyAuto {
				t.Errorf("Evaluate(%q, nil) = %v, want PolicyAuto", tool, got)
			}
		})
	}
}

func TestToolPolicyEvaluateDeny(t *testing.T) {
	t.Parallel()

	p := config.ToolPolicy{Deny: []string{"Write"}}

	t.Run("blocked tool returns PolicyDeny", func(t *testing.T) {
		t.Parallel()

		got := p.Evaluate("Write", nil)
		if got != config.PolicyDeny {
			t.Errorf("Evaluate(Write) = %v, want PolicyDeny", got)
		}
	})

	t.Run("unrelated tool returns PolicyAuto", func(t *testing.T) {
		t.Parallel()

		got := p.Evaluate("Read", nil)
		if got != config.PolicyAuto {
			t.Errorf("Evaluate(Read) = %v, want PolicyAuto", got)
		}
	})
}

func TestToolPolicyEvaluateConfirm(t *testing.T) {
	t.Parallel()

	p := config.ToolPolicy{Confirm: []string{"Write"}}

	t.Run("matched tool returns PolicyConfirm", func(t *testing.T) {
		t.Parallel()

		got := p.Evaluate("Write", nil)
		if got != config.PolicyConfirm {
			t.Errorf("Evaluate(Write) = %v, want PolicyConfirm", got)
		}
	})

	t.Run("unmatched tool returns PolicyAuto", func(t *testing.T) {
		t.Parallel()

		got := p.Evaluate("Read", nil)
		if got != config.PolicyAuto {
			t.Errorf("Evaluate(Read) = %v, want PolicyAuto", got)
		}
	})
}

func TestToolPolicyEvaluatePrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy config.ToolPolicy
		tool   string
		want   config.PolicyAction
	}{
		{
			name:   "deny+auto returns deny",
			policy: config.ToolPolicy{Deny: []string{"Write"}, Auto: []string{"Write"}},
			tool:   "Write",
			want:   config.PolicyDeny,
		},
		{
			name:   "confirm+auto returns auto (auto overrides confirm)",
			policy: config.ToolPolicy{Confirm: []string{"Write"}, Auto: []string{"Write"}},
			tool:   "Write",
			want:   config.PolicyAuto,
		},
		{
			name:   "deny+confirm returns deny",
			policy: config.ToolPolicy{Deny: []string{"Write"}, Confirm: []string{"Write"}},
			tool:   "Write",
			want:   config.PolicyDeny,
		},
		{
			name:   "deny+confirm+auto returns deny",
			policy: config.ToolPolicy{Deny: []string{"Write"}, Confirm: []string{"Write"}, Auto: []string{"Write"}},
			tool:   "Write",
			want:   config.PolicyDeny,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.policy.Evaluate(tc.tool, nil)
			if got != tc.want {
				t.Errorf("Evaluate(%q) = %v, want %v", tc.tool, got, tc.want)
			}
		})
	}
}

func TestToolPolicyEvaluateBashCommand(t *testing.T) {
	t.Parallel()

	p := config.ToolPolicy{Confirm: []string{"Bash:git commit*"}}

	tests := []struct {
		name    string
		tool    string
		command string
		want    config.PolicyAction
	}{
		{
			name:    "matching command triggers confirm",
			tool:    "Bash",
			command: "git commit -m fix",
			want:    config.PolicyConfirm,
		},
		{
			name:    "non-matching command returns auto",
			tool:    "Bash",
			command: "npm install",
			want:    config.PolicyAuto,
		},
		{
			name:    "non-Bash tool with same pattern returns auto",
			tool:    "Write",
			command: "",
			want:    config.PolicyAuto,
		},
		{
			name:    "exact prefix match triggers confirm",
			tool:    "Bash",
			command: "git commit",
			want:    config.PolicyConfirm,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			args := map[string]any{"command": tc.command}

			got := p.Evaluate(tc.tool, args)
			if got != tc.want {
				t.Errorf("Evaluate(%q, command=%q) = %v, want %v", tc.tool, tc.command, got, tc.want)
			}
		})
	}
}

func TestToolPolicyEvaluateGlob(t *testing.T) {
	t.Parallel()

	p := config.ToolPolicy{Confirm: []string{"mcp__*"}}

	tests := []struct {
		name string
		tool string
		want config.PolicyAction
	}{
		{
			name: "mcp tool matches glob",
			tool: "mcp__server__tool",
			want: config.PolicyConfirm,
		},
		{
			name: "Bash does not match mcp glob",
			tool: "Bash",
			want: config.PolicyAuto,
		},
		{
			name: "Read does not match mcp glob",
			tool: "Read",
			want: config.PolicyAuto,
		},
		{
			name: "mcp prefix only matches glob",
			tool: "mcp__x",
			want: config.PolicyConfirm,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := p.Evaluate(tc.tool, nil)
			if got != tc.want {
				t.Errorf("Evaluate(%q) = %v, want %v", tc.tool, got, tc.want)
			}
		})
	}
}

func TestToolPolicyEvaluatePathPattern(t *testing.T) {
	t.Parallel()

	p := config.ToolPolicy{Confirm: []string{"Write:/tmp/*"}}

	tests := []struct {
		name string
		tool string
		args map[string]any
		want config.PolicyAction
	}{
		{
			name: "matching path triggers confirm",
			tool: "Write",
			args: map[string]any{"path": "/tmp/foo.txt"},
			want: config.PolicyConfirm,
		},
		{
			name: "subdirectory path also matches (wildcard crosses /)",
			tool: "Write",
			args: map[string]any{"path": "/tmp/sub/deep/file.txt"},
			want: config.PolicyConfirm,
		},
		{
			name: "non-matching path returns auto",
			tool: "Write",
			args: map[string]any{"path": "/home/user/file.txt"},
			want: config.PolicyAuto,
		},
		{
			name: "non-Write tool with same path returns auto",
			tool: "Read",
			args: map[string]any{"path": "/tmp/foo.txt"},
			want: config.PolicyAuto,
		},
		{
			name: "no path arg returns auto (bare name doesn't match path pattern)",
			tool: "Write",
			args: nil,
			want: config.PolicyAuto,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := p.Evaluate(tc.tool, tc.args)
			if got != tc.want {
				t.Errorf("Evaluate(%q, %v) = %v, want %v", tc.tool, tc.args, got, tc.want)
			}
		})
	}
}

func TestToolPolicyEvaluatePathAutoOverridesConfirm(t *testing.T) {
	t.Parallel()

	// Confirm all Write calls, but auto-approve writes under /tmp/.
	p := config.ToolPolicy{
		Confirm: []string{"Write"},
		Auto:    []string{"Write:/tmp/*"},
	}

	tests := []struct {
		name string
		args map[string]any
		want config.PolicyAction
	}{
		{
			name: "write to /tmp auto-approved",
			args: map[string]any{"path": "/tmp/foo.txt"},
			want: config.PolicyAuto,
		},
		{
			name: "write to /home still requires confirm",
			args: map[string]any{"path": "/home/user/bar.txt"},
			want: config.PolicyConfirm,
		},
		{
			name: "write with no path still requires confirm",
			args: nil,
			want: config.PolicyConfirm,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := p.Evaluate("Write", tc.args)
			if got != tc.want {
				t.Errorf("Evaluate(Write, %v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestToolPolicyDenyPathPattern(t *testing.T) {
	t.Parallel()

	p := config.ToolPolicy{Deny: []string{"Write:/etc/*"}}

	tests := []struct {
		name        string
		tool        string
		args        map[string]any
		wantAction  config.PolicyAction
		wantPattern string
	}{
		{
			name:        "write to /etc blocked",
			tool:        "Write",
			args:        map[string]any{"path": "/etc/hosts"},
			wantAction:  config.PolicyDeny,
			wantPattern: "Write:/etc/*",
		},
		{
			name:        "write to /tmp allowed",
			tool:        "Write",
			args:        map[string]any{"path": "/tmp/foo.txt"},
			wantAction:  config.PolicyAuto,
			wantPattern: "",
		},
		{
			name:        "read from /etc not blocked (different tool)",
			tool:        "Read",
			args:        map[string]any{"path": "/etc/hosts"},
			wantAction:  config.PolicyAuto,
			wantPattern: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := p.Evaluate(tc.tool, tc.args)
			if got != tc.wantAction {
				t.Errorf("Evaluate(%q, %v) = %v, want %v", tc.tool, tc.args, got, tc.wantAction)
			}

			gotPattern := p.DenyingPattern(tc.tool, tc.args)
			if gotPattern != tc.wantPattern {
				t.Errorf("DenyingPattern(%q, %v) = %q, want %q", tc.tool, tc.args, gotPattern, tc.wantPattern)
			}
		})
	}
}

func TestToolPolicyBareNameMatchesPathCall(t *testing.T) {
	t.Parallel()

	// A bare-name pattern ("Write" with no arg qualifier) matches all Write calls,
	// including those with path arguments.
	p := config.ToolPolicy{Auto: []string{"Write"}, Confirm: []string{"Write"}}

	got := p.Evaluate("Write", map[string]any{"path": "/any/path/file.txt"})
	if got != config.PolicyAuto {
		t.Errorf("bare Write pattern should match path-qualified call, got %v", got)
	}
}

func TestExtractToolDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		toolName string
		args     map[string]any
		want     string
	}{
		{
			name:     "Bash extracts command",
			toolName: "Bash",
			args:     map[string]any{"command": "git status"},
			want:     "git status",
		},
		{
			name:     "Write extracts path",
			toolName: "Write",
			args:     map[string]any{"path": "/tmp/foo.txt"},
			want:     "/tmp/foo.txt",
		},
		{
			name:     "Read extracts path",
			toolName: "Read",
			args:     map[string]any{"path": "/etc/hosts"},
			want:     "/etc/hosts",
		},
		{
			name:     "file_path fallback",
			toolName: "CustomTool",
			args:     map[string]any{"file_path": "/home/user/doc.md"},
			want:     "/home/user/doc.md",
		},
		{
			name:     "path takes precedence over file_path",
			toolName: "Write",
			args:     map[string]any{"path": "/tmp/a.txt", "file_path": "/tmp/b.txt"},
			want:     "/tmp/a.txt",
		},
		{
			name:     "no matching key returns empty",
			toolName: "Patch",
			args:     map[string]any{"patch": "some diff content"},
			want:     "",
		},
		{
			name:     "nil args returns empty",
			toolName: "Write",
			args:     nil,
			want:     "",
		},
		{
			name:     "Bash with no command returns empty",
			toolName: "Bash",
			args:     map[string]any{},
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := config.ExtractToolDetail(tc.toolName, tc.args)
			if got != tc.want {
				t.Errorf("ExtractToolDetail(%q, %v) = %q, want %q", tc.toolName, tc.args, got, tc.want)
			}
		})
	}
}

func TestToolPolicyMerge(t *testing.T) {
	t.Parallel()

	a := config.ToolPolicy{
		Auto:    []string{"Read", "Write"},
		Confirm: []string{"Bash"},
		Deny:    []string{"mcp__*"},
	}
	b := config.ToolPolicy{
		Auto:    []string{"Write", "Glob"},   // Write is duplicate
		Confirm: []string{"Edit", "Bash"},    // Bash is duplicate
		Deny:    []string{"mcp__*", "Patch"}, // mcp__* is duplicate
	}

	merged := a.Merge(b)

	// Auto: Read, Write, Glob — sorted, deduplicated
	wantAuto := []string{"Glob", "Read", "Write"}
	if len(merged.Auto) != len(wantAuto) {
		t.Fatalf("Auto len = %d, want %d; got %v", len(merged.Auto), len(wantAuto), merged.Auto)
	}

	for i, v := range wantAuto {
		if merged.Auto[i] != v {
			t.Errorf("Auto[%d] = %q, want %q", i, merged.Auto[i], v)
		}
	}

	// Confirm: Bash, Edit — sorted, deduplicated
	wantConfirm := []string{"Bash", "Edit"}
	if len(merged.Confirm) != len(wantConfirm) {
		t.Fatalf("Confirm len = %d, want %d; got %v", len(merged.Confirm), len(wantConfirm), merged.Confirm)
	}

	for i, v := range wantConfirm {
		if merged.Confirm[i] != v {
			t.Errorf("Confirm[%d] = %q, want %q", i, merged.Confirm[i], v)
		}
	}

	// Deny: Patch, mcp__* — sorted, deduplicated
	wantDeny := []string{"Patch", "mcp__*"}
	if len(merged.Deny) != len(wantDeny) {
		t.Fatalf("Deny len = %d, want %d; got %v", len(merged.Deny), len(wantDeny), merged.Deny)
	}

	for i, v := range wantDeny {
		if merged.Deny[i] != v {
			t.Errorf("Deny[%d] = %q, want %q", i, merged.Deny[i], v)
		}
	}
}

func TestToolPolicyIsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy config.ToolPolicy
		want   bool
	}{
		{
			name:   "zero value is empty",
			policy: config.ToolPolicy{},
			want:   true,
		},
		{
			name:   "auto list makes non-empty",
			policy: config.ToolPolicy{Auto: []string{"Read"}},
			want:   false,
		},
		{
			name:   "confirm list makes non-empty",
			policy: config.ToolPolicy{Confirm: []string{"Write"}},
			want:   false,
		},
		{
			name:   "deny list makes non-empty",
			policy: config.ToolPolicy{Deny: []string{"Bash"}},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.policy.IsEmpty()
			if got != tc.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestToolPolicyDenyingPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		policy      config.ToolPolicy
		tool        string
		args        map[string]any
		wantPattern string
	}{
		{
			name:        "exact name match returns pattern",
			policy:      config.ToolPolicy{Deny: []string{"Write"}},
			tool:        "Write",
			wantPattern: "Write",
		},
		{
			name:        "glob match returns pattern",
			policy:      config.ToolPolicy{Deny: []string{"mcp__*"}},
			tool:        "mcp__server__tool",
			wantPattern: "mcp__*",
		},
		{
			name:        "bash command match returns pattern",
			policy:      config.ToolPolicy{Deny: []string{"Bash:rm -rf*"}},
			tool:        "Bash",
			args:        map[string]any{"command": "rm -rf /tmp/foo"},
			wantPattern: "Bash:rm -rf*",
		},
		{
			name:        "no match returns empty string",
			policy:      config.ToolPolicy{Deny: []string{"Write"}},
			tool:        "Read",
			wantPattern: "",
		},
		{
			name:        "empty policy returns empty string",
			policy:      config.ToolPolicy{},
			tool:        "Write",
			wantPattern: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.policy.DenyingPattern(tc.tool, tc.args)
			if got != tc.wantPattern {
				t.Errorf("DenyingPattern(%q) = %q, want %q", tc.tool, got, tc.wantPattern)
			}
		})
	}
}

func TestToolPolicyDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		policy   config.ToolPolicy
		contains []string
		absent   []string
	}{
		{
			name: "auto section present",
			policy: config.ToolPolicy{
				Auto: []string{"Read", "Glob"},
			},
			contains: []string{"Auto-approved:", "Read", "Glob"},
		},
		{
			name: "confirm section present",
			policy: config.ToolPolicy{
				Confirm: []string{"Write", "Edit"},
			},
			contains: []string{"Requires confirmation:", "Write", "Edit", "pause for user confirmation"},
		},
		{
			name: "deny section present",
			policy: config.ToolPolicy{
				Deny: []string{"Bash"},
			},
			contains: []string{"Blocked:", "Bash"},
		},
		{
			name: "all sections present",
			policy: config.ToolPolicy{
				Auto:    []string{"Read"},
				Confirm: []string{"Write"},
				Deny:    []string{"Bash"},
			},
			contains: []string{
				"Auto-approved:", "Read",
				"Requires confirmation:", "Write",
				"Blocked:", "Bash",
				"Precedence:",
			},
		},
		{
			name:     "empty policy still contains precedence line",
			policy:   config.ToolPolicy{},
			contains: []string{"Precedence:"},
			absent:   []string{"Auto-approved:", "Requires confirmation:", "Blocked:"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.policy.Display()

			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Display() does not contain %q\nOutput:\n%s", want, got)
				}
			}

			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Errorf("Display() should not contain %q\nOutput:\n%s", absent, got)
				}
			}
		})
	}
}

func TestDenyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
	}{
		{pattern: "Bash"},
		{pattern: "Write"},
		{pattern: "mcp__*"},
		{pattern: "Bash:git commit*"},
	}

	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			t.Parallel()

			err := config.DenyError(tc.pattern)
			if err == nil {
				t.Fatal("DenyError returned nil, want non-nil error")
			}

			if !strings.Contains(err.Error(), tc.pattern) {
				t.Errorf("DenyError(%q).Error() = %q, want it to contain the pattern", tc.pattern, err.Error())
			}
		})
	}
}
