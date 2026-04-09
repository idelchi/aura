package gotify

import "fmt"

// Level represents a notification severity.
type Level string

const (
	LevelInfo    Level = "INFO"
	LevelWarning Level = "WARNING"
	LevelError   Level = "ERROR"
)

// Priority returns the Gotify priority for the level.
func (l Level) Priority() int {
	switch l {
	case LevelWarning:
		return 5
	case LevelError:
		return 10
	default:
		return 0
	}
}

// ShouldSend reports whether this level meets the minimum threshold.
func (l Level) ShouldSend(min Level) bool {
	return l.Priority() >= min.Priority()
}

// ParseLevel validates and returns a Level from a raw string.
func ParseLevel(s string) (Level, error) {
	switch Level(s) {
	case LevelInfo, LevelWarning, LevelError:
		return Level(s), nil
	default:
		return "", fmt.Errorf("invalid level %q — expected INFO, WARNING, or ERROR", s)
	}
}
