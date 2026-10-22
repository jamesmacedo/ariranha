package ui

import rl "github.com/gen2brain/raylib-go/raylib"

const (
	ActionNeutral = iota
	ActionPrimary
	ActionDanger
)

type action struct {
	Label string
	Kind  int
	Rect  rl.Rectangle
	hover bool
	armed bool
}

func (a *action) width(t *Theme) float32 {
	return MeasureText(t.Medium, a.Label, 14) + 36
}

func (a *action) update() bool {
	mouse := rl.GetMousePosition()
	a.hover = rl.CheckCollisionPointRec(mouse, a.Rect)
	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		a.armed = a.hover
	}
	clicked := false
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		clicked = a.armed && a.hover
		a.armed = false
	}
	return clicked
}

func (a *action) draw(t *Theme) {
	bg, fg := t.Raised, t.Text
	switch a.Kind {
	case ActionPrimary:
		bg, fg = t.Accent, t.OnAccent
	case ActionDanger:
		bg, fg = rl.Fade(t.Favorite, 0.14), t.Favorite
	}
	if a.hover {
		bg = rl.ColorBrightness(bg, 0.1)
		if a.Kind == ActionNeutral {
			bg = t.RaisedHover
		}
	}
	rl.DrawRectangleRounded(a.Rect, 1, 24, bg)
	tw := MeasureText(t.Medium, a.Label, 14)
	DrawText(t.Medium, a.Label, a.Rect.X+(a.Rect.Width-tw)/2, a.Rect.Y+(a.Rect.Height-14)/2-1, 14, fg)
}

func drawScrim(screen rl.Rectangle, alpha float32) {
	rl.DrawRectangleRec(screen, rl.Fade(rl.Black, 0.55*alpha))
}

func drawCard(t *Theme, r rl.Rectangle) {
	for i := 1; i <= 4; i++ {
		f := float32(i) * 4
		DrawRoundedRect(rl.NewRectangle(r.X-f, r.Y-f+6, r.Width+f*2, r.Height+f*2), 18+f, rl.Fade(rl.Black, 0.12/float32(i)))
	}
	DrawRoundedRect(r, 18, t.Surface)
}

func centered(screen rl.Rectangle, w, h float32) rl.Rectangle {
	w = min(w, screen.Width-32)
	h = min(h, screen.Height-32)
	return rl.NewRectangle(screen.X+(screen.Width-w)/2, screen.Y+(screen.Height-h)/2, w, h)
}

func plusIcon(c rl.Vector2, size float32, col rl.Color) {
	rl.DrawRectangleRounded(rl.NewRectangle(c.X-size, c.Y-0.9, size*2, 1.8), 1, 4, col)
	rl.DrawRectangleRounded(rl.NewRectangle(c.X-0.9, c.Y-size, 1.8, size*2), 1, 4, col)
}

func closeIcon(c rl.Vector2, size float32, col rl.Color) {
	rl.DrawLineEx(rl.NewVector2(c.X-size, c.Y-size), rl.NewVector2(c.X+size, c.Y+size), 1.8, col)
	rl.DrawLineEx(rl.NewVector2(c.X-size, c.Y+size), rl.NewVector2(c.X+size, c.Y-size), 1.8, col)
}

func checkIcon(c rl.Vector2, size float32, col rl.Color) {
	polyline([]rl.Vector2{{X: c.X - size, Y: c.Y}, {X: c.X - size*0.3, Y: c.Y + size*0.7}, {X: c.X + size, Y: c.Y - size*0.6}}, 2, col)
}

type Toast struct {
	text string
	left float32
}

func (t *Toast) Show(text string) {
	t.text = text
	t.left = 2.5
}

func (t *Toast) Update() {
	t.left = max(0, t.left-rl.GetFrameTime())
}

func (t *Toast) Draw(th *Theme, above rl.Rectangle) {
	if t.left <= 0 || t.text == "" {
		return
	}
	a := min(1, t.left*3)
	w := MeasureText(th.Regular, t.text, 13) + 28
	r := rl.NewRectangle(above.X+(above.Width-w)/2, above.Y-44, w, 32)
	rl.DrawRectangleRounded(r, 1, 24, rl.Fade(th.Raised, a))
	DrawText(th.Regular, t.text, r.X+14, r.Y+(r.Height-13)/2-1, 13, rl.Fade(th.Text, a))
}
