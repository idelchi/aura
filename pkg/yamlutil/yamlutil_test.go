package yamlutil_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/yamlutil"
)

type testConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

func TestStrictUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantCfg testConfig
	}{
		{
			name:    "valid YAML into struct",
			input:   "name: hello\nport: 8080\n",
			wantErr: false,
			wantCfg: testConfig{Name: "hello", Port: 8080},
		},
		{
			name:    "unknown field rejected",
			input:   "name: hello\nport: 8080\nunknown: extra\n",
			wantErr: true,
		},
		{
			name:    "empty document returns no error",
			input:   "",
			wantErr: false,
			wantCfg: testConfig{},
		},
		{
			name:    "comment-only document returns no error",
			input:   "# just a comment\n",
			wantErr: false,
			wantCfg: testConfig{},
		},
		{
			name:    "type mismatch returns error",
			input:   "name: hello\nport: notanint\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg testConfig

			err := yamlutil.StrictUnmarshal([]byte(tt.input), &cfg)

			if tt.wantErr {
				if err == nil {
					t.Errorf("StrictUnmarshal(%q) = nil, want error", tt.input)
				}

				return
			}

			if err != nil {
				t.Errorf("StrictUnmarshal(%q) unexpected error: %v", tt.input, err)

				return
			}

			if cfg.Name != tt.wantCfg.Name {
				t.Errorf("Name = %q, want %q", cfg.Name, tt.wantCfg.Name)
			}

			if cfg.Port != tt.wantCfg.Port {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantCfg.Port)
			}
		})
	}
}
