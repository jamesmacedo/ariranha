package ui

import (
	"fmt"
	"math"
	"sort"
	"time"
	"unicode"
	"unicode/utf8"

	"ariranha/gallery"
	"ariranha/layout"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	tlPad       = 24
	TimelineBar = 56
	tlTrack     = 14
)

type tile struct {
	photo *gallery.Photo
	rect  rl.Rectangle
}

type dayHeader struct {
	text string
	rect rl.Rectangle
}

type Timeline struct {
	Title       string
	Status      string
	EmptyText   string
	KeysEnabled bool

	theme   *Theme
	section string
	thumbs  *gallery.ThumbCache
	photos  []*gallery.Photo
	tiles   []tile
	headers []dayHeader
	height  float32
	laidOut float32

	scroll     float32
	target     float32
	dragging   bool
	dragOffset float32
	direction  int
	hover      int
}

func NewTimeline(theme *Theme, thumbs *gallery.ThumbCache) *Timeline {
	return &Timeline{theme: theme, thumbs: thumbs, hover: -1, laidOut: -1, KeysEnabled: true}
}

func (t *Timeline) SetPhotos(title string, photos []*gallery.Photo) {
	t.set(title, "", photos)
}

func (t *Timeline) SetResults(title, section string, photos []*gallery.Photo) {
	t.set(title, section, photos)
}

func (t *Timeline) Refresh(photos []*gallery.Photo) {
	t.photos = photos
	t.laidOut = -1
}

func (t *Timeline) set(title, section string, photos []*gallery.Photo) {
	t.Title = title
	t.section = section
	t.photos = photos
	t.laidOut = -1
	t.scroll, t.target = 0, 0
}

func (t *Timeline) view(bounds rl.Rectangle) rl.Rectangle {
	return rl.NewRectangle(bounds.X, bounds.Y+TimelineBar, bounds.Width, max(0, bounds.Height-TimelineBar))
}

func (t *Timeline) layout(width float32) {
	t.laidOut = width
	t.tiles = t.tiles[:0]
	t.headers = t.headers[:0]

	var days [][]*gallery.Photo
	if t.section != "" && len(t.photos) > 0 {
		days = append(days, t.photos)
	}
	for i := 0; t.section == "" && i < len(t.photos); {
		j := i
		for j < len(t.photos) && sameDay(t.photos[j].Date, t.photos[i].Date) {
			j++
		}
		days = append(days, t.photos[i:j])
		i = j
	}

	sections := make([][]float32, len(days))
	for s, day := range days {
		sections[s] = make([]float32, len(day))
		for i, p := range day {
			sections[s][i] = p.Aspect()
		}
	}

	res := layout.Justify(layout.Responsive(width-tlTrack), sections)

	now := time.Now()
	for _, h := range res.Headers {
		text := t.section
		if text == "" {
			text = formatDay(days[h.Section][0].Date, now)
		}
		t.headers = append(t.headers, dayHeader{text: text, rect: toRect(h.Rect)})
	}
	for _, b := range res.Boxes {
		t.tiles = append(t.tiles, tile{photo: days[b.Section][b.Index], rect: toRect(b.Rect)})
	}
	t.height = res.Height
}

func toRect(r layout.Rect) rl.Rectangle {
	return rl.NewRectangle(r.X, r.Y, r.W, r.H)
}

func (t *Timeline) maxScroll(v rl.Rectangle) float32 {
	return max(0, t.height-v.Height)
}

func (t *Timeline) track(v rl.Rectangle) rl.Rectangle {
	return rl.NewRectangle(v.X+v.Width-tlTrack, v.Y, tlTrack, v.Height)
}

func (t *Timeline) thumb(v rl.Rectangle) rl.Rectangle {
	if t.height <= 0 {
		return rl.Rectangle{}
	}
	h := max(40, v.Height*v.Height/t.height)
	frac := float32(0)
	if m := t.maxScroll(v); m > 0 {
		frac = t.scroll / m
	}
	return rl.NewRectangle(v.X+v.Width-tlTrack+4, v.Y+(v.Height-h)*frac, 6, h)
}

func (t *Timeline) visible(top, bottom float32) (int, int) {
	first := sort.Search(len(t.tiles), func(i int) bool {
		r := t.tiles[i].rect
		return r.Y+r.Height >= top
	})
	last := sort.Search(len(t.tiles), func(i int) bool {
		return t.tiles[i].rect.Y > bottom
	})
	return first, max(first, last)
}

func (t *Timeline) Update(bounds rl.Rectangle) {
	v := t.view(bounds)
	if t.laidOut != v.Width {
		t.layout(v.Width)
	}
	maxS := t.maxScroll(v)
	mouse := rl.GetMousePosition()
	inside := rl.CheckCollisionPointRec(mouse, v)

	if inside {
		t.target -= rl.GetMouseWheelMove() * 120
	}

	switch {
	case !t.KeysEnabled:
	case rl.IsKeyPressed(rl.KeyPageDown):
		t.target += v.Height * 0.9
	case rl.IsKeyPressed(rl.KeyPageUp):
		t.target -= v.Height * 0.9
	case rl.IsKeyPressed(rl.KeyHome):
		t.target = 0
	case rl.IsKeyPressed(rl.KeyEnd):
		t.target = maxS
	}

	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) && maxS > 0 && rl.CheckCollisionPointRec(mouse, t.track(v)) {
		th := t.thumb(v)
		t.dragging = true
		if mouse.Y >= th.Y && mouse.Y <= th.Y+th.Height {
			t.dragOffset = mouse.Y - th.Y
		} else {
			t.dragOffset = th.Height / 2
		}
	}
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		t.dragging = false
	}

	if t.dragging {
		th := t.thumb(v)
		span := max(1, v.Height-th.Height)
		t.target = clamp((mouse.Y-t.dragOffset-v.Y)/span, 0, 1) * maxS
		t.scroll = t.target
	}

	t.target = clamp(t.target, 0, maxS)
	if d := t.target - t.scroll; d > 1 {
		t.direction = 1
	} else if d < -1 {
		t.direction = -1
	}
	if !t.dragging {
		t.scroll = smooth(t.scroll, t.target)
	}

	t.hover = -1
	if inside && !t.dragging && !rl.CheckCollisionPointRec(mouse, t.track(v)) {
		local := rl.NewVector2(mouse.X-v.X, mouse.Y-v.Y+t.scroll)
		first, last := t.visible(t.scroll, t.scroll+v.Height)
		for i := first; i < last; i++ {
			if rl.CheckCollisionPointRec(local, t.tiles[i].rect) {
				t.hover = i
				break
			}
		}
	}
}

func (t *Timeline) Draw(bounds rl.Rectangle) {
	th := t.theme
	v := t.view(bounds)

	rl.DrawRectangleRec(bounds, th.Background)
	DrawText(th.Medium, t.Title, bounds.X+tlPad, bounds.Y+18, 24, th.Text)
	tw := MeasureText(th.Medium, t.Title, 24)
	label := itemsLabel(len(t.photos))
	if t.Status != "" {
		label += "  •  " + t.Status
	}
	DrawText(th.Regular, label, bounds.X+tlPad+tw+14, bounds.Y+25, 15, th.TextMuted)

	if len(t.photos) == 0 {
		msg := t.EmptyText
		if msg == "" {
			msg = "Nenhuma foto encontrada"
		}
		mw := MeasureText(th.Regular, msg, 18)
		DrawText(th.Regular, msg, v.X+(v.Width-mw)/2, v.Y+v.Height/2-9, 18, th.TextMuted)
		return
	}

	top, bottom := t.scroll, t.scroll+v.Height

	rl.BeginScissorMode(int32(v.X), int32(v.Y), int32(v.Width), int32(v.Height))

	for _, h := range t.headers {
		if h.rect.Y+h.rect.Height < top || h.rect.Y > bottom {
			continue
		}
		text := FitText(th.Medium, h.text, 17, h.rect.Width)
		DrawText(th.Medium, text, v.X+h.rect.X, v.Y+h.rect.Y-t.scroll+h.rect.Height-30, 17, th.Text)
	}

	first, last := t.visible(top, bottom)
	for i := first; i < last; i++ {
		tl := t.tiles[i]
		r := rl.NewRectangle(v.X+tl.rect.X, v.Y+tl.rect.Y-t.scroll, tl.rect.Width, tl.rect.Height)
		rl.DrawRectangleRec(r, tl.photo.Color)
		if tex, ok := t.thumbs.Get(tl.photo); ok {
			DrawCover(tex, r, rl.White)
		}
		if i == t.hover {
			rl.DrawRectangleRec(r, rl.Fade(rl.White, 0.08))
		}
	}

	h := v.Height
	ranges := [][2]float32{
		{bottom, bottom + h},
		{top - h/2, top},
		{bottom + h, bottom + 4*h},
		{top - 2*h, top - h/2},
	}
	if t.direction < 0 {
		ranges = [][2]float32{
			{top - h, top},
			{bottom, bottom + h/2},
			{top - 4*h, top - h},
			{bottom + h/2, bottom + 2*h},
		}
	}
	for _, rng := range ranges {
		a, b := t.visible(rng[0], rng[1])
		for i := a; i < b; i++ {
			t.thumbs.Prefetch(t.tiles[i].photo)
		}
	}

	rl.EndScissorMode()

	if t.maxScroll(v) > 0 {
		bar := t.thumb(v)
		c := rl.Fade(th.TextMuted, 0.35)
		if t.dragging || rl.CheckCollisionPointRec(rl.GetMousePosition(), t.track(v)) {
			c = rl.Fade(th.TextMuted, 0.7)
		}
		rl.DrawRectangleRounded(bar, 1, 6, c)
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

var weekdays = [...]string{"dom.", "seg.", "ter.", "qua.", "qui.", "sex.", "sáb."}
var months = [...]string{"jan.", "fev.", "mar.", "abr.", "mai.", "jun.", "jul.", "ago.", "set.", "out.", "nov.", "dez."}

func formatDay(d, now time.Time) string {
	y, m, day := d.Date()
	ny, nm, nd := now.Date()
	a := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	b := time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)

	switch int(math.Round(b.Sub(a).Hours() / 24)) {
	case 0:
		return "Hoje"
	case 1:
		return "Ontem"
	}

	s := fmt.Sprintf("%s, %d de %s", weekdays[d.Weekday()], day, months[m-1])
	if y != ny {
		s += fmt.Sprintf(" de %d", y)
	}
	return capitalize(s)
}

func capitalize(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}
