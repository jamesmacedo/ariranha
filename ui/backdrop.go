package ui

import rl "github.com/gen2brain/raylib-go/raylib"

const blurScale = 4

const (
	glZero    = 0
	glOne     = 1
	glFuncAdd = 0x8006
)

const blurShader = `#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
uniform sampler2D texture0;
uniform vec2 dir;
out vec4 finalColor;
const float w[5] = float[](0.227027, 0.1945946, 0.1216216, 0.054054, 0.016216);
void main() {
	vec3 c = texture(texture0, fragTexCoord).rgb * w[0];
	for (int i = 1; i < 5; i++) {
		c += texture(texture0, fragTexCoord + dir * float(i)).rgb * w[i];
		c += texture(texture0, fragTexCoord - dir * float(i)).rgb * w[i];
	}
	finalColor = vec4(c, 1.0);
}
`

const glassShader = `#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
uniform sampler2D texture0;
uniform vec2 uvMin;
uniform vec2 uvMax;
uniform vec2 size;
uniform float radius;
uniform vec4 tint;
out vec4 finalColor;
void main() {
	vec2 p = (fragTexCoord - uvMin) / (uvMax - uvMin) * size;
	vec2 q = abs(p - size * 0.5) - (size * 0.5 - vec2(radius));
	float d = length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - radius;
	float a = clamp(0.5 - d, 0.0, 1.0);
	vec3 c = mix(texture(texture0, fragTexCoord).rgb, tint.rgb, tint.a);
	finalColor = vec4(c, a);
}
`

type Backdrop struct {
	scene rl.RenderTexture2D
	small [2]rl.RenderTexture2D
	w, h  int32
	blur  rl.Shader
	glass rl.Shader

	locDir    int32
	locUVMin  int32
	locUVMax  int32
	locSize   int32
	locRadius int32
	locTint   int32
}

func NewBackdrop() *Backdrop {
	b := &Backdrop{
		blur:  rl.LoadShaderFromMemory("", blurShader),
		glass: rl.LoadShaderFromMemory("", glassShader),
	}
	b.locDir = rl.GetShaderLocation(b.blur, "dir")
	b.locUVMin = rl.GetShaderLocation(b.glass, "uvMin")
	b.locUVMax = rl.GetShaderLocation(b.glass, "uvMax")
	b.locSize = rl.GetShaderLocation(b.glass, "size")
	b.locRadius = rl.GetShaderLocation(b.glass, "radius")
	b.locTint = rl.GetShaderLocation(b.glass, "tint")
	return b
}

func (b *Backdrop) resize(w, h int32) {
	if w == b.w && h == b.h {
		return
	}
	b.release()
	b.w, b.h = w, h
	b.scene = rl.LoadRenderTexture(w, h)
	rl.SetTextureFilter(b.scene.Texture, rl.FilterBilinear)
	for i := range b.small {
		b.small[i] = rl.LoadRenderTexture(max(1, w/blurScale), max(1, h/blurScale))
		rl.SetTextureFilter(b.small[i].Texture, rl.FilterBilinear)
	}
}

func (b *Backdrop) release() {
	if b.w == 0 {
		return
	}
	rl.UnloadRenderTexture(b.scene)
	for _, s := range b.small {
		rl.UnloadRenderTexture(s)
	}
	b.w, b.h = 0, 0
}

func (b *Backdrop) Unload() {
	b.release()
	rl.UnloadShader(b.blur)
	rl.UnloadShader(b.glass)
}

func (b *Backdrop) Begin() {
	b.resize(int32(rl.GetScreenWidth()), int32(rl.GetScreenHeight()))
	rl.BeginTextureMode(b.scene)
}

func flipped(t rl.Texture2D) rl.Rectangle {
	return rl.NewRectangle(0, 0, float32(t.Width), -float32(t.Height))
}

func (b *Backdrop) pass(dst rl.RenderTexture2D, src rl.Texture2D, dir rl.Vector2) {
	rl.BeginTextureMode(dst)
	rl.BeginShaderMode(b.blur)
	rl.SetShaderValue(b.blur, b.locDir, []float32{dir.X, dir.Y}, rl.ShaderUniformVec2)
	rl.DrawTexturePro(src, flipped(src), rl.NewRectangle(0, 0, float32(dst.Texture.Width), float32(dst.Texture.Height)), rl.Vector2{}, 0, rl.White)
	rl.EndShaderMode()
	rl.EndTextureMode()
}

func (b *Backdrop) End() {
	rl.EndTextureMode()

	s0, s1 := b.small[0], b.small[1]
	sw, sh := float32(s0.Texture.Width), float32(s0.Texture.Height)
	rl.BeginTextureMode(s0)
	rl.DrawTexturePro(b.scene.Texture, flipped(b.scene.Texture), rl.NewRectangle(0, 0, sw, sh), rl.Vector2{}, 0, rl.White)
	rl.EndTextureMode()
	b.pass(s1, s0.Texture, rl.NewVector2(1/sw, 0))
	b.pass(s0, s1.Texture, rl.NewVector2(0, 1/sh))

	rl.SetBlendFactors(glOne, glZero, glFuncAdd)
	rl.BeginBlendMode(rl.BlendCustom)
	rl.DrawTexturePro(b.scene.Texture, flipped(b.scene.Texture), rl.NewRectangle(0, 0, float32(b.w), float32(b.h)), rl.Vector2{}, 0, rl.White)
	rl.EndBlendMode()
}

func (b *Backdrop) Glass(r rl.Rectangle, radius float32, tint rl.Color) {
	tex := b.small[0].Texture
	tw, th := float32(tex.Width), float32(tex.Height)
	k := tw / float32(b.w)
	src := rl.NewRectangle(r.X*k, th-(r.Y+r.Height)*k, r.Width*k, -r.Height*k)

	rl.BeginShaderMode(b.glass)
	rl.SetShaderValue(b.glass, b.locUVMin, []float32{src.X / tw, src.Y / th}, rl.ShaderUniformVec2)
	rl.SetShaderValue(b.glass, b.locUVMax, []float32{(src.X + src.Width) / tw, (src.Y - src.Height) / th}, rl.ShaderUniformVec2)
	rl.SetShaderValue(b.glass, b.locSize, []float32{r.Width, r.Height}, rl.ShaderUniformVec2)
	rl.SetShaderValue(b.glass, b.locRadius, []float32{radius}, rl.ShaderUniformFloat)
	rl.SetShaderValue(b.glass, b.locTint, []float32{float32(tint.R) / 255, float32(tint.G) / 255, float32(tint.B) / 255, float32(tint.A) / 255}, rl.ShaderUniformVec4)
	rl.DrawTexturePro(tex, src, r, rl.Vector2{}, 0, rl.White)
	rl.EndShaderMode()
}
