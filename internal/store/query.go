package store

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func (s *Store) activeRecords(sha string) ([]Record, error) {
	r, e := s.db.Query(`SELECT id,created_at,original_filename,COALESCE(content_type,''),COALESCE(description,'') FROM artifact_records WHERE sha256=? AND deleted_at IS NULL ORDER BY created_at ASC,id ASC`, sha)
	if e != nil {
		return nil, e
	}
	var recs []Record
	for r.Next() {
		recs = append(recs, Record{ID: r.Text(0), SHA256: sha, CreatedAt: r.Text(1), OriginalFilename: r.Text(2), ContentType: r.Text(3), Description: r.Text(4)})
	}
	e = r.Err()
	r.Close()
	if e != nil {
		return nil, e
	}
	for i := range recs {
		tags, e := s.tags(recs[i].ID)
		if e != nil {
			return nil, e
		}
		recs[i].Tags = tags
	}
	return recs, nil
}
func (s *Store) tags(id string) ([]string, error) {
	r, e := s.db.Query(`SELECT tag FROM tags WHERE record_id=? ORDER BY tag`, id)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	out := []string{}
	for r.Next() {
		out = append(out, r.Text(0))
	}
	return out, r.Err()
}
func (s *Store) Get(sha string) (ArtifactView, error) {
	sha, e := NormalizeHash(sha)
	if e != nil {
		return ArtifactView{}, e
	}
	size, created, rel, _, found, e := s.objectRow(sha)
	if e != nil {
		return ArtifactView{}, e
	}
	if !found {
		return ArtifactView{}, ErrNotFound
	}
	if rel != expectedRel(sha) {
		return ArtifactView{}, ErrMetadataInconsistency
	}
	recs, e := s.activeRecords(sha)
	if e != nil {
		return ArtifactView{}, e
	}
	if len(recs) == 0 {
		return ArtifactView{}, ErrNotFound
	}
	for i := range recs {
		recs[i].SizeBytes = size
	}
	return ArtifactView{SHA256: sha, SizeBytes: size, ObjectCreatedAt: created, Records: recs}, nil
}
func encodeCursor(created, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(created + "\x00" + id))
}
func decodeCursor(v string) (string, string, error) {
	if v == "" {
		return "", "", nil
	}
	b, e := base64.RawURLEncoding.DecodeString(v)
	if e != nil {
		return "", "", fmt.Errorf("invalid cursor")
	}
	p := strings.SplitN(string(b), "\x00", 2)
	if len(p) != 2 {
		return "", "", fmt.Errorf("invalid cursor")
	}
	return p[0], p[1], nil
}
func (s *Store) List(o ListOptions) (ListResult, error) {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	if o.Limit > 100 {
		o.Limit = 100
	}
	cc, ci, e := decodeCursor(o.Cursor)
	if e != nil {
		return ListResult{}, e
	}
	q := `SELECT r.id,r.sha256,o.size_bytes,r.created_at,r.original_filename,COALESCE(r.content_type,''),COALESCE(r.description,'') FROM artifact_records r JOIN objects o ON o.sha256=r.sha256 WHERE r.deleted_at IS NULL`
	args := []any{}
	if o.Tag != "" {
		q += ` AND EXISTS(SELECT 1 FROM tags t WHERE t.record_id=r.id AND t.tag=?)`
		args = append(args, o.Tag)
	}
	if o.CreatedAfter != "" {
		q += ` AND r.created_at>=?`
		args = append(args, o.CreatedAfter)
	}
	if o.CreatedBefore != "" {
		q += ` AND r.created_at<=?`
		args = append(args, o.CreatedBefore)
	}
	if cc != "" {
		q += ` AND (r.created_at<? OR (r.created_at=? AND r.id<?))`
		args = append(args, cc, cc, ci)
	}
	q += ` ORDER BY r.created_at DESC,r.id DESC LIMIT ?`
	args = append(args, o.Limit+1)
	r, e := s.db.Query(q, args...)
	if e != nil {
		return ListResult{}, e
	}
	var items []Record
	for r.Next() {
		items = append(items, Record{ID: r.Text(0), SHA256: r.Text(1), SizeBytes: r.Int64(2), CreatedAt: r.Text(3), OriginalFilename: r.Text(4), ContentType: r.Text(5), Description: r.Text(6)})
	}
	e = r.Err()
	r.Close()
	if e != nil {
		return ListResult{}, e
	}
	more := len(items) > o.Limit
	if more {
		items = items[:o.Limit]
	}
	for i := range items {
		t, e := s.tags(items[i].ID)
		if e != nil {
			return ListResult{}, e
		}
		items[i].Tags = t
	}
	out := ListResult{Items: items}
	if more && len(items) > 0 {
		x := items[len(items)-1]
		out.NextCursor = encodeCursor(x.CreatedAt, x.ID)
	}
	return out, nil
}

func (s *Store) OpenContent(sha string) (*os.File, int64, time.Time, func(), error) {
	sha, e := NormalizeHash(sha)
	if e != nil {
		return nil, 0, time.Time{}, nil, e
	}
	s.lifecycle.RLock()
	release := func() { s.lifecycle.RUnlock() }
	v, e := s.Get(sha)
	if e != nil {
		release()
		return nil, 0, time.Time{}, nil, e
	}
	p := s.expectedPath(sha)
	f, e := os.Open(p)
	if e != nil {
		release()
		if os.IsNotExist(e) {
			return nil, 0, time.Time{}, nil, ErrMetadataInconsistency
		}
		return nil, 0, time.Time{}, nil, e
	}
	st, e := f.Stat()
	if e != nil {
		f.Close()
		release()
		return nil, 0, time.Time{}, nil, e
	}
	if st.Size() != v.SizeBytes {
		f.Close()
		release()
		return nil, 0, time.Time{}, nil, ErrMetadataInconsistency
	}
	h := sha256.New()
	n, he := io.CopyBuffer(h, f, make([]byte, 64*1024))
	if he != nil {
		f.Close()
		release()
		return nil, 0, time.Time{}, nil, he
	}
	if n != v.SizeBytes || hex.EncodeToString(h.Sum(nil)) != sha {
		f.Close()
		release()
		return nil, 0, time.Time{}, nil, ErrCorruptExisting
	}
	if _, he = f.Seek(0, io.SeekStart); he != nil {
		f.Close()
		release()
		return nil, 0, time.Time{}, nil, he
	}
	return f, v.SizeBytes, st.ModTime(), func() { _ = f.Close(); release() }, nil
}
func (s *Store) Verify(sha string) (VerifyResult, error) {
	sha, e := NormalizeHash(sha)
	if e != nil {
		return VerifyResult{}, e
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	size, _, rel, _, found, e := s.objectRow(sha)
	if e != nil {
		return VerifyResult{}, e
	}
	if !found {
		return VerifyResult{}, ErrNotFound
	}
	recs, e := s.activeRecords(sha)
	if e != nil {
		return VerifyResult{}, e
	}
	if len(recs) == 0 {
		return VerifyResult{}, ErrNotFound
	}
	res := VerifyResult{SHA256: sha, ExpectedSize: size}
	if rel != expectedRel(sha) {
		res.Status = "metadata_inconsistency"
		res.Message = "stored path metadata differs from content-addressed path"
		return res, nil
	}
	n, h, e := hashFile(s.expectedPath(sha))
	if e != nil {
		if os.IsNotExist(e) {
			res.Status = "missing_physical_object"
			res.Message = "metadata exists but object file is missing"
			return res, nil
		}
		return VerifyResult{}, e
	}
	res.ActualSize = &n
	res.ActualSHA256 = h
	if h != sha {
		res.Status = "corrupted_object"
		res.Message = "recomputed sha256 does not match identity"
		return res, nil
	}
	if n != size {
		res.Status = "metadata_inconsistency"
		res.Message = "stored size metadata differs from physical object"
		return res, nil
	}
	res.Status = "verified"
	return res, nil
}
