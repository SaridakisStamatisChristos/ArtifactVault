package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/config"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/db"
)

func testStore(t *testing.T) (*Store, *db.DB, config.Config) {
	t.Helper()
	root := t.TempDir()
	c := config.Default()
	c.DBPath = filepath.Join(root, "meta.db")
	c.StorageRoot = filepath.Join(root, "storage")
	c.GCGrace = time.Hour
	c.StagingTTL = time.Hour
	d, e := db.Open(c.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	if e = d.MigrateUp(); e != nil {
		t.Fatal(e)
	}
	s, e := New(d, c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = d.Close() })
	return s, d, c
}
func digest(b []byte) string { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
func countObjectFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(filepath.Join(root, "objects"), func(_ string, d os.DirEntry, e error) error {
		if e == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestUploadRoundTripAndDuplicatePhysicalDedup(t *testing.T) {
	s, _, c := testStore(t)
	data := []byte{0, 1, 2, 3, 0xff, 0x10}
	a, e := s.Upload(bytes.NewReader(data), MetadataInput{Filename: "x.bin", Tags: []string{"bench"}})
	if e != nil {
		t.Fatal(e)
	}
	if a.Deduplicated {
		t.Fatal("first upload marked deduplicated")
	}
	b, e := s.Upload(bytes.NewReader(data), MetadataInput{Filename: "copy.bin", Tags: []string{"model"}})
	if e != nil {
		t.Fatal(e)
	}
	if !b.Deduplicated {
		t.Fatal("duplicate not recognized")
	}
	if a.Record.SHA256 != b.Record.SHA256 || a.Record.ID == b.Record.ID {
		t.Fatal("identity/reference semantics wrong")
	}
	if got := countObjectFiles(t, c.StorageRoot); got != 1 {
		t.Fatalf("physical objects=%d", got)
	}
	v, e := s.Get(a.Record.SHA256)
	if e != nil || len(v.Records) != 2 {
		t.Fatalf("records=%d err=%v", len(v.Records), e)
	}
	f, _, _, closeFn, e := s.OpenContent(a.Record.SHA256)
	if e != nil {
		t.Fatal(e)
	}
	out, e := io.ReadAll(f)
	closeFn()
	if e != nil || !bytes.Equal(out, data) {
		t.Fatalf("round trip mismatch %v", e)
	}
}

func TestConcurrentDuplicateUploads(t *testing.T) {
	s, _, c := testStore(t)
	data := bytes.Repeat([]byte("same-content-"), 4096)
	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Upload(bytes.NewReader(data), MetadataInput{Filename: fmt.Sprintf("f-%d.bin", i)})
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if got := countObjectFiles(t, c.StorageRoot); got != 1 {
		t.Fatalf("physical objects=%d", got)
	}
	v, e := s.Get(digest(data))
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Records) != n {
		t.Fatalf("records=%d want %d", len(v.Records), n)
	}
}

func TestConcurrentDistinctUploads(t *testing.T) {
	s, _, c := testStore(t)
	const n = 10
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := bytes.Repeat([]byte(fmt.Sprintf("content-%02d", i)), 1024)
			_, e := s.Upload(bytes.NewReader(data), MetadataInput{Filename: "x"})
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if got := countObjectFiles(t, c.StorageRoot); got != n {
		t.Fatalf("physical objects=%d want %d", got, n)
	}
}

type boundedReader struct {
	remaining    int64
	maxRequested int
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRequested {
		r.maxRequested = len(p)
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	for i := 0; i < n; i++ {
		p[i] = byte(i)
	}
	r.remaining -= int64(n)
	return n, nil
}
func TestEmptyAndLargeStreamedUpload(t *testing.T) {
	s, _, _ := testStore(t)
	empty, e := s.Upload(bytes.NewReader(nil), MetadataInput{Filename: "empty"})
	if e != nil {
		t.Fatal(e)
	}
	if empty.Record.SHA256 != digest(nil) || empty.Record.SizeBytes != 0 {
		t.Fatal("empty artifact identity wrong")
	}
	r := &boundedReader{remaining: 16 << 20}
	res, e := s.Upload(r, MetadataInput{Filename: "large.bin"})
	if e != nil {
		t.Fatal(e)
	}
	if res.Record.SizeBytes != 16<<20 {
		t.Fatalf("size=%d", res.Record.SizeBytes)
	}
	if r.maxRequested > 64*1024 {
		t.Fatalf("stream reader asked for %d bytes", r.maxRequested)
	}
}

func TestTimestampOrderingFiltersAndCursorAcrossExactSecondBoundary(t *testing.T) {
	s, _, _ := testStore(t)
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	now := base
	s.now = func() time.Time { return now }

	a, e := s.Upload(bytes.NewReader([]byte("exact-second")), MetadataInput{Filename: "a"})
	if e != nil {
		t.Fatal(e)
	}
	now = base.Add(100 * time.Millisecond)
	b, e := s.Upload(bytes.NewReader([]byte("fractional")), MetadataInput{Filename: "b"})
	if e != nil {
		t.Fatal(e)
	}
	if a.Record.CreatedAt != "2026-10-11T12:00:00.000000000Z" || b.Record.CreatedAt != "2026-10-11T12:00:00.100000000Z" {
		t.Fatalf("timestamps are not canonical: a=%q b=%q", a.Record.CreatedAt, b.Record.CreatedAt)
	}

	first, e := s.List(ListOptions{Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	if len(first.Items) != 1 || first.Items[0].ID != b.Record.ID || first.NextCursor == "" {
		t.Fatalf("newest-first ordering/cursor incorrect: %+v", first)
	}
	second, e := s.List(ListOptions{Limit: 1, Cursor: first.NextCursor})
	if e != nil {
		t.Fatal(e)
	}
	if len(second.Items) != 1 || second.Items[0].ID != a.Record.ID || second.NextCursor != "" {
		t.Fatalf("second page incorrect: %+v", second)
	}

	before, e := s.List(ListOptions{CreatedBefore: FormatTimestamp(base)})
	if e != nil {
		t.Fatal(e)
	}
	if len(before.Items) != 1 || before.Items[0].ID != a.Record.ID {
		t.Fatalf("created_before exact-second boundary incorrect: %+v", before)
	}
	after, e := s.List(ListOptions{CreatedAfter: FormatTimestamp(base.Add(50 * time.Millisecond))})
	if e != nil {
		t.Fatal(e)
	}
	if len(after.Items) != 1 || after.Items[0].ID != b.Record.ID {
		t.Fatalf("created_after fractional boundary incorrect: %+v", after)
	}
}
