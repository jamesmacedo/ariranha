package gallery

import (
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	_ "golang.org/x/image/webp"
)

type Photo struct {
	ID          int64
	Path        string
	Rel         string
	Album       string
	Date        time.Time
	Size        int64
	ModTime     time.Time
	Width       int
	Height      int
	Orientation int
	Favorite    bool
	Kind        Kind
	Duration    time.Duration
	HasAudio    bool
	HDR         bool
	Color       color.RGBA
}

func (p *Photo) Aspect() float32 {
	if p.Width <= 0 || p.Height <= 0 {
		return 1
	}
	return float32(p.Width) / float32(p.Height)
}

type Album struct {
	Name   string
	Photos []*Photo
}

type Library struct {
	Root   string
	Photos []*Photo
	Albums []*Album
}

type File struct {
	Path    string
	Rel     string
	Size    int64
	ModTime time.Time
}

var extensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

var LoadingColor = color.RGBA{48, 49, 54, 255}

func Discover(root string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := strings.ToLower(filepath.Ext(path)); !extensions[ext] && !videoExtensions[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, File{Path: path, Rel: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })
	return files, err
}

func Scan(root string) (*Library, error) {
	files, err := Discover(root)
	if err != nil {
		return nil, err
	}
	return NewLibrary(root, ReadPhotos(files)), nil
}

func ReadPhotos(files []File) []*Photo {
	photos := make([]*Photo, len(files))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				photos[i] = ReadPhoto(files[i])
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	out := photos[:0]
	for _, p := range photos {
		if p != nil {
			out = append(out, p)
		}
	}
	return out
}

func ReadPhoto(file File) *Photo {
	if IsVideoFile(file.Path) {
		return ReadVideo(file)
	}
	f, err := os.Open(file.Path)
	if err != nil {
		return nil
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil
	}

	p := &Photo{
		Path:        file.Path,
		Rel:         file.Rel,
		Album:       AlbumOf(file.Rel),
		Date:        file.ModTime,
		Size:        file.Size,
		ModTime:     file.ModTime,
		Width:       cfg.Width,
		Height:      cfg.Height,
		Orientation: 1,
		Color:       LoadingColor,
	}

	ext := strings.ToLower(filepath.Ext(file.Path))
	if ext == ".jpg" || ext == ".jpeg" {
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			if x, err := exif.Decode(f); err == nil {
				if t, err := x.DateTime(); err == nil {
					p.Date = t
				}
				if tag, err := x.Get(exif.Orientation); err == nil {
					if o, err := tag.Int(0); err == nil && o >= 1 && o <= 8 {
						p.Orientation = o
					}
				}
			}
		}
	}

	if p.Orientation >= 5 {
		p.Width, p.Height = p.Height, p.Width
	}
	return p
}

func AlbumOf(rel string) string {
	if i := strings.IndexByte(rel, '/'); i > 0 {
		return rel[:i]
	}
	return ""
}

func NewLibrary(root string, photos []*Photo) *Library {
	l := &Library{Root: root, Photos: append([]*Photo(nil), photos...)}
	sort.SliceStable(l.Photos, func(i, j int) bool {
		if !l.Photos[i].Date.Equal(l.Photos[j].Date) {
			return l.Photos[i].Date.After(l.Photos[j].Date)
		}
		return l.Photos[i].Rel < l.Photos[j].Rel
	})

	byName := map[string]*Album{}
	for _, p := range l.Photos {
		if p.Album == "" {
			continue
		}
		a, ok := byName[p.Album]
		if !ok {
			a = &Album{Name: p.Album}
			byName[p.Album] = a
			l.Albums = append(l.Albums, a)
		}
		a.Photos = append(a.Photos, p)
	}
	return l
}
