package patch

import (
	"context"
	"errors"
	"strings"
)

// Hunk represents a single patch operation with behavioral methods.
type Hunk interface {
	isHunk()
	Execute(ctx context.Context) (string, error)
	Validate(ctx context.Context) error
	Track(ctx context.Context)
	WritePaths() []string
}

// AddFile represents a file creation operation.
type AddFile struct {
	Path    string
	Content string
}

func (*AddFile) isHunk() {}

// DeleteFile represents a file deletion operation.
type DeleteFile struct {
	Path string
}

func (*DeleteFile) isHunk() {}

// UpdateFile represents a file modification operation.
type UpdateFile struct {
	Path   string
	MoveTo string // Optional: rename/move destination
	Chunks []Chunk
}

func (*UpdateFile) isHunk() {}

// Chunk represents a single change within an UpdateFile.
type Chunk struct {
	Context  string   // @@ marker content for locating the change
	OldLines []string // Lines to find and remove (from - and space lines)
	NewLines []string // Lines to insert (from + and space lines)
}

// Parse parses a patch string into a slice of Hunks.
// Supports multiple *** Begin Patch / *** End Patch blocks in a single input.
func Parse(input string) ([]Hunk, error) {
	lines := strings.Split(input, "\n")

	var hunks []Hunk

	i := 0
	foundPatch := false

	for i < len(lines) {
		// Find next *** Begin Patch
		for i < len(lines) {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "*** Begin Patch") {
				foundPatch = true
				i++

				break
			}

			i++
		}

		if i >= len(lines) {
			break
		}

		// Parse hunks until *** End Patch
		for i < len(lines) {
			line := strings.TrimSpace(lines[i])

			if strings.HasPrefix(line, "*** End Patch") {
				i++

				break
			}

			if strings.HasPrefix(line, "*** Add File:") {
				hunk, newIdx, err := parseAddFile(lines, i)
				if err != nil {
					return nil, err
				}

				hunks = append(hunks, hunk)
				i = newIdx

				continue
			}

			if after, ok := strings.CutPrefix(line, "*** Delete File:"); ok {
				path := after

				path = strings.TrimSpace(path)
				hunks = append(hunks, &DeleteFile{Path: path})
				i++

				continue
			}

			if strings.HasPrefix(line, "*** Update File:") {
				hunk, newIdx, err := parseUpdateFile(lines, i)
				if err != nil {
					return nil, err
				}

				hunks = append(hunks, hunk)
				i = newIdx

				continue
			}

			i++
		}
	}

	if !foundPatch {
		return nil, errors.New("no '*** Begin Patch' marker found")
	}

	return hunks, nil
}

func parseAddFile(lines []string, start int) (*AddFile, int, error) {
	line := strings.TrimSpace(lines[start])
	path := strings.TrimPrefix(line, "*** Add File:")

	path = strings.TrimSpace(path)

	i := start + 1

	var content []string

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Stop at next operation or end
		if strings.HasPrefix(trimmed, "*** ") {
			break
		}

		// Add lines must start with +
		if after, ok := strings.CutPrefix(line, "+"); ok {
			content = append(content, after)
		} else if line == "" {
			// Allow empty lines in content
			content = append(content, "")
		}

		i++
	}

	return &AddFile{
		Path:    path,
		Content: strings.Join(content, "\n"),
	}, i, nil
}

func parseUpdateFile(lines []string, start int) (*UpdateFile, int, error) {
	line := strings.TrimSpace(lines[start])
	path := strings.TrimPrefix(line, "*** Update File:")

	path = strings.TrimSpace(path)

	update := &UpdateFile{Path: path}
	i := start + 1

	// Check for *** Move to:
	if i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if after, ok := strings.CutPrefix(trimmed, "*** Move to:"); ok {
			update.MoveTo = strings.TrimSpace(after)
			i++
		}
	}

	// Parse chunks
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Stop at next file operation
		if strings.HasPrefix(trimmed, "*** Add File:") ||
			strings.HasPrefix(trimmed, "*** Update File:") ||
			strings.HasPrefix(trimmed, "*** Delete File:") ||
			strings.HasPrefix(trimmed, "*** End Patch") {
			break
		}

		// Start of a chunk - either @@ context or direct diff lines
		if strings.HasPrefix(trimmed, "@@") || strings.HasPrefix(line, "+") ||
			strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") {
			chunk, newIdx := parseChunk(lines, i)

			update.Chunks = append(update.Chunks, chunk)
			i = newIdx

			continue
		}

		i++
	}

	return update, i, nil
}

func parseChunk(lines []string, start int) (Chunk, int) {
	chunk := Chunk{}
	i := start

	// Check for @@ context marker
	trimmed := strings.TrimSpace(lines[i])
	if after, ok := strings.CutPrefix(trimmed, "@@"); ok {
		context := after

		context = strings.TrimSpace(context)
		chunk.Context = context
		i++
	}

	// Parse diff lines
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Stop at next chunk marker, file operation, or end
		if strings.HasPrefix(trimmed, "@@") ||
			strings.HasPrefix(trimmed, "*** ") {
			break
		}

		if after, ok := strings.CutPrefix(line, "+"); ok {
			// Add line - goes to new lines only
			chunk.NewLines = append(chunk.NewLines, after)
			i++
		} else if after, ok := strings.CutPrefix(line, "-"); ok {
			// Remove line - goes to old lines only
			chunk.OldLines = append(chunk.OldLines, after)
			i++
		} else if after, ok := strings.CutPrefix(line, " "); ok {
			// Context line - goes to both
			content := after

			chunk.OldLines = append(chunk.OldLines, content)
			chunk.NewLines = append(chunk.NewLines, content)
			i++
		} else if line == "" {
			// Empty line in diff - treat as context
			chunk.OldLines = append(chunk.OldLines, "")
			chunk.NewLines = append(chunk.NewLines, "")
			i++
		} else {
			// Non-diff line - stop parsing this chunk
			break
		}
	}

	return chunk, i
}
