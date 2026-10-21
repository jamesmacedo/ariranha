package search

import (
	"container/heap"
	"math"
	"runtime"
	"sort"
	"sync"
)

type Hit struct {
	ID    int
	Score float32
}

type Index struct {
	dim  int
	mu   sync.RWMutex
	ids  []int
	vecs []float32
	rows map[int]int
}

func NewIndex(dim int) *Index {
	return &Index{dim: dim, rows: map[int]int{}}
}

func (x *Index) Dim() int {
	return x.dim
}

func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.ids)
}

func (x *Index) Has(id int) bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	_, ok := x.rows[id]
	return ok
}

func (x *Index) Remove(id int) {
	x.mu.Lock()
	defer x.mu.Unlock()
	row, ok := x.rows[id]
	if !ok {
		return
	}
	last := len(x.ids) - 1
	if row != last {
		x.ids[row] = x.ids[last]
		copy(x.vecs[row*x.dim:(row+1)*x.dim], x.vecs[last*x.dim:(last+1)*x.dim])
		x.rows[x.ids[row]] = row
	}
	x.ids = x.ids[:last]
	x.vecs = x.vecs[:last*x.dim]
	delete(x.rows, id)
}

func (x *Index) Add(id int, vec []float32) {
	v := Normalize(vec)
	x.mu.Lock()
	defer x.mu.Unlock()
	if row, ok := x.rows[id]; ok {
		copy(x.vecs[row*x.dim:(row+1)*x.dim], v)
		return
	}
	x.rows[id] = len(x.ids)
	x.ids = append(x.ids, id)
	x.vecs = append(x.vecs, v...)
}

func (x *Index) Search(query []float32, k int) []Hit {
	q := Normalize(query)
	x.mu.RLock()
	defer x.mu.RUnlock()

	n := len(x.ids)
	if n == 0 || k <= 0 {
		return nil
	}

	workers := max(1, min(runtime.NumCPU(), n/2048))
	chunk := (n + workers - 1) / workers
	partial := make([][]Hit, workers)

	var wg sync.WaitGroup
	for w := range workers {
		lo, hi := w*chunk, min(n, (w+1)*chunk)
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := make(minHeap, 0, k)
			for row := lo; row < hi; row++ {
				s := dot(q, x.vecs[row*x.dim:(row+1)*x.dim])
				if len(h) < k {
					heap.Push(&h, Hit{ID: x.ids[row], Score: s})
				} else if s > h[0].Score {
					h[0] = Hit{ID: x.ids[row], Score: s}
					heap.Fix(&h, 0)
				}
			}
			partial[w] = h
		}()
	}
	wg.Wait()

	var all []Hit
	for _, p := range partial {
		all = append(all, p...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	if len(all) > k {
		all = all[:k]
	}
	return all
}

func Normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	out := make([]float32, len(v))
	if sum == 0 {
		return out
	}
	inv := float32(1 / math.Sqrt(sum))
	for i, x := range v {
		out[i] = x * inv
	}
	return out
}

func dot(a, b []float32) float32 {
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	for ; i < len(a); i++ {
		s0 += a[i] * b[i]
	}
	return s0 + s1 + s2 + s3
}

type minHeap []Hit

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].Score < h[j].Score }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(Hit)) }
func (h *minHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
