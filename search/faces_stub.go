//go:build !cgo

package search

import "image"

type FaceEncoder struct{}

type FaceResult struct {
	Detection Detection
	Embedding []float32
	Crop      []byte
}

func NewFaceEncoder(p Paths, threads int) (*FaceEncoder, error) {
	return nil, ErrUnavailable
}

func (f *FaceEncoder) Process(img *image.RGBA) ([]FaceResult, error) {
	return nil, ErrUnavailable
}

func (f *FaceEncoder) Close() {}
