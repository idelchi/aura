package ollama

import (
	"errors"
	"testing"
)

func TestToolParserFailures(t *testing.T) {
	for _, message := range []string{"error parsing tool call: invalid JSON", "XML syntax error on line 18: element <function> closed by </parameter>"} {
		if !isToolParseError(errors.New(message)) {
			t.Errorf("not classified: %s", message)
		}
	}
	if isToolParseError(errors.New("connection reset")) {
		t.Fatal("transport failure classified as parser failure")
	}
}
