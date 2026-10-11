package metrics

import (
	"fmt"
	"io"
	"sync/atomic"
)

type Metrics struct {
	Uploads        atomic.Uint64
	UploadBytes    atomic.Uint64
	DownloadBytes  atomic.Uint64
	VerifyRuns     atomic.Uint64
	VerifyFailures atomic.Uint64
	GCRuns         atomic.Uint64
	GCErrors       atomic.Uint64
	HTTPRequests   atomic.Uint64
	HTTP5xx        atomic.Uint64
}

func (m *Metrics) WritePrometheus(w io.Writer) {
	vals := []struct {
		name, help string
		v          uint64
	}{
		{"artifactvault_uploads_total", "Completed upload requests", m.Uploads.Load()},
		{"artifactvault_upload_bytes_total", "Bytes accepted by uploads", m.UploadBytes.Load()},
		{"artifactvault_download_bytes_total", "Bytes served by complete or ranged downloads", m.DownloadBytes.Load()},
		{"artifactvault_verify_runs_total", "Integrity verification requests", m.VerifyRuns.Load()},
		{"artifactvault_verify_failures_total", "Verification results other than verified", m.VerifyFailures.Load()},
		{"artifactvault_gc_runs_total", "Garbage collection requests", m.GCRuns.Load()},
		{"artifactvault_gc_errors_total", "Garbage collection object errors", m.GCErrors.Load()},
		{"artifactvault_http_requests_total", "HTTP requests", m.HTTPRequests.Load()},
		{"artifactvault_http_5xx_total", "HTTP responses with 5xx status", m.HTTP5xx.Load()},
	}
	for _, x := range vals {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", x.name, x.help, x.name, x.name, x.v)
	}
}
