package ui

import rl "github.com/gen2brain/raylib-go/raylib"

type ButtonState int

const (
	StateNormal ButtonState = iota
	StateHover
	StatePressed
	StateDisabled
)

type ButtonStyle struct {
	Background [4]rl.Color
	Text       [4]rl.Color
	Border     rl.Color
	BorderSize float32
	FontSize   int32
	Roundness  float32
}

func DefaultButtonStyle() ButtonStyle {
	return ButtonStyle{
		Background: [4]rl.Color{
			StateNormal:   rl.NewColor(60, 90, 140, 255),
			StateHover:    rl.NewColor(80, 115, 170, 255),
			StatePressed:  rl.NewColor(40, 65, 105, 255),
			StateDisabled: rl.NewColor(150, 150, 150, 255),
		},
		Text: [4]rl.Color{
			StateNormal:   rl.White,
			StateHover:    rl.White,
			StatePressed:  rl.LightGray,
			StateDisabled: rl.NewColor(210, 210, 210, 255),
		},
		Border:     rl.NewColor(25, 40, 70, 255),
		BorderSize: 2,
		FontSize:   20,
		Roundness:  0.3,
	}
}

type Button struct {
	Bounds   rl.Rectangle
	Label    string
	Style    ButtonStyle
	Disabled bool
	Font     *rl.Font
	OnClick  func()

	state   ButtonState
	armed   bool
	clicked bool
}

func NewButton(x, y, w, h float32, label string, onClick func()) *Button {
	return &Button{
		Bounds:  rl.NewRectangle(x, y, w, h),
		Label:   label,
		Style:   DefaultButtonStyle(),
		OnClick: onClick,
	}
}

func (b *Button) Update() bool {
	b.clicked = false

	if b.Disabled {
		b.state = StateDisabled
		b.armed = false
		return false
	}

	hover := rl.CheckCollisionPointRec(rl.GetMousePosition(), b.Bounds)

	if hover && rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		b.armed = true
	}

	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		if b.armed && hover {
			b.clicked = true
			if b.OnClick != nil {
				b.OnClick()
			}
		}
		b.armed = false
	}

	switch {
	case b.armed && hover:
		b.state = StatePressed
	case hover:
		b.state = StateHover
	default:
		b.state = StateNormal
	}

	return b.clicked
}

func (b *Button) Draw() {
	s := b.Style
	rl.DrawRectangleRounded(b.Bounds, s.Roundness, 8, s.Background[b.state])
	if s.BorderSize > 0 {
		rl.DrawRectangleRoundedLinesEx(b.Bounds, s.Roundness, 8, s.BorderSize, s.Border)
	}

	if b.Font != nil {
		size := float32(s.FontSize)
		m := rl.MeasureTextEx(*b.Font, b.Label, size, 0)
		x := b.Bounds.X + (b.Bounds.Width-m.X)/2
		y := b.Bounds.Y + (b.Bounds.Height-m.Y)/2
		if b.state == StatePressed {
			y += 2
		}
		DrawText(*b.Font, b.Label, x, y, size, s.Text[b.state])
		return
	}

	tw := rl.MeasureText(b.Label, s.FontSize)
	tx := int32(b.Bounds.X+b.Bounds.Width/2) - tw/2
	ty := int32(b.Bounds.Y+b.Bounds.Height/2) - s.FontSize/2
	if b.state == StatePressed {
		ty += 2
	}
	rl.DrawText(b.Label, tx, ty, s.FontSize, s.Text[b.state])
}

func (b *Button) State() ButtonState {
	return b.state
}

func (b *Button) Clicked() bool {
	return b.clicked
}

func (b *Button) Hovered() bool {
	return b.state == StateHover || b.state == StatePressed
}
