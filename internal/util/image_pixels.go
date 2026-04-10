package util

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

func LoadImagePixels(imagePath string) ([]byte, int, int, int, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	channels := 3
	pixels := make([]byte, width*height*channels)

	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			pixels[idx] = byte(r >> 8)
			pixels[idx+1] = byte(g >> 8)
			pixels[idx+2] = byte(b >> 8)
			idx += channels
		}
	}

	return pixels, width, height, channels, nil
}
