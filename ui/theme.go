package ui

import (
	_ "embed"
	"math"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

//go:embed fonts/Roboto-Regular.ttf
var robotoRegular []byte

//go:embed fonts/Roboto-Medium.ttf
var robotoMedium []byte

type Theme struct {
	Regular rl.Font
	Medium  rl.Font
	Dark    bool

	Background  rl.Color
	Surface     rl.Color
	Raised      rl.Color
	RaisedHover rl.Color
	Divider     rl.Color
	Text        rl.Color
	TextMuted   rl.Color
	Accent      rl.Color
	OnAccent    rl.Color
	People      rl.Color
	Favorite    rl.Color
	Glass       rl.Color
	GlassEdge   rl.Color
	Shadow      rl.Color
}

var albumPalette = [...][2]uint32{
	{0x7FE3C4, 0x0A7D61},
	{0xA4B8FF, 0x3A55D4},
	{0xFF8CA1, 0xD0314C},
	{0xFFC56E, 0xB86E00},
	{0xC9A7FF, 0x7442C8},
	{0xFF9E7A, 0xC2410C},
	{0x7CD4FF, 0x0369A1},
	{0xB8E07A, 0x4D7C0F},
}

func AlbumColors() int {
	return len(albumPalette)
}

func (t *Theme) AlbumColor(i int) rl.Color {
	c := albumPalette[clampInt(i, 0, len(albumPalette)-1)]
	if t.Dark {
		return hex(c[0])
	}
	return hex(c[1])
}

func hex(v uint32) rl.Color {
	return rl.NewColor(uint8(v>>16), uint8(v>>8), uint8(v), 255)
}

func LoadTheme() *Theme {
	t := &Theme{
		Regular: loadFont(robotoRegular),
		Medium:  loadFont(robotoMedium),
	}
	t.SetDark(true)
	return t
}

func (t *Theme) SetDark(dark bool) {
	t.Dark = dark
	if dark {
		t.Background = hex(0x050607)
		t.Surface = hex(0x13161B)
		t.Raised = hex(0x1D2128)
		t.RaisedHover = hex(0x272C35)
		t.Divider = hex(0x262A31)
		t.Text = hex(0xE8EBEF)
		t.TextMuted = hex(0x8A919B)
		t.Accent = hex(0x7FE3C4)
		t.OnAccent = hex(0x04261C)
		t.People = hex(0xA4B8FF)
		t.Favorite = hex(0xFF8CA1)
		t.Glass = rl.NewColor(22, 25, 31, 150)
		t.GlassEdge = rl.NewColor(255, 255, 255, 22)
		t.Shadow = rl.NewColor(0, 0, 0, 90)
		return
	}
	t.Background = hex(0xD9DEE6)
	t.Surface = hex(0xFFFFFF)
	t.Raised = hex(0xEDF0F4)
	t.RaisedHover = hex(0xE2E6EC)
	t.Divider = hex(0xC4CAD3)
	t.Text = hex(0x15181D)
	t.TextMuted = hex(0x5D6470)
	t.Accent = hex(0x0A7D61)
	t.OnAccent = hex(0xFFFFFF)
	t.People = hex(0x3A55D4)
	t.Favorite = hex(0xD0314C)
	t.Glass = rl.NewColor(255, 255, 255, 165)
	t.GlassEdge = rl.NewColor(255, 255, 255, 140)
	t.Shadow = rl.NewColor(20, 30, 45, 40)
}

func (t *Theme) Unload() {
	rl.UnloadFont(t.Regular)
	rl.UnloadFont(t.Medium)
	unloadRounded()
}

func (t *Theme) ButtonStyle() ButtonStyle {
	return ButtonStyle{
		Background: [4]rl.Color{
			StateNormal:   t.Raised,
			StateHover:    t.RaisedHover,
			StatePressed:  t.Surface,
			StateDisabled: t.Surface,
		},
		Text: [4]rl.Color{
			StateNormal:   t.Accent,
			StateHover:    t.Accent,
			StatePressed:  t.Accent,
			StateDisabled: t.TextMuted,
		},
		FontSize:  15,
		Roundness: 0.5,
	}
}

func loadFont(data []byte) rl.Font {
	f := rl.LoadFontFromMemory(".ttf", data, 48, codepoints())
	rl.GenTextureMipmaps(&f.Texture)
	rl.SetTextureFilter(f.Texture, rl.FilterTrilinear)
	return f
}

func codepoints() []rune {
	var cp []rune
	for r := rune(32); r < 127; r++ {
		cp = append(cp, r)
	}
	for r := rune(160); r < 256; r++ {
		cp = append(cp, r)
	}
	return append(cp, '…', '•', '–', '—', '−', '“', '”', '‘', '’', '‹', '›')
}

func DrawText(font rl.Font, text string, x, y, size float32, c rl.Color) {
	rl.DrawTextEx(font, text, rl.NewVector2(float32(math.Round(float64(x))), float32(math.Round(float64(y)))), size, 0, c)
}

func MeasureText(font rl.Font, text string, size float32) float32 {
	return rl.MeasureTextEx(font, text, size, 0).X
}

func FitText(font rl.Font, text string, size, maxWidth float32) string {
	if MeasureText(font, text, size) <= maxWidth {
		return text
	}
	r := []rune(text)
	for len(r) > 0 {
		r = r[:len(r)-1]
		s := strings.TrimRight(string(r), " ") + "…"
		if MeasureText(font, s, size) <= maxWidth {
			return s
		}
	}
	return ""
}

func DrawCover(tex rl.Texture2D, dst rl.Rectangle, tint rl.Color) {
	if dst.Height <= 0 || tex.Height <= 0 {
		return
	}
	rl.DrawTexturePro(tex, coverSource(tex, dst), dst, rl.Vector2{}, 0, tint)
}

func smooth(current, target float32) float32 {
	k := 1 - float32(math.Exp(-float64(rl.GetFrameTime())*18))
	v := current + (target-current)*k
	if float32(math.Abs(float64(target-v))) < 0.5 {
		return target
	}
	return v
}

func clamp(v, lo, hi float32) float32 {
	return max(lo, min(v, hi))
}
