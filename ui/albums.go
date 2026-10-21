package ui

import (
	"fmt"
	"math"

	"ariranha/gallery"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	albumPad     = 24
	albumTitle   = 60
	albumCard    = 150
	albumGap     = 18
	AlbumsHeight = albumTitle + albumCard + 72
)

type AlbumStrip struct {
	Albums   []*gallery.Album
	Selected *gallery.Album
	Empty    string

	theme  *Theme
	thumbs *gallery.ThumbCache
	scroll float32
	target float32
	hover  int
	armed  int
}

func NewAlbumStrip(theme *Theme, thumbs *gallery.ThumbCache, albums []*gallery.Album) *AlbumStrip {
	return &AlbumStrip{
		Albums: albums,
		theme:  theme,
		thumbs: thumbs,
		hover:  -1,
		armed:  -1,
	}
}

func (s *AlbumStrip) SetAlbums(albums []*gallery.Album) {
	var selected string
	if s.Selected != nil {
		selected = s.Selected.Name
	}
	s.Albums = albums
	s.Selected = nil
	for _, a := range albums {
		if a.Name == selected {
			s.Selected = a
		}
	}
}

func (s *AlbumStrip) cardRect(bounds rl.Rectangle, i int) rl.Rectangle {
	x := bounds.X + albumPad + float32(i)*(albumCard+albumGap) - s.scroll
	return rl.NewRectangle(x, bounds.Y+albumTitle, albumCard, albumCard)
}

func (s *AlbumStrip) maxScroll(bounds rl.Rectangle) float32 {
	content := albumPad*2 + float32(len(s.Albums))*(albumCard+albumGap) - albumGap
	return max(0, content-bounds.Width)
}

func (s *AlbumStrip) Hovered() bool {
	return s.hover >= 0
}

func (s *AlbumStrip) Update(bounds rl.Rectangle) bool {
	mouse := rl.GetMousePosition()
	inside := rl.CheckCollisionPointRec(mouse, bounds)

	if inside {
		wheel := rl.GetMouseWheelMoveV()
		d := wheel.Y
		if math.Abs(float64(wheel.X)) > math.Abs(float64(wheel.Y)) {
			d = wheel.X
		}
		s.target -= d * 90
	}
	s.target = clamp(s.target, 0, s.maxScroll(bounds))
	s.scroll = smooth(s.scroll, s.target)

	s.hover = -1
	if inside {
		for i := range s.Albums {
			r := s.cardRect(bounds, i)
			r.Height += 48
			if rl.CheckCollisionPointRec(mouse, r) {
				s.hover = i
				break
			}
		}
	}

	changed := false
	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		s.armed = s.hover
	}
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		if s.armed >= 0 && s.armed == s.hover {
			a := s.Albums[s.hover]
			if s.Selected == a {
				s.Selected = nil
			} else {
				s.Selected = a
			}
			changed = true
		}
		s.armed = -1
	}

	return changed
}

func (s *AlbumStrip) Draw(bounds rl.Rectangle) {
	t := s.theme
	rl.DrawRectangleRec(bounds, t.Background)
	DrawText(t.Medium, "Álbuns", bounds.X+albumPad, bounds.Y+20, 24, t.Text)

	if len(s.Albums) == 0 {
		DrawText(t.Regular, s.Empty, bounds.X+albumPad, bounds.Y+albumTitle+8, 16, t.TextMuted)
		return
	}

	rl.BeginScissorMode(int32(bounds.X), int32(bounds.Y+albumTitle-6), int32(bounds.Width), int32(bounds.Height-albumTitle+6))
	for i, a := range s.Albums {
		r := s.cardRect(bounds, i)
		if r.X+r.Width < bounds.X || r.X > bounds.X+bounds.Width {
			continue
		}

		cover := a.Photos[0]
		rl.DrawRectangleRec(r, cover.Color)
		if tex, ok := s.thumbs.Get(cover); ok {
			DrawCover(tex, r, rl.White)
		}
		if i == s.hover {
			rl.DrawRectangleRec(r, rl.Fade(rl.White, 0.08))
		}
		if a == s.Selected {
			rl.DrawRectangleLinesEx(rl.NewRectangle(r.X-4, r.Y-4, r.Width+8, r.Height+8), 3, t.Accent)
		}

		DrawText(t.Medium, FitText(t.Medium, a.Name, 16, albumCard), r.X, r.Y+r.Height+10, 16, t.Text)
		DrawText(t.Regular, itemsLabel(len(a.Photos)), r.X, r.Y+r.Height+30, 14, t.TextMuted)
	}
	rl.EndScissorMode()
}

func itemsLabel(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d itens", n)
}
