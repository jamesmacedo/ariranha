package layout

import (
	"math"
	"math/rand/v2"
	"testing"
)

func randomSections(r *rand.Rand, n int) [][]float32 {
	aspects := []float32{4.0 / 3, 3.0 / 4, 16.0 / 9, 9.0 / 16, 1, 3.0 / 2, 2.0 / 3, 3}
	sections := make([][]float32, n)
	for s := range sections {
		count := 1 + r.IntN(25)
		for range count {
			sections[s] = append(sections[s], aspects[r.IntN(len(aspects))])
		}
	}
	return sections
}

func TestJustifyKeepsAspectAndBounds(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, width := range []float32{640, 1024, 1280, 1920} {
		cfg := Responsive(width)
		sections := randomSections(r, 60)
		res := Justify(cfg, sections)

		seen := 0
		for _, s := range sections {
			seen += len(s)
		}
		if len(res.Boxes) != seen {
			t.Fatalf("width %v: %d boxes, want %d", width, len(res.Boxes), seen)
		}

		for _, b := range res.Boxes {
			want := clamp(sections[b.Section][b.Index], cfg.MinAspect, cfg.MaxAspect)
			got := b.Rect.W / b.Rect.H
			if math.Abs(float64(got-want)) > 1e-3 {
				t.Fatalf("aspect %v, want %v", got, want)
			}
			if b.Rect.X < cfg.Padding-0.01 || b.Rect.X+b.Rect.W > width-cfg.Padding+0.5 {
				t.Fatalf("width %v: box out of bounds %+v", width, b.Rect)
			}
		}

		for i := 1; i < len(res.Boxes); i++ {
			if res.Boxes[i].Rect.Y < res.Boxes[i-1].Rect.Y {
				t.Fatalf("boxes not sorted by Y")
			}
		}
	}
}

func TestJustifiedRowsFillWidth(t *testing.T) {
	cfg := Responsive(1280)
	sections := [][]float32{make([]float32, 40)}
	for i := range sections[0] {
		sections[0][i] = 4.0 / 3
	}
	res := Justify(cfg, sections)

	rows := map[float32]float32{}
	for _, b := range res.Boxes {
		rows[b.Rect.Y] = max(rows[b.Rect.Y], b.Rect.X+b.Rect.W)
	}
	full := 0
	for _, right := range rows {
		if math.Abs(float64(right-(cfg.Width-cfg.Padding))) < 0.5 {
			full++
		}
	}
	if full < len(rows)-1 {
		t.Fatalf("only %d of %d rows justified", full, len(rows))
	}
}
