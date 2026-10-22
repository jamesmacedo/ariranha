package gallery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/bmp"
)

type Kind int

const (
	KindPhoto Kind = iota
	KindVideo
)

var videoExtensions = map[string]bool{
	".mp4":  true,
	".m4v":  true,
	".mov":  true,
	".mkv":  true,
	".webm": true,
	".avi":  true,
	".3gp":  true,
}

const (
	probeTimeout = 20 * time.Second
	frameTimeout = 30 * time.Second
)

var (
	toolsMu   sync.RWMutex
	toolsDirs []string
	toolCache = map[string]string{}
)

func IsVideoFile(path string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(path))]
}

func (p *Photo) IsVideo() bool {
	return p.Kind == KindVideo
}

func SetToolsDir(dirs ...string) {
	toolsMu.Lock()
	toolsDirs = dirs
	toolCache = map[string]string{}
	toolsMu.Unlock()
}

func Tool(name string) (string, error) {
	toolsMu.RLock()
	path, ok := toolCache[name]
	dirs := toolsDirs
	toolsMu.RUnlock()
	if ok {
		if path == "" {
			return "", fmt.Errorf("%s not found", name)
		}
		return path, nil
	}

	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	for _, dir := range dirs {
		candidate := filepath.Join(dir, exe)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			path = candidate
			break
		}
	}
	if path == "" {
		path, _ = exec.LookPath(exe)
	}

	toolsMu.Lock()
	toolCache[name] = path
	toolsMu.Unlock()
	if path == "" {
		return "", fmt.Errorf("%s not found", name)
	}
	return path, nil
}

func runTool(timeout time.Duration, name string, args ...string) ([]byte, error) {
	bin, err := Tool(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	hideWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		if msg != "" {
			return nil, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}

type probeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		SAR       string `json:"sample_aspect_ratio"`
		Transfer  string `json:"color_transfer"`
		Tags      struct {
			Rotate       string `json:"rotate"`
			CreationTime string `json:"creation_time"`
		} `json:"tags"`
		SideData []struct {
			Rotation *float64 `json:"rotation"`
		} `json:"side_data_list"`
		Disposition struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func ReadVideo(file File) *Photo {
	out, err := runTool(probeTimeout, "ffprobe",
		"-v", "error",
		"-show_entries", "stream=codec_type,width,height,sample_aspect_ratio,color_transfer:stream_tags=rotate,creation_time:stream_side_data=rotation:stream_disposition=attached_pic:format=duration:format_tags",
		"-of", "json",
		file.Path)
	if err != nil {
		return nil
	}
	var probe probeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil
	}

	p := &Photo{
		Path:        file.Path,
		Rel:         file.Rel,
		Date:        file.ModTime,
		Size:        file.Size,
		ModTime:     file.ModTime,
		Orientation: 1,
		Kind:        KindVideo,
		Color:       LoadingColor,
	}

	hasVideo := false
	var streamDate string
	for _, s := range probe.Streams {
		switch s.CodecType {
		case "audio":
			p.HasAudio = true
		case "video":
			if hasVideo || s.Disposition.AttachedPic != 0 || s.Width <= 0 || s.Height <= 0 {
				continue
			}
			hasVideo = true
			p.Width, p.Height = s.Width, s.Height
			if num, den, ok := parseRatio(s.SAR); ok && num != den {
				p.Width = int(math.Round(float64(p.Width) * float64(num) / float64(den)))
			}
			rotation := 0.0
			if r, err := strconv.ParseFloat(s.Tags.Rotate, 64); err == nil {
				rotation = r
			}
			for _, sd := range s.SideData {
				if sd.Rotation != nil {
					rotation = *sd.Rotation
				}
			}
			if quarter := int(math.Round(rotation/90)) % 2; quarter != 0 {
				p.Width, p.Height = p.Height, p.Width
			}
			streamDate = s.Tags.CreationTime
			p.HDR = s.Transfer == "smpte2084" || s.Transfer == "arib-std-b67"
		}
	}
	if !hasVideo {
		return nil
	}

	if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil && d > 0 {
		p.Duration = time.Duration(d * float64(time.Second))
	}

	for _, value := range []string{
		probe.Format.Tags["com.apple.quicktime.creationdate"],
		probe.Format.Tags["creation_time"],
		streamDate,
	} {
		if t, ok := parseVideoDate(value); ok {
			p.Date = t.Local()
			break
		}
	}
	return p
}

func parseRatio(s string) (int, int, bool) {
	a, b, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, false
	}
	num, err1 := strconv.Atoi(a)
	den, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return 0, 0, false
	}
	return num, den, true
}

func parseVideoDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			if t.Year() < 1990 {
				return time.Time{}, false
			}
			return t, true
		}
	}
	return time.Time{}, false
}

func VideoPosterTime(p *Photo) time.Duration {
	switch {
	case p.Duration <= 0:
		return 0
	case p.Duration < 3*time.Second:
		return p.Duration / 3
	default:
		return time.Second
	}
}

func LoadVideoFrame(p *Photo, at time.Duration, maxSide int) *image.RGBA {
	type attempt struct {
		at      time.Duration
		tonemap bool
	}
	tries := []attempt{{at, p.HDR}}
	if p.HDR {
		tries = append(tries, attempt{at, false})
	}
	if at > 0 {
		tries = append(tries, attempt{0, false})
	}
	for _, try := range tries {
		if img := videoFrame(p, try.at, maxSide, try.tonemap); img != nil {
			return img
		}
	}
	return nil
}

const hdrFilter = "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv"

func videoFrame(p *Photo, at time.Duration, maxSide int, tonemap bool) *image.RGBA {
	filter := fmt.Sprintf("scale=w='min(iw,%d)':h='min(ih,%d)':force_original_aspect_ratio=decrease,setsar=1", maxSide, maxSide)
	if tonemap {
		filter += "," + hdrFilter
	}
	out, err := runTool(frameTimeout, "ffmpeg",
		"-v", "error",
		"-nostdin",
		"-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64),
		"-i", p.Path,
		"-map", "0:V:0",
		"-frames:v", "1",
		"-an", "-sn", "-dn",
		"-vf", filter,
		"-pix_fmt", "bgra",
		"-c:v", "bmp",
		"-f", "image2pipe",
		"-")
	if err != nil || len(out) == 0 {
		return nil
	}
	src, err := bmp.Decode(bytes.NewReader(out))
	if err != nil {
		return nil
	}
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func FormatDuration(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
