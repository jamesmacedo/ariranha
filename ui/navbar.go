package ui

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type View int

const (
	ViewPhotos View = iota
	ViewPeople
	ViewAlbums
)

const (
	NavHeight   = 56
	NavMargin   = 18
	NavInset    = NavHeight + NavMargin + 16
	navPad      = 8
	navItemH    = 40
	navItemW    = 44
	navGap      = 4
	navSepGap   = 10
	navSearchW  = 170
	navSearchWF = 320
	navRing     = 40
	navLabel    = 15
)

var navLabels = [3]string{"Photos", "People", "Albums"}

type NavBar struct {
	View     View
	Search   *TextInput
	Progress float32
	Done     bool
	Status   string
	Playable bool
	Paused   bool
	Toggled  bool

	theme   *Theme
	bounds  rl.Rectangle
	items   [3]rl.Rectangle
	ring    rl.Rectangle
	widths  [3]float32
	searchW float32
	hover   int
	armed   int
	spin    float32
	ready   bool
}

func NewNavBar(theme *Theme, search *TextInput) *NavBar {
	return &NavBar{theme: theme, Search: search, hover: -1, armed: -1}
}

func (n *NavBar) Bounds() rl.Rectangle {
	return n.bounds
}

func (n *NavBar) Hovered() bool {
	return (n.hover >= 0 && n.hover < 3) || (n.hover == 3 && n.Playable)
}

func (n *NavBar) Contains(p rl.Vector2) bool {
	return rl.CheckCollisionPointRec(p, n.bounds)
}

func (n *NavBar) itemWidth(i int) float32 {
	if View(i) != n.View {
		return navItemW
	}
	return 12 + 20 + 8 + MeasureText(n.theme.Medium, navLabels[i], navLabel) + 16
}

func (n *NavBar) Update(screen rl.Rectangle) bool {
	searchTarget := float32(navSearchW)
	if n.Search.Focused || n.Search.Text != "" {
		searchTarget = navSearchWF
	}
	fixed := float32(navPad*2 + navGap*2 + navSepGap*2 + 1 + navGap*2 + navRing)
	for i := range n.widths {
		fixed += n.itemWidth(i)
	}
	searchTarget = max(110, min(searchTarget, screen.Width-32-fixed))

	if !n.ready {
		for i := range n.widths {
			n.widths[i] = n.itemWidth(i)
		}
		n.searchW = searchTarget
		n.ready = true
	}
	for i := range n.widths {
		n.widths[i] = smooth(n.widths[i], n.itemWidth(i))
	}
	n.searchW = smooth(n.searchW, searchTarget)

	total := float32(navPad*2 + navGap*2 + navSepGap*2 + 1 + navGap*2 + navRing)
	for _, w := range n.widths {
		total += w
	}
	total += n.searchW
	n.bounds = rl.NewRectangle(
		float32(math.Round(float64(screen.X+(screen.Width-total)/2))),
		screen.Y+screen.Height-NavHeight-NavMargin,
		float32(math.Round(float64(total))),
		NavHeight,
	)

	x := n.bounds.X + navPad
	iy := n.bounds.Y + (NavHeight-navItemH)/2
	for i, w := range n.widths {
		n.items[i] = rl.NewRectangle(x, iy, w, navItemH)
		x += w + navGap
	}
	x += navSepGap - navGap + 1 + navSepGap
	n.Search.Bounds = rl.NewRectangle(x, n.bounds.Y+(NavHeight-38)/2, n.searchW, 38)
	x += n.searchW + navGap*2
	n.ring = rl.NewRectangle(x, iy, navRing, navItemH)

	n.Search.Update()
	n.spin += rl.GetFrameTime() * 300

	mouse := rl.GetMousePosition()
	n.hover = -1
	for i, r := range n.items {
		if rl.CheckCollisionPointRec(mouse, r) {
			n.hover = i
		}
	}
	if rl.CheckCollisionPointRec(mouse, n.ring) {
		n.hover = 3
	}

	changed := false
	n.Toggled = false
	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		n.armed = n.hover
	}
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		if n.armed >= 0 && n.armed < 3 && n.armed == n.hover && View(n.hover) != n.View {
			n.View = View(n.hover)
			changed = true
		}
		if n.armed == 3 && n.hover == 3 && n.Playable {
			n.Toggled = true
		}
		n.armed = -1
	}
	return changed
}

func (n *NavBar) Draw(bd *Backdrop) {
	t := n.theme
	b := n.bounds

	for i := 1; i <= 4; i++ {
		f := float32(i) * 3
		r := rl.NewRectangle(b.X-f, b.Y-f+4, b.Width+f*2, b.Height+f*2)
		rl.DrawRectangleRounded(r, 1, 32, rl.Fade(t.Shadow, 0.35/float32(i)))
	}
	bd.Glass(b, b.Height/2, t.Glass)
	rl.DrawRectangleRoundedLinesEx(b, 1, 32, 1, t.GlassEdge)

	for i, r := range n.items {
		active := View(i) == n.View
		fg := rl.Fade(t.Text, 0.85)
		if active {
			rl.DrawRectangleRounded(r, 1, 24, t.Accent)
			fg = t.OnAccent
		} else if n.hover == i {
			rl.DrawRectangleRounded(r, 1, 24, rl.Fade(t.Text, 0.08))
			fg = t.Text
		}
		ix := r.X + 12
		if !active {
			ix = r.X + (r.Width-20)/2
		}
		drawNavIcon(i, ix, r.Y+(r.Height-20)/2, fg)
		if r.Width > navItemW+4 {
			rl.BeginScissorMode(int32(r.X), int32(r.Y), int32(r.Width-10), int32(r.Height))
			DrawText(t.Medium, navLabels[i], r.X+40, r.Y+(r.Height-navLabel)/2-1, navLabel, fg)
			rl.EndScissorMode()
		}
	}

	last := n.items[2]
	sx := last.X + last.Width + navSepGap
	rl.DrawRectangleRec(rl.NewRectangle(sx, b.Y+16, 1, b.Height-32), t.Divider)

	n.Search.Draw()
	n.drawRing()
	if n.hover == 3 && n.Status != "" {
		n.drawTooltip()
	}
}

func (n *NavBar) drawRing() {
	t := n.theme
	c := rl.NewVector2(n.ring.X+n.ring.Width/2, n.ring.Y+n.ring.Height/2)
	const r0, r1 = 10.5, 13
	if n.Playable && n.hover == 3 {
		rl.DrawCircleV(c, 17, rl.Fade(t.Text, 0.08))
	}
	rl.DrawRing(c, r0, r1, 0, 360, 48, rl.Fade(t.TextMuted, 0.25))
	switch {
	case n.Done:
		rl.DrawRing(c, r0, r1, 0, 360, 48, t.Accent)
		polyline([]rl.Vector2{{X: c.X - 3.5, Y: c.Y + 0.2}, {X: c.X - 1, Y: c.Y + 2.7}, {X: c.X + 3.8, Y: c.Y - 2.6}}, 1.8, t.Accent)
	case n.Progress < 0:
		a := float32(math.Mod(float64(n.spin), 360))
		rl.DrawRing(c, r0, r1, a, a+100, 24, t.Accent)
	case n.Progress > 0:
		rl.DrawRing(c, r0, r1, -90, -90+360*min(1, n.Progress), 48, t.Accent)
	}
	if !n.Playable {
		return
	}
	if n.Paused {
		rl.DrawTriangle(rl.NewVector2(c.X-2.5, c.Y-4.5), rl.NewVector2(c.X-2.5, c.Y+4.5), rl.NewVector2(c.X+4.5, c.Y), t.Text)
		rl.DrawTriangle(rl.NewVector2(c.X-2.5, c.Y-4.5), rl.NewVector2(c.X+4.5, c.Y), rl.NewVector2(c.X-2.5, c.Y+4.5), t.Text)
		return
	}
	rl.DrawRectangleRounded(rl.NewRectangle(c.X-4, c.Y-4.5, 2.8, 9), 0.5, 4, t.Text)
	rl.DrawRectangleRounded(rl.NewRectangle(c.X+1.2, c.Y-4.5, 2.8, 9), 0.5, 4, t.Text)
}

func (n *NavBar) drawTooltip() {
	t := n.theme
	const size = 13
	w := MeasureText(t.Regular, n.Status, size) + 24
	sw := float32(rl.GetScreenWidth())
	x := clamp(n.ring.X+n.ring.Width/2-w/2, 8, sw-w-8)
	r := rl.NewRectangle(x, n.bounds.Y-42, w, 32)
	rl.DrawRectangleRounded(rl.NewRectangle(r.X-2, r.Y, r.Width+4, r.Height+4), 0.5, 16, rl.Fade(t.Shadow, 0.5))
	rl.DrawRectangleRounded(r, 0.45, 16, t.Surface)
	DrawText(t.Regular, n.Status, r.X+12, r.Y+(r.Height-size)/2-1, size, t.Text)
}

func polyline(pts []rl.Vector2, thick float32, c rl.Color) {
	for i := 1; i < len(pts); i++ {
		rl.DrawLineEx(pts[i-1], pts[i], thick, c)
	}
	for _, p := range pts {
		rl.DrawCircleV(p, thick/2, c)
	}
}

func arc(c rl.Vector2, r, a0, a1, thick float32, col rl.Color) {
	rl.DrawRing(c, r-thick/2, r+thick/2, a0, a1, 24, col)
}

func drawNavIcon(i int, x, y float32, c rl.Color) {
	const s = 1.8
	p := func(px, py float32) rl.Vector2 { return rl.NewVector2(x+px, y+py) }
	switch i {
	case 0:
		rl.DrawRectangleRoundedLinesEx(rl.NewRectangle(x+2.5, y+2.5, 15, 15), 0.25, 8, s, c)
		rl.DrawCircleV(p(7.4, 7.4), 1.7, c)
		polyline([]rl.Vector2{p(17.2, 12.6), p(13.3, 8.6), p(4.4, 17.3)}, s, c)
	case 1:
		arc(p(7.5, 6.2), 3.3, 0, 360, s, c)
		arc(p(7.5, 18), 5.6, 180, 360, s, c)
		arc(p(12.6, 6.2), 3.3, -70, 70, s, c)
		arc(p(14.6, 18), 4.2, 270, 360, s, c)
	case 2:
		polyline([]rl.Vector2{p(10, 1.8), p(1.8, 5.9), p(10, 10), p(18.2, 5.9), p(10, 1.8)}, s, c)
		polyline([]rl.Vector2{p(1.8, 10), p(10, 14.1), p(18.2, 10)}, s, c)
		polyline([]rl.Vector2{p(1.8, 14.1), p(10, 18.2), p(18.2, 14.1)}, s, c)
	}
}
