package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Recover performs conservative startup cleanup. It never invents metadata.
// Stale staging files are removed; interrupted-GC trash is restored when metadata
// still exists; old finalized files with no metadata are moved to lost+found.
func (s *Store) Recover() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	var errs []error
	cutoff := s.now().Add(-s.cfg.StagingTTL)
	staging := filepath.Join(s.cfg.StorageRoot, "staging")
	if entries, e := os.ReadDir(staging); e == nil {
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			p := filepath.Join(staging, ent.Name())
			if st, e := ent.Info(); e == nil && st.ModTime().Before(cutoff) {
				if e = os.Remove(p); e != nil {
					errs = append(errs, fmt.Errorf("remove stale staging %s: %w", ent.Name(), e))
				}
			}
		}
	} else {
		errs = append(errs, e)
	}

	trash := filepath.Join(s.cfg.StorageRoot, "trash")
	if entries, e := os.ReadDir(trash); e == nil {
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			name := ent.Name()
			if len(name) < 64 {
				continue
			}
			sha, ve := NormalizeHash(name[:64])
			if ve != nil {
				continue
			}
			p := filepath.Join(trash, name)
			_, _, rel, _, found, qe := s.objectRow(sha)
			if qe != nil {
				errs = append(errs, qe)
				continue
			}
			if !found {
				if e = os.Remove(p); e != nil {
					errs = append(errs, e)
				}
				continue
			}
			if rel != expectedRel(sha) {
				errs = append(errs, ErrMetadataInconsistency)
				continue
			}
			final := s.expectedPath(sha)
			if _, e = os.Stat(final); os.IsNotExist(e) {
				if e = os.MkdirAll(filepath.Dir(final), 0o750); e == nil {
					e = os.Rename(p, final)
				}
				if e != nil {
					errs = append(errs, fmt.Errorf("restore trash %s: %w", sha, e))
				}
			} else if e == nil {
				n, h, he := hashFile(final)
				_, _, _, _, _ = n, h, he, found, qe
				if he == nil && h == sha {
					_ = os.Remove(p)
				}
			}
		}
	} else {
		errs = append(errs, e)
	}

	objects := filepath.Join(s.cfg.StorageRoot, "objects")
	_ = filepath.WalkDir(objects, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			errs = append(errs, walkErr)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		sha, ve := NormalizeHash(name)
		if ve != nil {
			return nil
		}
		if filepath.Clean(path) != filepath.Clean(s.expectedPath(sha)) {
			errs = append(errs, fmt.Errorf("unexpected object path %s", path))
			return nil
		}
		_, _, _, _, found, qe := s.objectRow(sha)
		if qe != nil {
			errs = append(errs, qe)
			return nil
		}
		if found {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			errs = append(errs, e)
			return nil
		}
		if !info.ModTime().Before(cutoff) {
			return nil
		}
		target := filepath.Join(s.cfg.StorageRoot, "lost+found", sha+".orphan")
		if _, e = os.Stat(target); e == nil {
			target = target + "." + strings.ReplaceAll(s.now().UTC().Format("20060102T150405.000000000"), ".", "-")
		}
		if e = os.Rename(path, target); e != nil {
			errs = append(errs, fmt.Errorf("quarantine orphan %s: %w", sha, e))
		}
		return nil
	})
	joined := errors.Join(errs...)
	s.recoveryErr = joined
	return joined
}
