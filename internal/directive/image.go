package directive

import (
	"fmt"
	"regexp"

	"github.com/idelchi/aura/pkg/image"
	"github.com/idelchi/godyl/pkg/path/file"
)

// imagePattern matches @Image[path].
var imagePattern = regexp.MustCompile(`@Image\[([^\]]+)\]`)

// parseImages processes all @Image[path] directives in the input.
// For each match it resolves the path, loads and compresses the image,
// and replaces the token with "[Image N: filename]" where N is sequential.
// On error the token is left as-is and a warning is added.
func parseImages(input, workdir string, cfg ImageConfig) (string, image.Images, []string) {
	var images image.Images

	var warnings []string

	imageIndex := 0

	result := imagePattern.ReplaceAllStringFunc(input, func(match string) string {
		subs := imagePattern.FindStringSubmatch(match)

		img := file.New(subs[1])
		if !img.IsAbs() {
			img = file.New(workdir, subs[1])
		}

		img = img.Expanded()

		if !img.Exists() {
			warnings = append(warnings, fmt.Sprintf("@Image[%s]: file not found", subs[1]))

			return match
		}

		imgs, err := loadImages(img.Path(), cfg.Dimension, cfg.Quality)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("@Image[%s]: %v", subs[1], err))

			return match
		}

		images = append(images, imgs...)
		imageIndex++

		return fmt.Sprintf("[Image %d: %s]", imageIndex, img.Path())
	})

	return result, images, warnings
}

// loadImages reads the file at path and returns compressed image bytes.
// For PDFs, returns one image per page. For regular images, returns a single-element slice.
func loadImages(path string, dimension, quality int) (image.Images, error) {
	ext := file.New(path).Extension()

	if ext == "pdf" {
		imgs, err := image.FromPDF(path)
		if err != nil {
			return nil, err
		}

		return imgs.Compress(dimension, quality)
	}

	img, err := image.New(path)
	if err != nil {
		return nil, err
	}

	compressed, err := img.Compress(dimension, quality)
	if err != nil {
		return nil, err
	}

	return image.Images{compressed}, nil
}
