//go:build cgo

package search

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var (
	envOnce sync.Once
	envErr  error
)

type ImageEncoder struct {
	session *ort.DynamicAdvancedSession
}

type TextEncoder struct {
	session   *ort.DynamicAdvancedSession
	tokenizer *Tokenizer
}

type Encoder struct {
	*ImageEncoder
	*TextEncoder
}

func initEnvironment(lib string) error {
	envOnce.Do(func() {
		if lib == "" {
			envErr = errors.New("ONNX Runtime library not found")
			return
		}
		ort.SetSharedLibraryPath(lib)
		envErr = ort.InitializeEnvironment()
	})
	return envErr
}

func CheckModels(p Paths) error {
	if p.Models == "" {
		return errors.New("models not found, run make models")
	}
	for _, f := range []string{p.Vision(), p.Text(), p.Tokenizer()} {
		if !exists(f) {
			return fmt.Errorf("%s not found, run make models", f)
		}
	}
	return initEnvironment(p.Runtime)
}

func CheckFaceModels(p Paths) error {
	if !exists(p.FaceDetector()) || !exists(p.FaceRecognizer()) {
		return errors.New("face models not found, run make models")
	}
	return initEnvironment(p.Runtime)
}

func defaultThreads(threads int) int {
	if threads <= 0 {
		return max(1, runtime.NumCPU()/2)
	}
	return threads
}

func NewImageEncoder(p Paths, threads int) (*ImageEncoder, error) {
	if err := CheckModels(p); err != nil {
		return nil, err
	}
	session, err := newSession(p.Vision(), "pixel_values", defaultThreads(threads))
	if err != nil {
		return nil, fmt.Errorf("image model: %w", err)
	}
	return &ImageEncoder{session: session}, nil
}

func NewTextEncoder(p Paths, threads int) (*TextEncoder, error) {
	if err := CheckModels(p); err != nil {
		return nil, err
	}
	tok, err := LoadTokenizer(p.Tokenizer())
	if err != nil {
		return nil, fmt.Errorf("tokenizer: %w", err)
	}
	session, err := newSession(p.Text(), "input_ids", defaultThreads(threads))
	if err != nil {
		return nil, fmt.Errorf("text model: %w", err)
	}
	return &TextEncoder{session: session, tokenizer: tok}, nil
}

func NewEncoder(p Paths, threads int) (*Encoder, error) {
	img, err := NewImageEncoder(p, threads)
	if err != nil {
		return nil, err
	}
	text, err := NewTextEncoder(p, threads)
	if err != nil {
		img.Close()
		return nil, err
	}
	return &Encoder{ImageEncoder: img, TextEncoder: text}, nil
}

func newSession(path, input string, threads int) (*ort.DynamicAdvancedSession, error) {
	return newSessionIO(path, []string{input}, []string{"pooler_output"}, threads)
}

func newSessionIO(path string, inputs, outputs []string, threads int) (*ort.DynamicAdvancedSession, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	if err := opts.SetIntraOpNumThreads(threads); err != nil {
		return nil, err
	}
	if err := opts.SetInterOpNumThreads(1); err != nil {
		return nil, err
	}
	return ort.NewDynamicAdvancedSession(path, inputs, outputs, opts)
}

func (e *Encoder) Dim() int {
	return visionDim
}

func (e *Encoder) Close() {
	e.ImageEncoder.Close()
	e.TextEncoder.Close()
}

func (e *TextEncoder) Tokenizer() *Tokenizer {
	return e.tokenizer
}

func (e *ImageEncoder) EncodeImages(pixels []float32, n int) ([][]float32, error) {
	input, err := ort.NewTensor(ort.NewShape(int64(n), 3, ImageSize, ImageSize), pixels[:n*3*ImageSize*ImageSize])
	if err != nil {
		return nil, err
	}
	defer input.Destroy()
	return runPooled(e.session, input, n)
}

func (e *TextEncoder) EncodeText(text string) ([]float32, error) {
	ids := e.tokenizer.Encode(text)
	input, err := ort.NewTensor(ort.NewShape(1, int64(len(ids))), ids)
	if err != nil {
		return nil, err
	}
	defer input.Destroy()
	out, err := runPooled(e.session, input, 1)
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

func runPooled(s *ort.DynamicAdvancedSession, input ort.Value, n int) ([][]float32, error) {
	outputs := []ort.Value{nil}
	if err := s.Run([]ort.Value{input}, outputs); err != nil {
		return nil, err
	}
	defer outputs[0].Destroy()

	t, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, errors.New("unexpected model output")
	}
	data := t.GetData()
	if len(data) != n*visionDim {
		return nil, fmt.Errorf("output has %d values, expected %d", len(data), n*visionDim)
	}

	vecs := make([][]float32, n)
	for i := range vecs {
		vecs[i] = Normalize(data[i*visionDim : (i+1)*visionDim])
	}
	return vecs, nil
}

func (e *ImageEncoder) Close() {
	e.session.Destroy()
}

func (e *TextEncoder) Close() {
	e.session.Destroy()
}
