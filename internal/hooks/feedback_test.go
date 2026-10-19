package hooks

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPostFailureReportsStatusWithoutBlocking(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"exit 2", "echo 'invalid option' >&2; exit 2", "echo 'details on stdout'; exit 2", "exit 1"} {
		runner := Runner{Post: []Entry{{Name: "check", Command: command, Timeout: time.Second}}}
		result := runner.RunPost(context.Background(), "Patch", nil, "Success", t.TempDir())
		if result.Blocked || strings.Contains(result.Message, "blocked") || !strings.Contains(result.Message, "exited with status") {
			t.Fatalf("%q: %+v", command, result)
		}
		if strings.Contains(command, "invalid option") && !strings.Contains(result.Message, "invalid option") {
			t.Fatal("stderr was lost")
		}
		if strings.Contains(command, "details on stdout") && !strings.Contains(result.Message, "details on stdout") {
			t.Fatal("stdout was lost")
		}
	}
}

func TestPostJSONDenialIsOnlyFeedback(t *testing.T) {
	t.Parallel()
	runner := Runner{Post: []Entry{{Name: "check", Command: `echo '{"deny":true,"reason":"please review"}'`, Timeout: time.Second}}}
	result := runner.RunPost(context.Background(), "Patch", nil, "Success", t.TempDir())
	if result.Blocked || !strings.Contains(result.Message, "please review") {
		t.Fatalf("%+v", result)
	}
}

func TestHookPathsIncludeEditAndMoveDestination(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input map[string]any
		want  []string
	}{
		{map[string]any{"file_path": "file with spaces.go"}, []string{"file with spaces.go"}},
		{map[string]any{"patch": "*** Begin Patch\n*** Update File: old.go\n*** Move to: new.go\n@@\n-old\n+new\n*** Update File: new.go\n@@\n-new\n+next\n*** End Patch"}, []string{"old.go", "new.go"}},
	} {
		if got := extractPaths(test.input); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("got %v, want %v", got, test.want)
		}
	}
}
