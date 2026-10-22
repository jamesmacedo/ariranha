package ui

import (
	"fmt"
	"math"

	"ariranha/gallery"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	agPad    = 24
	agGap    = 18
	agMinW   = 190
	agText   = 58
	agRadius = 14
)

type AlbumGrid struct {
	Albums      []*gallery.Album
	BottomInset float32
	Obstacle    rl.Rectangle
	OnOpen      func(*gallery.Album)
	OnNew       func()

	theme  *Theme
	thumbs *gallery.ThumbCache
	scroll float32
	target float32
	hover  int
	armed  int
	pressY float32
	cols   int
	cardW  float32
}

func NewAlbumGrid(theme *Theme, thumbs *gallery.ThumbCache) *AlbumGrid {
	return &AlbumGrid{theme: theme, thumbs: thumbs, hover: -1, armed: -1}
}

func (g *AlbumGrid) Hovered() bool {
	return g.hover >= 0
}

func (g *AlbumGrid) view(bounds rl.Rectangle) rl.Rectangle {
	return rl.NewRectangle(bounds.X, bounds.Y+TimelineBar, bounds.Width, max(0, bounds.Height-TimelineBar))
}

func (g *AlbumGrid) measure(width float32) {
	inner := width - agPad*2
	g.cols = max(1, int((inner+agGap)/(agMinW+agGap)))
	g.cardW = (inner - float32(g.cols-1)*agGap) / float32(g.cols)
}

func (g *AlbumGrid) count() int {
	return len(g.Albums) + 1
}

func (g *AlbumGrid) card(v rl.Rectangle, i int) rl.Rectangle {
	col, row := i%g.cols, i/g.cols
	x := v.X + agPad + float32(col)*(g.cardW+agGap)
	y := v.Y + 8 + float32(row)*(g.cardW+agText+agGap) - g.scroll
	return rl.NewRectangle(x, y, g.cardW, g.cardW+agText)
}

func (g *AlbumGrid) height() float32 {
	rows := (g.count() + g.cols - 1) / g.cols
	return 8 + float32(rows)*(g.cardW+agText+agGap)
}

func (g *AlbumGrid) Update(bounds rl.Rectangle) {
	v := g.view(bounds)
	g.measure(v.Width)
	mouse := rl.GetMousePosition()
	inside := rl.CheckCollisionPointRec(mouse, v) && !rl.CheckCollisionPointRec(mouse, g.Obstacle)

	if rl.CheckCollisionPointRec(mouse, v) {
		g.target -= rl.GetMouseWheelMove() * 120
	}
	g.target = clamp(g.target, 0, max(0, g.height()+g.BottomInset-v.Height))
	g.scroll = smooth(g.scroll, g.target)

	g.hover = -1
	if inside {
		for i := range g.count() {
			if rl.CheckCollisionPointRec(mouse, g.card(v, i)) {
				g.hover = i
				break
			}
		}
	}

	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		g.armed = g.hover
		g.pressY = mouse.Y + g.scroll
	}
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		moved := math.Abs(float64(mouse.Y+g.scroll-g.pressY)) > 6
		if g.armed >= 0 && g.armed == g.hover && !moved {
			if g.armed == 0 {
				if g.OnNew != nil {
					g.OnNew()
				}
			} else if g.OnOpen != nil {
				g.OnOpen(g.Albums[g.armed-1])
			}
		}
		g.armed = -1
	}
}

func (g *AlbumGrid) Draw(bounds rl.Rectangle) {
	t := g.theme
	v := g.view(bounds)
	g.measure(v.Width)

	rl.DrawRectangleRec(bounds, t.Background)
	DrawText(t.Medium, "Albums", bounds.X+tlPad, bounds.Y+18, 24, t.Text)
	tw := MeasureText(t.Medium, "Albums", 24)
	label := "1 album"
	if len(g.Albums) != 1 {
		label = fmt.Sprintf("%d albums", len(g.Albums))
	}
	DrawText(t.Regular, label, bounds.X+tlPad+tw+14, bounds.Y+25, 15, t.TextMuted)

	rl.BeginScissorMode(int32(v.X), int32(v.Y), int32(v.Width), int32(v.Height))
	for i := range g.count() {
		r := g.card(v, i)
		if r.Y+r.Height < v.Y || r.Y > v.Y+v.Height {
			continue
		}
		cover := rl.NewRectangle(r.X, r.Y, r.Width, r.Width)
		hot := i == g.hover

		if i == 0 {
			bg := t.Surface
			if hot {
				bg = t.Raised
			}
			DrawRoundedRect(cover, agRadius, bg)
			c := rl.NewVector2(cover.X+cover.Width/2, cover.Y+cover.Height/2-10)
			rl.DrawCircleV(c, 26, rl.Fade(t.Accent, 0.14))
			plusIcon(c, 10, t.Accent)
			lw := MeasureText(t.Medium, "New album", 15)
			DrawText(t.Medium, "New album", c.X-lw/2, c.Y+38, 15, t.Accent)
			continue
		}

		a := g.Albums[i-1]
		drawAlbumCover(t, g.thumbs, a, cover, agRadius)
		if hot {
			DrawRoundedRect(cover, agRadius, rl.Fade(rl.White, 0.07))
		}

		col := t.AlbumColor(a.Color)
		ty := cover.Y + cover.Height + 10
		rl.DrawCircleV(rl.NewVector2(r.X+5, ty+9), 5, col)
		DrawText(t.Medium, FitText(t.Medium, a.Name, 16, r.Width-18), r.X+16, ty, 16, t.Text)
		sub := itemsLabel(len(a.Photos))
		if a.Description != "" {
			sub += "  ·  " + a.Description
		}
		DrawText(t.Regular, FitText(t.Regular, sub, 13, r.Width), r.X, ty+24, 13, t.TextMuted)
	}
	rl.EndScissorMode()
}

func itemsLabel(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
