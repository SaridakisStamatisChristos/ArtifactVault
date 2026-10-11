package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/db"
)

func (s *Store) Delete(sha, recordID string) (DeleteResult, error) {
	sha, e := NormalizeHash(sha)
	if e != nil {
		return DeleteResult{}, e
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	recs, e := s.activeRecords(sha)
	if e != nil {
		return DeleteResult{}, e
	}
	if len(recs) == 0 {
		return DeleteResult{}, ErrNotFound
	}
	var id string
	if recordID == "" {
		if len(recs) > 1 {
			return DeleteResult{}, ErrAmbiguousDelete
		}
		id = recs[0].ID
	} else {
		for _, r := range recs {
			if r.ID == recordID {
				id = recordID
				break
			}
		}
		if id == "" {
			return DeleteResult{}, ErrNotFound
		}
	}
	deletedAt := nowText(s.now())
	var remain int64
	e = s.db.WithTx(func(tx *db.Tx) error {
		if n, e := tx.Exec(`UPDATE artifact_records SET deleted_at=? WHERE id=? AND sha256=? AND deleted_at IS NULL`, deletedAt, id, sha); e != nil {
			return e
		} else if n != 1 {
			return ErrNotFound
		}
		if e := tx.QueryOne(`SELECT COUNT(*) FROM artifact_records WHERE sha256=? AND deleted_at IS NULL`, []any{sha}, func(r *db.Rows) error { remain = r.Int64(0); return nil }); e != nil {
			return e
		}
		if remain == 0 {
			_, e := tx.Exec(`UPDATE objects SET unreferenced_at=? WHERE sha256=?`, deletedAt, sha)
			return e
		}
		return nil
	})
	if e != nil {
		return DeleteResult{}, e
	}
	out := DeleteResult{SHA256: sha, DeletedRecordID: id, RemainingActiveRecords: remain}
	if remain == 0 {
		out.UnreferencedAt = deletedAt
	}
	return out, nil
}
func (s *Store) GC(grace time.Duration) (GCResult, error) {
	if grace < 0 {
		grace = s.cfg.GCGrace
	}
	cut := nowText(s.now().Add(-grace))
	r, e := s.db.Query(`SELECT sha256 FROM objects WHERE unreferenced_at IS NOT NULL AND unreferenced_at<=? ORDER BY unreferenced_at`, cut)
	if e != nil {
		return GCResult{}, e
	}
	var shas []string
	for r.Next() {
		shas = append(shas, r.Text(0))
	}
	e = r.Err()
	r.Close()
	if e != nil {
		return GCResult{}, e
	}
	res := GCResult{Eligible: len(shas)}
	for _, sha := range shas {
		s.lifecycle.Lock()
		err := s.gcOne(sha, cut)
		s.lifecycle.Unlock()
		if err != nil {
			res.Skipped++
			res.Errors = append(res.Errors, sha+": "+err.Error())
		} else {
			res.Deleted++
		}
	}
	return res, nil
}
func (s *Store) gcOne(sha, cut string) error {
	var active int64
	var unref string
	r, e := s.db.Query(`SELECT o.unreferenced_at,(SELECT COUNT(*) FROM artifact_records r WHERE r.sha256=o.sha256 AND r.deleted_at IS NULL) FROM objects o WHERE o.sha256=?`, sha)
	if e != nil {
		return e
	}
	if !r.Next() {
		r.Close()
		return nil
	}
	unref = r.Text(0)
	active = r.Int64(1)
	r.Close()
	if active != 0 || unref == "" || unref > cut {
		return fmt.Errorf("no longer eligible")
	}
	final := s.expectedPath(sha)
	if _, e := os.Stat(final); e != nil {
		return fmt.Errorf("physical object unavailable: %w", e)
	}
	rid, e := randomID()
	if e != nil {
		return e
	}
	trash := filepath.Join(s.cfg.StorageRoot, "trash", sha+"."+rid)
	if e := os.Rename(final, trash); e != nil {
		return e
	}
	if e := syncDir(filepath.Dir(final)); e != nil {
		_ = os.Rename(trash, final)
		return fmt.Errorf("gc source directory sync: %w", e)
	}
	if e := syncDir(filepath.Dir(trash)); e != nil {
		_ = os.Rename(trash, final)
		_ = syncDir(filepath.Dir(final))
		return fmt.Errorf("gc trash directory sync: %w", e)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Rename(trash, final)
		}
	}()
	e = s.db.WithTx(func(tx *db.Tx) error {
		var c int64
		if e := tx.QueryOne(`SELECT COUNT(*) FROM artifact_records WHERE sha256=? AND deleted_at IS NULL`, []any{sha}, func(r *db.Rows) error { c = r.Int64(0); return nil }); e != nil {
			return e
		}
		if c != 0 {
			return fmt.Errorf("artifact retained during gc")
		}
		if _, e := tx.Exec(`DELETE FROM artifact_records WHERE sha256=? AND deleted_at IS NOT NULL`, sha); e != nil {
			return e
		}
		n, e := tx.Exec(`DELETE FROM objects WHERE sha256=? AND unreferenced_at IS NOT NULL AND unreferenced_at<=?`, sha, cut)
		if e != nil {
			return e
		}
		if n != 1 {
			return fmt.Errorf("object gc predicate changed")
		}
		return nil
	})
	if e != nil {
		return e
	}
	committed = true
	if e := os.Remove(trash); e != nil && !os.IsNotExist(e) {
		return fmt.Errorf("metadata removed but trash cleanup failed: %w", e)
	}
	return nil
}
func (s *Store) Ready() error {
	if e := s.db.Ping(); e != nil {
		return fmt.Errorf("database: %w", e)
	}
	p := filepath.Join(s.cfg.StorageRoot, "staging")
	f, e := os.CreateTemp(p, "ready-*")
	if e != nil {
		return fmt.Errorf("storage: %w", e)
	}
	name := f.Name()
	if _, e = f.Write([]byte("ok")); e == nil {
		e = f.Sync()
	}
	_ = f.Close()
	_ = os.Remove(name)
	if e != nil {
		return fmt.Errorf("storage: %w", e)
	}
	return nil
}

// TestHookBeforeMetadataCommit is deliberately package-visible only to tests in this package through a helper.
func (s *Store) setBeforeMetadataCommitForTest(fn func() error) { s.beforeMetadataCommit = fn }
