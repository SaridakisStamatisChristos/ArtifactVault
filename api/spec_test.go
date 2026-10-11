package api

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func TestOpenAPIIsValidJSONAndMatchesDeclaredRoutes(t *testing.T) {
	var doc struct {
		OpenAPI string                                `json:"openapi"`
		Info    map[string]any                        `json:"info"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}
	if e := json.Unmarshal(OpenAPI(), &doc); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("unexpected OpenAPI version %q", doc.OpenAPI)
	}
	if len(doc.Info) == 0 || len(doc.Paths) == 0 {
		t.Fatal("OpenAPI missing info or paths")
	}
	var got []string
	for p, ops := range doc.Paths {
		for m := range ops {
			switch m {
			case "get", "post", "put", "patch", "delete", "head", "options":
				got = append(got, strings.ToUpper(m)+" "+p)
			}
		}
	}
	var want []string
	for _, r := range Routes {
		want = append(want, r.Method+" "+r.Path)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("OpenAPI/runtime route declaration drift\nGOT:\n%s\nWANT:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for p, ops := range doc.Paths {
		for m, raw := range ops {
			if m == "parameters" {
				continue
			}
			var op map[string]any
			if e := json.Unmarshal(raw, &op); e != nil {
				t.Fatalf("%s %s: %v", m, p, e)
			}
			if _, ok := op["responses"]; !ok {
				t.Fatalf("%s %s has no responses", m, p)
			}
		}
	}
}
