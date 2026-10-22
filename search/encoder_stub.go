//go:build !cgo

package search

type ImageEncoder struct{}

type TextEncoder struct{}

type Encoder struct {
	*ImageEncoder
	*TextEncoder
}

func CheckModels(p Paths) error     { return ErrUnavailable }
func CheckFaceModels(p Paths) error { return ErrUnavailable }

func NewImageEncoder(p Paths, threads int) (*ImageEncoder, error) { return nil, ErrUnavailable }
func NewTextEncoder(p Paths, threads int) (*TextEncoder, error)   { return nil, ErrUnavailable }
func NewEncoder(p Paths, threads int) (*Encoder, error)           { return nil, ErrUnavailable }

func (e *Encoder) Dim() int                  { return 0 }
func (e *Encoder) Close()                    {}
func (e *TextEncoder) Tokenizer() *Tokenizer { return nil }
func (e *ImageEncoder) EncodeImages(pixels []float32, n int) ([][]float32, error) {
	return nil, ErrUnavailable
}
func (e *TextEncoder) EncodeText(text string) ([]float32, error) { return nil, ErrUnavailable }
func (e *ImageEncoder) Close()                                   {}
func (e *TextEncoder) Close()                                    {}
