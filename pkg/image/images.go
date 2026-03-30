package image

type Images []Image

func (is Images) Compress(dimension, quality int) (Images, error) {
	var compressed Images

	for _, img := range is {
		cImg, err := img.Compress(dimension, quality)
		if err != nil {
			return nil, err
		}

		compressed = append(compressed, cImg)
	}

	return compressed, nil
}
