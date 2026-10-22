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
	tileRadius  = 12
)

type tile struct {
	photo *gallery.Photo
	rect  rl.Rectangle
	month int
}

type dayHeader struct {
	text  string
	sub   string
	count string
	rect  rl.Rectangle
}

type Timeline struct {
	Title       string
	Status      string
	EmptyText   string
	KeysEnabled bool
	BottomInset float32
	Obstacle    rl.Rectangle
	OnOpen      func(photos []*gallery.Photo, index int)

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
	armed      int
	pressY     float32
	scrub      scrubber
}

func NewTimeline(theme *Theme, thumbs *gallery.ThumbCache) *Timeline {
	return &Timeline{theme: theme, thumbs: thumbs, hover: -1, armed: -1, laidOut: -1, KeysEnabled: true}
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

func (t *Timeline) Remove(p *gallery.Photo) {
	for i, q := range t.photos {
		if q == p {
			t.photos = append(append([]*gallery.Photo(nil), t.photos[:i]...), t.photos[i+1:]...)
			t.laidOut = -1
			return
		}
	}
}

func (t *Timeline) Reveal(p *gallery.Photo, bounds rl.Rectangle) {
	v := t.view(bounds)
	for _, tl := range t.tiles {
		if tl.photo != p {
			continue
		}
		if tl.rect.Y < t.target || tl.rect.Y+tl.rect.Height > t.target+v.Height-t.BottomInset {
			t.target = clamp(tl.rect.Y-v.Height/3, 0, t.maxScroll(v))
			t.scroll = t.target
		}
		return
	}
}

func (t *Timeline) TileHovered() bool {
	return t.hover >= 0
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

	cfg := layout.Responsive(width - t.gutter())
	cfg.PackSections = false
	cfg.Gap = 6
	cfg.Padding = 16
	cfg.HeaderHeight = 42
	cfg.SectionGap = 12
	res := layout.Justify(cfg, sections)

	now := time.Now()
	var marks []monthMark
	months := make([]int, len(days))
	for _, h := range res.Headers {
		day := days[h.Section]
		hd := dayHeader{text: t.section, count: itemsLabel(len(day)), rect: toRect(h.Rect)}
		if t.dated() {
			d := day[0].Date
			hd.text, hd.sub = formatDay(d, now), daySub(d, now)
			y, m, _ := d.Date()
			if n := len(marks); n == 0 || marks[n-1].year != y || marks[n-1].month != m {
				marks = append(marks, monthMark{year: y, month: m, y: h.Rect.Y})
			}
			months[h.Section] = len(marks) - 1
		}
		t.headers = append(t.headers, hd)
	}
	for _, b := range res.Boxes {
		t.tiles = append(t.tiles, tile{photo: days[b.Section][b.Index], rect: toRect(b.Rect), month: months[b.Section]})
	}
	t.height = res.Height
	t.scrub.set(marks)
}

func (t *Timeline) dated() bool {
	return t.section == ""
}

func (t *Timeline) gutter() float32 {
	if t.dated() {
		return scrubW
	}
	return tlTrack
}

func (t *Timeline) scrubArea(v rl.Rectangle) rl.Rectangle {
	return rl.NewRectangle(v.X+v.Width-scrubW, v.Y+10, scrubW, max(0, v.Height-50))
}

func (t *Timeline) ScrubHovered() bool {
	return t.dated() && t.scrub.hover >= 0
}

func (t *Timeline) activeMonth() int {
	first, _ := t.visible(t.scroll+1, t.scroll+2)
	if first >= len(t.tiles) {
		return -1
	}
	return t.tiles[first].month
}

func toRect(r layout.Rect) rl.Rectangle {
	return rl.NewRectangle(r.X, r.Y, r.W, r.H)
}

func (t *Timeline) maxScroll(v rl.Rectangle) float32 {
	return max(0, t.height+t.BottomInset-v.Height)
}

func (t *Timeline) track(v rl.Rectangle) rl.Rectangle {
	return rl.NewRectangle(v.X+v.Width-t.gutter(), v.Y, t.gutter(), v.Height)
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

	t.scrub.hover = -1
	if t.dated() {
		area := t.scrubArea(v)
		if t.scrub.placedH != area.Height {
			t.scrub.place(area.Height)
		}
		if rl.CheckCollisionPointRec(mouse, area) || t.scrub.dragging {
			t.scrub.hover = t.scrub.itemAt(mouse.Y - area.Y)
		}
		if rl.IsMouseButtonPressed(rl.MouseButtonLeft) && t.scrub.hover >= 0 && rl.CheckCollisionPointRec(mouse, area) {
			t.scrub.dragging = true
		}
		if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
			t.scrub.dragging = false
		}
		if t.scrub.dragging && t.scrub.hover >= 0 {
			t.target = t.scrub.markY(t.scrub.hover) - 8
		}
	}

	if !t.dated() && rl.IsMouseButtonPressed(rl.MouseButtonLeft) && maxS > 0 && rl.CheckCollisionPointRec(mouse, t.track(v)) {
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
	if inside && !t.dragging && !t.scrub.dragging && !rl.CheckCollisionPointRec(mouse, t.track(v)) && !rl.CheckCollisionPointRec(mouse, t.Obstacle) {
		local := rl.NewVector2(mouse.X-v.X, mouse.Y-v.Y+t.scroll)
		first, last := t.visible(t.scroll, t.scroll+v.Height)
		for i := first; i < last; i++ {
			if rl.CheckCollisionPointRec(local, t.tiles[i].rect) {
				t.hover = i
				break
			}
		}
	}

	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		t.armed = t.hover
		t.pressY = mouse.Y + t.scroll
	}
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		moved := math.Abs(float64(mouse.Y+t.scroll-t.pressY)) > 6
		if t.armed >= 0 && t.armed == t.hover && !moved && t.OnOpen != nil {
			p := t.tiles[t.armed].photo
			for i, q := range t.photos {
				if q == p {
					t.OnOpen(t.photos, i)
					break
				}
			}
		}
		t.armed = -1
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
			msg = "No photos found"
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
		x := v.X + h.rect.X
		y := v.Y + h.rect.Y - t.scroll + h.rect.Height - 30
		cw := MeasureText(th.Regular, h.count, 13)
		DrawText(th.Regular, h.count, x+h.rect.Width-cw, y+2, 13, th.TextMuted)
		room := h.rect.Width - cw - 16
		text := FitText(th.Medium, h.text, 16, room)
		DrawText(th.Medium, text, x, y, 16, th.Text)
		if h.sub != "" {
			tw := MeasureText(th.Medium, text, 16) + 10
			DrawText(th.Regular, FitText(th.Regular, h.sub, 14, room-tw), x+tw, y+1.5, 14, th.TextMuted)
		}
	}

	first, last := t.visible(top, bottom)
	for i := first; i < last; i++ {
		tl := t.tiles[i]
		r := rl.NewRectangle(v.X+tl.rect.X, v.Y+tl.rect.Y-t.scroll, tl.rect.Width, tl.rect.Height)
		if tex, ok := t.thumbs.Get(tl.photo); ok {
			DrawCoverRounded(tex, r, tileRadius, rl.White)
		} else {
			DrawRoundedRect(r, tileRadius, tl.photo.Color)
		}
		if i == t.hover {
			DrawRoundedRect(r, tileRadius, rl.Fade(rl.White, 0.08))
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

	if t.dated() {
		t.scrub.draw(th, t.scrubArea(v), t.activeMonth())
	} else if t.maxScroll(v) > 0 {
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

var weekdays = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
var months = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func dayDiff(d, now time.Time) int {
	y, m, day := d.Date()
	ny, nm, nd := now.Date()
	a := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	b := time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)
	return int(math.Round(b.Sub(a).Hours() / 24))
}

func dayLabel(d, now time.Time) string {
	s := fmt.Sprintf("%s, %s %d", weekdays[d.Weekday()], months[d.Month()-1], d.Day())
	if d.Year() != now.Year() {
		s += fmt.Sprintf(", %d", d.Year())
	}
	return s
}

func formatDay(d, now time.Time) string {
	switch dayDiff(d, now) {
	case 0:
		return "Today"
	case 1:
		return "Yesterday"
	}
	return capitalize(dayLabel(d, now))
}

func daySub(d, now time.Time) string {
	if n := dayDiff(d, now); n == 0 || n == 1 {
		return dayLabel(d, now)
	}
	return ""
}

func capitalize(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}
