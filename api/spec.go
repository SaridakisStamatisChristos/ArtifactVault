package api

import "embed"

//go:embed openapi.json
var fs embed.FS

func OpenAPI() []byte { b, _ := fs.ReadFile("openapi.json"); return b }

type Route struct{ Method, Path string }

var Routes = []Route{
	{"POST", "/artifacts"}, {"GET", "/artifacts"}, {"GET", "/artifacts/{sha256}"}, {"GET", "/artifacts/{sha256}/content"},
	{"DELETE", "/artifacts/{sha256}"}, {"POST", "/artifacts/{sha256}/verify"}, {"POST", "/gc"},
	{"GET", "/health/live"}, {"GET", "/health/ready"}, {"GET", "/metrics"}, {"GET", "/openapi.json"},
}
