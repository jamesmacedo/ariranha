package gallery

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
)

const (
	diskMaxPhotoSide = 512
	diskQuality      = 85
)

var (
	diskMu  sync.RWMutex
	diskDir string
)

func SetDiskCache(dir string) {
	if dir != "" {
		os.MkdirAll(dir, 0o755)
	}
	diskMu.Lock()
	diskDir = dir
	diskMu.Unlock()
}

func diskPath(p *Photo, maxSide int) string {
	diskMu.RLock()
	dir := diskDir
	diskMu.RUnlock()
	if dir == "" {
		return ""
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d", p.Rel, p.Size, p.ModTime.UnixNano(), maxSide)))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(dir, name[:2], name+".jpg")
}

func loadDisk(p *Photo, maxSide int) *image.RGBA {
	path := diskPath(p, maxSide)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, err := jpeg.Decode(f)
	if err != nil {
		return nil
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func saveDisk(p *Photo, maxSide int, img *image.RGBA) {
	path := diskPath(p, maxSide)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".part-*")
	if err != nil {
		return
	}
	tmp := f.Name()
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: diskQuality})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil || os.Rename(tmp, path) != nil {
		os.Remove(tmp)
	}
}
