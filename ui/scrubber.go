package ui

import (
	"fmt"
	"math"
	"strconv"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	scrubW      = 56
	scrubYearH  = 30
	scrubMonthH = 33
	scrubLabelH = 24
)

var monthShort = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
var monthLong = [...]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

type monthMark struct {
	year  int
	month time.Month
	y     float32
}

type scrubItem struct {
	year  bool
	mark  int
	y     float32
	h     float32
	label string
}

type scrubber struct {
	marks    []monthMark
	items    []scrubItem
	placedH  float32
	labels   bool
	hover    int
	dragging bool
}

func (s *scrubber) set(marks []monthMark) {
	s.marks = marks
	s.placedH = -1
}

func (s *scrubber) place(h float32) {
	s.placedH = h
	s.items = s.items[:0]
	if len(s.marks) == 0 {
		return
	}

	years := 1
	for i := 1; i < len(s.marks); i++ {
		if s.marks[i].year != s.marks[i-1].year {
			years++
		}
	}
	months := float32(len(s.marks))
	yearH, monthH := float32(scrubYearH), float32(scrubMonthH)
	if float32(years)*yearH+months*monthH > h {
		monthH = max(4, (h-float32(years)*yearH)/months)
	}
	s.labels = monthH >= scrubLabelH
	if total := float32(years)*yearH + months*monthH; total > h {
		k := h / total
		yearH *= k
		monthH *= k
	}

	var y float32
	for i, m := range s.marks {
		if i == 0 || m.year != s.marks[i-1].year {
			s.items = append(s.items, scrubItem{year: true, mark: i, y: y, h: yearH, label: strconv.Itoa(m.year)})
			y += yearH
		}
		s.items = append(s.items, scrubItem{mark: i, y: y, h: monthH, label: monthShort[m.month-1]})
		y += monthH
	}
}

func (s *scrubber) itemAt(y float32) int {
	if len(s.items) == 0 {
		return -1
	}
	best, dist := 0, float32(math.MaxFloat32)
	for i, it := range s.items {
		if d := float32(math.Abs(float64(it.y + it.h/2 - y))); d < dist {
			best, dist = i, d
		}
	}
	return best
}

func (s *scrubber) markY(item int) float32 {
	return s.marks[s.items[item].mark].y
}

func (s *scrubber) draw(th *Theme, area rl.Rectangle, active int) {
	cx := area.X + area.Width/2
	lastYear := float32(-100)
	for i, it := range s.items {
		y := area.Y + it.y
		if it.year {
			if y-lastYear < 16 {
				continue
			}
			lastYear = y
			c := th.Text
			if i == s.hover {
				c = th.Accent
			}
			w := MeasureText(th.Medium, it.label, 13)
			DrawText(th.Medium, it.label, cx-w/2, y+(it.h-13)/2, 13, c)
			continue
		}

		on := it.mark == active
		hot := i == s.hover
		if s.labels {
			font, c := th.Regular, th.TextMuted
			switch {
			case on:
				font, c = th.Medium, th.Accent
			case hot:
				c = th.Text
			}
			w := MeasureText(font, it.label, 13)
			DrawText(font, it.label, cx-w/2, y+3, 13, c)
			dy := y + min(it.h-5, 24)
			if on {
				rl.DrawCircleV(rl.NewVector2(cx, dy), 3.5, th.Accent)
			} else {
				rl.DrawCircleV(rl.NewVector2(cx, dy), 1.3, rl.Fade(th.TextMuted, 0.5))
			}
			continue
		}

		dy := y + it.h/2
		switch {
		case on:
			rl.DrawCircleV(rl.NewVector2(cx, dy), 3.5, th.Accent)
		case hot:
			rl.DrawCircleV(rl.NewVector2(cx, dy), 2.5, th.Text)
		default:
			rl.DrawCircleV(rl.NewVector2(cx, dy), 1.3, rl.Fade(th.TextMuted, 0.6))
		}
	}

	if s.hover >= 0 && (s.dragging || !s.labels) {
		it := s.items[s.hover]
		m := s.marks[it.mark]
		text := fmt.Sprintf("%s %d", monthLong[m.month-1], m.year)
		if it.year {
			text = it.label
		}
		text = capitalize(text)
		w := MeasureText(th.Medium, text, 13) + 20
		r := rl.NewRectangle(area.X-w-4, area.Y+it.y+it.h/2-14, w, 28)
		rl.DrawRectangleRounded(r, 1, 16, th.Raised)
		DrawText(th.Medium, text, r.X+10, r.Y+7, 13, th.Text)
	}
}
