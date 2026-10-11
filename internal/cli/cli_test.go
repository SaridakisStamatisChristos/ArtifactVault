package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/config"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/db"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/httpapi"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/metrics"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/store"
)

func TestCLISmoke(t *testing.T) {
	root := t.TempDir()
	c := config.Default()
	c.DBPath = filepath.Join(root, "db")
	c.StorageRoot = filepath.Join(root, "storage")
	d, e := db.Open(c.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	if e = d.MigrateUp(); e != nil {
		t.Fatal(e)
	}
	st, _ := store.New(d, c)
	ts := httptest.NewServer(httpapi.New(st, c, slog.New(slog.NewTextHandler(io.Discard, nil)), &metrics.Metrics{}).Handler())
	defer ts.Close()
	file := filepath.Join(root, "input.bin")
	want := []byte("cli-round-trip")
	if e = os.WriteFile(file, want, 0o600); e != nil {
		t.Fatal(e)
	}
	var out, errout bytes.Buffer
	r := &Runner{Out: &out, Err: &errout, BaseURL: ts.URL, Client: ts.Client()}
	if code := r.Run([]string{"health"}); code != 0 {
		t.Fatalf("health code=%d err=%s", code, errout.String())
	}
	out.Reset()
	errout.Reset()
	if code := r.Run([]string{"upload", "--tag", "cli", file}); code != 0 {
		t.Fatalf("upload code=%d err=%s", code, errout.String())
	}
	var up store.UploadResult
	if e = json.Unmarshal(out.Bytes(), &up); e != nil {
		t.Fatal(e)
	}
	out.Reset()
	errout.Reset()
	dest := filepath.Join(root, "output.bin")
	if code := r.Run([]string{"get", "--output", dest, up.Record.SHA256}); code != 0 {
		t.Fatalf("get code=%d err=%s", code, errout.String())
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, want) {
		t.Fatal("CLI get byte mismatch")
	}
	out.Reset()
	if code := r.Run([]string{"list", "--tag", "cli"}); code != 0 {
		t.Fatal(errout.String())
	}
	out.Reset()
	if code := r.Run([]string{"verify", up.Record.SHA256}); code != 0 {
		t.Fatal(errout.String())
	}
	out.Reset()
	if code := r.Run([]string{"delete", up.Record.SHA256}); code != 0 {
		t.Fatal(errout.String())
	}
	out.Reset()
	if code := r.Run([]string{"gc", "--grace", "0s"}); code != 0 {
		t.Fatal(errout.String())
	}
}
