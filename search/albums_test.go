package search

import (
	"path/filepath"
	"testing"
	"time"

	"ariranha/gallery"
)

func TestAlbums(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "index.db")
	newService := func() *Service {
		s := &Service{byID: map[int64]*gallery.Photo{}, byRel: map[string]*gallery.Photo{}}
		store, err := OpenStore(db, "test")
		if err != nil {
			t.Fatal(err)
		}
		s.store = store
		s.loadAlbums()
		return s
	}

	s := newService()
	now := time.Now()
	photos := []*gallery.Photo{
		{ID: 1, Rel: "a.jpg", Date: now},
		{ID: 2, Rel: "b.jpg", Date: now.Add(-time.Hour)},
		{ID: 3, Rel: "c.jpg", Date: now.Add(-2 * time.Hour)},
	}
	s.putPhotos(photos)

	if _, err := s.CreateAlbum("   ", "", 0); err == nil {
		t.Fatal("empty name accepted")
	}
	id, err := s.CreateAlbum("  Praia   2026 ", " férias ", 3)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.AddToAlbum(id, []*gallery.Photo{photos[2], photos[0], photos[0]}); err != nil || n != 2 {
		t.Fatalf("add: n=%d err=%v", n, err)
	}
	if n, _ := s.AddToAlbum(id, photos); n != 1 {
		t.Fatalf("add again: n=%d", n)
	}
	if n, _ := s.RemoveFromAlbum(id, photos[1:2]); n != 1 {
		t.Fatalf("remove: n=%d", n)
	}
	if err := s.UpdateAlbum(id, "Praia", "verão", 5); err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateAlbum("Outro", "", 1)
	s.AddToAlbum(other, photos[:1])
	s.store.Close()

	s = newService()
	defer s.store.Close()
	s.putPhotos(photos)
	albums := s.Albums()
	if len(albums) != 2 || albums[0].ID != other {
		t.Fatalf("albums order: %+v", albums)
	}
	a := albums[1]
	if a.Name != "Praia" || a.Description != "verão" || a.Color != 5 {
		t.Fatalf("album fields: %+v", a)
	}
	if len(a.Photos) != 2 || a.Photos[0].Rel != "a.jpg" || a.Photos[1].Rel != "c.jpg" {
		t.Fatalf("album photos: %v", a.Photos)
	}
	if in := s.AlbumsOf(photos[0]); len(in) != 2 {
		t.Fatalf("albums of a.jpg: %d", len(in))
	}
	if err := s.DeleteAlbum(id); err != nil {
		t.Fatal(err)
	}
	if in := s.AlbumsOf(photos[2]); len(in) != 0 {
		t.Fatalf("deleted album still listed")
	}
	recs, _ := s.store.LoadAlbums()
	if len(recs) != 1 {
		t.Fatalf("store has %d albums", len(recs))
	}
}
