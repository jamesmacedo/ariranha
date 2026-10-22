//go:build cgo

package search

import (
	"errors"
	"fmt"
	"image"
	"runtime"

	ort "github.com/yalue/onnxruntime_go"
)

type FaceEncoder struct {
	det     *ort.DynamicAdvancedSession
	rec     *ort.DynamicAdvancedSession
	detBuf  []float32
	faceBuf []float32
}

type FaceResult struct {
	Detection Detection
	Embedding []float32
	Crop      []byte
}

func NewFaceEncoder(p Paths, threads int) (*FaceEncoder, error) {
	if !exists(p.FaceDetector()) || !exists(p.FaceRecognizer()) {
		return nil, errors.New("face models not found, run make models")
	}
	if err := initEnvironment(p.Runtime); err != nil {
		return nil, err
	}
	if threads <= 0 {
		threads = max(1, runtime.NumCPU()/2)
	}

	detOut := []string{"448", "471", "494", "451", "474", "497", "454", "477", "500"}
	det, err := newSessionIO(p.FaceDetector(), []string{"input.1"}, detOut, threads)
	if err != nil {
		return nil, fmt.Errorf("face detector: %w", err)
	}
	rec, err := newSessionIO(p.FaceRecognizer(), []string{"input.1"}, []string{"683"}, threads)
	if err != nil {
		det.Destroy()
		return nil, fmt.Errorf("face recognizer: %w", err)
	}

	return &FaceEncoder{
		det:     det,
		rec:     rec,
		detBuf:  make([]float32, 3*DetSize*DetSize),
		faceBuf: make([]float32, 3*FaceSize*FaceSize),
	}, nil
}

func (f *FaceEncoder) Detect(img *image.RGBA) ([]Detection, error) {
	scale := PrepareDetInput(img, f.detBuf)
	input, err := ort.NewTensor(ort.NewShape(1, 3, DetSize, DetSize), f.detBuf)
	if err != nil {
		return nil, err
	}
	defer input.Destroy()

	outputs := make([]ort.Value, 9)
	if err := f.det.Run([]ort.Value{input}, outputs); err != nil {
		return nil, err
	}
	defer func() {
		for _, o := range outputs {
			if o != nil {
				o.Destroy()
			}
		}
	}()

	var scores, boxes, kps [3][]float32
	for i := range 3 {
		scores[i] = outputs[i].(*ort.Tensor[float32]).GetData()
		boxes[i] = outputs[3+i].(*ort.Tensor[float32]).GetData()
		kps[i] = outputs[6+i].(*ort.Tensor[float32]).GetData()
	}
	return DecodeDetections(scores, boxes, kps, scale), nil
}

func (f *FaceEncoder) Embed(img *image.RGBA, d Detection) ([]float32, error) {
	AlignFace(img, d.Landmarks, f.faceBuf)
	input, err := ort.NewTensor(ort.NewShape(1, 3, FaceSize, FaceSize), f.faceBuf)
	if err != nil {
		return nil, err
	}
	defer input.Destroy()

	outputs := []ort.Value{nil}
	if err := f.rec.Run([]ort.Value{input}, outputs); err != nil {
		return nil, err
	}
	defer outputs[0].Destroy()

	data := outputs[0].(*ort.Tensor[float32]).GetData()
	if len(data) != FaceDim {
		return nil, fmt.Errorf("face vector has %d values", len(data))
	}
	return Normalize(data), nil
}

func (f *FaceEncoder) Process(img *image.RGBA) ([]FaceResult, error) {
	dets, err := f.Detect(img)
	if err != nil {
		return nil, err
	}
	var out []FaceResult
	for _, d := range dets {
		emb, err := f.Embed(img, d)
		if err != nil {
			return nil, err
		}
		out = append(out, FaceResult{Detection: d, Embedding: emb, Crop: CropFace(img, d)})
	}
	return out, nil
}

func (f *FaceEncoder) Close() {
	f.det.Destroy()
	f.rec.Destroy()
}
