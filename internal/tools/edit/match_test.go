package edit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplace(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, content, old, replacement, want string
		all                                   bool
		count                                 int
	}{
		{name: "exact", content: "value = 1\n", old: "1", replacement: "2", want: "value = 2\n", count: 1},
		{name: "exact before relaxed", content: "a  \nb\na\nb\n", old: "a\nb", replacement: "c", want: "a  \nb\nc\n", count: 1},
		{name: "crlf", content: "a\r\nb\r\nc\r\n", old: "a\nb", replacement: "A\nB", want: "A\r\nB\r\nc\r\n", count: 1},
		{name: "trailing whitespace", content: "\told  \n\tnext\n", old: "\told\n\tnext", replacement: "\tnew\n\tnext", want: "\tnew\n\tnext\n", count: 1},
		{name: "unicode offsets", content: "åäö\n\told \r\n\tnext\r\n", old: "\told\n\tnext", replacement: "\tändrat\n\tnext", want: "åäö\n\tändrat\r\n\tnext\r\n", count: 1},
		{name: "all normalized matches", content: "a \nb\na\r\nb\n", old: "a\nb", replacement: "A\nB", want: "A\nB\nA\r\nB\n", all: true, count: 2},
		{name: "inline insertion keeps crlf", content: "old\r\n", old: "old", replacement: "new\nline", want: "new\r\nline\r\n", count: 1},
		{name: "replacement keeps lf", content: "old\n", old: "old", replacement: "new\r\nline", want: "new\nline\n", count: 1},
		{name: "does not consume following whitespace", content: "old  \nnext\n", old: "old\r\nnext", replacement: "new", want: "new\n", count: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, count, err := replace(test.content, test.old, test.replacement, test.all)
			if err != nil || got != test.want || count != test.count {
				t.Fatalf("replace = %q, %d, %v; want %q, %d", got, count, err, test.want, test.count)
			}
		})
	}
}

func TestEditAllowsDeletionButRequiresReplacementArgument(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "text.txt")
	if err := os.WriteFile(path, []byte("keep remove\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := New()
	args := map[string]any{"file_path": path, "old_string": " remove"}
	for _, value := range []any{nil, 42} {
		args["new_string"] = value
		if _, err := tool.Execute(context.Background(), args); err == nil {
			t.Fatalf("accepted new_string=%v", value)
		}
	}
	delete(args, "new_string")
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("accepted missing new_string")
	}
	args["new_string"] = ""
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("file: %q, %v", got, err)
	}
}

func TestReplaceRejectsAmbiguityAndMissingContent(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, content, old, message string }{
		{"exact ambiguity", "one\none\n", "one", "lines 1, 2"},
		{"normalized ambiguity", "one \ntwo\none\r\ntwo\n", "one\ntwo", "2 matches"},
		{"empty", "abc", "", "must not be empty"},
		{"missing", "func calculate() { return 1 }\n", "func calculate() { return 2 }", "Nearby candidate"},
		{"leading indentation", "\tfirst\n\tsecond\n", "first\nsecond", "not found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := replace(test.content, test.old, "replacement", false)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v; want %q", err, test.message)
			}
		})
	}
}

func TestMissingMatchDiagnosticIsBoundedAndUnicodeSafe(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("åäö unrelated\n", 15) + "func calculate() { return 1 }\n" + strings.Repeat("extra\n", 20)
	message := missingMatch(content, "func calculate() { return 2 }")
	if !strings.Contains(message, "16: func calculate") || len(message) > 1800 {
		t.Fatalf("unexpected diagnostic: %s", message)
	}
}

func TestEditPreviewAndExecutionAgree(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "text.txt")
	if err := os.WriteFile(path, []byte("a\r\nb\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := New()
	args := map[string]any{"file_path": path, "old_string": "a\nb", "new_string": "A\nB"}
	preview, err := tool.Preview(context.Background(), args)
	if err != nil || !strings.Contains(preview, "+A") {
		t.Fatalf("preview: %q, %v", preview, err)
	}
	out, err := tool.Execute(context.Background(), args)
	if err != nil || !strings.Contains(out, "1 replacement") {
		t.Fatalf("execute: %q, %v", out, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "A\r\nB\r\n" {
		t.Fatalf("file: %q, %v", got, err)
	}
	args["old_string"] = "something missing"
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("missing match succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(got) {
		t.Fatal("rejected edit changed the file")
	}
}
