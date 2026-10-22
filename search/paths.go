package search

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

var ErrUnavailable = errors.New("semantic search unavailable in this build")

type Paths struct {
	Models     string
	Runtime    string
	Database   string
	ModelName  string
	VisionFile string
	TextFile   string
	Trash      string
}

const (
	VisionInt8 = "vision_model_quantized.onnx"
	VisionFP32 = "vision_model.onnx"
	TextInt8   = "text_model_quantized.onnx"
)

func (p Paths) Vision() string    { return filepath.Join(p.Models, p.VisionFile) }
func (p Paths) Text() string      { return filepath.Join(p.Models, p.TextFile) }
func (p Paths) Tokenizer() string { return filepath.Join(p.Models, "tokenizer.json") }

func (p Paths) FaceDetector() string   { return filepath.Join(p.Models, "det_10g.onnx") }
func (p Paths) FaceRecognizer() string { return filepath.Join(p.Models, "w600k_r50.onnx") }

func NewPaths(models, runtimeDir, database string) Paths {
	return Paths{
		Models:     models,
		Runtime:    filepath.Join(runtimeDir, RuntimeLibrary()),
		Database:   database,
		ModelName:  "siglip-base-patch16-256-multilingual-q8",
		VisionFile: VisionInt8,
		TextFile:   TextInt8,
	}
}

func RuntimeLibrary() string {
	switch runtime.GOOS {
	case "windows":
		return "onnxruntime.dll"
	case "darwin":
		return "libonnxruntime.dylib"
	default:
		return "libonnxruntime.so"
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
