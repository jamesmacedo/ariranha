package search

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"ariranha/gallery"
)

const (
	MaxAlbumName        = 60
	MaxAlbumDescription = 200
)

func (s *Service) loadAlbums() {
	if s.store == nil {
		return
	}
	albums, err := s.store.LoadAlbums()
	if err != nil {
		s.setStatus(func(st *Status) { st.Err = err })
		return
	}
	s.albums = albums
	for _, a := range albums {
		s.nextAlbum = max(s.nextAlbum, a.ID)
	}
}

func (s *Service) AlbumsVersion() int {
	s.albumMu.Lock()
	defer s.albumMu.Unlock()
	return s.albumVersion
}

func (s *Service) Albums() []*gallery.Album {
	photos := s.Library().Photos
	s.albumMu.Lock()
	defer s.albumMu.Unlock()
	out := make([]*gallery.Album, 0, len(s.albums))
	for _, r := range s.albums {
		a := albumOf(r)
		for _, p := range photos {
			if r.Paths[p.Rel] {
				a.Photos = append(a.Photos, p)
			}
		}
		out = append(out, a)
	}
	return out
}

func (s *Service) AlbumsOf(p *gallery.Photo) []*gallery.Album {
	s.albumMu.Lock()
	defer s.albumMu.Unlock()
	var out []*gallery.Album
	for _, r := range s.albums {
		if r.Paths[p.Rel] {
			out = append(out, albumOf(r))
		}
	}
	return out
}

func albumOf(r *AlbumRecord) *gallery.Album {
	return &gallery.Album{ID: r.ID, Name: r.Name, Description: r.Description, Color: r.Color, Created: r.Created}
}

func cleanAlbum(name, description string) (string, string, error) {
	name = strings.Join(strings.Fields(name), " ")
	description = strings.TrimSpace(description)
	switch {
	case name == "":
		return "", "", errors.New("album name is empty")
	case utf8.RuneCountInString(name) > MaxAlbumName:
		return "", "", errors.New("album name is too long")
	case utf8.RuneCountInString(description) > MaxAlbumDescription:
		return "", "", errors.New("description is too long")
	}
	return name, description, nil
}

func (s *Service) CreateAlbum(name, description string, color int) (int64, error) {
	name, description, err := cleanAlbum(name, description)
	if err != nil {
		return 0, err
	}
	s.albumMu.Lock()
	defer s.albumMu.Unlock()
	now := time.Now()
	var id int64
	if s.store != nil {
		if id, err = s.store.CreateAlbum(name, description, color, now); err != nil {
			return 0, err
		}
	} else {
		s.nextAlbum++
		id = s.nextAlbum
	}
	r := &AlbumRecord{ID: id, Name: name, Description: description, Color: color, Created: now, Paths: map[string]bool{}}
	s.albums = append([]*AlbumRecord{r}, s.albums...)
	s.albumVersion++
	return id, nil
}

func (s *Service) UpdateAlbum(id int64, name, description string, color int) error {
	name, description, err := cleanAlbum(name, description)
	if err != nil {
		return err
	}
	s.albumMu.Lock()
	defer s.albumMu.Unlock()
	r := s.album(id)
	if r == nil {
		return errors.New("album not found")
	}
	if s.store != nil {
		if err := s.store.UpdateAlbum(id, name, description, color); err != nil {
			return err
		}
	}
	r.Name, r.Description, r.Color = name, description, color
	s.albumVersion++
	return nil
}

func (s *Service) DeleteAlbum(id int64) error {
	s.albumMu.Lock()
	defer s.albumMu.Unlock()
	if s.store != nil {
		if err := s.store.DeleteAlbum(id); err != nil {
			return err
		}
	}
	for i, r := range s.albums {
		if r.ID == id {
			s.albums = append(s.albums[:i:i], s.albums[i+1:]...)
			break
		}
	}
	s.albumVersion++
	return nil
}

func (s *Service) AddToAlbum(id int64, photos []*gallery.Photo) (int, error) {
	return s.setAlbumPhotos(id, photos, true)
}

func (s *Service) RemoveFromAlbum(id int64, photos []*gallery.Photo) (int, error) {
	return s.setAlbumPhotos(id, photos, false)
}

func (s *Service) setAlbumPhotos(id int64, photos []*gallery.Photo, add bool) (int, error) {
	s.albumMu.Lock()
	defer s.albumMu.Unlock()
	r := s.album(id)
	if r == nil {
		return 0, errors.New("album not found")
	}
	var paths []string
	seen := map[string]bool{}
	for _, p := range photos {
		if r.Paths[p.Rel] != add && !seen[p.Rel] {
			seen[p.Rel] = true
			paths = append(paths, p.Rel)
		}
	}
	if len(paths) == 0 {
		return 0, nil
	}
	if s.store != nil {
		if err := s.store.SetAlbumPhotos(id, paths, add); err != nil {
			return 0, err
		}
	}
	for _, p := range paths {
		if add {
			r.Paths[p] = true
		} else {
			delete(r.Paths, p)
		}
	}
	s.albumVersion++
	return len(paths), nil
}

func (s *Service) album(id int64) *AlbumRecord {
	for _, r := range s.albums {
		if r.ID == id {
			return r
		}
	}
	return nil
}
