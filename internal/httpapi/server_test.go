package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/config"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/db"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/metrics"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/store"
)

func setupAPI(t *testing.T) (*httptest.Server, *store.Store, *db.DB, config.Config) {
	t.Helper()
	root := t.TempDir()
	c := config.Default()
	c.DBPath = filepath.Join(root, "m.db")
	c.StorageRoot = filepath.Join(root, "storage")
	d, e := db.Open(c.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	if e = d.MigrateUp(); e != nil {
		t.Fatal(e)
	}
	st, e := store.New(d, c)
	if e != nil {
		t.Fatal(e)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(st, c, log, &metrics.Metrics{}).Handler())
	t.Cleanup(func() { srv.Close(); _ = d.Close() })
	return srv, st, d, c
}
func request(t *testing.T, client *http.Client, method, u string, body io.Reader, headers map[string]string) *http.Response {
	t.Helper()
	req, e := http.NewRequest(method, u, body)
	if e != nil {
		t.Fatal(e)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	return resp
}

func TestAPIRoundTripRangeFiltersAndErrors(t *testing.T) {
	srv, _, _, _ := setupAPI(t)
	data := []byte("0123456789abcdef")
	resp := request(t, srv.Client(), "POST", srv.URL+"/artifacts", bytes.NewReader(data), map[string]string{"X-Artifact-Filename": "x.bin", "X-Artifact-Tags": "alpha,beta", "Content-Type": "application/octet-stream"})
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload %d %s", resp.StatusCode, b)
	}
	var up store.UploadResult
	if e := json.NewDecoder(resp.Body).Decode(&up); e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	resp = request(t, srv.Client(), "GET", srv.URL+"/artifacts/"+up.Record.SHA256+"/content", nil, map[string]string{"Range": "bytes=2-5"})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || string(b) != "2345" {
		t.Fatalf("range status=%d body=%q", resp.StatusCode, b)
	}
	resp = request(t, srv.Client(), "GET", srv.URL+"/artifacts?tag=alpha&limit=1", nil, nil)
	var list store.ListResult
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(list.Items) != 1 || list.Items[0].ID != up.Record.ID {
		t.Fatalf("filter failed: %+v status=%d", list, resp.StatusCode)
	}
	resp = request(t, srv.Client(), "GET", srv.URL+"/artifacts/not-a-hash", nil, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("malformed hash status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = request(t, srv.Client(), "GET", srv.URL+"/artifacts/"+up.Record.SHA256+"/content", nil, map[string]string{"Range": "bytes=999-1000"})
	if resp.StatusCode != 416 {
		t.Fatalf("invalid range status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = request(t, srv.Client(), "POST", srv.URL+"/artifacts", strings.NewReader("x"), map[string]string{"X-Artifact-Filename": strings.Repeat("a", 2000)})
	if resp.StatusCode != 400 {
		t.Fatalf("oversized metadata status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestVerifyCorruptionAndReadinessFailure(t *testing.T) {
	srv, _, d, c := setupAPI(t)
	data := []byte("integrity")
	resp := request(t, srv.Client(), "POST", srv.URL+"/artifacts", bytes.NewReader(data), map[string]string{"X-Artifact-Filename": "x"})
	var up store.UploadResult
	_ = json.NewDecoder(resp.Body).Decode(&up)
	resp.Body.Close()
	p := filepath.Join(c.StorageRoot, "objects", up.Record.SHA256[:2], up.Record.SHA256[2:4], up.Record.SHA256)
	if e := os.WriteFile(p, []byte("tamperity"), 0o640); e != nil {
		t.Fatal(e)
	}
	resp = request(t, srv.Client(), "GET", srv.URL+"/artifacts/"+up.Record.SHA256+"/content", nil, nil)
	if resp.StatusCode != 409 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("corrupted download was served: status=%d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
	resp = request(t, srv.Client(), "POST", srv.URL+"/artifacts/"+up.Record.SHA256+"/verify", nil, nil)
	var vr store.VerifyResult
	_ = json.NewDecoder(resp.Body).Decode(&vr)
	resp.Body.Close()
	if vr.Status != "corrupted_object" {
		t.Fatalf("verify=%+v", vr)
	}
	if e := d.Close(); e != nil {
		t.Fatal(e)
	}
	resp = request(t, srv.Client(), "GET", srv.URL+"/health/live", nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("liveness=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = request(t, srv.Client(), "GET", srv.URL+"/health/ready", nil, nil)
	if resp.StatusCode != 503 {
		t.Fatalf("readiness=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestTimeFilterValidation(t *testing.T) {
	srv, _, _, _ := setupAPI(t)
	resp := request(t, srv.Client(), "GET", srv.URL+"/artifacts?created_after=yesterday", nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	_ = time.RFC3339
}
