package search

import (
	"errors"
	"image"

	"ariranha/gallery"

	"golang.org/x/image/draw"
)

const ImageSize = 256

var errDecode = errors.New("falha ao decodificar imagem")

func PreparePixels(path string, orientation int, dst []float32) error {
	src := gallery.LoadScaled(path, orientation, ImageSize*2)
	if src == nil {
		return errDecode
	}
	PreparePixelsFrom(src, dst)
	return nil
}

func PreparePixelsFrom(src *image.RGBA, dst []float32) {
	img := image.NewRGBA(image.Rect(0, 0, ImageSize, ImageSize))
	draw.CatmullRom.Scale(img, img.Bounds(), src, src.Bounds(), draw.Src, nil)

	plane := ImageSize * ImageSize
	for y := range ImageSize {
		for x := range ImageSize {
			o := img.PixOffset(x, y)
			i := y*ImageSize + x
			a := float32(img.Pix[o+3]) / 255
			for c := range 3 {
				v := float32(img.Pix[o+c])/255 + (1 - a)
				dst[c*plane+i] = (v - 0.5) / 0.5
			}
		}
	}
}
