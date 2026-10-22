package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"time"

	"ariranha/gallery"
	"ariranha/search"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	vwPad       = 12
	vwStripH    = 64
	vwSideW     = 300
	vwSideMin   = 980
	vwThumb     = 44
	vwThumbOn   = 52
	vwThumbGap  = 6
	vwBarH      = 52
	vwBtn       = 36
	vwRadius    = 16
	vwSlide     = 48
	vwPrefetch  = 3
	vwFullSide  = 2048
	vwFullGPU   = 8
	vwFullRAM   = 256 << 20
	vwToastTime = 2.5
)

const (
	vbNone = iota
	vbBack
	vbPrev
	vbNext
	vbArrowL
	vbArrowR
	vbFav
	vbTrash
	vbClose
	vbThumb
)

type vbutton struct {
	id    int
	index int
	rect  rl.Rectangle
}

type Viewer struct {
	Open       bool
	OnFavorite func(*gallery.Photo, bool) error
	OnDelete   func(*gallery.Photo) error
	OnClose    func(*gallery.Photo)
	PeopleOf   func(*gallery.Photo) []search.PersonInfo
	Avatars    *PeopleStrip

	theme  *Theme
	thumbs *gallery.ThumbCache
	full   *gallery.ThumbCache
	photos []*gallery.Photo
	index  int
	dir    int

	people    []search.PersonInfo
	peopleFor *gallery.Photo

	fullTex   rl.Texture2D
	hasFull   bool
	fullFor   *gallery.Photo
	fullAlpha float32
	offset    float32
	strip     float32
	stripSet  bool
	pop       float32
	toast     string
	toastLeft float32

	buttons []vbutton
	hover   vbutton
	armed   vbutton
	stripR  rl.Rectangle
	imageR  rl.Rectangle
	sideR   rl.Rectangle
	barR    rl.Rectangle
	photoR  rl.Rectangle
}

func NewViewer(theme *Theme, thumbs *gallery.ThumbCache) *Viewer {
	full := gallery.NewThumbCache(vwFullSide, vwFullGPU, vwFullRAM)
	full.SetUploadLimit(1)
	return &Viewer{theme: theme, thumbs: thumbs, full: full}
}

func (v *Viewer) Unload() {
	v.full.Close()
}

func (v *Viewer) Show(photos []*gallery.Photo, index int) {
	if len(photos) == 0 {
		return
	}
	v.photos = photos
	v.index = clampInt(index, 0, len(photos)-1)
	v.Open = true
	v.stripSet = false
	v.offset = 0
	v.fullAlpha = 0
	v.fullFor = nil
	v.peopleFor = nil
	v.hover, v.armed = vbutton{}, vbutton{}
}

func (v *Viewer) Current() *gallery.Photo {
	if !v.Open || len(v.photos) == 0 {
		return nil
	}
	return v.photos[v.index]
}

func (v *Viewer) Hovered() bool {
	return v.hover.id != vbNone
}

func (v *Viewer) close() {
	p := v.Current()
	v.Open = false
	if v.OnClose != nil && p != nil {
		v.OnClose(p)
	}
}

func (v *Viewer) jump(i int) {
	if i < 0 || i >= len(v.photos) || i == v.index {
		return
	}
	v.dir = 1
	if i < v.index {
		v.dir = -1
	}
	v.index = i
	v.offset = float32(v.dir) * vwSlide
}

func (v *Viewer) notify(msg string) {
	v.toast = msg
	v.toastLeft = vwToastTime
}

func (v *Viewer) toggleFavorite() {
	p := v.Current()
	if p == nil || v.OnFavorite == nil {
		return
	}
	if err := v.OnFavorite(p, !p.Favorite); err != nil {
		v.notify("Error: " + err.Error())
		return
	}
	if p.Favorite {
		v.pop = 10
		v.notify("Added to favorites")
	} else {
		v.notify("Removed from favorites")
	}
}

func (v *Viewer) trash() {
	p := v.Current()
	if p == nil || v.OnDelete == nil {
		return
	}
	if err := v.OnDelete(p); err != nil {
		v.notify("Error: " + err.Error())
		return
	}
	v.photos = append(append([]*gallery.Photo(nil), v.photos[:v.index]...), v.photos[v.index+1:]...)
	if len(v.photos) == 0 {
		v.Open = false
		return
	}
	v.index = min(v.index, len(v.photos)-1)
	v.offset = vwSlide
	v.fullFor = nil
	v.notify("Moved to system/trash")
}

func (v *Viewer) layout(screen rl.Rectangle) {
	side := float32(0)
	if screen.Width >= vwSideMin {
		side = vwSideW + vwPad
	}
	left := rl.NewRectangle(screen.X+vwPad, screen.Y+vwPad, screen.Width-vwPad*2-side, screen.Height-vwPad*2)
	v.stripR = rl.NewRectangle(left.X, left.Y, left.Width, vwStripH)
	v.imageR = rl.NewRectangle(left.X, left.Y+vwStripH+vwPad, left.Width, left.Height-vwStripH-vwPad)
	v.sideR = rl.Rectangle{}
	if side > 0 {
		v.sideR = rl.NewRectangle(left.X+left.Width+vwPad, left.Y, vwSideW, left.Height)
	}

	inner := rl.NewRectangle(v.imageR.X+vwPad, v.imageR.Y+vwPad, v.imageR.Width-vwPad*2, v.imageR.Height-vwPad*2)
	aspect := v.Current().Aspect()
	if v.hasFull && v.fullTex.Height > 0 {
		aspect = float32(v.fullTex.Width) / float32(v.fullTex.Height)
	}
	w, h := inner.Width, inner.Width/aspect
	if h > inner.Height {
		h, w = inner.Height, inner.Height*aspect
	}
	v.photoR = rl.NewRectangle(inner.X+(inner.Width-w)/2, inner.Y+(inner.Height-h)/2, w, h)

	v.buttons = v.buttons[:0]
	add := func(id, index int, r rl.Rectangle) {
		v.buttons = append(v.buttons, vbutton{id: id, index: index, rect: r})
	}

	cy := v.stripR.Y + v.stripR.Height/2
	center := v.stripR.X + v.stripR.Width/2
	for i := range v.photos {
		x := center + float32(i)*(vwThumb+vwThumbGap) - v.strip - vwThumb/2
		if x+vwThumbOn < v.stripR.X || x > v.stripR.X+v.stripR.Width {
			continue
		}
		size := float32(vwThumb)
		if i == v.index {
			size = vwThumbOn
		}
		add(vbThumb, i, rl.NewRectangle(x+(vwThumb-size)/2, cy-size/2, size, size))
	}

	add(vbBack, 0, rl.NewRectangle(v.imageR.X+10, v.imageR.Y+10, 30, 30))
	if v.index > 0 {
		add(vbArrowL, 0, rl.NewRectangle(v.imageR.X+10, v.imageR.Y+v.imageR.Height/2-16, 32, 32))
	}
	if v.index < len(v.photos)-1 {
		add(vbArrowR, 0, rl.NewRectangle(v.imageR.X+v.imageR.Width-42, v.imageR.Y+v.imageR.Height/2-16, 32, 32))
	}

	t := v.theme
	counter := v.counterWidth()
	closeW := MeasureText(t.Medium, "Close", 14) + MeasureText(t.Medium, "Esc", 11) + 12 + 30
	barW := float32(8) + vwBtn + counter + vwBtn + 17 + vwBtn*2 + 4 + 17 + closeW + 8
	v.barR = rl.NewRectangle(v.imageR.X+(v.imageR.Width-barW)/2, v.imageR.Y+v.imageR.Height-vwBarH-14, barW, vwBarH)
	by := v.barR.Y + (vwBarH-vwBtn)/2
	x := v.barR.X + 8
	add(vbPrev, 0, rl.NewRectangle(x, by, vwBtn, vwBtn))
	x += vwBtn + counter
	add(vbNext, 0, rl.NewRectangle(x, by, vwBtn, vwBtn))
	x += vwBtn + 17
	add(vbFav, 0, rl.NewRectangle(x, by, vwBtn, vwBtn))
	x += vwBtn + 4
	add(vbTrash, 0, rl.NewRectangle(x, by, vwBtn, vwBtn))
	x += vwBtn + 17
	add(vbClose, 0, rl.NewRectangle(x, by, closeW, vwBtn))
}

func (v *Viewer) counterWidth() float32 {
	t := v.theme
	return MeasureText(t.Medium, fmt.Sprint(v.index+1), 14) + MeasureText(t.Regular, fmt.Sprintf(" / %d", len(v.photos)), 14) + 12
}

func (v *Viewer) Update(screen rl.Rectangle) {
	v.full.BeginFrame()
	if !v.Open || len(v.photos) == 0 {
		v.Open = false
		return
	}
	dt := rl.GetFrameTime()

	switch {
	case rl.IsKeyPressed(rl.KeyEscape):
		v.close()
		return
	case rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressedRepeat(rl.KeyLeft):
		v.jump(v.index - 1)
	case rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressedRepeat(rl.KeyRight):
		v.jump(v.index + 1)
	case rl.IsKeyPressed(rl.KeyHome):
		v.jump(0)
	case rl.IsKeyPressed(rl.KeyEnd):
		v.jump(len(v.photos) - 1)
	case rl.IsKeyPressed(rl.KeyF):
		v.toggleFavorite()
	case rl.IsKeyPressed(rl.KeyDelete):
		v.trash()
		if !v.Open {
			return
		}
	}

	cur := v.Current()
	if v.fullFor != cur {
		v.fullFor = cur
		v.fullAlpha = 0
	}
	v.fullTex, v.hasFull = v.full.Get(cur)
	if v.hasFull {
		v.fullAlpha = min(1, v.fullAlpha+dt*6)
	}
	v.thumbs.Prefetch(cur)
	dir := v.dir
	if dir == 0 {
		dir = 1
	}
	for d := 1; d <= vwPrefetch; d++ {
		for _, i := range [2]int{v.index + dir*d, v.index - dir*d} {
			if i >= 0 && i < len(v.photos) {
				v.full.Prefetch(v.photos[i])
				v.thumbs.Prefetch(v.photos[i])
			}
		}
	}

	if v.peopleFor != cur {
		v.peopleFor = cur
		v.people = nil
		if v.PeopleOf != nil {
			v.people = v.PeopleOf(cur)
		}
	}

	target := float32(v.index) * (vwThumb + vwThumbGap)
	if !v.stripSet {
		v.strip, v.stripSet = target, true
	}
	v.strip = smooth(v.strip, target)
	if rl.CheckCollisionPointRec(rl.GetMousePosition(), v.stripR) {
		if w := rl.GetMouseWheelMove(); w != 0 {
			v.jump(clampInt(v.index-int(w), 0, len(v.photos)-1))
		}
	}
	v.offset = smooth(v.offset, 0)
	v.pop = smooth(v.pop, 0)
	v.toastLeft = max(0, v.toastLeft-dt)

	v.layout(screen)

	mouse := rl.GetMousePosition()
	v.hover = vbutton{}
	for _, b := range v.buttons {
		if rl.CheckCollisionPointRec(mouse, b.rect) {
			v.hover = b
		}
	}
	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		v.armed = v.hover
	}
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		if v.armed.id != vbNone && v.armed == v.hover {
			v.click(v.armed)
		}
		v.armed = vbutton{}
	}
}

func (v *Viewer) click(b vbutton) {
	switch b.id {
	case vbBack, vbClose:
		v.close()
	case vbPrev, vbArrowL:
		v.jump(v.index - 1)
	case vbNext, vbArrowR:
		v.jump(v.index + 1)
	case vbFav:
		v.toggleFavorite()
	case vbTrash:
		v.trash()
	case vbThumb:
		v.jump(b.index)
	}
}

func (v *Viewer) Draw() {
	if !v.Open {
		return
	}
	t := v.theme
	rl.ClearBackground(t.Background)
	DrawRoundedRect(v.stripR, vwRadius, t.Surface)
	DrawRoundedRect(v.imageR, vwRadius, t.Surface)
	v.drawStrip()
	v.drawPhoto()
	v.drawChrome()
	if v.sideR.Width > 0 {
		v.drawSide()
	}
}

func (v *Viewer) drawStrip() {
	r := v.stripR
	rl.BeginScissorMode(int32(r.X+6), int32(r.Y), int32(r.Width-12), int32(r.Height))
	for _, b := range v.buttons {
		if b.id != vbThumb {
			continue
		}
		p := v.photos[b.index]
		on := b.index == v.index
		if on {
			DrawRoundedRect(rl.NewRectangle(b.rect.X-3, b.rect.Y-3, b.rect.Width+6, b.rect.Height+6), 12, v.theme.Accent)
		}
		if tex, ok := v.thumbs.Get(p); ok {
			tint := rl.White
			if !on && !(v.hover.id == vbThumb && v.hover.index == b.index) {
				tint = rl.Fade(rl.White, 0.72)
			}
			DrawCoverRounded(tex, b.rect, 10, tint)
		} else {
			DrawRoundedRect(b.rect, 10, p.Color)
		}
	}
	rl.EndScissorMode()
}

func (v *Viewer) drawPhoto() {
	cur := v.Current()
	alpha := clamp(1-float32(math.Abs(float64(v.offset)))/vwSlide, 0, 1)
	r := v.photoR
	r.X += v.offset
	rl.BeginScissorMode(int32(v.imageR.X), int32(v.imageR.Y), int32(v.imageR.Width), int32(v.imageR.Height))
	thumb, hasThumb := v.thumbs.Get(cur)
	switch {
	case hasThumb:
		DrawCoverRounded(thumb, r, 14, rl.Fade(rl.White, alpha))
	case !v.hasFull:
		DrawRoundedRect(r, 14, rl.Fade(cur.Color, alpha))
	}
	if v.hasFull {
		DrawCoverRounded(v.fullTex, r, 14, rl.Fade(rl.White, alpha*v.fullAlpha))
	}
	rl.EndScissorMode()
}

func (v *Viewer) chromeButton(id int) (rl.Rectangle, bool, bool) {
	for _, b := range v.buttons {
		if b.id == id {
			return b.rect, true, v.hover.id == id
		}
	}
	return rl.Rectangle{}, false, false
}

func (v *Viewer) drawChrome() {
	t := v.theme
	glass := rl.NewColor(10, 12, 15, 170)
	glassHot := rl.NewColor(30, 34, 40, 210)

	if r, ok, hot := v.chromeButton(vbBack); ok {
		c := glass
		if hot {
			c = glassHot
		}
		label := longDate(v.Current().Date)
		lw := MeasureText(t.Medium, label, 13)
		pill := rl.NewRectangle(r.X+r.Width+4, r.Y, lw+24, r.Height)
		rl.DrawRectangleRounded(pill, 1, 24, glass)
		DrawText(t.Medium, label, pill.X+12, pill.Y+(pill.Height-13)/2-1, 13, rl.White)
		rl.DrawCircleV(rl.NewVector2(r.X+r.Width/2, r.Y+r.Height/2), r.Width/2, c)
		chevron(r, -1, rl.White)
	}
	for _, id := range []int{vbArrowL, vbArrowR} {
		if r, ok, hot := v.chromeButton(id); ok {
			c := glass
			if hot {
				c = glassHot
			}
			rl.DrawCircleV(rl.NewVector2(r.X+r.Width/2, r.Y+r.Height/2), r.Width/2, c)
			d := -1
			if id == vbArrowR {
				d = 1
			}
			chevron(r, d, rl.White)
		}
	}
}

func (v *Viewer) DrawOverlay(bd *Backdrop) {
	if !v.Open {
		return
	}
	t := v.theme
	b := v.barR
	for i := 1; i <= 4; i++ {
		f := float32(i) * 3
		rl.DrawRectangleRounded(rl.NewRectangle(b.X-f, b.Y-f+4, b.Width+f*2, b.Height+f*2), 1, 32, rl.Fade(t.Shadow, 0.35/float32(i)))
	}
	bd.Glass(b, b.Height/2, t.Glass)
	rl.DrawRectangleRoundedLinesEx(b, 1, 32, 1, t.GlassEdge)

	fg := rl.Fade(t.Text, 0.85)
	icon := func(id int, draw func(c rl.Vector2, col rl.Color)) {
		r, ok, hot := v.chromeButton(id)
		if !ok {
			return
		}
		col := fg
		if hot {
			rl.DrawCircleV(rl.NewVector2(r.X+r.Width/2, r.Y+r.Height/2), r.Width/2, rl.Fade(t.Text, 0.1))
			col = t.Text
		}
		draw(rl.NewVector2(r.X+r.Width/2, r.Y+r.Height/2), col)
	}

	icon(vbPrev, func(c rl.Vector2, col rl.Color) {
		col = dimIf(col, v.index == 0)
		chevron(rl.NewRectangle(c.X-15, c.Y-15, 30, 30), -1, col)
	})
	icon(vbNext, func(c rl.Vector2, col rl.Color) {
		col = dimIf(col, v.index == len(v.photos)-1)
		chevron(rl.NewRectangle(c.X-15, c.Y-15, 30, 30), 1, col)
	})
	if r, ok, _ := v.chromeButton(vbPrev); ok {
		n := fmt.Sprint(v.index + 1)
		total := fmt.Sprintf(" / %d", len(v.photos))
		x := r.X + r.Width + 6
		y := b.Y + (b.Height-14)/2 - 1
		DrawText(t.Medium, n, x, y, 14, t.Text)
		DrawText(t.Regular, total, x+MeasureText(t.Medium, n, 14), y, 14, t.TextMuted)
	}

	if r, ok, _ := v.chromeButton(vbFav); ok {
		x := r.X - 9
		rl.DrawRectangle(int32(x), int32(b.Y+16), 1, int32(b.Height-32), t.Divider)
	}
	icon(vbFav, func(c rl.Vector2, col rl.Color) {
		s := 1 + v.pop/30
		if v.Current().Favorite {
			heart(c, 8.5*s, t.Favorite, true)
		} else {
			heart(c, 8.5*s, col, false)
		}
	})
	icon(vbTrash, func(c rl.Vector2, col rl.Color) {
		trashIcon(c, col)
	})

	if r, ok, hot := v.chromeButton(vbClose); ok {
		rl.DrawRectangle(int32(r.X-9), int32(b.Y+16), 1, int32(b.Height-32), t.Divider)
		bg := t.Accent
		if hot {
			bg = rl.ColorBrightness(t.Accent, 0.12)
		}
		rl.DrawRectangleRounded(r, 1, 24, bg)
		DrawText(t.Medium, "Close", r.X+14, r.Y+(r.Height-14)/2-1, 14, t.OnAccent)
		cw := MeasureText(t.Medium, "Esc", 11) + 12
		chip := rl.NewRectangle(r.X+r.Width-cw-6, r.Y+(r.Height-20)/2, cw, 20)
		rl.DrawRectangleRounded(chip, 0.4, 8, rl.Fade(t.OnAccent, 0.14))
		DrawText(t.Medium, "Esc", chip.X+6, chip.Y+4, 11, t.OnAccent)
	}

	if v.toastLeft > 0 && v.toast != "" {
		a := min(1, v.toastLeft*3)
		w := MeasureText(t.Regular, v.toast, 13) + 28
		r := rl.NewRectangle(b.X+(b.Width-w)/2, b.Y-44, w, 32)
		rl.DrawRectangleRounded(r, 1, 24, rl.Fade(t.Surface, a))
		DrawText(t.Regular, v.toast, r.X+14, r.Y+(r.Height-13)/2-1, 13, rl.Fade(t.Text, a))
	}
}

func (v *Viewer) drawSide() {
	t := v.theme
	p := v.Current()
	x := v.sideR.X
	w := v.sideR.Width

	details := rl.NewRectangle(x, v.sideR.Y, w, 60+3*54)
	DrawRoundedRect(details, vwRadius, t.Surface)
	panelHeader(t, details, "Details", func(c rl.Vector2, col rl.Color) {
		rl.DrawRing(c, 6.2, 7.6, 0, 360, 32, col)
		rl.DrawRectangleRec(rl.NewRectangle(c.X-0.8, c.Y-1, 1.6, 4.5), col)
		rl.DrawCircleV(rl.NewVector2(c.X, c.Y-3.6), 1, col)
	})

	local := p.Date.Local()
	_, off := local.Zone()
	zone := fmt.Sprintf("GMT%+d", off/3600)
	if off%3600 != 0 {
		zone = fmt.Sprintf("GMT%+d:%02d", off/3600, abs(off%3600/60))
	}
	mp := float64(p.Width*p.Height) / 1e6
	rows := []struct {
		icon       func(rl.Vector2, rl.Color)
		title, sub string
	}{
		{calendarIcon, longDate(local), local.Format("15:04") + " · " + zone},
		{func(c rl.Vector2, col rl.Color) { drawNavIcon(0, c.X-10, c.Y-10, col) }, filepath.Base(p.Path), fmt.Sprintf("%d × %d · %.0f MP · %s", p.Width, p.Height, mp, humanSize(p.Size))},
		{folderIcon, "On disk", "storage/" + p.Rel},
	}
	y := details.Y + 60
	for _, row := range rows {
		c := rl.NewVector2(x+30, y+18)
		row.icon(c, t.TextMuted)
		tw := w - 62 - 14
		DrawText(t.Medium, FitText(t.Medium, row.title, 14, tw), x+52, y+4, 14, t.Text)
		DrawText(t.Regular, FitText(t.Regular, row.sub, 12, tw), x+52, y+23, 12, t.TextMuted)
		y += 54
	}

	chips := v.chipLayout(x+14, w-28)
	ph := float32(60 + 44)
	if n := len(chips); n > 0 {
		ph = 60 + chips[n-1].Y + chips[n-1].Height + 16
	}
	people := rl.NewRectangle(x, details.Y+details.Height+vwPad, w, ph)
	DrawRoundedRect(people, vwRadius, t.Surface)
	panelHeader(t, people, "People", func(c rl.Vector2, col rl.Color) { drawNavIcon(1, c.X-10, c.Y-10, col) })
	if len(v.people) == 0 {
		DrawText(t.Regular, "No people in this photo", x+16, people.Y+62, 13, t.TextMuted)
		return
	}
	for i, pi := range v.people {
		r := chips[i]
		r.Y += people.Y + 56
		rl.DrawRectangleRounded(r, 1, 24, rl.Fade(t.People, 0.12))
		rl.DrawRectangleRoundedLinesEx(r, 1, 24, 1, rl.Fade(t.People, 0.6))
		ac := rl.NewVector2(r.X+17, r.Y+r.Height/2)
		if tex, ok := v.avatar(pi); ok {
			rl.DrawTexturePro(tex, rl.NewRectangle(0, 0, float32(tex.Width), float32(tex.Height)), rl.NewRectangle(ac.X-12, ac.Y-12, 24, 24), rl.Vector2{}, 0, rl.White)
		} else {
			rl.DrawCircleV(ac, 12, t.Raised)
		}
		DrawText(t.Medium, personLabel(pi), r.X+36, r.Y+(r.Height-13)/2-1, 13, t.Text)
	}
}

func (v *Viewer) avatar(p search.PersonInfo) (rl.Texture2D, bool) {
	if v.Avatars == nil {
		return rl.Texture2D{}, false
	}
	return v.Avatars.Avatar(p)
}

func (v *Viewer) chipLayout(x0, width float32) []rl.Rectangle {
	t := v.theme
	var out []rl.Rectangle
	x, y := float32(0), float32(0)
	for _, p := range v.people {
		w := MeasureText(t.Medium, personLabel(p), 13) + 50
		if x > 0 && x+w > width {
			x, y = 0, y+40
		}
		out = append(out, rl.NewRectangle(x0+x, y, w, 32))
		x += w + 8
	}
	return out
}

func personLabel(p search.PersonInfo) string {
	if p.Name != "" {
		return p.Name
	}
	return fmt.Sprintf("Person %d", p.ID)
}

func panelHeader(t *Theme, r rl.Rectangle, title string, icon func(rl.Vector2, rl.Color)) {
	c := rl.NewVector2(r.X+30, r.Y+30)
	rl.DrawCircleV(c, 15, t.Raised)
	icon(c, t.Accent)
	DrawText(t.Medium, title, r.X+54, r.Y+21, 15, t.Text)
}

func longDate(d time.Time) string {
	return fmt.Sprintf("%s, %s %d, %d · %s", weekdays[d.Weekday()], months[d.Month()-1], d.Day(), d.Year(), d.Format("15:04"))
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%d KB", n>>10)
	}
}

func dimIf(c rl.Color, dim bool) rl.Color {
	if dim {
		return rl.Fade(c, 0.3)
	}
	return c
}

func chevron(r rl.Rectangle, dir int, c rl.Color) {
	cx, cy := r.X+r.Width/2, r.Y+r.Height/2
	d := float32(dir) * 2.5
	polyline([]rl.Vector2{{X: cx - d, Y: cy - 5}, {X: cx + d, Y: cy}, {X: cx - d, Y: cy + 5}}, 1.9, c)
}

func heart(c rl.Vector2, size float32, col rl.Color, filled bool) {
	const n = 40
	pts := make([]rl.Vector2, n)
	for i := range n {
		a := float64(i) / n * 2 * math.Pi
		s := math.Sin(a)
		x := 16 * s * s * s
		y := 13*math.Cos(a) - 5*math.Cos(2*a) - 2*math.Cos(3*a) - math.Cos(4*a)
		pts[i] = rl.NewVector2(c.X+float32(x)*size/16, c.Y-float32(y)*size/16-size*0.08)
	}
	if filled {
		center := rl.NewVector2(c.X, c.Y)
		for i := range n {
			a, b := pts[i], pts[(i+1)%n]
			rl.DrawTriangle(center, a, b, col)
			rl.DrawTriangle(center, b, a, col)
		}
	}
	polyline(append(pts, pts[0]), 1.8, col)
}

func trashIcon(c rl.Vector2, col rl.Color) {
	x, y := c.X-10, c.Y-10
	p := func(px, py float32) rl.Vector2 { return rl.NewVector2(x+px, y+py) }
	polyline([]rl.Vector2{p(2.5, 5), p(17.5, 5)}, 1.8, col)
	polyline([]rl.Vector2{p(7.5, 5), p(7.5, 2.8), p(12.5, 2.8), p(12.5, 5)}, 1.8, col)
	polyline([]rl.Vector2{p(4.5, 5), p(5.5, 17.2), p(14.5, 17.2), p(15.5, 5)}, 1.8, col)
	polyline([]rl.Vector2{p(8.3, 8.5), p(8.3, 14)}, 1.6, col)
	polyline([]rl.Vector2{p(11.7, 8.5), p(11.7, 14)}, 1.6, col)
}

func calendarIcon(c rl.Vector2, col rl.Color) {
	x, y := c.X-10, c.Y-10
	rl.DrawRectangleRoundedLinesEx(rl.NewRectangle(x+2.5, y+3.5, 15, 14), 0.25, 8, 1.7, col)
	polyline([]rl.Vector2{{X: x + 2.5, Y: y + 8}, {X: x + 17.5, Y: y + 8}}, 1.6, col)
	polyline([]rl.Vector2{{X: x + 6.5, Y: y + 1.8}, {X: x + 6.5, Y: y + 5}}, 1.7, col)
	polyline([]rl.Vector2{{X: x + 13.5, Y: y + 1.8}, {X: x + 13.5, Y: y + 5}}, 1.7, col)
}

func folderIcon(c rl.Vector2, col rl.Color) {
	x, y := c.X-10, c.Y-10
	p := func(px, py float32) rl.Vector2 { return rl.NewVector2(x+px, y+py) }
	polyline([]rl.Vector2{p(2.5, 4), p(7.5, 4), p(9.5, 6.3), p(17.5, 6.3), p(17.5, 16.5), p(2.5, 16.5), p(2.5, 4)}, 1.7, col)
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
