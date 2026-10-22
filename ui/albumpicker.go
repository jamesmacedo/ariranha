package ui

import (
	"fmt"

	"ariranha/gallery"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	pkWidth = 420
	pkRow   = 60
	pkHead  = 64
	pkPad   = 12
)

type AlbumPicker struct {
	Open   bool
	Albums []*gallery.Album
	OnPick func(a *gallery.Album)
	OnNew  func()

	theme  *Theme
	thumbs *gallery.ThumbCache
	count  int
	card   rl.Rectangle
	list   rl.Rectangle
	scroll float32
	target float32
	hover  int
	armed  int
	fade   float32
}

func NewAlbumPicker(theme *Theme, thumbs *gallery.ThumbCache) *AlbumPicker {
	return &AlbumPicker{theme: theme, thumbs: thumbs, hover: -1, armed: -1}
}

func (p *AlbumPicker) Show(count int) {
	p.Open = true
	p.count = count
	p.scroll, p.target = 0, 0
	p.hover, p.armed = -1, -1
	p.fade = 0
}

func (p *AlbumPicker) Close() {
	p.Open = false
}

func (p *AlbumPicker) Hovered() bool {
	return p.Open && p.hover >= 0
}

func (p *AlbumPicker) rows() int {
	return len(p.Albums) + 1
}

func (p *AlbumPicker) layout(screen rl.Rectangle) {
	h := float32(pkHead + p.rows()*pkRow + pkPad)
	p.card = centered(screen, pkWidth, min(h, 560))
	p.list = rl.NewRectangle(p.card.X, p.card.Y+pkHead, p.card.Width, p.card.Height-pkHead-pkPad)
}

func (p *AlbumPicker) rowRect(i int) rl.Rectangle {
	return rl.NewRectangle(p.list.X+pkPad, p.list.Y+float32(i)*pkRow-p.scroll, p.list.Width-pkPad*2, pkRow)
}

func (p *AlbumPicker) Update(screen rl.Rectangle) {
	if !p.Open {
		return
	}
	p.fade = smooth(p.fade, 1)
	p.layout(screen)
	if rl.IsKeyPressed(rl.KeyEscape) {
		p.Close()
		return
	}

	mouse := rl.GetMousePosition()
	maxS := max(0, float32(p.rows()*pkRow)-p.list.Height)
	if rl.CheckCollisionPointRec(mouse, p.list) {
		p.target -= rl.GetMouseWheelMove() * 80
	}
	p.target = clamp(p.target, 0, maxS)
	p.scroll = smooth(p.scroll, p.target)

	p.hover = -1
	if rl.CheckCollisionPointRec(mouse, p.list) {
		for i := range p.rows() {
			if rl.CheckCollisionPointRec(mouse, p.rowRect(i)) {
				p.hover = i
				break
			}
		}
	}

	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		p.armed = p.hover
		if !rl.CheckCollisionPointRec(mouse, p.card) {
			p.armed = -2
		}
	}
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		switch {
		case p.armed == -2 && !rl.CheckCollisionPointRec(mouse, p.card):
			p.Close()
		case p.armed >= 0 && p.armed == p.hover:
			p.Close()
			if p.armed == 0 {
				if p.OnNew != nil {
					p.OnNew()
				}
			} else if p.OnPick != nil {
				p.OnPick(p.Albums[p.armed-1])
			}
		}
		p.armed = -1
	}
}

func (p *AlbumPicker) Draw(screen rl.Rectangle) {
	if !p.Open {
		return
	}
	t := p.theme
	p.layout(screen)
	drawScrim(screen, p.fade)
	drawCard(t, p.card)

	title := "Add to album"
	if p.count > 1 {
		title = fmt.Sprintf("Add %d items to album", p.count)
	}
	DrawText(t.Medium, title, p.card.X+24, p.card.Y+24, 18, t.Text)

	rl.BeginScissorMode(int32(p.list.X), int32(p.list.Y), int32(p.list.Width), int32(p.list.Height))
	for i := range p.rows() {
		r := p.rowRect(i)
		if r.Y+r.Height < p.list.Y || r.Y > p.list.Y+p.list.Height {
			continue
		}
		if i == p.hover {
			DrawRoundedRect(r, 12, t.Raised)
		}
		icon := rl.NewRectangle(r.X+8, r.Y+8, r.Height-16, r.Height-16)
		tx := icon.X + icon.Width + 14
		if i == 0 {
			DrawRoundedRect(icon, 10, rl.Fade(t.Accent, 0.14))
			plusIcon(rl.NewVector2(icon.X+icon.Width/2, icon.Y+icon.Height/2), 8, t.Accent)
			DrawText(t.Medium, "New album", tx, r.Y+(r.Height-15)/2-1, 15, t.Accent)
			continue
		}
		a := p.Albums[i-1]
		col := t.AlbumColor(a.Color)
		drawAlbumCover(t, p.thumbs, a, icon, 10)
		DrawText(t.Medium, FitText(t.Medium, a.Name, 15, r.X+r.Width-tx-10), tx, r.Y+12, 15, t.Text)
		rl.DrawCircleV(rl.NewVector2(tx+4, r.Y+r.Height-19), 4, col)
		DrawText(t.Regular, itemsLabel(len(a.Photos)), tx+14, r.Y+r.Height-26, 13, t.TextMuted)
	}
	rl.EndScissorMode()
}

func drawAlbumCover(t *Theme, thumbs *gallery.ThumbCache, a *gallery.Album, r rl.Rectangle, radius float32) {
	col := t.AlbumColor(a.Color)
	if len(a.Photos) > 0 {
		cover := a.Photos[0]
		if tex, ok := thumbs.Get(cover); ok {
			DrawCoverRounded(tex, r, radius, rl.White)
			return
		}
		DrawRoundedRect(r, radius, cover.Color)
		return
	}
	DrawRoundedRect(r, radius, rl.Fade(col, 0.16))
	s := min(r.Width, r.Height) / 20 * 0.45
	if s >= 0.6 {
		drawScaledAlbumIcon(rl.NewVector2(r.X+r.Width/2, r.Y+r.Height/2), s, col)
	}
}

func drawScaledAlbumIcon(c rl.Vector2, s float32, col rl.Color) {
	p := func(px, py float32) rl.Vector2 { return rl.NewVector2(c.X+(px-10)*s, c.Y+(py-10)*s) }
	th := max(1.6, 1.8*s*0.7)
	polyline([]rl.Vector2{p(10, 1.8), p(1.8, 5.9), p(10, 10), p(18.2, 5.9), p(10, 1.8)}, th, col)
	polyline([]rl.Vector2{p(1.8, 10), p(10, 14.1), p(18.2, 10)}, th, col)
	polyline([]rl.Vector2{p(1.8, 14.1), p(10, 18.2), p(18.2, 14.1)}, th, col)
}
