package ui

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type TextInput struct {
	Bounds      rl.Rectangle
	Text        string
	Placeholder string
	Hint        string
	Focused     bool
	OnSubmit    func(string)

	theme *Theme
	blink float32
	hover bool
}

func NewTextInput(theme *Theme, placeholder string) *TextInput {
	return &TextInput{theme: theme, Placeholder: placeholder}
}

func (t *TextInput) Hovered() bool {
	return t.hover
}

func (t *TextInput) Update() {
	mouse := rl.GetMousePosition()
	t.hover = rl.CheckCollisionPointRec(mouse, t.Bounds)

	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		t.Focused = t.hover
	}
	ctrl := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)
	if ctrl && (rl.IsKeyPressed(rl.KeyK) || rl.IsKeyPressed(rl.KeyF)) {
		t.Focused = true
	}

	if !t.Focused {
		return
	}
	t.blink += rl.GetFrameTime()

	for r := rl.GetCharPressed(); r > 0; r = rl.GetCharPressed() {
		if r >= 32 {
			t.Text += string(r)
			t.blink = 0
		}
	}

	if rl.IsKeyPressed(rl.KeyBackspace) || rl.IsKeyPressedRepeat(rl.KeyBackspace) {
		runes := []rune(t.Text)
		if len(runes) > 0 {
			t.Text = string(runes[:len(runes)-1])
		}
		t.blink = 0
	}

	switch {
	case rl.IsKeyPressed(rl.KeyEnter), rl.IsKeyPressed(rl.KeyKpEnter):
		t.submit()
	case rl.IsKeyPressed(rl.KeyEscape):
		if t.Text != "" {
			t.Text = ""
			t.submit()
		} else {
			t.Focused = false
		}
	}
}

func (t *TextInput) submit() {
	if t.OnSubmit != nil {
		t.OnSubmit(t.Text)
	}
}

func (t *TextInput) Draw() {
	th := t.theme
	b := t.Bounds

	bg := th.Raised
	if t.hover || t.Focused {
		bg = th.RaisedHover
	}
	rl.DrawRectangleRounded(b, 1, 24, rl.Fade(bg, 0.85))
	if t.Focused {
		rl.DrawRectangleRoundedLinesEx(b, 1, 24, 1.5, th.Accent)
	}

	cy := b.Y + b.Height/2
	icon := rl.NewVector2(b.X+19, cy-1.5)
	ic := th.Text
	if !t.Focused && t.Text == "" {
		ic = rl.Fade(th.Text, 0.8)
	}
	rl.DrawRing(icon, 5.2, 6.8, 0, 360, 32, ic)
	rl.DrawLineEx(rl.NewVector2(icon.X+4.8, icon.Y+4.8), rl.NewVector2(icon.X+8.5, icon.Y+8.5), 1.8, ic)

	const size = 15
	left := b.X + 36
	inner := b.Width - 36 - 14

	if t.Hint != "" && !t.Focused && t.Text == "" {
		hw := MeasureText(th.Medium, t.Hint, 12) + 12
		chip := rl.NewRectangle(b.X+b.Width-hw-8, cy-11, hw, 22)
		if chip.X > left+40 {
			rl.DrawRectangleRounded(chip, 0.35, 8, th.Surface)
			DrawText(th.Medium, t.Hint, chip.X+6, cy-7, 12, th.Text)
			inner = chip.X - left - 8
		}
	}

	if t.Text == "" {
		DrawText(th.Regular, FitText(th.Regular, t.Placeholder, size, inner), left, cy-size/2-1, size, ic)
	} else {
		w := MeasureText(th.Regular, t.Text, size)
		x := left + min(0, inner-w)
		rl.BeginScissorMode(int32(left), int32(b.Y), int32(inner+2), int32(b.Height))
		DrawText(th.Regular, t.Text, x, cy-size/2-1, size, th.Text)
		rl.EndScissorMode()
	}

	if t.Focused && math.Mod(float64(t.blink), 1) < 0.5 {
		cx := left + min(MeasureText(th.Regular, t.Text, size), inner) + 1
		rl.DrawRectangle(int32(cx), int32(cy-8), 2, 16, th.Accent)
	}
}
