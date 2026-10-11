package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/config"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/db"
)

func TestOneByteChangeCreatesDifferentIdentity(t *testing.T) {
	s, _, _ := testStore(t)
	a, e := s.Upload(bytes.NewReader([]byte{0x00}), MetadataInput{Filename: "a"})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Upload(bytes.NewReader([]byte{0x01}), MetadataInput{Filename: "b"})
	if e != nil {
		t.Fatal(e)
	}
	if a.Record.SHA256 == b.Record.SHA256 {
		t.Fatal("one-byte content change produced identical identity")
	}
}

func TestContentOpenRejectsSameSizeCorruption(t *testing.T) {
	s, _, _ := testStore(t)
	u, e := s.Upload(bytes.NewReader([]byte("good")), MetadataInput{Filename: "x"})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(s.expectedPath(u.Record.SHA256), []byte("evil"), 0o640); e != nil {
		t.Fatal(e)
	}
	if _, _, _, _, e = s.OpenContent(u.Record.SHA256); !errors.Is(e, ErrCorruptExisting) {
		t.Fatalf("expected corrupted content rejection, got %v", e)
	}
}

func TestMetadataPersistsAcrossReopen(t *testing.T) {
	root := t.TempDir()
	c := config.Default()
	c.DBPath = filepath.Join(root, "meta.db")
	c.StorageRoot = filepath.Join(root, "storage")
	d1, e := db.Open(c.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	if e = d1.MigrateUp(); e != nil {
		t.Fatal(e)
	}
	s1, e := New(d1, c)
	if e != nil {
		t.Fatal(e)
	}
	u, e := s1.Upload(bytes.NewReader([]byte("persist-me")), MetadataInput{Filename: "persist.bin", Tags: []string{"restart"}, Description: "durable metadata"})
	if e != nil {
		t.Fatal(e)
	}
	if e = d1.Close(); e != nil {
		t.Fatal(e)
	}
	d2, e := db.Open(c.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	defer d2.Close()
	s2, e := New(d2, c)
	if e != nil {
		t.Fatal(e)
	}
	v, e := s2.Get(u.Record.SHA256)
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Records) != 1 || v.Records[0].OriginalFilename != "persist.bin" || len(v.Records[0].Tags) != 1 || v.Records[0].Tags[0] != "restart" {
		t.Fatalf("metadata did not survive reopen: %+v", v)
	}
}

func TestListPaginationIsStableAndNonRepeating(t *testing.T) {
	s, _, _ := testStore(t)
	for _, v := range []string{"one", "two", "three"} {
		if _, e := s.Upload(bytes.NewReader([]byte(v)), MetadataInput{Filename: v, Tags: []string{"page"}}); e != nil {
			t.Fatal(e)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for i := 0; i < 3; i++ {
		page, e := s.List(ListOptions{Limit: 1, Cursor: cursor, Tag: "page"})
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Items) != 1 {
			t.Fatalf("page %d size=%d", i, len(page.Items))
		}
		if seen[page.Items[0].ID] {
			t.Fatal("pagination repeated a record")
		}
		seen[page.Items[0].ID] = true
		cursor = page.NextCursor
		if i < 2 && cursor == "" {
			t.Fatal("pagination ended early")
		}
		if i == 2 && cursor != "" {
			t.Fatal("terminal page exposed a continuation cursor")
		}
	}
}

func TestVerifyConcurrentWithUploads(t *testing.T) {
	s, _, _ := testStore(t)
	base := []byte("concurrent-verification-target")
	u, e := s.Upload(bytes.NewReader(base), MetadataInput{Filename: "base"})
	if e != nil {
		t.Fatal(e)
	}
	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				v, ve := s.Verify(u.Record.SHA256)
				if ve != nil {
					errs <- ve
					return
				}
				if v.Status != "verified" {
					errs <- fmt.Errorf("verification status=%s", v.Status)
					return
				}
				errs <- nil
				return
			}
			_, ue := s.Upload(bytes.NewReader([]byte(fmt.Sprintf("other-%02d", i))), MetadataInput{Filename: "other"})
			errs <- ue
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
}

func TestRecoveryRestoresInterruptedGCTrash(t *testing.T) {
	s, _, c := testStore(t)
	u, e := s.Upload(bytes.NewReader([]byte("gc-crash-state")), MetadataInput{Filename: "x"})
	if e != nil {
		t.Fatal(e)
	}
	final := s.expectedPath(u.Record.SHA256)
	trash := filepath.Join(c.StorageRoot, "trash", u.Record.SHA256+".simulated-crash")
	if e = os.Rename(final, trash); e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(final); e != nil {
		t.Fatalf("recovery did not restore object: %v", e)
	}
	if _, e = os.Stat(trash); !os.IsNotExist(e) {
		t.Fatalf("trash remains after restoration: %v", e)
	}
	v, e := s.Verify(u.Record.SHA256)
	if e != nil || v.Status != "verified" {
		t.Fatalf("restored object did not verify: %+v %v", v, e)
	}
}

func TestRecoveryRejectsCorruptCanonicalWhenTrashIsRecoverable(t *testing.T) {
	s, _, c := testStore(t)
	data := []byte("gc-conflict-state")
	u, e := s.Upload(bytes.NewReader(data), MetadataInput{Filename: "x"})
	if e != nil {
		t.Fatal(e)
	}
	final := s.expectedPath(u.Record.SHA256)
	trash := filepath.Join(c.StorageRoot, "trash", u.Record.SHA256+".simulated-crash")
	if e = os.WriteFile(trash, data, 0o640); e != nil {
		t.Fatal(e)
	}
	corrupt := bytes.Repeat([]byte("x"), len(data))
	if e = os.WriteFile(final, corrupt, 0o640); e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(); !errors.Is(e, ErrCorruptExisting) {
		t.Fatalf("expected recovery corruption error, got %v", e)
	}
	if e = s.Ready(); e == nil {
		t.Fatal("readiness passed with unresolved recovery integrity conflict")
	}
	if _, e = os.Stat(trash); e != nil {
		t.Fatalf("recoverable trash was discarded: %v", e)
	}
}

func TestRecoveryRejectsCorruptTrashBeforeRestore(t *testing.T) {
	s, _, c := testStore(t)
	data := []byte("gc-trash-integrity")
	u, e := s.Upload(bytes.NewReader(data), MetadataInput{Filename: "x"})
	if e != nil {
		t.Fatal(e)
	}
	final := s.expectedPath(u.Record.SHA256)
	trash := filepath.Join(c.StorageRoot, "trash", u.Record.SHA256+".simulated-crash")
	if e = os.Rename(final, trash); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(trash, bytes.Repeat([]byte("x"), len(data)), 0o640); e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(); !errors.Is(e, ErrCorruptExisting) {
		t.Fatalf("expected corrupt trash rejection, got %v", e)
	}
	if e = s.Ready(); e == nil {
		t.Fatal("readiness passed after corrupt trash recovery failure")
	}
	if _, e = os.Stat(final); !os.IsNotExist(e) {
		t.Fatalf("corrupt trash was restored to canonical path: %v", e)
	}
	if _, e = os.Stat(trash); e != nil {
		t.Fatalf("corrupt trash evidence was not preserved: %v", e)
	}
}
