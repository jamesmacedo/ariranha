//go:build cgo

package search

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"

	"ariranha/gallery"
)

func cpuTime() time.Duration {
	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

func measure(fn func()) (wall, cpu time.Duration) {
	c0, w0 := cpuTime(), time.Now()
	fn()
	return time.Since(w0), cpuTime() - c0
}

func benchPhotos(t *testing.T, n int) []*gallery.Photo {
	dir := os.Getenv("ARIRANHA_BENCH_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Imagens")
	}
	lib, err := gallery.Scan(dir)
	if err != nil || len(lib.Photos) == 0 {
		t.Skip("sem fotos em", dir)
	}
	var out []*gallery.Photo
	photos := append([]*gallery.Photo(nil), lib.Photos...)
	sort.Slice(photos, func(i, j int) bool {
		return photos[i].Width*photos[i].Height > photos[j].Width*photos[j].Height
	})
	seen := map[*gallery.Photo]bool{}
	for _, p := range out {
		seen[p] = true
	}
	step := max(1, len(photos)/n)
	for i := 0; i < len(photos) && len(out) < n; i += step {
		if !seen[photos[i]] {
			out = append(out, photos[i])
		}
	}
	return out
}

func testPaths(t *testing.T) Paths {
	wd, _ := os.Getwd()
	root := filepath.Dir(wd)
	p := Paths{
		Models:     filepath.Join(root, "models"),
		Runtime:    filepath.Join(root, "lib", runtime.GOOS+"-"+runtime.GOARCH, RuntimeLibrary()),
		ModelName:  "test",
		VisionFile: VisionInt8,
		TextFile:   TextInt8,
	}
	if !exists(p.Vision()) || !exists(p.Runtime) {
		t.Skip("modelos ou ONNX Runtime ausentes, rode make setup")
	}
	return p
}

func TestSigLIPBenchmark(t *testing.T) {
	paths := testPaths(t)
	photos := benchPhotos(t, 5)
	plane := 3 * ImageSize * ImageSize

	t.Logf("CPU: %d núcleos lógicos", runtime.NumCPU())

	pixels := make([]float32, len(photos)*plane)
	for i, p := range photos {
		wall, cpu := measure(func() {
			if err := PreparePixels(p.Path, p.Orientation, pixels[i*plane:(i+1)*plane]); err != nil {
				t.Fatal(err)
			}
		})
		t.Logf("pré-processamento %-45s %5dx%-5d wall %6.1f ms  cpu %6.1f ms",
			trim(filepath.Base(p.Path), 45), p.Width, p.Height, ms(wall), ms(cpu))
	}

	queries := []string{"um gato", "captura de tela de um site", "uma pessoa de óculos", "videogame", "texto em inglês sobre servidores"}
	vecsBy := map[string][][]float32{}

	for _, vision := range []string{VisionInt8, VisionFP32} {
		paths.VisionFile = vision
		if !exists(paths.Vision()) {
			t.Logf("=== %s ausente, pulando", vision)
			continue
		}
		t.Logf("=== %s (%s)", vision, fileSize(paths.Vision()))

		for _, threads := range []int{1, 4, runtime.NumCPU()} {
			var enc *Encoder
			wall, _ := measure(func() {
				var err error
				enc, err = NewEncoder(paths, threads)
				if err != nil {
					t.Fatal(err)
				}
			})

			enc.EncodeImages(pixels, 1)
			enc.EncodeText("aquecimento")

			var single, singleCPU time.Duration
			var vecs [][]float32
			for i := range photos {
				w, c := measure(func() {
					v, err := enc.EncodeImages(pixels[i*plane:], 1)
					if err != nil {
						t.Fatal(err)
					}
					vecs = append(vecs, v[0])
				})
				single += w
				singleCPU += c
			}
			n := time.Duration(len(photos))

			bw, bc := measure(func() {
				if _, err := enc.EncodeImages(pixels, len(photos)); err != nil {
					t.Fatal(err)
				}
			})

			var text, textCPU time.Duration
			for _, q := range queries {
				w, c := measure(func() {
					if _, err := enc.EncodeText(q); err != nil {
						t.Fatal(err)
					}
				})
				text += w
				textCPU += c
			}
			q := time.Duration(len(queries))

			t.Logf("%d threads | carga %5.0f ms | img lote1 %6.1f ms (cpu %6.1f) | img lote%d %6.1f ms/img (cpu %6.1f) | texto %5.1f ms (cpu %6.1f)",
				threads, ms(wall), ms(single/n), ms(singleCPU/n), len(photos), ms(bw/n), ms(bc/n), ms(text/q), ms(textCPU/q))

			if threads == 4 {
				vecsBy[vision] = vecs
				idx := NewIndex(enc.Dim())
				for i, v := range vecs {
					idx.Add(i, v)
				}
				for _, q := range queries {
					v, _ := enc.EncodeText(q)
					hits := idx.Search(v, len(photos))
					var parts []string
					for _, h := range hits[:min(3, len(hits))] {
						parts = append(parts, fmt.Sprintf("%s %.3f", trim(filepath.Base(photos[h.ID].Path), 22), h.Score))
					}
					t.Logf("   %-34q %v", q, parts)
				}
			}
			enc.Close()
		}
	}

	if a, b := vecsBy[VisionInt8], vecsBy[VisionFP32]; a != nil && b != nil {
		var sims []string
		for i := range a {
			sims = append(sims, fmt.Sprintf("%.4f", dot(a[i], b[i])))
		}
		t.Logf("similaridade int8 x fp32 por imagem: %v", sims)
	}
}

func TestTokenizerSample(t *testing.T) {
	paths := testPaths(t)
	tok, err := LoadTokenizer(paths.Tokenizer())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Um gato dormindo no sofá!", "a photo of a dog", "Fotos da praia em São Paulo"} {
		t.Logf("%q → %v", s, tok.Tokenize(s))
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func formatScore(s float32) string { return fmt.Sprintf("%.3f", s) }

func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func fileSize(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "?"
	}
	return fmt.Sprintf("%.0f MB", float64(info.Size())/1e6)
}
