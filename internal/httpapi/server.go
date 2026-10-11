package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	apispec "github.com/SaridakisStamatisChristos/ArtifactVault/api"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/config"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/metrics"
	"github.com/SaridakisStamatisChristos/ArtifactVault/internal/store"
)

type Server struct {
	store   *store.Store
	cfg     config.Config
	log     *slog.Logger
	metrics *metrics.Metrics
	mux     *http.ServeMux
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func New(s *store.Store, c config.Config, l *slog.Logger, m *metrics.Metrics) *Server {
	x := &Server{store: s, cfg: c, log: l, metrics: m, mux: http.NewServeMux()}
	x.routes()
	return x
}
func (s *Server) Handler() http.Handler { return s.middleware(s.mux) }
func (s *Server) routes() {
	handlers := map[string]http.HandlerFunc{
		"POST /artifacts":                 s.upload,
		"GET /artifacts":                  s.list,
		"GET /artifacts/{sha256}":         s.getMeta,
		"DELETE /artifacts/{sha256}":      s.delete,
		"GET /artifacts/{sha256}/content": s.content,
		"POST /artifacts/{sha256}/verify": s.verify,
		"POST /gc":                        s.gc,
		"GET /health/live":                s.live,
		"GET /health/ready":               s.ready,
		"GET /metrics":                    s.metric,
		"GET /openapi.json":               s.openapi,
	}
	if len(handlers) != len(apispec.Routes) {
		panic("runtime handler set does not match API route declaration")
	}
	for _, route := range apispec.Routes {
		pattern := route.Method + " " + route.Path
		h, ok := handlers[pattern]
		if !ok {
			panic("missing runtime handler for " + pattern)
		}
		s.mux.HandleFunc(pattern, h)
	}
}
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		s.metrics.HTTPRequests.Add(1)
		next.ServeHTTP(sw, r)
		if sw.status >= 500 {
			s.metrics.HTTP5xx.Add(1)
		}
		s.log.Info("http_request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration_ms", time.Since(start).Milliseconds(), "remote", r.RemoteAddr)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, status int, code, msg string) {
	var e errorEnvelope
	e.Error.Code = code
	e.Error.Message = msg
	writeJSON(w, status, e)
}
func classify(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, store.ErrInvalidHash), errors.Is(e, store.ErrInvalidMetadata):
		writeErr(w, 400, "invalid_request", e.Error())
	case errors.Is(e, store.ErrTooLarge):
		writeErr(w, 413, "artifact_too_large", e.Error())
	case errors.Is(e, store.ErrNotFound):
		writeErr(w, 404, "not_found", "artifact not found")
	case errors.Is(e, store.ErrAmbiguousDelete):
		writeErr(w, 409, "ambiguous_delete", e.Error())
	case errors.Is(e, store.ErrCorruptExisting), errors.Is(e, store.ErrMetadataInconsistency):
		writeErr(w, 409, "integrity_conflict", e.Error())
	default:
		writeErr(w, 500, "internal_error", "internal server error")
	}
}
func tagsHeader(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.Split(v, ",")
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MaxArtifactBytes > 0 && r.ContentLength > s.cfg.MaxArtifactBytes {
		writeErr(w, 413, "artifact_too_large", "artifact exceeds configured maximum size")
		return
	}
	m := store.MetadataInput{Filename: r.Header.Get("X-Artifact-Filename"), ContentType: r.Header.Get("Content-Type"), Description: r.Header.Get("X-Artifact-Description"), Tags: tagsHeader(r.Header.Get("X-Artifact-Tags"))}
	res, e := s.store.Upload(r.Body, m)
	if e != nil {
		if errors.Is(e, io.ErrUnexpectedEOF) {
			writeErr(w, 400, "incomplete_upload", "request body ended before declared content length")
			return
		}
		classify(w, e)
		return
	}
	s.metrics.Uploads.Add(1)
	s.metrics.UploadBytes.Add(uint64(res.Record.SizeBytes))
	status := 201
	if res.Deduplicated {
		status = 200
	}
	writeJSON(w, status, res)
}
func parseTimeParam(r *http.Request, name string) (string, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return "", nil
	}
	t, e := time.Parse(time.RFC3339Nano, v)
	if e != nil {
		return "", fmt.Errorf("%s must be RFC3339", name)
	}
	return store.FormatTimestamp(t), nil
}
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 100 {
			writeErr(w, 400, "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = n
	}
	after, e := parseTimeParam(r, "created_after")
	if e != nil {
		writeErr(w, 400, "invalid_time", e.Error())
		return
	}
	before, e := parseTimeParam(r, "created_before")
	if e != nil {
		writeErr(w, 400, "invalid_time", e.Error())
		return
	}
	res, e := s.store.List(store.ListOptions{Limit: limit, Cursor: r.URL.Query().Get("cursor"), Tag: r.URL.Query().Get("tag"), CreatedAfter: after, CreatedBefore: before})
	if e != nil {
		writeErr(w, 400, "invalid_query", e.Error())
		return
	}
	writeJSON(w, 200, res)
}
func (s *Server) getMeta(w http.ResponseWriter, r *http.Request) {
	v, e := s.store.Get(r.PathValue("sha256"))
	if e != nil {
		classify(w, e)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) content(w http.ResponseWriter, r *http.Request) {
	sha, e := store.NormalizeHash(r.PathValue("sha256"))
	if e != nil {
		classify(w, e)
		return
	}
	f, size, mod, closeFn, e := s.store.OpenContent(sha)
	if e != nil {
		classify(w, e)
		return
	}
	defer closeFn()
	w.Header().Set("ETag", `"sha256:`+sha+`"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	cw := &countingWriter{ResponseWriter: w}
	http.ServeContent(cw, r, sha, mod, f)
	s.metrics.DownloadBytes.Add(uint64(min64(cw.n, size)))
}

type countingWriter struct {
	http.ResponseWriter
	n int64
}

func (w *countingWriter) Write(b []byte) (int, error) {
	n, e := w.ResponseWriter.Write(b)
	w.n += int64(n)
	return n, e
}
func (w *countingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	s.metrics.VerifyRuns.Add(1)
	v, e := s.store.Verify(r.PathValue("sha256"))
	if e != nil {
		classify(w, e)
		return
	}
	if v.Status != "verified" {
		s.metrics.VerifyFailures.Add(1)
	}
	writeJSON(w, 200, v)
}
func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	v, e := s.store.Delete(r.PathValue("sha256"), r.URL.Query().Get("record_id"))
	if e != nil {
		classify(w, e)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) gc(w http.ResponseWriter, r *http.Request) {
	grace := s.cfg.GCGrace
	if r.Body != nil && r.ContentLength != 0 {
		var in struct {
			GraceSeconds *int64 `json:"grace_seconds"`
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		dec.DisallowUnknownFields()
		if e := dec.Decode(&in); e != nil {
			writeErr(w, 400, "invalid_json", "invalid gc request body")
			return
		}
		if in.GraceSeconds != nil {
			if *in.GraceSeconds < 0 {
				writeErr(w, 400, "invalid_grace", "grace_seconds must be non-negative")
				return
			}
			grace = time.Duration(*in.GraceSeconds) * time.Second
		}
	}
	s.metrics.GCRuns.Add(1)
	v, e := s.store.GC(grace)
	if e != nil {
		classify(w, e)
		return
	}
	if v.Skipped > 0 {
		s.metrics.GCErrors.Add(uint64(v.Skipped))
	}
	writeJSON(w, 200, v)
}
func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "live"})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if e := s.store.Ready(); e != nil {
		writeJSON(w, 503, map[string]any{"status": "not_ready", "reason": e.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}
func (s *Server) metric(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	s.metrics.WritePrometheus(w)
}
func (s *Server) openapi(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(apispec.OpenAPI())
}

func HTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
}
func Shutdown(ctx context.Context, srv *http.Server) error { return srv.Shutdown(ctx) }
