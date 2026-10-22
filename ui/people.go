package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	"ariranha/search"

	rl "github.com/gen2brain/raylib-go/raylib"
	"golang.org/x/image/draw"
)

const (
	peoplePad    = 24
	peopleTitle  = 44
	peopleAvatar = 84
	peopleGap    = 22
	PeopleHeight = peopleTitle + peopleAvatar + 52
	avatarPixels = 168
)

type avatar struct {
	tex    rl.Texture2D
	source []byte
}

type PeopleStrip struct {
	People    []search.PersonInfo
	Selected  int64
	MinPhotos int
	Empty     string
	expanded  bool

	theme   *Theme
	avatars map[int64]*avatar
	scroll  float32
	target  float32
	hover   int
	armed   int
}

func NewPeopleStrip(theme *Theme) *PeopleStrip {
	return &PeopleStrip{theme: theme, avatars: map[int64]*avatar{}, hover: -1, armed: -1, MinPhotos: 2}
}

func (s *PeopleStrip) visible() ([]search.PersonInfo, int) {
	var shown []search.PersonInfo
	hidden := 0
	for _, p := range s.People {
		if len(p.Paths) >= s.MinPhotos || p.ID == s.Selected {
			shown = append(shown, p)
		} else {
			hidden++
		}
	}
	if s.expanded {
		return s.People, hidden
	}
	return shown, hidden
}

func (s *PeopleStrip) slots() int {
	shown, hidden := s.visible()
	if hidden > 0 {
		return len(shown) + 1
	}
	return len(shown)
}

func (s *PeopleStrip) SetPeople(people []search.PersonInfo) {
	s.People = people
	alive := map[int64]bool{}
	for _, p := range people {
		alive[p.ID] = true
	}
	for id, a := range s.avatars {
		if !alive[id] {
			rl.UnloadTexture(a.tex)
			delete(s.avatars, id)
		}
	}
}

func (s *PeopleStrip) Unload() {
	for id, a := range s.avatars {
		rl.UnloadTexture(a.tex)
		delete(s.avatars, id)
	}
}

func (s *PeopleStrip) Hovered() bool {
	return s.hover >= 0
}

func (s *PeopleStrip) SelectedPerson() (search.PersonInfo, bool) {
	for _, p := range s.People {
		if p.ID == s.Selected {
			return p, true
		}
	}
	return search.PersonInfo{}, false
}

func (s *PeopleStrip) slot(bounds rl.Rectangle, i int) rl.Rectangle {
	x := bounds.X + peoplePad + float32(i)*(peopleAvatar+peopleGap) - s.scroll
	return rl.NewRectangle(x, bounds.Y+peopleTitle, peopleAvatar, peopleAvatar)
}

func (s *PeopleStrip) Update(bounds rl.Rectangle) bool {
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
	content := peoplePad*2 + float32(s.slots())*(peopleAvatar+peopleGap) - peopleGap
	s.target = clamp(s.target, 0, max(0, content-bounds.Width))
	s.scroll = smooth(s.scroll, s.target)

	s.hover = -1
	if inside {
		for i := range s.slots() {
			r := s.slot(bounds, i)
			r.Height += 40
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
		shown, _ := s.visible()
		if s.armed >= 0 && s.armed == s.hover && s.hover == len(shown) {
			s.expanded = !s.expanded
			if !s.expanded {
				s.target = 0
			}
		} else if s.armed >= 0 && s.armed == s.hover {
			id := shown[s.hover].ID
			if s.Selected == id {
				s.Selected = 0
			} else {
				s.Selected = id
			}
			changed = true
		}
		s.armed = -1
	}
	return changed
}

func (s *PeopleStrip) Draw(bounds rl.Rectangle) {
	t := s.theme
	rl.DrawRectangleRec(bounds, t.Background)
	DrawText(t.Medium, "People", bounds.X+peoplePad, bounds.Y+12, 20, t.Text)

	rl.BeginScissorMode(int32(bounds.X), int32(bounds.Y+peopleTitle-6), int32(bounds.Width), int32(bounds.Height-peopleTitle+6))
	shown, hidden := s.visible()
	if len(shown) == 0 && hidden == 0 {
		DrawText(t.Regular, s.Empty, bounds.X+peoplePad, bounds.Y+peopleTitle+8, 16, t.TextMuted)
	}
	for i, p := range shown {
		r := s.slot(bounds, i)
		if r.X+r.Width < bounds.X || r.X > bounds.X+bounds.Width {
			continue
		}

		center := rl.NewVector2(r.X+r.Width/2, r.Y+r.Height/2)
		if a := s.avatar(p); a != nil {
			rl.DrawTexturePro(a.tex, rl.NewRectangle(0, 0, float32(a.tex.Width), float32(a.tex.Height)), r, rl.Vector2{}, 0, rl.White)
		} else {
			rl.DrawCircleV(center, r.Width/2, t.Surface)
		}
		if i == s.hover {
			rl.DrawCircleV(center, r.Width/2, rl.Fade(rl.White, 0.08))
		}
		if p.ID == s.Selected {
			rl.DrawRing(center, r.Width/2+3, r.Width/2+6, 0, 360, 64, t.People)
		}

		name := p.Name
		if name == "" {
			name = fmt.Sprintf("Person %d", p.ID)
		}
		name = FitText(t.Medium, name, 14, peopleAvatar+peopleGap-4)
		nw := MeasureText(t.Medium, name, 14)
		DrawText(t.Medium, name, center.X-nw/2, r.Y+r.Height+8, 14, t.Text)
		count := itemsLabel(len(p.Paths))
		cw := MeasureText(t.Regular, count, 12)
		DrawText(t.Regular, count, center.X-cw/2, r.Y+r.Height+26, 12, t.TextMuted)
	}
	if hidden > 0 {
		s.drawToggle(s.slot(bounds, len(shown)), hidden, s.hover == len(shown))
	}
	rl.EndScissorMode()
}

func (s *PeopleStrip) Avatar(p search.PersonInfo) (rl.Texture2D, bool) {
	if a := s.avatar(p); a != nil {
		return a.tex, true
	}
	return rl.Texture2D{}, false
}

func (s *PeopleStrip) avatar(p search.PersonInfo) *avatar {
	if a, ok := s.avatars[p.ID]; ok && bytes.Equal(a.source, p.Crop) {
		return a
	}
	if len(p.Crop) == 0 {
		return nil
	}
	src, err := jpeg.Decode(bytes.NewReader(p.Crop))
	if err != nil {
		return nil
	}

	img := image.NewRGBA(image.Rect(0, 0, avatarPixels, avatarPixels))
	draw.CatmullRom.Scale(img, img.Bounds(), src, src.Bounds(), draw.Src, nil)
	circleMask(img)

	rimg := rl.NewImage(img.Pix, avatarPixels, avatarPixels, 1, rl.UncompressedR8g8b8a8)
	tex := rl.LoadTextureFromImage(rimg)
	rl.GenTextureMipmaps(&tex)
	rl.SetTextureFilter(tex, rl.FilterTrilinear)

	if old, ok := s.avatars[p.ID]; ok {
		rl.UnloadTexture(old.tex)
	}
	a := &avatar{tex: tex, source: p.Crop}
	s.avatars[p.ID] = a
	return a
}

func circleMask(img *image.RGBA) {
	n := img.Bounds().Dx()
	c := float64(n) / 2
	for y := range n {
		for x := range n {
			d := math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c)
			a := clamp(float32(c-d+0.5), 0, 1)
			o := img.PixOffset(x, y)
			for k := range 4 {
				img.Pix[o+k] = uint8(float32(img.Pix[o+k]) * a)
			}
		}
	}
}

func (s *PeopleStrip) drawToggle(r rl.Rectangle, hidden int, hover bool) {
	t := s.theme
	center := rl.NewVector2(r.X+r.Width/2, r.Y+r.Height/2)
	bg := t.Raised
	if hover {
		bg = t.RaisedHover
	}
	rl.DrawCircleV(center, r.Width/2, bg)

	icon, label := fmt.Sprintf("+%d", hidden), "Others"
	if s.expanded {
		icon, label = "−", "Hide"
	}
	iw := MeasureText(t.Medium, icon, 24)
	DrawText(t.Medium, icon, center.X-iw/2, center.Y-13, 24, t.People)

	lw := MeasureText(t.Medium, label, 14)
	DrawText(t.Medium, label, center.X-lw/2, r.Y+r.Height+8, 14, t.Text)
	sub := fmt.Sprintf("%d people", hidden)
	if hidden == 1 {
		sub = "1 person"
	}
	sw := MeasureText(t.Regular, sub, 12)
	DrawText(t.Regular, sub, center.X-sw/2, r.Y+r.Height+26, 12, t.TextMuted)
}
