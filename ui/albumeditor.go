package ui

import (
	"fmt"
	"unicode/utf8"

	"ariranha/gallery"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	edWidth  = 460
	edPad    = 24
	edInputH = 42
	edSwatch = 30
	edGap    = 10
)

type AlbumEditor struct {
	Open     bool
	OnSave   func(id int64, name, description string, color int) error
	OnDelete func(id int64) error

	theme    *Theme
	id       int64
	color    int
	name     *TextInput
	desc     *TextInput
	err      string
	confirm  bool
	card     rl.Rectangle
	swatches []rl.Rectangle
	hoverSw  int
	save     action
	cancel   action
	del      action
	fade     float32
}

func NewAlbumEditor(theme *Theme, maxName, maxDesc int) *AlbumEditor {
	e := &AlbumEditor{
		theme:  theme,
		name:   &TextInput{theme: theme, Placeholder: "Album name", Plain: true, MaxLen: maxName},
		desc:   &TextInput{theme: theme, Placeholder: "Description (optional)", Plain: true, MaxLen: maxDesc},
		save:   action{Label: "Save", Kind: ActionPrimary},
		cancel: action{Label: "Cancel"},
		del:    action{Label: "Delete album", Kind: ActionDanger},
	}
	e.name.OnSubmit = func(string) { e.submit() }
	e.desc.OnSubmit = func(string) { e.submit() }
	return e
}

func (e *AlbumEditor) New(color int) {
	e.show(0, "", "", color)
	e.save.Label = "Create"
}

func (e *AlbumEditor) Edit(a *gallery.Album) {
	e.show(a.ID, a.Name, a.Description, a.Color)
	e.save.Label = "Save"
}

func (e *AlbumEditor) show(id int64, name, desc string, color int) {
	e.Open = true
	e.id = id
	e.color = color
	e.name.Text = name
	e.desc.Text = desc
	e.name.Focused = true
	e.desc.Focused = false
	e.err = ""
	e.confirm = false
	e.del.Label = "Delete album"
	e.fade = 0
}

func (e *AlbumEditor) close() {
	e.Open = false
	e.name.Focused = false
	e.desc.Focused = false
}

func (e *AlbumEditor) submit() {
	if e.OnSave == nil {
		return
	}
	if err := e.OnSave(e.id, e.name.Text, e.desc.Text, e.color); err != nil {
		e.err = err.Error()
		return
	}
	e.close()
}

func (e *AlbumEditor) Hovered() bool {
	return e.Open && (e.save.hover || e.cancel.hover || (e.id != 0 && e.del.hover) || e.hoverSw >= 0)
}

func (e *AlbumEditor) InputHovered() bool {
	return e.Open && (e.name.Hovered() || e.desc.Hovered())
}

func (e *AlbumEditor) layout(screen rl.Rectangle) {
	h := float32(edPad + 30 + 3*(22+edInputH+16) + 24 + 40 + edPad)
	e.card = centered(screen, edWidth, h)
	x := e.card.X + edPad
	w := e.card.Width - edPad*2
	y := e.card.Y + edPad + 30 + 22
	e.name.Bounds = rl.NewRectangle(x, y, w, edInputH)
	y += edInputH + 16 + 22
	e.desc.Bounds = rl.NewRectangle(x, y, w, edInputH)
	y += edInputH + 16 + 22

	e.swatches = e.swatches[:0]
	for i := range AlbumColors() {
		sx := x + float32(i)*(edSwatch+edGap)
		e.swatches = append(e.swatches, rl.NewRectangle(sx, y+(edInputH-edSwatch)/2, edSwatch, edSwatch))
	}

	by := e.card.Y + e.card.Height - edPad - 40
	e.save.Rect = rl.NewRectangle(e.card.X+e.card.Width-edPad-e.save.width(e.theme), by, e.save.width(e.theme), 40)
	e.cancel.Rect = rl.NewRectangle(e.save.Rect.X-10-e.cancel.width(e.theme), by, e.cancel.width(e.theme), 40)
	e.del.Rect = rl.NewRectangle(x, by, e.del.width(e.theme), 40)
}

func (e *AlbumEditor) Update(screen rl.Rectangle) {
	if !e.Open {
		return
	}
	e.fade = smooth(e.fade, 1)
	e.layout(screen)

	if rl.IsKeyPressed(rl.KeyEscape) {
		e.close()
		return
	}
	if rl.IsKeyPressed(rl.KeyTab) {
		e.name.Focused, e.desc.Focused = !e.name.Focused, e.name.Focused
	}

	e.name.Update()
	e.desc.Update()

	mouse := rl.GetMousePosition()
	e.hoverSw = -1
	for i, r := range e.swatches {
		if rl.CheckCollisionPointRec(mouse, rl.NewRectangle(r.X-4, r.Y-4, r.Width+8, r.Height+8)) {
			e.hoverSw = i
		}
	}
	if e.hoverSw >= 0 && rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		e.color = e.hoverSw
	}

	if e.cancel.update() {
		e.close()
		return
	}
	if e.save.update() {
		e.submit()
		return
	}
	if e.id != 0 && e.del.update() {
		if !e.confirm {
			e.confirm = true
			e.del.Label = "Click again to delete"
			e.layout(screen)
			return
		}
		if e.OnDelete != nil {
			if err := e.OnDelete(e.id); err != nil {
				e.err = err.Error()
				return
			}
		}
		e.close()
	}
}

func (e *AlbumEditor) Draw(screen rl.Rectangle) {
	if !e.Open {
		return
	}
	t := e.theme
	e.layout(screen)
	drawScrim(screen, e.fade)
	drawCard(t, e.card)

	x := e.card.X + edPad
	title := "New album"
	if e.id != 0 {
		title = "Edit album"
	}
	DrawText(t.Medium, title, x, e.card.Y+edPad, 20, t.Text)

	label := func(text string, input rl.Rectangle, count string) {
		DrawText(t.Medium, text, x, input.Y-21, 13, t.TextMuted)
		if count != "" {
			cw := MeasureText(t.Regular, count, 12)
			DrawText(t.Regular, count, input.X+input.Width-cw, input.Y-20, 12, t.TextMuted)
		}
	}
	label("Name", e.name.Bounds, counter(e.name))
	e.name.Draw()
	label("Description", e.desc.Bounds, counter(e.desc))
	e.desc.Draw()

	label("Color", e.swatches[0], "")
	for i, r := range e.swatches {
		c := rl.NewVector2(r.X+r.Width/2, r.Y+r.Height/2)
		col := t.AlbumColor(i)
		if i == e.color {
			rl.DrawRing(c, r.Width/2+2, r.Width/2+4, 0, 360, 48, col)
		} else if i == e.hoverSw {
			rl.DrawRing(c, r.Width/2+2, r.Width/2+3.5, 0, 360, 48, rl.Fade(col, 0.5))
		}
		rl.DrawCircleV(c, r.Width/2-1, col)
		if i == e.color {
			checkIcon(c, 6, t.Surface)
		}
	}

	if e.err != "" {
		y := e.swatches[0].Y + edSwatch + 14
		DrawText(t.Regular, FitText(t.Regular, e.err, 13, e.card.Width-edPad*2), x, y, 13, t.Favorite)
	}

	if e.id != 0 {
		e.del.draw(t)
	}
	e.cancel.draw(t)
	e.save.draw(t)
}

func counter(in *TextInput) string {
	if !in.Focused || in.MaxLen <= 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", utf8.RuneCountInString(in.Text), in.MaxLen)
}
