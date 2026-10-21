package search

import (
	"sort"
	"sync"
)

const personThreshold = 0.45

type Face struct {
	ID        int64
	Path      string
	Box       [4]float32
	Landmarks [10]float32
	Score     float32
	Embedding []float32
	Crop      []byte
	Person    *Person
}

type Person struct {
	ID        int64
	Name      string
	Count     int
	Cover     *Face
	CoverCrop []byte
	sum       []float32
}

func (p *Person) centroid() []float32 {
	return Normalize(p.sum)
}

type PersonInfo struct {
	ID    int64
	Name  string
	Count int
	Crop  []byte
	Paths []string
}

type People struct {
	mu      sync.RWMutex
	persons map[int64]*Person
	pending []*Person
	byPath  map[string][]*Face
	version int
}

func NewPeople() *People {
	return &People{persons: map[int64]*Person{}, byPath: map[string][]*Face{}}
}

func (pp *People) Version() int {
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	return pp.version
}

func (pp *People) faceCount() int {
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	n := 0
	for _, faces := range pp.byPath {
		n += len(faces)
	}
	return n
}

func (pp *People) restorePerson(p *Person) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	p.sum = make([]float32, FaceDim)
	pp.persons[p.ID] = p
}

func (pp *People) restoreFace(f *Face, personID int64) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	if p := pp.persons[personID]; p != nil {
		f.Person = p
		pp.attach(p, f)
	}
	pp.byPath[f.Path] = append(pp.byPath[f.Path], f)
	pp.version++
}

func (pp *People) attach(p *Person, f *Face) {
	for i, v := range f.Embedding {
		p.sum[i] += v
	}
	p.Count++
	if p.Cover == nil || f.Score > p.Cover.Score {
		p.Cover = f
		if f.Crop != nil {
			p.CoverCrop = f.Crop
		}
	}
}

func (pp *People) Assign(f *Face) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	var best *Person
	var bestScore float32 = -1
	for _, p := range pp.allLocked() {
		if p.Count == 0 {
			continue
		}
		if s := dot(p.centroid(), f.Embedding); s > bestScore {
			best, bestScore = p, s
		}
	}
	if best == nil || bestScore < personThreshold {
		best = &Person{sum: make([]float32, FaceDim)}
		pp.pending = append(pp.pending, best)
	}
	f.Person = best
	pp.attach(best, f)
}

func (pp *People) allLocked() []*Person {
	all := make([]*Person, 0, len(pp.persons)+len(pp.pending))
	for _, p := range pp.persons {
		all = append(all, p)
	}
	return append(all, pp.pending...)
}

func (pp *People) Commit(path string, faces []*Face) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	for _, p := range pp.pending {
		if p.ID != 0 {
			pp.persons[p.ID] = p
		}
	}
	pp.pending = pp.pending[:0]
	pp.byPath[path] = faces
	pp.version++
}

func (pp *People) Remove(path string) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	for _, f := range pp.byPath[path] {
		p := f.Person
		if p == nil {
			continue
		}
		for i, v := range f.Embedding {
			p.sum[i] -= v
		}
		p.Count--
		if p.Cover == f {
			p.Cover = nil
		}
	}
	delete(pp.byPath, path)
	pp.version++
}

func (pp *People) List() []PersonInfo {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	paths := map[int64]map[string]bool{}
	for path, faces := range pp.byPath {
		for _, f := range faces {
			if f.Person == nil {
				continue
			}
			if paths[f.Person.ID] == nil {
				paths[f.Person.ID] = map[string]bool{}
			}
			paths[f.Person.ID][path] = true
		}
	}

	var out []PersonInfo
	for _, p := range pp.persons {
		if p.Count <= 0 {
			continue
		}
		info := PersonInfo{ID: p.ID, Name: p.Name, Count: p.Count, Crop: p.CoverCrop}
		for path := range paths[p.ID] {
			info.Paths = append(info.Paths, path)
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].ID < out[j].ID
	})
	return out
}
