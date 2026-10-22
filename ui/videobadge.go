package ui

import (
	"ariranha/gallery"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawVideoBadge(t *Theme, r rl.Rectangle, p *gallery.Photo) {
	if r.Width < 48 || r.Height < 32 {
		return
	}
	label := gallery.FormatDuration(p.Duration)
	const size, pad, icon, gap = 12, 7, 7, 5
	tw := MeasureText(t.Medium, label, size)
	w := pad + icon + gap + tw + pad
	h := float32(20)
	box := rl.NewRectangle(r.X+r.Width-w-8, r.Y+8, w, h)
	DrawRoundedRect(box, h/2, rl.Fade(rl.Black, 0.55))

	cx := box.X + pad
	cy := box.Y + h/2
	rl.DrawTriangle(
		rl.NewVector2(cx, cy-icon/2-0.5),
		rl.NewVector2(cx, cy+icon/2+0.5),
		rl.NewVector2(cx+icon, cy),
		rl.White,
	)
	DrawText(t.Medium, label, cx+icon+gap, cy-size/2-0.5, size, rl.White)
}
