package diffpreview

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	original := "line1\nline2\nline3\n"
	modified := "line1\nline2_modified\nline3\nline4\n"

	result := Generate("test.txt", original, modified)

	if !strings.Contains(result, "--- a/test.txt") {
		t.Errorf("expected --- a/test.txt header, got:\n%s", result)
	}

	if !strings.Contains(result, "+++ b/test.txt") {
		t.Errorf("expected +++ b/test.txt header, got:\n%s", result)
	}

	if !strings.Contains(result, "-line2") {
		t.Errorf("expected deleted line, got:\n%s", result)
	}

	if !strings.Contains(result, "+line2_modified") {
		t.Errorf("expected added line, got:\n%s", result)
	}

	if !strings.Contains(result, "+line4") {
		t.Errorf("expected new line4, got:\n%s", result)
	}
}

func TestForNewFile(t *testing.T) {
	result := ForNewFile("new.txt", "hello\nworld\n")

	if !strings.Contains(result, "new file mode") {
		t.Errorf("expected new file mode header, got:\n%s", result)
	}

	if !strings.Contains(result, "--- /dev/null") {
		t.Errorf("expected --- /dev/null, got:\n%s", result)
	}

	if !strings.Contains(result, "+hello") {
		t.Errorf("expected +hello, got:\n%s", result)
	}
}

func TestForDeletedFile(t *testing.T) {
	result := ForDeletedFile("old.txt", "goodbye\nworld\n")

	if !strings.Contains(result, "deleted file mode") {
		t.Errorf("expected deleted file mode header, got:\n%s", result)
	}

	if !strings.Contains(result, "+++ /dev/null") {
		t.Errorf("expected +++ /dev/null, got:\n%s", result)
	}

	if !strings.Contains(result, "-goodbye") {
		t.Errorf("expected -goodbye, got:\n%s", result)
	}
}

func TestGenerateWithRename(t *testing.T) {
	result := Generate("old.go", "package main\n", "package main\n\nfunc init() {}\n", "new.go")

	if !strings.Contains(result, "rename from old.go") {
		t.Errorf("expected rename from header, got:\n%s", result)
	}

	if !strings.Contains(result, "rename to new.go") {
		t.Errorf("expected rename to header, got:\n%s", result)
	}
}

func TestGenerateIdentical(t *testing.T) {
	result := Generate("same.txt", "hello\n", "hello\n")

	// Identical content should produce empty diff
	if strings.Contains(result, "---") {
		t.Errorf("expected empty diff for identical content, got:\n%s", result)
	}
}
