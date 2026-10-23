package edit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/diffpreview"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/tool"
)

func replacement(old, text string, all bool) Replacement {
	return Replacement{OldString: old, NewString: &text, ReplaceAll: all}
}

func TestMultipleEditsPreserveOriginalBytes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, original, want string
		edits                []Replacement
		count                int
	}{
		{"reverse order and changing lengths", "left=old\nright=old\n", "left=longer\nright=x\n", []Replacement{replacement("right=old", "right=x", false), replacement("left=old", "left=longer", false)}, 2},
		{"deletion and adjacent matches", "abc", "AC", []Replacement{replacement("a", "A", false), replacement("b", "", false), replacement("c", "C", false)}, 3},
		{"replacement is another original match", "left=old\nright=old\n", "right=old\nright=new\n", []Replacement{replacement("left=old", "right=old", false), replacement("right=old", "right=new", false)}, 2},
		{"replace all", "old x old y", "new X new y", []Replacement{replacement("old", "new", true), replacement(" x ", " X ", false)}, 3},
		{"exact before normalized lookalike", "a  \nb\na\nb\nend\n", "a  \nb\nchanged\nEND\n", []Replacement{replacement("a\nb", "changed", false), replacement("end", "END", false)}, 2},
		{"mixed endings unicode and whitespace", "\ufefflabel=Ａ—Ｂ; count=1  \r\nnext=2\r\nkeep  \nlast=3\n", "\ufefflabel=Ａ—Ｂ; count=4\r\nnext=5\r\nkeep  \nlast=6\n", []Replacement{replacement("count=1\nnext=2", "count=4\nnext=5", false), replacement("last=3", "last=6", false)}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, count, err := replaceMany(test.original, test.edits)
			if err != nil || got != test.want || count != test.count {
				t.Fatalf("replaceMany = %q, %d, %v; want %q, %d", got, count, err, test.want, test.count)
			}
		})
	}
}

func TestMultipleEditsRejectWholeGroup(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, original, message string
		edits                   []Replacement
	}{
		{"missing later match", "abc def", "edit 2", []Replacement{replacement("abc", "changed", false), replacement("missing", "X", false)}},
		{"depends on earlier replacement", "abc", "edit 2", []Replacement{replacement("abc", "def", false), replacement("def", "xyz", false)}},
		{"ambiguous later match", "abc def def", "2 matches", []Replacement{replacement("abc", "changed", false), replacement("def", "X", false)}},
		{"overlapping ranges", "abcdef", "overlap", []Replacement{replacement("abc", "X", false), replacement("bcde", "Y", false)}},
		{"duplicate ranges", "abc", "overlap", []Replacement{replacement("abc", "X", false), replacement("abc", "Y", false)}},
		{"replace all overlaps another edit", "abc abc def", "overlap", []Replacement{replacement("abc", "X", true), replacement("abc def", "Y", false)}},
		{"normalized range overlaps exact", "a  \r\nb\r\n", "overlap", []Replacement{replacement("a\nb", "X", false), replacement("b", "Y", false)}},
		{"no-op later edit", "abc def", "identical", []Replacement{replacement("abc", "X", false), replacement("def", "def", false)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "file.txt")
			if err := os.WriteFile(path, []byte(test.original), 0o600); err != nil {
				t.Fatal(err)
			}
			tool := New(true)
			args := map[string]any{"file_path": path, "edits": test.edits}
			if _, err := tool.Preview(t.Context(), args); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("preview error: %v, want %q", err, test.message)
			}
			if _, err := tool.Execute(t.Context(), args); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("execute error: %v, want %q", err, test.message)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != test.original {
				t.Fatalf("rejected group changed file: %q, %v", got, err)
			}
		})
	}
}

func TestMultipleEditsValidateNestedArguments(t *testing.T) {
	t.Parallel()
	tool := New(true)
	for _, edits := range []any{
		nil, []any{}, "not an array", []any{nil}, []any{map[string]any{}},
		[]any{map[string]any{"old_string": "x"}},
		[]any{map[string]any{"old_string": "", "new_string": "y"}},
		[]any{map[string]any{"old_string": "x", "new_string": nil}},
		[]any{map[string]any{"old_string": "x", "new_string": 3}},
		[]any{map[string]any{"old_string": "x", "new_string": "y", "replace_all": "true"}},
		[]any{map[string]any{"old_string": "x", "new_string": "y", "typo": true}},
	} {
		if _, err := tool.inputs(map[string]any{"file_path": "/tmp/unused", "edits": edits}); err == nil {
			t.Errorf("accepted invalid edits: %#v", edits)
		}
	}
	if _, err := tool.inputs(map[string]any{"file_path": "/tmp/unused"}); err == nil {
		t.Error("accepted missing edits")
	}
}

func TestMultipleEditsUseExistingLifecycle(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "file.txt")
	const original = "first=1\r\nsecond=2\r\n"
	const want = "first=3\r\nsecond=4\r\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	editTool := New(true)
	args := map[string]any{"file_path": path, "edits": []Replacement{replacement("first=1", "first=3", false), replacement("second=2", "second=4", false)}}
	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())
	ctx := filetime.WithTracker(t.Context(), tracker)
	if err := editTool.Pre(ctx, args); err == nil {
		t.Fatal("allowed editing without a prior read")
	}
	tracker.RecordRead(path)
	if err := editTool.Pre(ctx, args); err != nil {
		t.Fatal(err)
	}
	read, write, err := editTool.Paths(ctx, args)
	if err != nil || !reflect.DeepEqual(read, []string{path}) || !reflect.DeepEqual(write, []string{path}) {
		t.Fatalf("paths: %v, %v, %v", read, write, err)
	}
	preview, err := editTool.Preview(ctx, args)
	if err != nil || preview != diffpreview.Generate(path, original, want) {
		t.Fatalf("preview: %q, %v", preview, err)
	}
	if output, err := editTool.Execute(ctx, args); err != nil || !strings.Contains(output, "2 replacement(s)") {
		t.Fatalf("execute: %q, %v", output, err)
	}
	tracker.ClearRead(path)
	editTool.Post(ctx, args)
	if err := tracker.AssertRead(path); err != nil {
		t.Fatalf("post hook did not update known content: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("file: %q, %v", got, err)
	}
}

func TestEditExposesOnlySelectedArgumentForm(t *testing.T) {
	t.Parallel()
	if got := New(false).Schema().ParamNames(); !reflect.DeepEqual(got, []string{"file_path", "new_string", "old_string", "replace_all"}) {
		t.Fatalf("single-edit schema changed: %v", got)
	}
	grouped := New(true).Schema()
	if got := grouped.ParamNames(); !reflect.DeepEqual(got, []string{"edits", "file_path"}) {
		t.Fatalf("grouped schema: %v", got)
	}
	edits, _ := grouped.Parameters.Properties.Get("edits")
	if edits.Items == nil || edits.Items.Properties == nil || !reflect.DeepEqual(edits.Items.Required, []string{"old_string", "new_string"}) {
		t.Fatalf("replacement schema lost its required text fields: %#v", edits.Items)
	}
}
