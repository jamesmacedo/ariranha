//go:build !cgo

package search

type Encoder struct{}

func NewEncoder(p Paths, threads int) (*Encoder, error) {
	return nil, ErrUnavailable
}

func (e *Encoder) Dim() int              { return 0 }
func (e *Encoder) Tokenizer() *Tokenizer { return nil }
func (e *Encoder) EncodeImages(pixels []float32, n int) ([][]float32, error) {
	return nil, ErrUnavailable
}
func (e *Encoder) EncodeText(text string) ([]float32, error) { return nil, ErrUnavailable }
func (e *Encoder) Close()                                    {}
