package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Runner struct {
	Out, Err io.Writer
	BaseURL  string
	Client   *http.Client
}

func New() *Runner {
	base := os.Getenv("ARTIFACTVAULT_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	return &Runner{Out: os.Stdout, Err: os.Stderr, BaseURL: strings.TrimRight(base, "/"), Client: &http.Client{}}
}

type tagsFlag []string

func (t *tagsFlag) String() string     { return strings.Join(*t, ",") }
func (t *tagsFlag) Set(v string) error { *t = append(*t, v); return nil }
func (r *Runner) usage() {
	fmt.Fprintln(r.Err, "usage: artifactvault <serve|upload|get|list|verify|delete|gc|health|migrate> ...")
}
func (r *Runner) Run(args []string) int {
	if len(args) == 0 {
		r.usage()
		return 2
	}
	var e error
	switch args[0] {
	case "upload":
		e = r.upload(args[1:])
	case "get":
		e = r.get(args[1:])
	case "list":
		e = r.list(args[1:])
	case "verify":
		e = r.verify(args[1:])
	case "delete":
		e = r.delete(args[1:])
	case "gc":
		e = r.gc(args[1:])
	case "health":
		e = r.health(args[1:])
	default:
		r.usage()
		return 2
	}
	if e != nil {
		fmt.Fprintln(r.Err, "error:", e)
		return 1
	}
	return 0
}
func (r *Runner) do(req *http.Request) (*http.Response, error) {
	resp, e := r.Client.Do(req)
	if e != nil {
		return nil, e
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("server returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return resp, nil
}
func (r *Runner) upload(args []string) error {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(r.Err)
	var tags tagsFlag
	fs.Var(&tags, "tag", "tag (repeatable)")
	desc := fs.String("description", "", "description")
	ctype := fs.String("content-type", "application/octet-stream", "content type")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 1 {
		return errors.New("upload requires FILE")
	}
	path := fs.Arg(0)
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	req, e := http.NewRequest(http.MethodPost, r.BaseURL+"/artifacts", f)
	if e != nil {
		return e
	}
	req.ContentLength = st.Size()
	req.Header.Set("Content-Type", *ctype)
	req.Header.Set("X-Artifact-Filename", filepath.Base(path))
	if *desc != "" {
		req.Header.Set("X-Artifact-Description", *desc)
	}
	if len(tags) > 0 {
		req.Header.Set("X-Artifact-Tags", strings.Join(tags, ","))
	}
	resp, e := r.do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	return prettyCopy(r.Out, resp.Body)
}
func (r *Runner) get(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(r.Err)
	out := fs.String("output", "", "output file (default stdout)")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 1 {
		return errors.New("get requires SHA256")
	}
	resp, e := r.do(mustReq(http.MethodGet, r.BaseURL+"/artifacts/"+url.PathEscape(fs.Arg(0))+"/content", nil))
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	var w io.Writer = r.Out
	var f *os.File
	if *out != "" {
		f, e = os.Create(*out)
		if e != nil {
			return e
		}
		defer f.Close()
		w = f
	}
	_, e = io.Copy(w, resp.Body)
	return e
}
func (r *Runner) list(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(r.Err)
	limit := fs.Int("limit", 50, "page size")
	cursor := fs.String("cursor", "", "cursor")
	tag := fs.String("tag", "", "tag filter")
	after := fs.String("created-after", "", "RFC3339 lower bound")
	before := fs.String("created-before", "", "RFC3339 upper bound")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("list accepts flags only")
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(*limit))
	if *cursor != "" {
		q.Set("cursor", *cursor)
	}
	if *tag != "" {
		q.Set("tag", *tag)
	}
	if *after != "" {
		q.Set("created_after", *after)
	}
	if *before != "" {
		q.Set("created_before", *before)
	}
	resp, e := r.do(mustReq(http.MethodGet, r.BaseURL+"/artifacts?"+q.Encode(), nil))
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	return prettyCopy(r.Out, resp.Body)
}
func (r *Runner) verify(args []string) error {
	if len(args) != 1 {
		return errors.New("verify requires SHA256")
	}
	resp, e := r.do(mustReq(http.MethodPost, r.BaseURL+"/artifacts/"+url.PathEscape(args[0])+"/verify", nil))
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	return prettyCopy(r.Out, resp.Body)
}
func (r *Runner) delete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(r.Err)
	record := fs.String("record", "", "logical record UUID when artifact has multiple active records")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 1 {
		return errors.New("delete requires SHA256")
	}
	u := r.BaseURL + "/artifacts/" + url.PathEscape(fs.Arg(0))
	if *record != "" {
		u += "?record_id=" + url.QueryEscape(*record)
	}
	resp, e := r.do(mustReq(http.MethodDelete, u, nil))
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	return prettyCopy(r.Out, resp.Body)
}
func (r *Runner) gc(args []string) error {
	fs := flag.NewFlagSet("gc", flag.ContinueOnError)
	fs.SetOutput(r.Err)
	grace := fs.String("grace", "", "override configured grace (for example 0s or 24h)")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("gc accepts flags only")
	}
	body := []byte("{}")
	if *grace != "" {
		d, e := time.ParseDuration(*grace)
		if e != nil || d < 0 {
			return errors.New("grace must be a non-negative duration")
		}
		body, _ = json.Marshal(map[string]int64{"grace_seconds": int64(d / time.Second)})
	}
	req := mustReq(http.MethodPost, r.BaseURL+"/gc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, e := r.do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	return prettyCopy(r.Out, resp.Body)
}
func (r *Runner) health(args []string) error {
	if len(args) != 0 {
		return errors.New("health accepts no arguments")
	}
	resp, e := r.do(mustReq(http.MethodGet, r.BaseURL+"/health/ready", nil))
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	return prettyCopy(r.Out, resp.Body)
}
func mustReq(method, u string, body io.Reader) *http.Request {
	req, e := http.NewRequest(method, u, body)
	if e != nil {
		panic(e)
	}
	return req
}
func prettyCopy(w io.Writer, rd io.Reader) error {
	var v any
	dec := json.NewDecoder(rd)
	if e := dec.Decode(&v); e != nil {
		return e
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
