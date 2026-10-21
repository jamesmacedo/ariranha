//go:build cgo

package search

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ariranha/gallery"
)

func TestFacePipeline(t *testing.T) {
	paths := testPaths(t)
	if !exists(paths.FaceDetector()) {
		t.Skip("modelos de rosto ausentes")
	}
	photos := benchPhotos(t, 5)

	enc, err := NewFaceEncoder(paths, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()

	debug := os.Getenv("ARIRANHA_DEBUG_DIR")
	var embs [][]float32

	for _, p := range photos {
		img := gallery.LoadScaled(p.Path, p.Orientation, FaceSourceSize)
		var results []FaceResult
		wall, cpu := measure(func() {
			results, err = enc.Process(img)
			if err != nil {
				t.Fatal(err)
			}
		})
		t.Logf("%-45s %d rosto(s)  wall %6.1f ms  cpu %6.1f ms", trim(filepath.Base(p.Path), 45), len(results), ms(wall), ms(cpu))

		for i, r := range results {
			d := r.Detection
			t.Logf("   rosto %d: score %.3f caixa %.0fx%.0f crop %d bytes", i, d.Score, d.Width(), d.Height(), len(r.Crop))
			embs = append(embs, r.Embedding)

			if debug != "" {
				name := filepath.Join(debug, filepath.Base(p.Path)+"-"+string(rune('0'+i)))
				os.WriteFile(name+"-crop.jpg", r.Crop, 0o644)
				buf := make([]float32, 3*FaceSize*FaceSize)
				AlignFace(img, d.Landmarks, buf)
				writeAligned(name+"-aligned.png", buf)

				flipped := flipRGBA(img)
				fd := d
				w := float32(img.Bounds().Dx())
				fd.Box = [4]float32{w - d.Box[2], d.Box[1], w - d.Box[0], d.Box[3]}
				for j := range 5 {
					fd.Landmarks[j*2] = w - d.Landmarks[j*2]
				}
				fd.Landmarks[0], fd.Landmarks[2] = fd.Landmarks[2], fd.Landmarks[0]
				fd.Landmarks[1], fd.Landmarks[3] = fd.Landmarks[3], fd.Landmarks[1]
				fd.Landmarks[6], fd.Landmarks[8] = fd.Landmarks[8], fd.Landmarks[6]
				fd.Landmarks[7], fd.Landmarks[9] = fd.Landmarks[9], fd.Landmarks[7]
				fe, err := enc.Embed(flipped, fd)
				if err == nil {
					t.Logf("   similaridade com o rosto espelhado: %.3f", dot(r.Embedding, fe))
				}
			}
		}
	}

	for i := 0; i < len(embs); i++ {
		for j := i + 1; j < len(embs); j++ {
			t.Logf("similaridade rosto %d x %d: %.3f", i, j, dot(embs[i], embs[j]))
		}
	}
}

func flipRGBA(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := range b.Dy() {
		for x := range b.Dx() {
			copy(dst.Pix[dst.PixOffset(b.Dx()-1-x, y):][:4], src.Pix[src.PixOffset(x, y):][:4])
		}
	}
	return dst
}

func writeAligned(path string, buf []float32) {
	img := image.NewRGBA(image.Rect(0, 0, FaceSize, FaceSize))
	plane := FaceSize * FaceSize
	for i := range plane {
		for c := range 3 {
			img.Pix[i*4+c] = uint8(max(0, min(255, buf[c*plane+i]*127.5+127.5)))
		}
		img.Pix[i*4+3] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	png.Encode(f, img)
}
