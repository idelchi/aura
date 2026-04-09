//go:build !(linux || darwin)

package image

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// FromPDF extracts each page of a PDF as a JPEG image using pdftoppm.
func FromPDF(path string) (Images, error) {
	if !file.New("pdftoppm").InPath() {
		return nil, fmt.Errorf("PDF support requires pdftoppm — install poppler-utils")
	}

	tmp, err := folder.CreateRandomInDir("", "aura-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	tmpDir := tmp.Path()
	defer folder.New(tmpDir).Remove()

	prefix := folder.New(tmpDir).WithFile("page").Path()

	cmd := exec.Command("pdftoppm", "-jpeg", "-r", "150", path, prefix)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %w: %s", err, stderr.String())
	}

	entries, err := folder.New(tmpDir).ListFiles()
	if err != nil {
		return nil, fmt.Errorf("reading converted pages: %w", err)
	}

	var images Images
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Base(), ".jpg") {
			continue
		}

		data, err := entry.Read()
		if err != nil {
			return nil, fmt.Errorf("reading page %s: %w", entry.Base(), err)
		}

		images = append(images, Image(data))
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no output for %s", path)
	}

	return images, nil
}
