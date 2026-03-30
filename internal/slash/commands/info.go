package commands

import (
	"context"
	"math"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/godyl/pkg/pretty"
)

type infoData struct {
	Agent     infoAgent     `yaml:"agent"`
	Runtime   infoRuntime   `yaml:"runtime"`
	Policy    *infoPolicy   `yaml:"policy,omitempty"`
	Features  infoFeatures  `yaml:"features"`
	MCP       infoMCP       `yaml:"mcp"`
	Available infoAvailable `yaml:"available"`
}

type infoApprovals struct {
	Project []string `yaml:"project,omitempty"`
	Global  []string `yaml:"global,omitempty"`
}

type infoPolicy struct {
	Auto      []string      `yaml:"auto,omitempty"`
	Confirm   []string      `yaml:"confirm,omitempty"`
	Deny      []string      `yaml:"deny,omitempty"`
	Approvals infoApprovals `yaml:"approvals,omitempty"`
}

type infoAgent struct {
	Name     string   `yaml:"name"`
	Inherit  []string `yaml:"inherit,omitempty"`
	Mode     string   `yaml:"mode"`
	Provider string   `yaml:"provider"`
	Model    string   `yaml:"model"`
	Think    string   `yaml:"think"`
	Context  int      `yaml:"context"`
}

type infoSandbox struct {
	Enabled   bool `yaml:"enabled"`
	Requested bool `yaml:"requested"`
}

type infoTokens struct {
	Used    int     `yaml:"used"`
	Max     int     `yaml:"max"`
	Percent float64 `yaml:"percent"`
}

type infoRuntime struct {
	Auto      bool        `yaml:"auto"`
	Sandbox   infoSandbox `yaml:"sandbox"`
	Verbose   bool        `yaml:"verbose"`
	Tokens    infoTokens  `yaml:"tokens"`
	Iteration int         `yaml:"iteration"`
}

type infoFeatures struct {
	Compaction infoCompaction `yaml:"compaction"`
	Embeddings infoEmbeddings `yaml:"embeddings"`
	Vision     infoVision     `yaml:"vision"`
	Title      infoTitle      `yaml:"title"`
	Thinking   infoThinking   `yaml:"thinking"`
}

type infoCompaction struct {
	Threshold float64 `yaml:"threshold"`
	Agent     string  `yaml:"agent,omitempty"`
}

type infoEmbeddings struct {
	Agent      string `yaml:"agent,omitempty"`
	MaxResults int    `yaml:"max_results"`
	Strategy   string `yaml:"strategy"`
}

type infoVision struct {
	Agent string `yaml:"agent,omitempty"`
}

type infoTitle struct {
	Agent string `yaml:"agent,omitempty"`
}

type infoThinking struct {
	Agent string `yaml:"agent,omitempty"`
}

type infoMCP struct {
	Count   int      `yaml:"count"`
	Servers []string `yaml:"servers,omitempty"`
}

type infoAvailable struct {
	Agents []string `yaml:"agents"`
	Modes  []string `yaml:"modes"`
}

// Info creates the /info command to show runtime state overview.
func Info() slash.Command {
	return slash.Command{
		Name:        "/info",
		Description: "Show runtime state overview",
		Category:    "config",
		Execute: func(_ context.Context, c slash.Context, _ ...string) (string, error) {
			r := c.Resolved()
			s := c.Status()
			cfg := c.Cfg()

			// Collect MCP server names
			var mcpNames []string

			for _, session := range c.MCPSessions() {
				mcpNames = append(mcpNames, session.Name)
			}

			// Build policy section (nil = omitted from YAML)
			tp := c.ToolPolicy()

			var policy *infoPolicy

			if !tp.IsEmpty() || len(tp.ApprovalsProject()) > 0 || len(tp.ApprovalsGlobal()) > 0 {
				policy = &infoPolicy{
					Auto:    tp.Auto,
					Confirm: tp.Confirm,
					Deny:    tp.Deny,
					Approvals: infoApprovals{
						Project: tp.ApprovalsProject(),
						Global:  tp.ApprovalsGlobal(),
					},
				}
			}

			var agentInherit []string

			if agCfg := cfg.Agents.Get(r.Agent); agCfg != nil {
				agentInherit = agCfg.Metadata.Inherit
			}

			info := infoData{
				Agent: infoAgent{
					Name:     r.Agent,
					Inherit:  agentInherit,
					Mode:     r.Mode,
					Provider: r.Provider,
					Model:    r.Model,
					Think:    r.Think.AsString(),
					Context:  r.Context,
				},
				Policy: policy,
				Runtime: infoRuntime{
					Auto: r.Auto,
					Sandbox: infoSandbox{
						Enabled:   s.Sandbox.Enabled,
						Requested: s.Sandbox.Requested,
					},
					Verbose: r.Verbose,
					Tokens: infoTokens{
						Used:    s.Tokens.Used,
						Max:     s.Tokens.Max,
						Percent: math.Round(s.Tokens.Percent*100) / 100,
					},
					Iteration: c.SessionStats().Iterations,
				},
				Features: infoFeatures{
					Compaction: infoCompaction{
						Threshold: cfg.Features.Compaction.Threshold,
						Agent:     cfg.Features.Compaction.Agent,
					},
					Embeddings: infoEmbeddings{
						Agent:      cfg.Features.Embeddings.Agent,
						MaxResults: cfg.Features.Embeddings.MaxResults,
						Strategy:   cfg.Features.Embeddings.Chunking.Strategy,
					},
					Vision: infoVision{
						Agent: cfg.Features.Vision.Agent,
					},
					Title: infoTitle{
						Agent: cfg.Features.Title.Agent,
					},
					Thinking: infoThinking{
						Agent: cfg.Features.Thinking.Agent,
					},
				},
				MCP: infoMCP{
					Count:   len(c.MCPSessions()),
					Servers: mcpNames,
				},
				Available: infoAvailable{
					Agents: cfg.Agents.Filter(config.Agent.Visible).Names(),
					Modes:  cfg.Modes.Filter(config.Mode.Visible).Names(),
				},
			}

			return pretty.YAML(info), nil
		},
	}
}
