package ui

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type SelectionBar struct {
	Count int

	theme   *Theme
	bar     rl.Rectangle
	clear   rl.Rectangle
	hoverX  bool
	armedX  bool
	actions []action
}

func NewSelectionBar(theme *Theme) *SelectionBar {
	return &SelectionBar{theme: theme}
}

func (b *SelectionBar) SetActions(labels ...string) {
	if len(labels) == len(b.actions) {
		same := true
		for i, l := range labels {
			same = same && b.actions[i].Label == l
		}
		if same {
			return
		}
	}
	b.actions = b.actions[:0]
	for i, l := range labels {
		kind := ActionNeutral
		if i == 0 {
			kind = ActionPrimary
		}
		b.actions = append(b.actions, action{Label: l, Kind: kind})
	}
}

func (b *SelectionBar) Hovered() bool {
	if b.hoverX {
		return true
	}
	for _, a := range b.actions {
		if a.hover {
			return true
		}
	}
	return false
}

func (b *SelectionBar) layout(bounds rl.Rectangle) {
	b.bar = rl.NewRectangle(bounds.X+12, bounds.Y+6, bounds.Width-24, TimelineBar-12)
	cy := b.bar.Y + b.bar.Height/2
	b.clear = rl.NewRectangle(b.bar.X+8, cy-16, 32, 32)
	x := b.bar.X + b.bar.Width - 8
	for i := len(b.actions) - 1; i >= 0; i-- {
		w := b.actions[i].width(b.theme)
		x -= w
		b.actions[i].Rect = rl.NewRectangle(x, cy-17, w, 34)
		x -= 8
	}
}

func (b *SelectionBar) Update(bounds rl.Rectangle) (int, bool) {
	b.layout(bounds)
	mouse := rl.GetMousePosition()
	b.hoverX = rl.CheckCollisionPointRec(mouse, b.clear)
	cleared := false
	if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		b.armedX = b.hoverX
	}
	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		cleared = b.armedX && b.hoverX
		b.armedX = false
	}
	clicked := -1
	for i := range b.actions {
		if b.actions[i].update() {
			clicked = i
		}
	}
	return clicked, cleared
}

func (b *SelectionBar) Draw(bounds rl.Rectangle) {
	t := b.theme
	b.layout(bounds)
	rl.DrawRectangleRec(bounds, t.Background)
	DrawRoundedRect(b.bar, b.bar.Height/2, t.Surface)
	c := rl.NewVector2(b.clear.X+b.clear.Width/2, b.clear.Y+b.clear.Height/2)
	if b.hoverX {
		rl.DrawCircleV(c, 16, t.Raised)
	}
	closeIcon(c, 5.5, t.Text)
	label := "1 selected"
	if b.Count != 1 {
		label = fmt.Sprintf("%d selected", b.Count)
	}
	DrawText(t.Medium, label, b.clear.X+b.clear.Width+10, c.Y-9, 17, t.Text)
	for i := range b.actions {
		b.actions[i].draw(t)
	}
}
