package gotify

import (
	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

// Schema declares the tool's function-calling schema.
func Schema() sdk.ToolSchema {
	return sdk.ToolSchema{
		Name: "Gotify",
		Description: heredoc.Doc(`
			Send a push notification via Gotify.
			Use this to alert the user about important findings, task completions,
			or critical issues that need attention.
		`),
		Usage: heredoc.Doc(`
			Provide a title and message for the notification.
			Set level to indicate severity: INFO for status updates,
			WARNING for issues that need attention, ERROR for critical problems.
		`),
		Examples: heredoc.Doc(`
			{"title": "Task Complete", "message": "Finished processing all files"}
			{"title": "Alert", "message": "Found 3 critical vulnerabilities", "level": "ERROR"}
			{"title": "Status Update", "message": "Build succeeded", "level": "INFO"}
		`),
		Parameters: sdk.ToolParameters{
			Type: "object",
			Properties: map[string]sdk.ToolProperty{
				"title": {
					Type:        "string",
					Description: "Notification title",
				},
				"message": {
					Type:        "string",
					Description: "Notification body",
				},
				"level": {
					Type:        "string",
					Description: "Severity level (default INFO)",
					Enum:        []any{"INFO", "WARNING", "ERROR"},
				},
			},
			Required: []string{"title", "message"},
		},
	}
}
