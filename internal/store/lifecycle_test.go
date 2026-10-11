package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type failReader struct{ done bool }

func (r *failReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("client disconnected")
	}
	r.done = true
	copy(p, "partial")
	return 7, nil
}
func TestInterruptedUploadAndMetadataFailureDoNotCreateRecord(t *testing.T) {
	s, _, c := testStore(t)
	_, e := s.Upload(&failReader{}, MetadataInput{Filename: "partial"})
	if e == nil {
		t.Fatal("expected interrupted upload failure")
	}
	entries, _ := os.ReadDir(filepath.Join(c.StorageRoot, "staging"))
	if len(entries) != 0 {
		t.Fatalf("staging leak: %d", len(entries))
	}
	s.setBeforeMetadataCommitForTest(func() error { return errors.New("injected metadata failure") })
	data := []byte("finalized-before-db")
	_, e = s.Upload(bytes.NewReader(data), MetadataInput{Filename: "x"})
	if e == nil {
		t.Fatal("expected metadata failure")
	}
	if _, e = s.Get(digest(data)); !errors.Is(e, ErrNotFound) {
		t.Fatalf("record should not exist: %v", e)
	}
	if _, e = os.Stat(s.expectedPath(digest(data))); !os.IsNotExist(e) {
		t.Fatalf("physical object should be rolled back, stat=%v", e)
	}
}

func TestVerificationStates(t *testing.T) {
	t.Run("corrupt", func(t *testing.T) {
		s, _, _ := testStore(t)
		data := []byte("good")
		u, e := s.Upload(bytes.NewReader(data), MetadataInput{Filename: "x"})
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(s.expectedPath(u.Record.SHA256), []byte("evil"), 0o640); e != nil {
			t.Fatal(e)
		}
		v, e := s.Verify(u.Record.SHA256)
		if e != nil || v.Status != "corrupted_object" {
			t.Fatalf("%+v %v", v, e)
		}
	})
	t.Run("missing", func(t *testing.T) {
		s, _, _ := testStore(t)
		u, e := s.Upload(bytes.NewReader([]byte("gone")), MetadataInput{Filename: "x"})
		if e != nil {
			t.Fatal(e)
		}
		_ = os.Remove(s.expectedPath(u.Record.SHA256))
		v, e := s.Verify(u.Record.SHA256)
		if e != nil || v.Status != "missing_physical_object" {
			t.Fatalf("%+v %v", v, e)
		}
	})
	t.Run("metadata", func(t *testing.T) {
		s, d, _ := testStore(t)
		u, e := s.Upload(bytes.NewReader([]byte("meta")), MetadataInput{Filename: "x"})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = d.Exec(`UPDATE objects SET size_bytes=size_bytes+1 WHERE sha256=?`, u.Record.SHA256); e != nil {
			t.Fatal(e)
		}
		v, e := s.Verify(u.Record.SHA256)
		if e != nil || v.Status != "metadata_inconsistency" {
			t.Fatalf("%+v %v", v, e)
		}
	})
}

func TestDeletionAmbiguityGraceAndGCSafety(t *testing.T) {
	s, _, c := testStore(t)
	base := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	now := base
	s.now = func() time.Time { return now }
	data := []byte("retained")
	a, _ := s.Upload(bytes.NewReader(data), MetadataInput{Filename: "a"})
	b, _ := s.Upload(bytes.NewReader(data), MetadataInput{Filename: "b"})
	if _, e := s.Delete(a.Record.SHA256, ""); !errors.Is(e, ErrAmbiguousDelete) {
		t.Fatalf("expected ambiguous delete, got %v", e)
	}
	d, e := s.Delete(a.Record.SHA256, a.Record.ID)
	if e != nil || d.RemainingActiveRecords != 1 {
		t.Fatalf("%+v %v", d, e)
	}
	g, e := s.GC(0)
	if e != nil || g.Eligible != 0 {
		t.Fatalf("retained object eligible: %+v %v", g, e)
	}
	if _, e = os.Stat(s.expectedPath(a.Record.SHA256)); e != nil {
		t.Fatal("retained object removed")
	}
	if _, e = s.Delete(a.Record.SHA256, b.Record.ID); e != nil {
		t.Fatal(e)
	}
	g, _ = s.GC(time.Hour)
	if g.Deleted != 0 {
		t.Fatal("gc ignored grace")
	}
	now = base.Add(2 * time.Hour)
	g, e = s.GC(time.Hour)
	if e != nil || g.Deleted != 1 {
		t.Fatalf("gc=%+v %v", g, e)
	}
	if _, e = os.Stat(s.expectedPath(a.Record.SHA256)); !os.IsNotExist(e) {
		t.Fatalf("object still exists: %v", e)
	}
	if countObjectFiles(t, c.StorageRoot) != 0 {
		t.Fatal("object tree not empty")
	}
}

func TestReadLeasePreventsConcurrentDelete(t *testing.T) {
	s, _, _ := testStore(t)
	u, _ := s.Upload(bytes.NewReader(bytes.Repeat([]byte("x"), 1024)), MetadataInput{Filename: "x"})
	_, _, _, closeFn, e := s.OpenContent(u.Record.SHA256)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		if _, de := s.Delete(u.Record.SHA256, u.Record.ID); de != nil {
			done <- de
			return
		}
		g, ge := s.GC(0)
		if ge == nil && g.Deleted != 1 {
			ge = fmt.Errorf("gc deleted=%d", g.Deleted)
		}
		done <- ge
	}()
	select {
	case <-done:
		t.Fatal("delete/gc completed while content lease held")
	case <-time.After(50 * time.Millisecond):
	}
	closeFn()
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(s.expectedPath(u.Record.SHA256)); !os.IsNotExist(e) {
		t.Fatalf("gc did not remove unreferenced object after lease release: %v", e)
	}
}

func TestRecoveryCleansStagingAndQuarantinesOldOrphan(t *testing.T) {
	s, d, c := testStore(t)
	old := time.Now().Add(-2 * time.Hour)
	p := filepath.Join(c.StorageRoot, "staging", "stale.part")
	if e := os.WriteFile(p, []byte("x"), 0o600); e != nil {
		t.Fatal(e)
	}
	_ = os.Chtimes(p, old, old)
	data := []byte("orphan")
	sha := digest(data)
	op := s.expectedPath(sha)
	_ = os.MkdirAll(filepath.Dir(op), 0o750)
	if e := os.WriteFile(op, data, 0o640); e != nil {
		t.Fatal(e)
	}
	_ = os.Chtimes(op, old, old)
	s.now = func() time.Time { return time.Now() }
	if e := s.Recover(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("stale staging not removed")
	}
	if _, e := os.Stat(op); !os.IsNotExist(e) {
		t.Fatal("orphan not quarantined")
	}
	if _, e := os.Stat(filepath.Join(c.StorageRoot, "lost+found", sha+".orphan")); e != nil {
		t.Fatalf("orphan quarantine missing: %v", e)
	}
	_ = d
}
