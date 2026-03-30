package show

import (
	"fmt"
	"strings"
)

// detailBuilder builds Field: Value output, skipping empty values.
// Used by entity detail views in aura show <entity> <name>.
type detailBuilder struct {
	sb    strings.Builder
	width int
}

func newDetail(width int) *detailBuilder {
	return &detailBuilder{width: width}
}

// Field writes "Label:  Value\n" if value is non-empty.
// Trailing whitespace and newlines are trimmed from the value.
func (d *detailBuilder) Field(label, value string) {
	value = strings.TrimRight(value, " \t\n\r")
	if value == "" {
		return
	}

	fmt.Fprintf(&d.sb, "%-*s  %s\n", d.width, label+":", value)
}

// BoolField writes the field only if the pointer is non-nil and true.
func (d *detailBuilder) BoolField(label string, p *bool) {
	if p != nil && *p {
		fmt.Fprintf(&d.sb, "%-*s  true\n", d.width, label+":")
	}
}

// SliceField writes a numbered list if non-empty.
func (d *detailBuilder) SliceField(label string, items []string) {
	if len(items) == 0 {
		return
	}

	fmt.Fprintf(&d.sb, "%s:\n", label)

	for i, item := range items {
		fmt.Fprintf(&d.sb, "  %d. %s\n", i+1, item)
	}
}

// Line writes a raw line.
func (d *detailBuilder) Line(s string) {
	d.sb.WriteString(s)
	d.sb.WriteByte('\n')
}

// Blank writes an empty line.
func (d *detailBuilder) Blank() {
	d.sb.WriteByte('\n')
}

// Body appends a separator and raw body text (for prompts/skills).
func (d *detailBuilder) Body(body string) {
	if body == "" {
		return
	}

	d.sb.WriteString("---\n")
	d.sb.WriteString(strings.TrimRight(body, "\n"))
	d.sb.WriteByte('\n')
}

func (d *detailBuilder) String() string {
	return strings.TrimRight(d.sb.String(), "\n")
}
