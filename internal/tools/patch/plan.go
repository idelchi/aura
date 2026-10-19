package patch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/idelchi/aura/internal/diffpreview"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
)

type stagedFile struct {
	content string
	exists  bool
}

// preparation applies operations to an in-memory view before any file is changed.
// Later operations see earlier adds, edits, deletions and moves in the same patch.
type preparation struct {
	ctx   context.Context
	files map[string]stagedFile
}

func (p *preparation) path(path string) string {
	return filepath.Clean(tool.ResolvePath(p.ctx, os.ExpandEnv(path)))
}

func (p *preparation) read(path string, required bool) (stagedFile, error) {
	state, found := p.files[path]
	if !found {
		data, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return stagedFile{}, tool.FileError("reading", path, err)
		}
		state = stagedFile{content: string(data), exists: err == nil}
		p.files[path] = state
	}
	if required && !state.exists {
		return stagedFile{}, fmt.Errorf("patch failed: %s does not exist", path)
	}
	return state, nil
}

type change struct {
	kind   string
	path   string
	target string
	before string
	after  string
}

func (h *AddFile) Prepare(p *preparation) (change, error) {
	path := p.path(h.Path)
	previous, err := p.read(path, false)
	if err != nil {
		return change{}, err
	}
	p.files[path] = stagedFile{content: h.Content, exists: true}
	return change{kind: "A", path: path, target: path, before: previous.content, after: h.Content}, nil
}

func (h *DeleteFile) Prepare(p *preparation) (change, error) {
	path := p.path(h.Path)
	previous, err := p.read(path, true)
	if err != nil {
		return change{}, err
	}
	p.files[path] = stagedFile{}
	return change{kind: "D", path: path, before: previous.content}, nil
}

func (h *UpdateFile) Prepare(p *preparation) (change, error) {
	path := p.path(h.Path)
	previous, err := p.read(path, true)
	if err != nil {
		return change{}, err
	}
	content, err := h.ApplyToContent(previous.content)
	if err != nil {
		return change{}, err
	}
	target := path
	kind := "M"
	if h.MoveTo != "" {
		target = p.path(h.MoveTo)
		if target != path {
			if _, err := p.read(target, false); err != nil {
				return change{}, err
			}
			p.files[path] = stagedFile{}
			kind = "R"
		}
	}
	p.files[target] = stagedFile{content: content, exists: true}
	return change{kind: kind, path: path, target: target, before: previous.content, after: content}, nil
}

type plan struct{ changes []change }

func prepare(ctx context.Context, hunks []Hunk) (plan, error) {
	if len(hunks) == 0 {
		return plan{}, errors.New("no operations found in patch")
	}
	state := preparation{ctx: ctx, files: make(map[string]stagedFile)}
	var result plan
	for _, hunk := range hunks {
		if err := ctx.Err(); err != nil {
			return plan{}, err
		}
		prepared, err := hunk.Prepare(&state)
		if err != nil {
			return plan{}, err
		}
		result.changes = append(result.changes, prepared)
	}
	return result, nil
}

func (c change) String() string {
	if c.kind == "R" {
		return "R " + c.path + " -> " + c.target
	}
	return c.kind + " " + c.path
}

func (c change) Preview() string {
	switch c.kind {
	case "A":
		return diffpreview.Generate(c.path, c.before, c.after, "")
	case "D":
		return diffpreview.ForDeletedFile(c.path, c.before)
	case "R":
		return diffpreview.Generate(c.path, c.before, c.after, c.target)
	default:
		return diffpreview.Generate(c.path, c.before, c.after, "")
	}
}

func (c change) Apply() error {
	if c.kind == "D" {
		return os.Remove(c.path)
	}
	if err := os.MkdirAll(filepath.Dir(c.target), 0o755); err != nil {
		return tool.FileError("creating directory", filepath.Dir(c.target), err)
	}
	if err := file.New(c.target).Write([]byte(c.after)); err != nil {
		return tool.FileError("writing", c.target, err)
	}
	if c.kind == "R" {
		if err := os.Remove(c.path); err != nil {
			return fmt.Errorf("wrote %s but could not remove %s: %w", c.target, c.path, err)
		}
	}
	return nil
}

func (p plan) Apply(ctx context.Context) (string, error) {
	var applied []string
	for _, c := range p.changes {
		err := ctx.Err()
		if err == nil {
			err = c.Apply()
		}
		if err != nil {
			if len(applied) > 0 {
				err = fmt.Errorf("patch stopped; already applied:\n%s\n%w", strings.Join(applied, "\n"), err)
			}
			return "", err
		}
		applied = append(applied, c.String())
	}
	return "Success. Updated files:\n" + strings.Join(applied, "\n"), nil
}
