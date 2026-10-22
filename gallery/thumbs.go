package gallery

import (
	"image"
	"runtime"
	"sort"
	"sync"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	ramTTL = 2 * time.Second
	gpuTTL = 3 * time.Second
)

type gpuEntry struct {
	tex      rl.Texture2D
	lastUsed uint64
	seen     time.Time
}

type ramEntry struct {
	img      *image.RGBA
	bytes    int64
	lastUsed uint64
	seen     time.Time
}

type thumbResult struct {
	photo *Photo
	img   *image.RGBA
}

type ThumbCache struct {
	maxSide     int
	gpuLimit    int
	ramBudget   int64
	uploadLimit int

	mu      sync.Mutex
	cond    *sync.Cond
	queue   []*Photo
	pending map[*Photo]bool
	closed  bool

	results  chan thumbResult
	gpu      map[*Photo]*gpuEntry
	ram      map[*Photo]*ramEntry
	failed   map[*Photo]bool
	ramBytes int64
	frame    uint64
	now      time.Time
	uploads  int
}

func NewThumbCache(maxSide, gpuLimit int, ramBudget int64) *ThumbCache {
	c := &ThumbCache{
		maxSide:     maxSide,
		gpuLimit:    gpuLimit,
		ramBudget:   ramBudget,
		uploadLimit: 16,
		pending:     map[*Photo]bool{},
		results:     make(chan thumbResult, 256),
		gpu:         map[*Photo]*gpuEntry{},
		ram:         map[*Photo]*ramEntry{},
		failed:      map[*Photo]bool{},
	}
	c.cond = sync.NewCond(&c.mu)
	for range max(2, runtime.NumCPU()-1) {
		go c.worker()
	}
	return c
}

func (c *ThumbCache) worker() {
	for {
		c.mu.Lock()
		for len(c.queue) == 0 && !c.closed {
			c.cond.Wait()
		}
		if c.closed {
			c.mu.Unlock()
			return
		}
		p := c.queue[0]
		c.queue = c.queue[1:]
		c.mu.Unlock()

		c.results <- thumbResult{photo: p, img: LoadPreview(p, c.maxSide)}
	}
}

func (c *ThumbCache) BeginFrame() {
	c.frame++
	c.now = time.Now()
	c.uploads = 0

	c.mu.Lock()
	for _, p := range c.queue {
		delete(c.pending, p)
	}
	c.queue = c.queue[:0]
	c.mu.Unlock()

	for range 128 {
		select {
		case r := <-c.results:
			c.store(r)
			continue
		default:
		}
		break
	}
	c.evictRAM()
	c.evictGPU()
}

func (c *ThumbCache) store(r thumbResult) {
	c.mu.Lock()
	delete(c.pending, r.photo)
	c.mu.Unlock()

	if r.img == nil {
		c.failed[r.photo] = true
		return
	}
	size := int64(len(r.img.Pix))
	if old, ok := c.ram[r.photo]; ok {
		c.ramBytes -= old.bytes
	}
	c.ram[r.photo] = &ramEntry{img: r.img, bytes: size, lastUsed: c.frame, seen: c.now}
	c.ramBytes += size
}

func (c *ThumbCache) evictRAM() {
	for p, e := range c.ram {
		if c.now.Sub(e.seen) > ramTTL {
			c.ramBytes -= e.bytes
			delete(c.ram, p)
		}
	}
	if c.ramBytes <= c.ramBudget {
		return
	}
	type candidate struct {
		photo *Photo
		used  uint64
	}
	list := make([]candidate, 0, len(c.ram))
	for p, e := range c.ram {
		if e.lastUsed < c.frame {
			list = append(list, candidate{p, e.lastUsed})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].used < list[j].used })
	for _, it := range list {
		if c.ramBytes <= c.ramBudget*9/10 {
			break
		}
		c.ramBytes -= c.ram[it.photo].bytes
		delete(c.ram, it.photo)
	}
}

func (c *ThumbCache) evictGPU() {
	for p, e := range c.gpu {
		if c.now.Sub(e.seen) > gpuTTL {
			rl.UnloadTexture(e.tex)
			delete(c.gpu, p)
		}
	}
	if len(c.gpu) <= c.gpuLimit {
		return
	}
	type candidate struct {
		photo *Photo
		used  uint64
	}
	var list []candidate
	for p, e := range c.gpu {
		if e.lastUsed+1 < c.frame {
			list = append(list, candidate{p, e.lastUsed})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].used < list[j].used })
	for _, it := range list {
		if len(c.gpu) <= c.gpuLimit {
			break
		}
		rl.UnloadTexture(c.gpu[it.photo].tex)
		delete(c.gpu, it.photo)
	}
}

func (c *ThumbCache) upload(p *Photo, img *image.RGBA) rl.Texture2D {
	b := img.Bounds()
	rimg := rl.NewImage(img.Pix, int32(b.Dx()), int32(b.Dy()), 1, rl.UncompressedR8g8b8a8)
	tex := rl.LoadTextureFromImage(rimg)
	rl.GenTextureMipmaps(&tex)
	rl.SetTextureFilter(tex, rl.FilterTrilinear)
	c.gpu[p] = &gpuEntry{tex: tex, lastUsed: c.frame, seen: c.now}
	if r, ok := c.ram[p]; ok {
		c.ramBytes -= r.bytes
		delete(c.ram, p)
	}
	c.uploads++
	return tex
}

func (c *ThumbCache) request(p *Photo) {
	c.mu.Lock()
	if !c.pending[p] {
		c.pending[p] = true
		c.queue = append(c.queue, p)
		c.cond.Signal()
	}
	c.mu.Unlock()
}

func (c *ThumbCache) Get(p *Photo) (rl.Texture2D, bool) {
	if p.Path == "" || c.failed[p] {
		return rl.Texture2D{}, false
	}
	if e, ok := c.gpu[p]; ok {
		c.touchGPU(e)
		return e.tex, true
	}
	if r, ok := c.ram[p]; ok {
		c.touchRAM(r)
		if c.uploads < c.uploadLimit {
			return c.upload(p, r.img), true
		}
		return rl.Texture2D{}, false
	}
	c.request(p)
	return rl.Texture2D{}, false
}

func (c *ThumbCache) Prefetch(p *Photo) {
	if p.Path == "" || c.failed[p] {
		return
	}
	if e, ok := c.gpu[p]; ok {
		c.touchGPU(e)
		return
	}
	if r, ok := c.ram[p]; ok {
		c.touchRAM(r)
		return
	}
	c.request(p)
}

func (c *ThumbCache) touchGPU(e *gpuEntry) {
	e.lastUsed = c.frame
	e.seen = c.now
}

func (c *ThumbCache) touchRAM(e *ramEntry) {
	e.lastUsed = c.frame
	e.seen = c.now
}

func (c *ThumbCache) SetUploadLimit(n int) {
	c.uploadLimit = n
}

func (c *ThumbCache) Stats() (ramMB float64, ramCount, gpuCount int) {
	return float64(c.ramBytes) / (1 << 20), len(c.ram), len(c.gpu)
}

func (c *ThumbCache) Close() {
	c.mu.Lock()
	c.closed = true
	c.queue = nil
	c.cond.Broadcast()
	c.mu.Unlock()

	for p, e := range c.gpu {
		rl.UnloadTexture(e.tex)
		delete(c.gpu, p)
	}
	c.ram = nil
}

func orient(src *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return src
	}

	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			si := src.PixOffset(x, y)
			di := dst.PixOffset(dx, dy)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}
