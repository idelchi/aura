//go:build linux || darwin

package image

import (
	"bytes"
	"fmt"
	"image/jpeg"

	"github.com/gen2brain/go-fitz"
)

// FromPDF extracts each page of a PDF as a JPEG image.
func FromPDF(path string) (Images, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}
	defer doc.Close()

	pages := doc.NumPage()
	images := make(Images, 0, pages)

	for page := range pages {
		img, err := doc.Image(page)
		if err != nil {
			return nil, fmt.Errorf("extracting page %d: %w", page, err)
		}

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpeg.DefaultQuality}); err != nil {
			return nil, fmt.Errorf("encoding page %d as JPEG: %w", page, err)
		}

		images = append(images, Image(buf.Bytes()))
	}

	return images, nil
}
