package show

import (
	"fmt"
	"strings"
)

// table builds a simple aligned table with headers and dynamic column widths.
type table struct {
	headers []string
	rows    [][]string
}

func newTable(headers ...string) *table {
	return &table{headers: headers}
}

func (t *table) add(cols ...string) {
	// Trim trailing whitespace/newlines from all values.
	for i := range cols {
		cols[i] = strings.TrimRight(cols[i], " \t\n\r")
	}

	t.rows = append(t.rows, cols)
}

func (t *table) print() {
	if len(t.rows) == 0 {
		return
	}

	// Calculate column widths from headers and data.
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = len(h)
	}

	for _, row := range t.rows {
		for i, col := range row {
			if i < len(widths) && len(col) > widths[i] {
				widths[i] = len(col)
			}
		}
	}

	// Build format string with padding (last column gets no padding).
	fmtParts := make([]string, len(widths))
	for i, w := range widths {
		if i == len(widths)-1 {
			fmtParts[i] = "%s"
		} else {
			fmtParts[i] = fmt.Sprintf("%%-%ds", w+2)
		}
	}

	fmtStr := strings.Join(fmtParts, "")

	// Print header.
	headerArgs := make([]any, len(t.headers))
	for i, h := range t.headers {
		headerArgs[i] = h
	}

	fmt.Printf(fmtStr+"\n", headerArgs...)

	// Print rows.
	for _, row := range t.rows {
		args := make([]any, len(t.headers))
		for i := range t.headers {
			if i < len(row) {
				args[i] = row[i]
			} else {
				args[i] = ""
			}
		}

		fmt.Printf(fmtStr+"\n", args...)
	}
}
