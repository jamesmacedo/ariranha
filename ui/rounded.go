package ui

import rl "github.com/gen2brain/raylib-go/raylib"

const roundedShader = `#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
uniform sampler2D texture0;
uniform vec2 uvMin;
uniform vec2 uvMax;
uniform vec2 size;
uniform float radius;
out vec4 finalColor;
void main() {
	vec2 p = (fragTexCoord - uvMin) / (uvMax - uvMin) * size;
	vec2 q = abs(p - size * 0.5) - (size * 0.5 - vec2(radius));
	float d = length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - radius;
	vec4 c = texture(texture0, fragTexCoord) * fragColor;
	finalColor = vec4(c.rgb, c.a * clamp(0.5 - d, 0.0, 1.0));
}
`

var rounded struct {
	shader rl.Shader
	loaded bool
	uvMin  int32
	uvMax  int32
	size   int32
	radius int32
}

func roundedProgram() rl.Shader {
	if !rounded.loaded {
		rounded.shader = rl.LoadShaderFromMemory("", roundedShader)
		rounded.uvMin = rl.GetShaderLocation(rounded.shader, "uvMin")
		rounded.uvMax = rl.GetShaderLocation(rounded.shader, "uvMax")
		rounded.size = rl.GetShaderLocation(rounded.shader, "size")
		rounded.radius = rl.GetShaderLocation(rounded.shader, "radius")
		rounded.loaded = true
	}
	return rounded.shader
}

func unloadRounded() {
	if rounded.loaded {
		rl.UnloadShader(rounded.shader)
		rounded.loaded = false
	}
}

func coverSource(tex rl.Texture2D, dst rl.Rectangle) rl.Rectangle {
	tw, th := float32(tex.Width), float32(tex.Height)
	src := rl.NewRectangle(0, 0, tw, th)
	if dst.Height <= 0 || th <= 0 {
		return src
	}
	da := dst.Width / dst.Height
	if tw/th > da {
		w := th * da
		src.X = (tw - w) / 2
		src.Width = w
	} else {
		h := tw / da
		src.Y = (th - h) / 2
		src.Height = h
	}
	return src
}

func DrawCoverRounded(tex rl.Texture2D, dst rl.Rectangle, radius float32, tint rl.Color) {
	if dst.Width <= 0 || dst.Height <= 0 {
		return
	}
	sh := roundedProgram()
	src := coverSource(tex, dst)
	tw, th := float32(tex.Width), float32(tex.Height)
	rl.BeginShaderMode(sh)
	rl.SetShaderValue(sh, rounded.uvMin, []float32{src.X / tw, src.Y / th}, rl.ShaderUniformVec2)
	rl.SetShaderValue(sh, rounded.uvMax, []float32{(src.X + src.Width) / tw, (src.Y + src.Height) / th}, rl.ShaderUniformVec2)
	rl.SetShaderValue(sh, rounded.size, []float32{dst.Width, dst.Height}, rl.ShaderUniformVec2)
	rl.SetShaderValue(sh, rounded.radius, []float32{min(radius, dst.Width/2, dst.Height/2)}, rl.ShaderUniformFloat)
	rl.DrawTexturePro(tex, src, dst, rl.Vector2{}, 0, tint)
	rl.EndShaderMode()
}

func DrawRoundedRect(r rl.Rectangle, radius float32, c rl.Color) {
	m := min(r.Width, r.Height)
	if m <= 0 {
		return
	}
	rl.DrawRectangleRounded(r, min(1, 2*radius/m), 8, c)
}
