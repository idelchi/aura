package image

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"

	"github.com/idelchi/godyl/pkg/path/file"
)

// Image represents a collection of image byte slices.
type Image []byte

// New reads the image file from the given path and returns its raw bytes.
func New(path string) (Image, error) {
	data, err := file.New(path).Read()
	if err != nil {
		return nil, err
	}

	return Image(data), nil
}

func (i Image) Compress(dimension, quality int) (Image, error) {
	if dimension < 1 {
		dimension = 384
	}

	if quality < 1 || quality > 100 {
		quality = 75
	}

	image, _, err := image.Decode(bytes.NewReader(i))
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}

	// Resize to max dimension
	image = resizeImage(image, dimension)

	var buf bytes.Buffer

	err = jpeg.Encode(&buf, image, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, fmt.Errorf("encoding jpeg: %w", err)
	}

	return Image(buf.Bytes()), nil
}

func (i Image) Save(path string) error {
	return file.New(path).Write(i)
}

func resizeImage(src image.Image, maxDim int) image.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w <= maxDim && h <= maxDim {
		return src
	}

	var newW, newH int

	if w > h {
		newW = maxDim
		newH = h * maxDim / w
	} else {
		newH = maxDim
		newW = w * maxDim / h
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Rect, src, bounds, draw.Over, nil)

	return dst
}
