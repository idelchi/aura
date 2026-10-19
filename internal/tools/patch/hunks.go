package patch

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// AddFile behavioral methods

func (h *AddFile) Track(ctx context.Context) {
	filetime.FromContext(ctx).RecordRead(tool.ResolvePath(ctx, os.ExpandEnv(h.Path)))
}

func (h *AddFile) WritePaths() []string {
	return []string{os.ExpandEnv(h.Path)}
}

// DeleteFile behavioral methods

func (h *DeleteFile) Track(ctx context.Context) {
	filetime.FromContext(ctx).ClearRead(tool.ResolvePath(ctx, os.ExpandEnv(h.Path)))
}

func (h *DeleteFile) WritePaths() []string {
	return []string{os.ExpandEnv(h.Path)}
}

// UpdateFile behavioral methods

// ApplyToContent applies the chunks to the given content string and returns the result.
// Does not touch the filesystem.
func (h *UpdateFile) ApplyToContent(content string) (string, error) {
	lines := strings.Split(content, "\n")

	for _, chunk := range h.Chunks {
		startIdx := 0

		if chunk.Context != "" {
			ctxIdx := SeekLine(lines, chunk.Context, 0)
			if ctxIdx < 0 {
				return "", fmt.Errorf("finding context %q in %s", chunk.Context, h.Path)
			}

			startIdx = ctxIdx + 1
		}

		idx, found := SeekSequence(lines, chunk.OldLines, startIdx)
		if !found {
			if chunk.Context != "" {
				// Backtrack by OldLines length to catch overlap with context line.
				// Don't scan entire file — risks matching wrong location.
				backtrack := max(0, startIdx-len(chunk.OldLines))

				idx, found = SeekSequence(lines, chunk.OldLines, backtrack)
			} else {
				// No context marker — full file search is fine.
				idx, found = SeekSequence(lines, chunk.OldLines, 0)
			}

			if !found {
				preview := strings.Join(chunk.OldLines, "\n")
				if len(preview) > 100 {
					preview = preview[:100] + "..."
				}

				return "", fmt.Errorf("finding lines to replace in %s:\n%s", h.Path, preview)
			}
		}

		lines = slices.Replace(lines, idx, idx+len(chunk.OldLines), chunk.NewLines...)
	}

	return strings.Join(lines, "\n"), nil
}

func (h *UpdateFile) Track(ctx context.Context) {
	tracker := filetime.FromContext(ctx)

	if h.MoveTo != "" {
		tracker.ClearRead(tool.ResolvePath(ctx, os.ExpandEnv(h.Path)))
		tracker.RecordRead(tool.ResolvePath(ctx, os.ExpandEnv(h.MoveTo)))
	} else {
		tracker.RecordRead(tool.ResolvePath(ctx, os.ExpandEnv(h.Path)))
	}
}

func (h *UpdateFile) WritePaths() []string {
	paths := []string{os.ExpandEnv(h.Path)}
	if h.MoveTo != "" {
		paths = append(paths, os.ExpandEnv(h.MoveTo))
	}

	return paths
}
