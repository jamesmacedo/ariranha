package appdir

import (
	"os"
	"path/filepath"
	"strings"
)

type Dirs struct {
	Root    string
	Storage string
	System  string
	Models  string
	Runtime string
	DB      string
	Cache   string
	Logs    string
}

func Locate() (Dirs, error) {
	root, err := root()
	if err != nil {
		return Dirs{}, err
	}
	system := filepath.Join(root, "system")
	return Dirs{
		Root:    root,
		Storage: filepath.Join(root, "storage"),
		System:  system,
		Models:  filepath.Join(system, "models"),
		Runtime: filepath.Join(system, "runtime"),
		DB:      filepath.Join(system, "db"),
		Cache:   filepath.Join(system, "cache"),
		Logs:    filepath.Join(system, "logs"),
	}, nil
}

func root() (string, error) {
	if r := os.Getenv("ARIRANHA_ROOT"); r != "" {
		return filepath.Abs(r)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	if strings.Contains(dir, "go-build") {
		return os.Getwd()
	}
	return dir, nil
}

func (d Dirs) Ensure() error {
	for _, dir := range []string{d.Storage, d.Models, d.Runtime, d.DB, d.Cache, d.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (d Dirs) Database() string {
	return filepath.Join(d.DB, "index.db")
}

func (d Dirs) LogFile() string {
	return filepath.Join(d.Logs, "ariranha.log")
}

func (d Dirs) Relative(path string) string {
	rel, err := filepath.Rel(d.Root, path)
	if err != nil {
		return path
	}
	return "." + string(filepath.Separator) + rel
}
