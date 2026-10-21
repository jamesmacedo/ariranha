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

type Encoder struct {
	vision    *ort.DynamicAdvancedSession
	text      *ort.DynamicAdvancedSession
	tokenizer *Tokenizer
	dim       int
}

func initEnvironment(lib string) error {
	envOnce.Do(func() {
		if lib == "" {
			envErr = errors.New("biblioteca do ONNX Runtime não encontrada")
			return
		}
		ort.SetSharedLibraryPath(lib)
		envErr = ort.InitializeEnvironment()
	})
	return envErr
}

func NewEncoder(p Paths, threads int) (*Encoder, error) {
	if p.Models == "" {
		return nil, errors.New("modelos não encontrados, rode make models")
	}
	if err := initEnvironment(p.Runtime); err != nil {
		return nil, err
	}

	tok, err := LoadTokenizer(p.Tokenizer())
	if err != nil {
		return nil, fmt.Errorf("tokenizer: %w", err)
	}

	if threads <= 0 {
		threads = max(1, runtime.NumCPU()/2)
	}

	vision, err := newSession(p.Vision(), "pixel_values", threads)
	if err != nil {
		return nil, fmt.Errorf("modelo de imagem: %w", err)
	}
	text, err := newSession(p.Text(), "input_ids", threads)
	if err != nil {
		vision.Destroy()
		return nil, fmt.Errorf("modelo de texto: %w", err)
	}

	return &Encoder{vision: vision, text: text, tokenizer: tok, dim: 768}, nil
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
	return e.dim
}

func (e *Encoder) Tokenizer() *Tokenizer {
	return e.tokenizer
}

func (e *Encoder) EncodeImages(pixels []float32, n int) ([][]float32, error) {
	input, err := ort.NewTensor(ort.NewShape(int64(n), 3, ImageSize, ImageSize), pixels[:n*3*ImageSize*ImageSize])
	if err != nil {
		return nil, err
	}
	defer input.Destroy()
	return e.run(e.vision, input, n)
}

func (e *Encoder) EncodeText(text string) ([]float32, error) {
	ids := e.tokenizer.Encode(text)
	input, err := ort.NewTensor(ort.NewShape(1, int64(len(ids))), ids)
	if err != nil {
		return nil, err
	}
	defer input.Destroy()
	out, err := e.run(e.text, input, 1)
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

func (e *Encoder) run(s *ort.DynamicAdvancedSession, input ort.Value, n int) ([][]float32, error) {
	outputs := []ort.Value{nil}
	if err := s.Run([]ort.Value{input}, outputs); err != nil {
		return nil, err
	}
	defer outputs[0].Destroy()

	t, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, errors.New("saída inesperada do modelo")
	}
	data := t.GetData()
	if len(data) != n*e.dim {
		return nil, fmt.Errorf("saída com %d valores, esperado %d", len(data), n*e.dim)
	}

	vecs := make([][]float32, n)
	for i := range vecs {
		vecs[i] = Normalize(data[i*e.dim : (i+1)*e.dim])
	}
	return vecs, nil
}

func (e *Encoder) Close() {
	e.vision.Destroy()
	e.text.Destroy()
}
