package message

import "encoding/base64"

// Image represents an image attached to a message as raw bytes.
// Providers handle encoding (base64 data URL, raw bytes, etc.) in their own convert layer.
type Image struct {
	Data []byte
}

// DataURL returns the image as a base64-encoded JPEG data URL.
// Used by OpenAI-compatible providers (OpenRouter, LlamaCPP) in their message conversion.
func (i Image) DataURL() string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(i.Data)
}

// Images is a collection of images.
type Images []Image
