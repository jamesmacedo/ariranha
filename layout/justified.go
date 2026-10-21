package layout

import "math"

type Rect struct {
	X, Y, W, H float32
}

type Box struct {
	Section int
	Index   int
	Rect    Rect
}

type Header struct {
	Section int
	Rect    Rect
}

type Result struct {
	Boxes   []Box
	Headers []Header
	Height  float32
}

type Config struct {
	Width        float32
	TargetHeight float32
	MinHeight    float32
	MaxHeight    float32
	Gap          float32
	Padding      float32
	HeaderHeight float32
	SectionGap   float32
	SectionGapX  float32
	MinAspect    float32
	MaxAspect    float32
	PackSections bool
}

func Responsive(width float32) Config {
	target := clamp(width/6.5, 140, 240)
	return Config{
		Width:        width,
		TargetHeight: target,
		MinHeight:    target * 0.7,
		MaxHeight:    target * 1.4,
		Gap:          4,
		Padding:      24,
		HeaderHeight: 48,
		SectionGap:   24,
		SectionGapX:  28,
		MinAspect:    0.25,
		MaxAspect:    5,
		PackSections: true,
	}
}

type row struct {
	start, end int
	height     float32
}

type builder struct {
	cfg   Config
	inner float32
	res   Result
	y     float32
	line  []int
	secs  [][]float32
}

func Justify(cfg Config, sections [][]float32) Result {
	b := &builder{
		cfg:   cfg,
		inner: max(1, cfg.Width-cfg.Padding*2),
		y:     cfg.Padding,
		secs:  make([][]float32, len(sections)),
	}

	for s, aspects := range sections {
		b.secs[s] = make([]float32, len(aspects))
		for i, a := range aspects {
			b.secs[s][i] = clamp(a, cfg.MinAspect, cfg.MaxAspect)
		}
	}

	for s, aspects := range b.secs {
		if len(aspects) == 0 {
			continue
		}

		if cfg.PackSections && b.natural(s) <= b.inner {
			if len(b.line) > 0 && b.lineNatural()+cfg.SectionGapX+b.natural(s) > b.inner {
				b.flushLine(false)
			}
			b.line = append(b.line, s)
			continue
		}

		b.flushLine(false)
		b.placeSection(s)
	}
	b.flushLine(true)

	b.res.Height = b.y - cfg.SectionGap + cfg.Padding
	if len(b.res.Boxes) == 0 {
		b.res.Height = cfg.Padding * 2
	}
	return b.res
}

func (b *builder) sum(s int) float32 {
	var total float32
	for _, a := range b.secs[s] {
		total += a
	}
	return total
}

func (b *builder) natural(s int) float32 {
	return b.sum(s)*b.cfg.TargetHeight + b.cfg.Gap*float32(len(b.secs[s])-1)
}

func (b *builder) lineNatural() float32 {
	var w float32
	for i, s := range b.line {
		if i > 0 {
			w += b.cfg.SectionGapX
		}
		w += b.natural(s)
	}
	return w
}

func (b *builder) flushLine(last bool) {
	if len(b.line) == 0 {
		return
	}
	cfg := b.cfg

	var aspects, fixed float32
	for i, s := range b.line {
		aspects += b.sum(s)
		fixed += cfg.Gap * float32(len(b.secs[s])-1)
		if i > 0 {
			fixed += cfg.SectionGapX
		}
	}

	fill := (b.inner - fixed) / aspects
	h := fill
	if last || fill > cfg.MaxHeight {
		h = min(fill, cfg.TargetHeight)
	}

	x := cfg.Padding
	for _, s := range b.line {
		width := b.sum(s)*h + cfg.Gap*float32(len(b.secs[s])-1)
		b.res.Headers = append(b.res.Headers, Header{
			Section: s,
			Rect:    Rect{X: x, Y: b.y, W: width, H: cfg.HeaderHeight},
		})
		b.placeRow(s, 0, len(b.secs[s]), x, b.y+cfg.HeaderHeight, h)
		x += width + cfg.SectionGapX
	}

	b.y += cfg.HeaderHeight + h + cfg.SectionGap
	b.line = b.line[:0]
}

func (b *builder) placeSection(s int) {
	cfg := b.cfg
	b.res.Headers = append(b.res.Headers, Header{
		Section: s,
		Rect:    Rect{X: cfg.Padding, Y: b.y, W: b.inner, H: cfg.HeaderHeight},
	})
	b.y += cfg.HeaderHeight

	for _, r := range partition(b.secs[s], b.inner, cfg) {
		b.placeRow(s, r.start, r.end, cfg.Padding, b.y, r.height)
		b.y += r.height + cfg.Gap
	}
	b.y += cfg.SectionGap - cfg.Gap
}

func (b *builder) placeRow(s, start, end int, x, y, h float32) {
	for i := start; i < end; i++ {
		w := b.secs[s][i] * h
		b.res.Boxes = append(b.res.Boxes, Box{
			Section: s,
			Index:   i,
			Rect:    Rect{X: x, Y: y, W: w, H: h},
		})
		x += w + b.cfg.Gap
	}
}

func partition(aspects []float32, width float32, cfg Config) []row {
	n := len(aspects)
	prefix := make([]float32, n+1)
	for i, a := range aspects {
		prefix[i+1] = prefix[i] + a
	}

	cost := make([]float64, n+1)
	prev := make([]int, n+1)
	heights := make([]float32, n+1)
	for i := 1; i <= n; i++ {
		cost[i] = math.Inf(1)
	}

	for i := 1; i <= n; i++ {
		for j := i - 1; j >= 0; j-- {
			count := i - j
			h := (width - cfg.Gap*float32(count-1)) / (prefix[i] - prefix[j])
			if h < cfg.MinHeight*0.75 && count > 1 {
				break
			}

			c, height := rowCost(h, i == n, cfg)
			if total := cost[j] + c; total < cost[i] {
				cost[i] = total
				prev[i] = j
				heights[i] = height
			}
		}
	}

	var rows []row
	for i := n; i > 0; i = prev[i] {
		rows = append(rows, row{start: prev[i], end: i, height: heights[i]})
	}
	for l, r := 0, len(rows)-1; l < r; l, r = l+1, r-1 {
		rows[l], rows[r] = rows[r], rows[l]
	}
	return rows
}

func rowCost(h float32, last bool, cfg Config) (float64, float32) {
	if last && h >= cfg.TargetHeight {
		return 0, cfg.TargetHeight
	}

	d := float64(h-cfg.TargetHeight) / float64(cfg.TargetHeight)
	c := d * d
	if h > cfg.MaxHeight {
		over := float64(h-cfg.MaxHeight) / float64(cfg.TargetHeight)
		c += over * over * 20
	}
	if h < cfg.MinHeight {
		under := float64(cfg.MinHeight-h) / float64(cfg.TargetHeight)
		c += under * under * 20
	}
	return c, h
}

func clamp(v, lo, hi float32) float32 {
	return max(lo, min(v, hi))
}
