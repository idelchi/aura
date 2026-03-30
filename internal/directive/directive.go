package directive

import (
	"context"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/image"
)

// ParsedInput is the result of parsing user input for directives.
type ParsedInput struct {
	// Text is the input with directive tokens replaced by placeholders.
	Text string
	// Images holds extracted and compressed images.
	Images image.Images
	// Warnings holds non-fatal issues (file not found, decode error).
	Warnings []string
	// Preamble holds prepended context from @File and @Bash expansions.
	Preamble string
}

// HasImages reports whether any images were extracted.
func (p ParsedInput) HasImages() bool {
	return len(p.Images) > 0
}

// ImageConfig holds compression settings for image directives.
type ImageConfig struct {
	Dimension int
	Quality   int
}

// Config holds all directive configuration.
type Config struct {
	Image   ImageConfig
	RunBash func(ctx context.Context, command string) (string, error)
}

// Parse processes raw user input, expanding all registered directives.
// The workdir is used to resolve relative paths.
func Parse(ctx context.Context, input, workdir string, cfg Config) ParsedInput {
	text := input

	var allImages image.Images

	var allWarnings []string

	var preambleParts []string

	// 1. @Image[path]
	text, images, warnings := parseImages(text, workdir, cfg.Image)

	allImages = append(allImages, images...)
	allWarnings = append(allWarnings, warnings...)

	// 2. @Bash[command]
	var bashPreamble string

	text, bashPreamble, warnings = parseShell(ctx, text, cfg.RunBash)

	if bashPreamble != "" {
		preambleParts = append(preambleParts, bashPreamble)
	}

	allWarnings = append(allWarnings, warnings...)

	// 3. @File[path]
	var filePreamble string

	text, filePreamble, warnings = parseFiles(text, workdir)

	if filePreamble != "" {
		preambleParts = append(preambleParts, filePreamble)
	}

	allWarnings = append(allWarnings, warnings...)

	// 4. @Path[path] — replace with bare path
	text = parsePaths(text)

	preamble := strings.Join(preambleParts, "\n\n")

	if len(allImages) > 0 || len(allWarnings) > 0 || len(preamble) > 0 {
		debug.Log(
			"[directive] parsed: images=%d warnings=%d preamble=%d chars",
			len(allImages),
			len(allWarnings),
			len(preamble),
		)
	}

	return ParsedInput{
		Text:     strings.TrimSpace(text),
		Images:   allImages,
		Warnings: allWarnings,
		Preamble: preamble,
	}
}
