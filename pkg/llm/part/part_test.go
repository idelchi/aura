package part_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/part"
)

func TestIsContent(t *testing.T) {
	t.Parallel()

	content := part.Part{Type: part.Content}
	thinking := part.Part{Type: part.Thinking}

	if !content.IsContent() {
		t.Errorf("Part{Type: Content}.IsContent() = false, want true")
	}

	if thinking.IsContent() {
		t.Errorf("Part{Type: Thinking}.IsContent() = true, want false")
	}
}

func TestIsThinking(t *testing.T) {
	t.Parallel()

	thinking := part.Part{Type: part.Thinking}
	content := part.Part{Type: part.Content}

	if !thinking.IsThinking() {
		t.Errorf("Part{Type: Thinking}.IsThinking() = false, want true")
	}

	if content.IsThinking() {
		t.Errorf("Part{Type: Content}.IsThinking() = true, want false")
	}
}

func TestIsTool(t *testing.T) {
	t.Parallel()

	tool := part.Part{Type: part.Tool}
	content := part.Part{Type: part.Content}

	if !tool.IsTool() {
		t.Errorf("Part{Type: Tool}.IsTool() = false, want true")
	}

	if content.IsTool() {
		t.Errorf("Part{Type: Content}.IsTool() = true, want false")
	}
}
