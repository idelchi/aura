package mcp

import (
	"errors"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func TestServerIsEnabled(t *testing.T) {
	t.Parallel()

	if got := (Server{Disabled: false}).IsEnabled(); !got {
		t.Errorf("Server{Disabled: false}.IsEnabled() = %v, want true", got)
	}

	if got := (Server{Disabled: true}).IsEnabled(); got {
		t.Errorf("Server{Disabled: true}.IsEnabled() = %v, want false", got)
	}
}

func TestToolName(t *testing.T) {
	t.Parallel()

	tool := &Tool{
		serverName: "myserver",
		mcpTool:    mcplib.Tool{Name: "mytool"},
	}

	got := tool.Name()
	want := "mcp__myserver__mytool"

	if got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestFormatResultText(t *testing.T) {
	t.Parallel()

	content := []mcplib.Content{
		mcplib.TextContent{Text: "hello"},
	}

	got := formatResult(content)
	if got != "hello" {
		t.Errorf("formatResult() = %q, want %q", got, "hello")
	}
}

func TestFormatResultMultiple(t *testing.T) {
	t.Parallel()

	content := []mcplib.Content{
		mcplib.TextContent{Text: "line one"},
		mcplib.TextContent{Text: "line two"},
		mcplib.TextContent{Text: "line three"},
	}

	got := formatResult(content)
	want := "line one\nline two\nline three"

	if got != want {
		t.Errorf("formatResult() = %q, want %q", got, want)
	}
}

func TestResultStatusDisplayError(t *testing.T) {
	t.Parallel()

	r := Result{
		Name:  "srv",
		Error: errors.New("fail"),
	}

	got := r.StatusDisplay()

	if !strings.Contains(got, "srv") {
		t.Errorf("StatusDisplay() = %q, want it to contain %q", got, "srv")
	}

	if !strings.Contains(got, "fail") {
		t.Errorf("StatusDisplay() = %q, want it to contain %q", got, "fail")
	}
}

func TestResultStatusDisplaySuccess(t *testing.T) {
	t.Parallel()

	sess := &Session{
		Name: "srv",
		MCPTools: []mcplib.Tool{
			{Name: "tool1"},
			{Name: "tool2"},
			{Name: "tool3"},
		},
	}

	r := Result{
		Name:    "srv",
		Session: sess,
	}

	got := r.StatusDisplay()

	if !strings.Contains(got, "srv") {
		t.Errorf("StatusDisplay() = %q, want it to contain %q", got, "srv")
	}

	if !strings.Contains(got, "3") {
		t.Errorf("StatusDisplay() = %q, want it to contain tool count %q", got, "3")
	}
}

func TestConvertSchema(t *testing.T) {
	t.Parallel()

	mcpTool := mcplib.Tool{
		Name:        "do_thing",
		Description: "does the thing",
		InputSchema: mcplib.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "the file path",
				},
				"count": map[string]any{
					"type":        "integer",
					"description": "how many times",
				},
			},
			Required: []string{"path"},
		},
	}

	schema := convertSchema("mcp__testserver__do_thing", mcpTool)

	if schema.Name != "mcp__testserver__do_thing" {
		t.Errorf("schema.Name = %q, want %q", schema.Name, "mcp__testserver__do_thing")
	}

	if schema.Description != "does the thing" {
		t.Errorf("schema.Description = %q, want %q", schema.Description, "does the thing")
	}

	if len(schema.Parameters.Required) != 1 || schema.Parameters.Required[0] != "path" {
		t.Errorf("schema.Parameters.Required = %v, want [path]", schema.Parameters.Required)
	}

	pathProp, ok := schema.Parameters.Properties["path"]
	if !ok {
		t.Fatalf("schema.Parameters.Properties missing %q key", "path")
	}

	if pathProp.Type != "string" {
		t.Errorf("path property Type = %q, want %q", pathProp.Type, "string")
	}

	if pathProp.Description != "the file path" {
		t.Errorf("path property Description = %q, want %q", pathProp.Description, "the file path")
	}

	countProp, ok := schema.Parameters.Properties["count"]
	if !ok {
		t.Fatalf("schema.Parameters.Properties missing %q key", "count")
	}

	if countProp.Type != "integer" {
		t.Errorf("count property Type = %q, want %q", countProp.Type, "integer")
	}
}
