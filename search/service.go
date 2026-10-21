package search

import (
	"image"
	"sort"
	"sync"
	"time"

	"ariranha/gallery"
)

type State int

const (
	StateLoading State = iota
	StateIndexing
	StateReady
	StateUnavailable
)

type Status struct {
	State      State
	Reading    bool
	Discovered int
	Cataloged  int
	Indexed    int
	Total      int
	Faces      int
	Remaining  time.Duration
	Err        error
	FaceErr    error
}

type Result struct {
	Query  string
	Photos []*gallery.Photo
	Scores []float32
	Took   time.Duration
	Err    error
}

const (
	readBatch  = 32
	indexBatch = 8
	maxResults = 300
	spread     = 0.08
	minScore   = -0.02
	visionDim  = 768
)

type Service struct {
	root  string
	paths Paths
	store *Store

	libMu      sync.RWMutex
	byID       map[int64]*gallery.Photo
	byRel      map[string]*gallery.Photo
	library    *gallery.Library
	libVersion int

	mu     sync.Mutex
	status Status

	scanMu sync.Mutex
	scans  map[string]Key

	enc    *Encoder
	faces  *FaceEncoder
	index  *Index
	people *People
	queue  *photoQueue

	catalogReady chan struct{}
	queries      chan string
	results      chan Result
	stop         chan struct{}
	wg           sync.WaitGroup
}

func Start(root string, paths Paths) *Service {
	s := &Service{
		root:         root,
		paths:        paths,
		byID:         map[int64]*gallery.Photo{},
		byRel:        map[string]*gallery.Photo{},
		library:      gallery.NewLibrary(root, nil),
		scans:        map[string]Key{},
		index:        NewIndex(visionDim),
		people:       NewPeople(),
		queue:        newPhotoQueue(),
		catalogReady: make(chan struct{}),
		queries:      make(chan string, 1),
		results:      make(chan Result, 1),
		stop:         make(chan struct{}),
	}
	s.status.Reading = true

	if paths.Database != "" {
		if store, err := OpenStore(paths.Database, paths.ModelName); err == nil {
			s.store = store
		} else {
			s.status.Err = err
		}
	}

	s.wg.Add(2)
	go s.catalogLoop()
	go s.indexLoop()
	return s
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) setStatus(fn func(*Status)) {
	s.mu.Lock()
	fn(&s.status)
	s.mu.Unlock()
}

func (s *Service) fail(err error) {
	s.setStatus(func(st *Status) {
		st.State = StateUnavailable
		st.Err = err
	})
}

func (s *Service) LibraryVersion() int {
	s.libMu.RLock()
	defer s.libMu.RUnlock()
	return s.libVersion
}

func (s *Service) Library() *gallery.Library {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	if s.library == nil {
		photos := make([]*gallery.Photo, 0, len(s.byRel))
		for _, p := range s.byRel {
			photos = append(photos, p)
		}
		s.library = gallery.NewLibrary(s.root, photos)
	}
	return s.library
}

func (s *Service) putPhotos(photos []*gallery.Photo) {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	for _, p := range photos {
		if old := s.byRel[p.Rel]; old != nil {
			delete(s.byID, old.ID)
		}
		s.byRel[p.Rel] = p
		s.byID[p.ID] = p
	}
	s.library = nil
	s.libVersion++
}

func (s *Service) dropPhotos(rels []string) {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	for _, rel := range rels {
		if p := s.byRel[rel]; p != nil {
			delete(s.byID, p.ID)
			delete(s.byRel, rel)
		}
	}
	s.library = nil
	s.libVersion++
}

func (s *Service) current(p *gallery.Photo) bool {
	s.libMu.RLock()
	defer s.libMu.RUnlock()
	return s.byRel[p.Rel] == p
}

func (s *Service) lookup(rel string) *gallery.Photo {
	s.libMu.RLock()
	defer s.libMu.RUnlock()
	return s.byRel[rel]
}

func (s *Service) Query(text string) {
	for {
		select {
		case s.queries <- text:
			return
		default:
			select {
			case <-s.queries:
			default:
			}
		}
	}
}

func (s *Service) Poll() (Result, bool) {
	select {
	case r := <-s.results:
		return r, true
	default:
		return Result{}, false
	}
}

func (s *Service) PeopleVersion() int {
	return s.people.Version()
}

func (s *Service) People() []PersonInfo {
	return s.people.List()
}

func (s *Service) PhotosOf(person PersonInfo) []*gallery.Photo {
	set := map[string]bool{}
	for _, p := range person.Paths {
		set[p] = true
	}
	var out []*gallery.Photo
	for _, p := range s.Library().Photos {
		if set[p.Rel] {
			out = append(out, p)
		}
	}
	return out
}

func (s *Service) Close() {
	close(s.stop)
	s.queue.Close()
	s.wg.Wait()
	if s.enc != nil {
		s.enc.Close()
	}
	if s.faces != nil {
		s.faces.Close()
	}
	if s.store != nil {
		s.store.Close()
	}
}

func (s *Service) stopped() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
}

func (s *Service) catalogLoop() {
	defer s.wg.Done()
	defer s.queue.Close()

	if s.store != nil {
		if photos, err := s.store.LoadPhotos(s.root); err == nil {
			s.putPhotos(photos)
			sort.SliceStable(photos, func(i, j int) bool { return photos[i].Date.After(photos[j].Date) })
			s.queue.Push(photos...)
			s.setStatus(func(st *Status) { st.Cataloged = len(photos) })
		}
	}
	close(s.catalogReady)

	files, _ := gallery.Discover(s.root)
	seen := make(map[string]bool, len(files))
	var todo []gallery.File
	for _, f := range files {
		seen[f.Rel] = true
		if p := s.lookup(f.Rel); p == nil || p.Size != f.Size || !p.ModTime.Equal(f.ModTime) {
			todo = append(todo, f)
		}
	}

	var gone []string
	s.libMu.RLock()
	for rel := range s.byRel {
		if !seen[rel] {
			gone = append(gone, rel)
		}
	}
	s.libMu.RUnlock()
	if len(gone) > 0 {
		for _, rel := range gone {
			s.forget(s.lookup(rel))
		}
		if s.store != nil {
			s.store.DeletePhotos(gone)
		}
		s.dropPhotos(gone)
	}

	s.setStatus(func(st *Status) {
		st.Discovered = len(files)
		st.Total = len(files)
	})

	for start := 0; start < len(todo); start += readBatch {
		if s.stopped() {
			return
		}
		photos := gallery.ReadPhotos(todo[start:min(start+readBatch, len(todo))])
		for _, p := range photos {
			if old := s.lookup(p.Rel); old != nil {
				s.forget(old)
			}
		}
		if s.store != nil {
			if err := s.store.SavePhotos(photos); err != nil {
				s.setStatus(func(st *Status) { st.Err = err })
			}
		} else {
			for i, p := range photos {
				p.ID = int64(start + i + 1)
			}
		}
		s.putPhotos(photos)
		s.queue.Push(photos...)

		n := s.catalogSize()
		s.setStatus(func(st *Status) { st.Cataloged = n })
	}

	s.setStatus(func(st *Status) {
		st.Reading = false
		st.Cataloged = s.catalogSize()
		st.Total = st.Cataloged
	})
}

func (s *Service) catalogSize() int {
	s.libMu.RLock()
	defer s.libMu.RUnlock()
	return len(s.byRel)
}

func (s *Service) forget(p *gallery.Photo) {
	if p == nil {
		return
	}
	s.index.Remove(int(p.ID))
	s.people.Remove(p.Rel)
	s.scanMu.Lock()
	delete(s.scans, p.Rel)
	s.scanMu.Unlock()
}

func (s *Service) indexLoop() {
	defer s.wg.Done()

	enc, err := NewEncoder(s.paths, 0)
	if err != nil {
		s.fail(err)
		return
	}
	s.enc = enc

	faces, err := NewFaceEncoder(s.paths, 0)
	if err != nil {
		s.setStatus(func(st *Status) { st.FaceErr = err })
	}
	s.faces = faces

	select {
	case <-s.catalogReady:
	case <-s.stop:
		return
	}
	if s.store != nil {
		s.loadStored()
	}

	s.setStatus(func(st *Status) {
		st.State = StateIndexing
		st.Indexed = s.done()
		st.Faces = s.people.faceCount()
	})

	s.wg.Add(1)
	go s.queryLoop()

	plane := 3 * ImageSize * ImageSize
	pixels := make([]float32, indexBatch*plane)
	images := make([]*image.RGBA, indexBatch)
	var perPhoto float64

	for {
		batch := s.nextBatch()
		if batch == nil {
			break
		}
		began := time.Now()

		var wg sync.WaitGroup
		for j, p := range batch {
			wg.Add(1)
			go func() {
				defer wg.Done()
				size := ImageSize * 2
				if s.needsFaces(p) {
					size = FaceSourceSize
				}
				images[j] = gallery.LoadScaled(p.Path, p.Orientation, size)
			}()
		}
		wg.Wait()

		if err := s.embedBatch(batch, images[:len(batch)], pixels); err != nil {
			s.fail(err)
			return
		}
		for j, p := range batch {
			if images[j] != nil && s.faces != nil && s.needsFaces(p) {
				if err := s.scanFaces(p, images[j]); err != nil {
					s.setStatus(func(st *Status) { st.FaceErr = err })
					s.faces.Close()
					s.faces = nil
				}
			}
			images[j] = nil
		}

		perPhoto = smoothRate(perPhoto, time.Since(began).Seconds()/float64(len(batch)))
		done := s.done()
		s.setStatus(func(st *Status) {
			st.Indexed = done
			st.Faces = s.people.faceCount()
			left := max(0, max(st.Total, st.Cataloged)-done)
			st.Remaining = time.Duration(perPhoto * float64(left) * float64(time.Second))
		})
	}

	if !s.stopped() {
		s.setStatus(func(st *Status) {
			st.State = StateReady
			st.Indexed = s.done()
			st.Remaining = 0
		})
	}
}

func (s *Service) nextBatch() []*gallery.Photo {
	var batch []*gallery.Photo
	for len(batch) < indexBatch {
		items := s.queue.Pop(indexBatch-len(batch), len(batch) == 0)
		if items == nil {
			break
		}
		for _, p := range items {
			if s.current(p) && s.needs(p) {
				batch = append(batch, p)
			}
		}
	}
	return batch
}

func (s *Service) loadStored() {
	s.store.Load(func(k Key, vec []float32) {
		if p := s.lookup(k.Path); p != nil && len(vec) == visionDim && fresh(p, k) {
			s.index.Add(int(p.ID), vec)
		}
	})

	if s.faces == nil {
		return
	}
	if scans, err := s.store.LoadFaceScans(); err == nil {
		s.scanMu.Lock()
		for rel, k := range scans {
			if p := s.lookup(rel); p != nil && fresh(p, k) {
				s.scans[rel] = k
			}
		}
		s.scanMu.Unlock()
	}
	s.store.LoadPeople(s.people)
}

func fresh(p *gallery.Photo, k Key) bool {
	return p.Size == k.Size && p.ModTime.Equal(k.ModTime)
}

func (s *Service) needsFaces(p *gallery.Photo) bool {
	if s.faces == nil {
		return false
	}
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	_, ok := s.scans[p.Rel]
	return !ok
}

func (s *Service) needs(p *gallery.Photo) bool {
	return !s.index.Has(int(p.ID)) || s.needsFaces(p)
}

func (s *Service) done() int {
	s.libMu.RLock()
	photos := make([]*gallery.Photo, 0, len(s.byRel))
	for _, p := range s.byRel {
		photos = append(photos, p)
	}
	s.libMu.RUnlock()

	n := 0
	for _, p := range photos {
		if !s.needs(p) {
			n++
		}
	}
	return n
}

func (s *Service) embedBatch(batch []*gallery.Photo, images []*image.RGBA, pixels []float32) error {
	plane := 3 * ImageSize * ImageSize
	var todo []*gallery.Photo
	for j, p := range batch {
		if images[j] != nil && !s.index.Has(int(p.ID)) {
			PreparePixelsFrom(images[j], pixels[len(todo)*plane:(len(todo)+1)*plane])
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		return nil
	}

	vecs, err := s.enc.EncodeImages(pixels, len(todo))
	if err != nil {
		return err
	}

	keys := make([]Key, len(todo))
	for j, p := range todo {
		keys[j] = Key{Path: p.Rel, Size: p.Size, ModTime: p.ModTime}
		s.index.Add(int(p.ID), vecs[j])
	}
	if s.store != nil {
		s.store.Save(keys, vecs)
	}
	return nil
}

func (s *Service) scanFaces(p *gallery.Photo, img *image.RGBA) error {
	results, err := s.faces.Process(img)
	if err != nil {
		return err
	}

	w := float32(img.Bounds().Dx())
	h := float32(img.Bounds().Dy())

	s.people.Remove(p.Rel)
	faces := make([]*Face, 0, len(results))
	for _, r := range results {
		d := r.Detection
		f := &Face{
			Path:      p.Rel,
			Box:       [4]float32{d.Box[0] / w, d.Box[1] / h, d.Box[2] / w, d.Box[3] / h},
			Score:     d.Score,
			Embedding: r.Embedding,
			Crop:      r.Crop,
		}
		for j := range 5 {
			f.Landmarks[j*2] = d.Landmarks[j*2] / w
			f.Landmarks[j*2+1] = d.Landmarks[j*2+1] / h
		}
		s.people.Assign(f)
		faces = append(faces, f)
	}

	k := Key{Path: p.Rel, Size: p.Size, ModTime: p.ModTime}
	if s.store != nil {
		if err := s.store.SaveFaces(k, faces); err != nil {
			return err
		}
	}
	s.people.Commit(p.Rel, faces)
	s.scanMu.Lock()
	s.scans[p.Rel] = k
	s.scanMu.Unlock()
	return nil
}

func (s *Service) queryLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stop:
			return
		case q := <-s.queries:
			r := s.search(q)
			select {
			case <-s.results:
			default:
			}
			s.results <- r
		}
	}
}

func (s *Service) search(q string) Result {
	start := time.Now()
	r := Result{Query: q}

	vec, err := s.enc.EncodeText(q)
	if err != nil {
		r.Err = err
		return r
	}

	hits := s.index.Search(vec, maxResults)
	if len(hits) > 0 {
		cut := max(minScore, hits[0].Score-spread)
		s.libMu.RLock()
		for _, h := range hits {
			if h.Score < cut {
				break
			}
			if p := s.byID[int64(h.ID)]; p != nil {
				r.Photos = append(r.Photos, p)
				r.Scores = append(r.Scores, h.Score)
			}
		}
		s.libMu.RUnlock()
	}
	r.Took = time.Since(start)
	return r
}

func smoothRate(current, sample float64) float64 {
	if current == 0 {
		return sample
	}
	return current*0.85 + sample*0.15
}

type photoQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  []*gallery.Photo
	closed bool
}

func newPhotoQueue() *photoQueue {
	q := &photoQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *photoQueue) Push(photos ...*gallery.Photo) {
	q.mu.Lock()
	q.items = append(q.items, photos...)
	q.mu.Unlock()
	q.cond.Broadcast()
}

func (q *photoQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
}

func (q *photoQueue) Pop(n int, wait bool) []*gallery.Photo {
	q.mu.Lock()
	defer q.mu.Unlock()
	for wait && len(q.items) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.items) == 0 {
		return nil
	}
	n = min(n, len(q.items))
	out := append([]*gallery.Photo(nil), q.items[:n]...)
	q.items = q.items[n:]
	return out
}
