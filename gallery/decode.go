package gallery

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
)

const (
	exifProbe    = 128 << 10
	minExifThumb = 320
	aspectSlack  = 0.03
)

func LoadScaled(path string, orientation, maxSide int) *image.RGBA {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return orient(Downscale(src, maxSide), orientation)
}

func LoadPreview(p *Photo, maxSide int) *image.RGBA {
	if img := loadDisk(p, maxSide); img != nil {
		return img
	}
	if p.IsVideo() {
		img := LoadVideoFrame(p, VideoPosterTime(p), maxSide)
		if img != nil {
			saveDisk(p, maxSide, img)
		}
		return img
	}
	if img := exifThumbnail(p, maxSide); img != nil {
		return img
	}
	img := LoadScaled(p.Path, p.Orientation, maxSide)
	if img != nil && maxSide <= diskMaxPhotoSide {
		saveDisk(p, maxSide, img)
	}
	return img
}

func exifThumbnail(p *Photo, maxSide int) *image.RGBA {
	ext := strings.ToLower(filepath.Ext(p.Path))
	if ext != ".jpg" && ext != ".jpeg" {
		return nil
	}
	f, err := os.Open(p.Path)
	if err != nil {
		return nil
	}
	buf := make([]byte, exifProbe)
	n, _ := io.ReadFull(f, buf)
	f.Close()

	x, err := exif.Decode(bytes.NewReader(buf[:n]))
	if err != nil {
		return nil
	}
	data, err := x.JpegThumbnail()
	if err != nil || len(data) == 0 {
		return nil
	}
	thumb, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}

	b := thumb.Bounds()
	if max(b.Dx(), b.Dy()) < max(min(maxSide, minExifThumb), maxSide*3/4) {
		return nil
	}

	want := float64(p.Aspect())
	if p.Orientation >= 5 {
		want = 1 / want
	}
	got := float64(b.Dx()) / float64(b.Dy())
	r := b
	if math.Abs(got-want)/want > aspectSlack {
		if got > want {
			w := int(float64(b.Dy()) * want)
			r = image.Rect(b.Min.X+(b.Dx()-w)/2, b.Min.Y, b.Min.X+(b.Dx()-w)/2+w, b.Max.Y)
		} else {
			h := int(float64(b.Dx()) / want)
			r = image.Rect(b.Min.X, b.Min.Y+(b.Dy()-h)/2, b.Max.X, b.Min.Y+(b.Dy()-h)/2+h)
		}
	}

	scale := min(1, float64(maxSide)/float64(max(r.Dx(), r.Dy())))
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(r.Dx())*scale)), max(1, int(float64(r.Dy())*scale))))
	draw.BiLinear.Scale(dst, dst.Bounds(), thumb, r, draw.Src, nil)
	return orient(dst, p.Orientation)
}

func Downscale(src image.Image, maxSide int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := min(1, float64(maxSide)/float64(max(w, h)))
	tw := max(1, int(float64(w)*scale))
	th := max(1, int(float64(h)*scale))

	mid := src
	if k := min(w/tw, h/th); k >= 2 {
		mid = boxReduce(src, k)
	}

	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.BiLinear.Scale(dst, dst.Bounds(), mid, mid.Bounds(), draw.Src, nil)
	return dst
}

func boxReduce(src image.Image, k int) *image.RGBA {
	b := src.Bounds()
	ow, oh := b.Dx()/k, b.Dy()/k
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	area := uint32(k * k)

	switch s := src.(type) {
	case *image.YCbCr:
		for oy := range oh {
			for ox := range ow {
				var sy, scb, scr uint32
				x0, y0 := b.Min.X+ox*k, b.Min.Y+oy*k
				for y := y0; y < y0+k; y++ {
					for x := x0; x < x0+k; x++ {
						ci := s.COffset(x, y)
						sy += uint32(s.Y[s.YOffset(x, y)])
						scb += uint32(s.Cb[ci])
						scr += uint32(s.Cr[ci])
					}
				}
				r, g, bl := color.YCbCrToRGB(uint8(sy/area), uint8(scb/area), uint8(scr/area))
				o := dst.PixOffset(ox, oy)
				dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = r, g, bl, 255
			}
		}
	case *image.RGBA:
		boxPix(dst, s.Pix, s.Stride, k, false)
	case *image.NRGBA:
		boxPix(dst, s.Pix, s.Stride, k, true)
	default:
		tmp := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(tmp, tmp.Bounds(), src, b.Min, draw.Src)
		boxPix(dst, tmp.Pix, tmp.Stride, k, false)
	}
	return dst
}

func boxPix(dst *image.RGBA, pix []uint8, stride int, k int, straight bool) {
	ow, oh := dst.Bounds().Dx(), dst.Bounds().Dy()
	area := uint32(k * k)
	for oy := range oh {
		for ox := range ow {
			var sr, sg, sb, sa uint32
			for y := oy * k; y < oy*k+k; y++ {
				row := y*stride + ox*k*4
				for x := 0; x < k; x++ {
					i := row + x*4
					a := uint32(pix[i+3])
					r, g, bl := uint32(pix[i]), uint32(pix[i+1]), uint32(pix[i+2])
					if straight {
						r, g, bl = r*a/255, g*a/255, bl*a/255
					}
					sr += r
					sg += g
					sb += bl
					sa += a
				}
			}
			o := dst.PixOffset(ox, oy)
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = uint8(sr/area), uint8(sg/area), uint8(sb/area), uint8(sa/area)
		}
	}
}
