package search

import (
	"database/sql"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"time"

	"ariranha/gallery"

	_ "modernc.org/sqlite"
)

type Store struct {
	db    *sql.DB
	model string
}

type Key struct {
	Path    string
	Size    int64
	ModTime time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS embeddings (
	path    TEXT    NOT NULL,
	model   TEXT    NOT NULL,
	size    INTEGER NOT NULL,
	mtime   INTEGER NOT NULL,
	dim     INTEGER NOT NULL,
	vector  BLOB    NOT NULL,
	PRIMARY KEY (path, model)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS persons (
	id         INTEGER PRIMARY KEY,
	name       TEXT    NOT NULL DEFAULT '',
	face_count INTEGER NOT NULL DEFAULT 0,
	cover_face INTEGER
);

CREATE TABLE IF NOT EXISTS faces (
	id        INTEGER PRIMARY KEY,
	path      TEXT    NOT NULL,
	x1 REAL NOT NULL, y1 REAL NOT NULL, x2 REAL NOT NULL, y2 REAL NOT NULL,
	landmarks BLOB    NOT NULL,
	score     REAL    NOT NULL,
	embedding BLOB    NOT NULL,
	crop      BLOB,
	person_id INTEGER REFERENCES persons(id),
	confirmed INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS faces_path ON faces(path);
CREATE INDEX IF NOT EXISTS faces_person ON faces(person_id);

CREATE TABLE IF NOT EXISTS photos (
	id          INTEGER PRIMARY KEY,
	path        TEXT    NOT NULL UNIQUE,
	size        INTEGER NOT NULL,
	mtime       INTEGER NOT NULL,
	taken       INTEGER NOT NULL,
	width       INTEGER NOT NULL,
	height      INTEGER NOT NULL,
	orientation INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS favorites (
	path  TEXT    PRIMARY KEY,
	added INTEGER NOT NULL
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS queries (
	text  TEXT    NOT NULL,
	model TEXT    NOT NULL,
	vec   BLOB    NOT NULL,
	used  INTEGER NOT NULL,
	PRIMARY KEY (text, model)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS face_scans (
	path  TEXT    PRIMARY KEY,
	size  INTEGER NOT NULL,
	mtime INTEGER NOT NULL,
	faces INTEGER NOT NULL
) WITHOUT ROWID;
`

func OpenStore(path, model string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, model: model}, nil
}

var photoColumns = []struct{ name, def string }{
	{"kind", "INTEGER NOT NULL DEFAULT 0"},
	{"duration", "INTEGER NOT NULL DEFAULT 0"},
	{"audio", "INTEGER NOT NULL DEFAULT 0"},
	{"hdr", "INTEGER NOT NULL DEFAULT 0"},
}

func migrate(db *sql.DB) error {
	rows, err := db.Query(`SELECT name FROM pragma_table_info('photos')`)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		have[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range photoColumns {
		if have[c.name] {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE photos ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Load(fn func(k Key, vec []float32)) error {
	rows, err := s.db.Query(`SELECT path, size, mtime, vector FROM embeddings WHERE model = ?`, s.model)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var k Key
		var mtime int64
		var blob []byte
		if err := rows.Scan(&k.Path, &k.Size, &mtime, &blob); err != nil {
			return err
		}
		k.ModTime = time.Unix(0, mtime)
		fn(k, decodeVector(blob))
	}
	return rows.Err()
}

func (s *Store) Save(keys []Key, vecs [][]float32) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO embeddings (path, model, size, mtime, dim, vector) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for i, k := range keys {
		if _, err := stmt.Exec(k.Path, s.model, k.Size, k.ModTime.UnixNano(), len(vecs[i]), encodeVector(vecs[i])); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func encodeVector(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func (s *Store) LoadFaceScans() (map[string]Key, error) {
	rows, err := s.db.Query(`SELECT path, size, mtime FROM face_scans`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]Key{}
	for rows.Next() {
		var k Key
		var mtime int64
		if err := rows.Scan(&k.Path, &k.Size, &mtime); err != nil {
			return nil, err
		}
		k.ModTime = time.Unix(0, mtime)
		out[k.Path] = k
	}
	return out, rows.Err()
}

func (s *Store) LoadPeople(pp *People) error {
	rows, err := s.db.Query(`SELECT id, name FROM persons`)
	if err != nil {
		return err
	}
	for rows.Next() {
		p := &Person{}
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			rows.Close()
			return err
		}
		pp.restorePerson(p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = s.db.Query(`SELECT id, path, x1, y1, x2, y2, landmarks, score, embedding, COALESCE(person_id, 0) FROM faces`)
	if err != nil {
		return err
	}
	for rows.Next() {
		f := &Face{}
		var lm, emb []byte
		var personID int64
		if err := rows.Scan(&f.ID, &f.Path, &f.Box[0], &f.Box[1], &f.Box[2], &f.Box[3], &lm, &f.Score, &emb, &personID); err != nil {
			rows.Close()
			return err
		}
		copy(f.Landmarks[:], decodeVector(lm))
		f.Embedding = decodeVector(emb)
		pp.restoreFace(f, personID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	pp.mu.Lock()
	defer pp.mu.Unlock()
	for _, p := range pp.persons {
		if p.Cover == nil {
			continue
		}
		s.db.QueryRow(`SELECT crop FROM faces WHERE id = ?`, p.Cover.ID).Scan(&p.CoverCrop)
	}
	return nil
}

func (s *Store) SaveFaces(k Key, faces []*Face) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM faces WHERE path = ?`, k.Path); err != nil {
		return err
	}

	touched := map[*Person]bool{}
	for _, f := range faces {
		p := f.Person
		if p != nil && p.ID == 0 {
			res, err := tx.Exec(`INSERT INTO persons (name) VALUES ('')`)
			if err != nil {
				return err
			}
			if p.ID, err = res.LastInsertId(); err != nil {
				return err
			}
		}
		var personID any
		if p != nil {
			personID = p.ID
			touched[p] = true
		}
		res, err := tx.Exec(`INSERT INTO faces (path, x1, y1, x2, y2, landmarks, score, embedding, crop, person_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.Path, f.Box[0], f.Box[1], f.Box[2], f.Box[3], encodeVector(f.Landmarks[:]), f.Score, encodeVector(f.Embedding), f.Crop, personID)
		if err != nil {
			return err
		}
		if f.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	}

	for p := range touched {
		var cover any
		if p.Cover != nil {
			cover = p.Cover.ID
		}
		if _, err := tx.Exec(`UPDATE persons SET face_count = ?, cover_face = ? WHERE id = ?`, p.Count, cover, p.ID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`INSERT OR REPLACE INTO face_scans (path, size, mtime, faces) VALUES (?, ?, ?, ?)`,
		k.Path, k.Size, k.ModTime.UnixNano(), len(faces)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LoadPhotos(root string) ([]*gallery.Photo, error) {
	rows, err := s.db.Query(`SELECT p.id, p.path, p.size, p.mtime, p.taken, p.width, p.height, p.orientation, p.kind, p.duration, p.audio, p.hdr, f.path IS NOT NULL FROM photos p LEFT JOIN favorites f ON f.path = p.path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*gallery.Photo
	for rows.Next() {
		p := &gallery.Photo{Color: gallery.LoadingColor}
		var mtime, taken, duration int64
		if err := rows.Scan(&p.ID, &p.Rel, &p.Size, &mtime, &taken, &p.Width, &p.Height, &p.Orientation, &p.Kind, &duration, &p.HasAudio, &p.HDR, &p.Favorite); err != nil {
			return nil, err
		}
		p.Path = filepath.Join(root, filepath.FromSlash(p.Rel))
		p.Album = gallery.AlbumOf(p.Rel)
		p.ModTime = time.Unix(0, mtime)
		p.Date = time.Unix(0, taken)
		p.Duration = time.Duration(duration)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) SavePhotos(photos []*gallery.Photo) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO photos (path, size, mtime, taken, width, height, orientation, kind, duration, audio, hdr) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET size = excluded.size, mtime = excluded.mtime, taken = excluded.taken,
		width = excluded.width, height = excluded.height, orientation = excluded.orientation,
		kind = excluded.kind, duration = excluded.duration, audio = excluded.audio, hdr = excluded.hdr
		RETURNING id`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range photos {
		if err := stmt.QueryRow(p.Rel, p.Size, p.ModTime.UnixNano(), p.Date.UnixNano(), p.Width, p.Height, p.Orientation, p.Kind, int64(p.Duration), p.HasAudio, p.HDR).Scan(&p.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetFavorite(rel string, fav bool) error {
	if fav {
		_, err := s.db.Exec(`INSERT INTO favorites (path, added) VALUES (?, ?) ON CONFLICT(path) DO NOTHING`, rel, time.Now().UnixNano())
		return err
	}
	_, err := s.db.Exec(`DELETE FROM favorites WHERE path = ?`, rel)
	return err
}

func (s *Store) DeletePhotos(rels []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, rel := range rels {
		for _, q := range []string{
			`DELETE FROM photos WHERE path = ?`,
			`DELETE FROM embeddings WHERE path = ?`,
			`DELETE FROM faces WHERE path = ?`,
			`DELETE FROM face_scans WHERE path = ?`,
			`DELETE FROM favorites WHERE path = ?`,
		} {
			if _, err := tx.Exec(q, rel); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

const maxCachedQueries = 5000

func (s *Store) LoadQuery(text string) ([]float32, bool) {
	var blob []byte
	if err := s.db.QueryRow(`SELECT vec FROM queries WHERE text = ? AND model = ?`, text, s.model).Scan(&blob); err != nil {
		return nil, false
	}
	vec := decodeVector(blob)
	if len(vec) != visionDim {
		return nil, false
	}
	s.db.Exec(`UPDATE queries SET used = ? WHERE text = ? AND model = ?`, time.Now().UnixNano(), text, s.model)
	return vec, true
}

func (s *Store) SaveQuery(text string, vec []float32) error {
	if _, err := s.db.Exec(`INSERT INTO queries (text, model, vec, used) VALUES (?, ?, ?, ?)
		ON CONFLICT(text, model) DO UPDATE SET vec = excluded.vec, used = excluded.used`,
		text, s.model, encodeVector(vec), time.Now().UnixNano()); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM queries WHERE used < (SELECT used FROM queries ORDER BY used DESC LIMIT 1 OFFSET ?)`, maxCachedQueries-1)
	return err
}
