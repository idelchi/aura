package directive

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

const maxFileLines = 500

var filePattern = regexp.MustCompile(`@File\[([^\]]+)\]`)

// parseFiles processes all @File[path] directives in the input.
// File contents are collected into preamble blocks.
// The token in the user text is replaced with "[File: path]".
func parseFiles(input, workdir string) (string, string, []string) {
	var blocks []string

	var warnings []string

	text := filePattern.ReplaceAllStringFunc(input, func(match string) string {
		subs := filePattern.FindStringSubmatch(match)
		original := subs[1]

		f := file.New(original)
		if !f.IsAbs() {
			f = file.New(workdir, original)
		}

		f = f.Expanded()

		if !f.Exists() {
			warnings = append(warnings, fmt.Sprintf("@File[%s]: file not found", original))

			return match
		}

		if f.IsDir() {
			listing, err := listDirectory(f.Path())
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("@File[%s]: %v", original, err))

				return match
			}

			blocks = append(blocks, fmt.Sprintf("### Directory: %s\nListing:\n%s", original, listing))

			return fmt.Sprintf("[File: %s]", original)
		}

		content, truncated, totalLines, err := readFileTruncated(f.Path(), maxFileLines)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("@File[%s]: %v", original, err))

			return match
		}

		block := fmt.Sprintf("### File: %s\nContent:\n%s", original, content)

		if truncated {
			block += fmt.Sprintf("\n... truncated at %d lines (file has %d total)", maxFileLines, totalLines)
		}

		blocks = append(blocks, block)

		return fmt.Sprintf("[File: %s]", original)
	})

	preamble := strings.Join(blocks, "\n\n")

	return text, preamble, warnings
}

// readFileTruncated reads a file, returning at most maxLines lines.
func readFileTruncated(path string, maxLines int) (string, bool, int, error) {
	data, err := file.New(path).Read()
	if err != nil {
		return "", false, 0, err
	}

	lines := strings.Split(string(data), "\n")
	total := len(lines)

	if total <= maxLines {
		return string(data), false, total, nil
	}

	return strings.Join(lines[:maxLines], "\n"), true, total, nil
}

// listDirectory produces a shallow directory listing.
func listDirectory(path string) (string, error) {
	var buf strings.Builder

	dir := folder.New(path)

	folders, err := dir.ListFolders()
	if err != nil {
		return "", err
	}

	for _, f := range folders {
		buf.WriteString(f.Base() + "/\n")
	}

	files, err := dir.ListFiles()
	if err != nil {
		return "", err
	}

	for _, f := range files {
		buf.WriteString(f.Base() + "\n")
	}

	return strings.TrimSpace(buf.String()), nil
}
