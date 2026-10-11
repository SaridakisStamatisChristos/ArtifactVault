package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/db"
)

func copyAndHash(dst *os.File, src io.Reader, max int64) (int64, string, error) {
	h := sha256.New()
	var r io.Reader = src
	if max > 0 {
		r = &io.LimitedReader{R: src, N: max + 1}
	}
	n, e := io.CopyBuffer(io.MultiWriter(dst, h), r, make([]byte, 64*1024))
	if e != nil {
		return n, "", e
	}
	if max > 0 && n > max {
		return n, "", ErrTooLarge
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
func hashFile(path string) (int64, string, error) {
	f, e := os.Open(path)
	if e != nil {
		return 0, "", e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.CopyBuffer(h, f, make([]byte, 64*1024))
	if e != nil {
		return n, "", e
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = f.Sync(); e != nil {
		return e
	}
	return nil
}

func (s *Store) objectRow(sha string) (size int64, created, rel string, unref *string, found bool, err error) {
	r, e := s.db.Query(`SELECT size_bytes,created_at,stored_relpath,unreferenced_at FROM objects WHERE sha256=?`, sha)
	if e != nil {
		return 0, "", "", nil, false, e
	}
	defer r.Close()
	if !r.Next() {
		return 0, "", "", nil, false, nil
	}
	size = r.Int64(0)
	created = r.Text(1)
	rel = r.Text(2)
	if !r.Null(3) {
		v := r.Text(3)
		unref = &v
	}
	return size, created, rel, unref, true, r.Err()
}
func (s *Store) finalFileValid(sha string, expectedSize int64) (bool, error) {
	p := s.expectedPath(sha)
	n, h, e := hashFile(p)
	if e != nil {
		if os.IsNotExist(e) {
			return false, nil
		}
		return false, e
	}
	if h != sha || n != expectedSize {
		return false, ErrCorruptExisting
	}
	return true, nil
}

func (s *Store) Upload(src io.Reader, m MetadataInput) (UploadResult, error) {
	var zero UploadResult
	if e := s.ValidateMetadata(&m); e != nil {
		return zero, e
	}
	tf, e := os.CreateTemp(filepath.Join(s.cfg.StorageRoot, "staging"), "upload-*.part")
	if e != nil {
		return zero, e
	}
	stage := tf.Name()
	keep := false
	defer func() {
		_ = tf.Close()
		if !keep {
			_ = os.Remove(stage)
		}
	}()
	n, sha, e := copyAndHash(tf, src, s.cfg.MaxArtifactBytes)
	if e != nil {
		return zero, e
	}
	if e = tf.Sync(); e != nil {
		return zero, e
	}
	if e = tf.Close(); e != nil {
		return zero, e
	}
	id, e := randomID()
	if e != nil {
		return zero, e
	}
	created := nowText(s.now())
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	dbSize, _, dbRel, _, dbFound, e := s.objectRow(sha)
	if e != nil {
		return zero, e
	}
	final := s.expectedPath(sha)
	dedup := false
	createdPhysical := false
	preexistingPhysical := false
	if st, e := os.Stat(final); e == nil && !st.IsDir() {
		preexistingPhysical = true
		fn, fh, he := hashFile(final)
		if he != nil {
			return zero, he
		}
		if fh != sha || fn != n {
			return zero, ErrCorruptExisting
		}
		dedup = true
	} else if e != nil && !os.IsNotExist(e) {
		return zero, e
	}
	if dbFound {
		if dbRel != expectedRel(sha) || dbSize != n {
			return zero, ErrMetadataInconsistency
		}
		if !preexistingPhysical {
			return zero, ErrMetadataInconsistency
		}
		dedup = true
	}
	if !preexistingPhysical {
		if e := os.MkdirAll(filepath.Dir(final), 0o750); e != nil {
			return zero, e
		}
		if e := os.Link(stage, final); e != nil {
			if errors.Is(e, syscall.EEXIST) {
				ok, ve := s.finalFileValid(sha, n)
				if ve != nil || !ok {
					return zero, coalesce(ve, ErrCorruptExisting)
				}
				dedup = true
			} else {
				return zero, fmt.Errorf("atomic finalize: %w", e)
			}
		} else {
			createdPhysical = true
		}
		if e := syncDir(filepath.Dir(final)); e != nil {
			if createdPhysical {
				_ = os.Remove(final)
			}
			return zero, fmt.Errorf("durable finalize directory sync: %w", e)
		}
		_ = os.Remove(stage)
	}
	if s.beforeMetadataCommit != nil {
		if e := s.beforeMetadataCommit(); e != nil {
			if createdPhysical {
				_ = os.Remove(final)
			}
			return zero, e
		}
	}
	e = s.db.WithTx(func(tx *db.Tx) error {
		if _, e := tx.Exec(`INSERT OR IGNORE INTO objects(sha256,size_bytes,created_at,stored_relpath,unreferenced_at) VALUES(?,?,?,?,NULL)`, sha, n, created, expectedRel(sha)); e != nil {
			return e
		}
		var qSize int64
		var qRel string
		if e := tx.QueryOne(`SELECT size_bytes,stored_relpath FROM objects WHERE sha256=?`, []any{sha}, func(r *db.Rows) error { qSize = r.Int64(0); qRel = r.Text(1); return nil }); e != nil {
			return e
		}
		if qSize != n || qRel != expectedRel(sha) {
			return ErrMetadataInconsistency
		}
		if _, e := tx.Exec(`UPDATE objects SET unreferenced_at=NULL WHERE sha256=?`, sha); e != nil {
			return e
		}
		if _, e := tx.Exec(`INSERT INTO artifact_records(id,sha256,created_at,original_filename,content_type,description,deleted_at) VALUES(?,?,?,?,?,?,NULL)`, id, sha, created, m.Filename, nullIfEmpty(m.ContentType), nullIfEmpty(m.Description)); e != nil {
			return e
		}
		for _, t := range m.Tags {
			if _, e := tx.Exec(`INSERT INTO tags(record_id,tag) VALUES(?,?)`, id, t); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		if createdPhysical {
			_ = os.Remove(final)
			_ = syncDir(filepath.Dir(final))
		}
		return zero, e
	}
	keep = false
	return UploadResult{Record: Record{ID: id, SHA256: sha, SizeBytes: n, CreatedAt: created, OriginalFilename: m.Filename, ContentType: m.ContentType, Description: m.Description, Tags: m.Tags}, Deduplicated: dedup}, nil
}
func coalesce(e, error2 error) error {
	if e != nil {
		return e
	}
	return error2
}
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
