package edit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// match identifies bytes in the original file, even when normalized text matched.
type match struct{ start, end int }

// normalizedText keeps byte positions so normalization never rewrites unrelated text.
type normalizedText struct {
	text   string
	starts []int
	ends   []int
}

func normalize(text string) normalizedText {
	var result normalizedText
	var out strings.Builder
	pos := 0
	for line := range strings.SplitAfterSeq(text, "\n") {
		body := strings.TrimSuffix(line, "\n")
		if strings.HasSuffix(line, "\n") {
			body = strings.TrimSuffix(body, "\r")
		}
		body = strings.TrimRight(body, " \t")
		for i := range len(body) {
			out.WriteByte(body[i])
			result.starts = append(result.starts, pos+i)
			result.ends = append(result.ends, pos+i+1)
		}
		if strings.HasSuffix(line, "\n") {
			out.WriteByte('\n')
			result.starts = append(result.starts, pos+len(line)-1)
			result.ends = append(result.ends, pos+len(line))
		}
		pos += len(line)
	}
	result.text = out.String()
	return result
}

func matches(content, old string) []match {
	var found []match
	for offset := 0; offset <= len(content); {
		index := strings.Index(content[offset:], old)
		if index < 0 {
			break
		}
		start := offset + index
		found = append(found, match{start, start + len(old)})
		offset = start + len(old)
	}
	return found
}

func replace(content, old, replacement string, all bool) (string, int, error) {
	if old == "" {
		return "", 0, errors.New("old_string must not be empty")
	}
	if old == replacement {
		return "", 0, errors.New("old_string and new_string are identical")
	}
	found := matches(content, old)
	if len(found) == 0 {
		original := normalize(content)
		pattern := normalize(old).text
		if pattern != "" {
			for _, candidate := range matches(original.text, pattern) {
				found = append(found, match{original.starts[candidate.start], original.ends[candidate.end-1]})
			}
		}
	}
	if len(found) == 0 {
		return "", 0, errors.New(missingMatch(content, old))
	}
	if len(found) > 1 && !all {
		lines := make([]string, 0, min(5, len(found)))
		for _, candidate := range found[:min(5, len(found))] {
			lines = append(lines, strconv.Itoa(1+strings.Count(content[:candidate.start], "\n")))
		}
		return "", 0, fmt.Errorf("found %d matches for old_string at lines %s; add context to select one, or use replace_all", len(found), strings.Join(lines, ", "))
	}
	var out strings.Builder
	position := 0
	for _, candidate := range found {
		out.WriteString(content[position:candidate.start])
		// Keep the matched region's newline style, falling back to the file's style
		// for an inline match that introduces new lines.
		region := content[candidate.start:candidate.end]
		if !strings.Contains(region, "\n") {
			region = content
		}
		text := strings.ReplaceAll(replacement, "\r\n", "\n")
		if i := strings.IndexByte(region, '\n'); i > 0 && region[i-1] == '\r' {
			text = strings.ReplaceAll(text, "\n", "\r\n")
		}
		out.WriteString(text)
		position = candidate.end
	}
	out.WriteString(content[position:])
	return out.String(), len(found), nil
}

// missingMatch offers bounded evidence, never an approximate replacement.
func missingMatch(content, old string) string {
	message := "old_string not found in file. Read the current file and copy the exact text, including indentation."
	pattern := ""
	for line := range strings.SplitSeq(old, "\n") {
		if len(strings.TrimSpace(line)) > len(pattern) {
			pattern = strings.TrimSpace(line)
		}
	}
	if len(pattern) < 4 {
		return message
	}
	runes := []rune(pattern)
	pattern = string(runes[:min(32, len(runes))])
	// The existing Bitap matcher uses a machine-word mask and byte offsets.
	pattern = pattern[:min(32, len(pattern))]
	matcher := diffmatchpatch.New()
	matcher.MatchDistance = max(1000, len(content))
	index := matcher.MatchMain(content, pattern, 0)
	if index < 0 {
		return message
	}
	line := strings.Count(content[:index], "\n")
	lines := strings.Split(content, "\n")
	var excerpt strings.Builder
	for i := max(0, line-2); i < min(len(lines), line+4); i++ {
		text := []rune(strings.TrimSuffix(lines[i], "\r"))
		if len(text) > 200 {
			text = append(text[:200], '…')
		}
		fmt.Fprintf(&excerpt, "%d: %s\n", i+1, string(text))
	}
	return message + "\nNearby candidate (not applied):\n" + excerpt.String()
}
