package search

import (
	"bytes"
	"image"
	"image/jpeg"
	"math"
	"sort"

	"golang.org/x/image/draw"
)

const (
	DetSize        = 640
	FaceSourceSize = 1600
	FaceSize       = 112
	CropSize       = 160
	FaceDim        = 512
	detThreshold   = 0.6
	nmsThreshold   = 0.4
	minFacePixels  = 28
)

var detStrides = []int{8, 16, 32}

var arcfaceTemplate = [5][2]float32{
	{38.2946, 51.6963},
	{73.5318, 51.5014},
	{56.0252, 71.7366},
	{41.5493, 92.3655},
	{70.7299, 92.2041},
}

type Detection struct {
	Box       [4]float32
	Landmarks [10]float32
	Score     float32
}

func (d Detection) Width() float32  { return d.Box[2] - d.Box[0] }
func (d Detection) Height() float32 { return d.Box[3] - d.Box[1] }

func PrepareDetInput(img *image.RGBA, dst []float32) float32 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := float32(DetSize) / float32(max(w, h))
	nw := max(1, int(float32(w)*scale))
	nh := max(1, int(float32(h)*scale))

	resized := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.BiLinear.Scale(resized, resized.Bounds(), img, b, draw.Src, nil)

	plane := DetSize * DetSize
	pad := float32(0-127.5) / 128
	for i := range dst[:3*plane] {
		dst[i] = pad
	}
	for y := range nh {
		for x := range nw {
			o := resized.PixOffset(x, y)
			i := y*DetSize + x
			for c := range 3 {
				dst[c*plane+i] = (float32(resized.Pix[o+c]) - 127.5) / 128
			}
		}
	}
	return scale
}

func DecodeDetections(scores, boxes, kps [3][]float32, scale float32) []Detection {
	var dets []Detection
	for level, stride := range detStrides {
		fmap := DetSize / stride
		s := float32(stride)
		for idx, score := range scores[level] {
			if score < detThreshold {
				continue
			}
			loc := idx / 2
			cx := float32(loc%fmap) * s
			cy := float32(loc/fmap) * s
			bb := boxes[level][idx*4 : idx*4+4]
			d := Detection{
				Score: score,
				Box: [4]float32{
					(cx - bb[0]*s) / scale,
					(cy - bb[1]*s) / scale,
					(cx + bb[2]*s) / scale,
					(cy + bb[3]*s) / scale,
				},
			}
			kp := kps[level][idx*10 : idx*10+10]
			for j := range 5 {
				d.Landmarks[j*2] = (cx + kp[j*2]*s) / scale
				d.Landmarks[j*2+1] = (cy + kp[j*2+1]*s) / scale
			}
			if d.Width() >= minFacePixels && d.Height() >= minFacePixels {
				dets = append(dets, d)
			}
		}
	}
	return nms(dets)
}

func nms(dets []Detection) []Detection {
	sort.Slice(dets, func(i, j int) bool { return dets[i].Score > dets[j].Score })
	var keep []Detection
	for _, d := range dets {
		ok := true
		for _, k := range keep {
			if iou(d.Box, k.Box) > nmsThreshold {
				ok = false
				break
			}
		}
		if ok {
			keep = append(keep, d)
		}
	}
	return keep
}

func iou(a, b [4]float32) float32 {
	x1, y1 := max(a[0], b[0]), max(a[1], b[1])
	x2, y2 := min(a[2], b[2]), min(a[3], b[3])
	inter := max(0, x2-x1) * max(0, y2-y1)
	union := (a[2]-a[0])*(a[3]-a[1]) + (b[2]-b[0])*(b[3]-b[1]) - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func similarity(src [10]float32) (a, b, tx, ty float32) {
	var smx, smy, dmx, dmy float32
	for i := range 5 {
		smx += src[i*2]
		smy += src[i*2+1]
		dmx += arcfaceTemplate[i][0]
		dmy += arcfaceTemplate[i][1]
	}
	smx, smy, dmx, dmy = smx/5, smy/5, dmx/5, dmy/5

	var num1, num2, den float32
	for i := range 5 {
		px, py := src[i*2]-smx, src[i*2+1]-smy
		qx, qy := arcfaceTemplate[i][0]-dmx, arcfaceTemplate[i][1]-dmy
		num1 += px*qx + py*qy
		num2 += px*qy - py*qx
		den += px*px + py*py
	}
	if den == 0 {
		return 1, 0, 0, 0
	}
	a, b = num1/den, num2/den
	tx = dmx - (a*smx - b*smy)
	ty = dmy - (b*smx + a*smy)
	return
}

func AlignFace(img *image.RGBA, landmarks [10]float32, dst []float32) {
	a, b, tx, ty := similarity(landmarks)
	inv := 1 / (a*a + b*b)
	plane := FaceSize * FaceSize
	for v := range FaceSize {
		for u := range FaceSize {
			qx, qy := float32(u)-tx, float32(v)-ty
			sx := (a*qx + b*qy) * inv
			sy := (-b*qx + a*qy) * inv
			r, g, bl := sample(img, sx, sy)
			i := v*FaceSize + u
			dst[i] = (r - 127.5) / 127.5
			dst[plane+i] = (g - 127.5) / 127.5
			dst[2*plane+i] = (bl - 127.5) / 127.5
		}
	}
}

func sample(img *image.RGBA, x, y float32) (float32, float32, float32) {
	b := img.Bounds()
	x0 := int(math.Floor(float64(x)))
	y0 := int(math.Floor(float64(y)))
	fx, fy := x-float32(x0), y-float32(y0)

	var out [3]float32
	for _, p := range [4]struct {
		dx, dy int
		w      float32
	}{
		{0, 0, (1 - fx) * (1 - fy)},
		{1, 0, fx * (1 - fy)},
		{0, 1, (1 - fx) * fy},
		{1, 1, fx * fy},
	} {
		px, py := x0+p.dx, y0+p.dy
		if px < b.Min.X || py < b.Min.Y || px >= b.Max.X || py >= b.Max.Y || p.w == 0 {
			continue
		}
		o := img.PixOffset(px, py)
		for c := range 3 {
			out[c] += float32(img.Pix[o+c]) * p.w
		}
	}
	return out[0], out[1], out[2]
}

func CropFace(img *image.RGBA, d Detection) []byte {
	b := img.Bounds()
	cx := (d.Box[0] + d.Box[2]) / 2
	cy := (d.Box[1] + d.Box[3]) / 2
	side := max(d.Width(), d.Height()) * 1.5
	r := image.Rect(int(cx-side/2), int(cy-side/2), int(cx+side/2), int(cy+side/2)).Intersect(b)
	if r.Empty() {
		return nil
	}

	dst := image.NewRGBA(image.Rect(0, 0, CropSize, CropSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, r, draw.Src, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil
	}
	return buf.Bytes()
}
