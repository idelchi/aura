// Package diffpreview generates unified diff strings for file modifications.
package diffpreview

import (
	"bytes"
	"crypto/sha1"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	godiff "github.com/go-git/go-git/v5/utils/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// Generate produces a unified diff string from original and modified content.
// path is used for the --- / +++ headers. moveTo optionally specifies a rename destination.
func Generate(path, original, modified string, moveTo ...string) string {
	diffs := godiff.Do(original, modified)
	chunks := convertChunks(diffs)

	fromFile := &file{path: path, content: original}
	toPath := path

	if len(moveTo) > 0 && moveTo[0] != "" {
		toPath = moveTo[0]
	}

	toFile := &file{path: toPath, content: modified}

	return encode(&patch{
		filePatches: []diff.FilePatch{
			&filePatch{from: fromFile, to: toFile, chunks: chunks},
		},
	})
}

// ForNewFile produces a unified diff showing all lines as additions.
func ForNewFile(path, content string) string {
	chunks := []diff.Chunk{&chunk{content: content, op: diff.Add}}

	return encode(&patch{
		filePatches: []diff.FilePatch{
			&filePatch{from: nil, to: &file{path: path, content: content}, chunks: chunks},
		},
	})
}

// ForDeletedFile produces a unified diff showing all lines as deletions.
func ForDeletedFile(path, content string) string {
	chunks := []diff.Chunk{&chunk{content: content, op: diff.Delete}}

	return encode(&patch{
		filePatches: []diff.FilePatch{
			&filePatch{from: &file{path: path, content: content}, to: nil, chunks: chunks},
		},
	})
}

func encode(p *patch) string {
	var buf bytes.Buffer

	if err := diff.NewUnifiedEncoder(&buf, diff.DefaultContextLines).Encode(p); err != nil {
		return "(diff preview unavailable)"
	}

	return strings.TrimRight(buf.String(), "\n")
}

// convertChunks maps diffmatchpatch diffs to go-git Chunk interface values.
// Critical: DiffDelete=-1 but go-git Delete=2 — explicit mapping required.
func convertChunks(diffs []diffmatchpatch.Diff) []diff.Chunk {
	chunks := make([]diff.Chunk, len(diffs))
	for i, d := range diffs {
		chunks[i] = &chunk{
			content: d.Text,
			op:      mapOperation(d.Type),
		}
	}

	return chunks
}

func mapOperation(op diffmatchpatch.Operation) diff.Operation {
	switch op {
	case diffmatchpatch.DiffInsert:
		return diff.Add
	case diffmatchpatch.DiffDelete:
		return diff.Delete
	default:
		return diff.Equal
	}
}

// Interface stubs for go-git's UnifiedEncoder.

type patch struct {
	filePatches []diff.FilePatch
}

func (p *patch) FilePatches() []diff.FilePatch { return p.filePatches }
func (p *patch) Message() string               { return "" }

type filePatch struct {
	from, to diff.File
	chunks   []diff.Chunk
}

func (fp *filePatch) IsBinary() bool                { return false }
func (fp *filePatch) Files() (diff.File, diff.File) { return fp.from, fp.to }
func (fp *filePatch) Chunks() []diff.Chunk          { return fp.chunks }

type file struct {
	path    string
	content string
}

func (f *file) Hash() plumbing.Hash     { return plumbing.Hash(sha1.Sum([]byte(f.content))) }
func (f *file) Mode() filemode.FileMode { return filemode.Regular }
func (f *file) Path() string            { return f.path }

type chunk struct {
	content string
	op      diff.Operation
}

func (c *chunk) Content() string      { return c.content }
func (c *chunk) Type() diff.Operation { return c.op }
