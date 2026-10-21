package search

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestSimilarityRecoversTransform(t *testing.T) {
	var src [10]float32
	angle, scale := 0.3, 2.5
	ca, sa := float32(math.Cos(angle)*scale), float32(math.Sin(angle)*scale)
	for i := range 5 {
		x, y := arcfaceTemplate[i][0], arcfaceTemplate[i][1]
		src[i*2] = ca*x - sa*y + 100
		src[i*2+1] = sa*x + ca*y + 40
	}

	a, b, tx, ty := similarity(src)
	for i := range 5 {
		px, py := src[i*2], src[i*2+1]
		qx, qy := a*px-b*py+tx, b*px+a*py+ty
		if math.Abs(float64(qx-arcfaceTemplate[i][0])) > 1e-2 || math.Abs(float64(qy-arcfaceTemplate[i][1])) > 1e-2 {
			t.Fatalf("ponto %d mapeado para (%v, %v)", i, qx, qy)
		}
	}
}

func randomUnit(r *rand.Rand) []float32 {
	v := make([]float32, FaceDim)
	for i := range v {
		v[i] = float32(r.NormFloat64())
	}
	return Normalize(v)
}

func noisy(r *rand.Rand, base []float32, amount float64) []float32 {
	v := make([]float32, len(base))
	for i := range v {
		v[i] = base[i] + float32(r.NormFloat64()*amount)
	}
	return Normalize(v)
}

func TestPeopleClustersSimilarFaces(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	a, b := randomUnit(r), randomUnit(r)
	pp := NewPeople()

	var faces []*Face
	for i := range 10 {
		base := a
		if i%2 == 1 {
			base = b
		}
		f := &Face{Path: string(rune('a' + i)), Embedding: noisy(r, base, 0.03), Score: 0.9}
		pp.Assign(f)
		for _, p := range pp.pending {
			if p.ID == 0 {
				p.ID = int64(len(pp.persons) + 1)
			}
		}
		pp.Commit(f.Path, []*Face{f})
		faces = append(faces, f)
	}

	people := pp.List()
	if len(people) != 2 {
		t.Fatalf("esperava 2 pessoas, veio %d", len(people))
	}
	for i, f := range faces {
		if f.Person != faces[i%2].Person {
			t.Fatalf("rosto %d na pessoa errada", i)
		}
	}

	pp.Remove(faces[0].Path)
	if got := faces[2].Person.Count; got != 4 {
		t.Fatalf("contagem após remover: %d", got)
	}
}
