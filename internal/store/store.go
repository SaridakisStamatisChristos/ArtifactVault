package store

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/config"
	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/db"
)

var hashRE = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

var (
	ErrNotFound              = errors.New("artifact not found")
	ErrInvalidHash           = errors.New("invalid sha256")
	ErrTooLarge              = errors.New("artifact exceeds configured maximum size")
	ErrCorruptExisting       = errors.New("existing physical object does not match its sha256 identity")
	ErrMetadataInconsistency = errors.New("artifact metadata is inconsistent with physical storage")
	ErrAmbiguousDelete       = errors.New("multiple logical records retain this artifact; specify record_id")
	ErrInvalidMetadata       = errors.New("invalid metadata")
)

type MetadataInput struct {
	Filename    string
	ContentType string
	Description string
	Tags        []string
}

type Record struct {
	ID               string   `json:"id"`
	SHA256           string   `json:"sha256"`
	SizeBytes        int64    `json:"size_bytes"`
	CreatedAt        string   `json:"created_at"`
	OriginalFilename string   `json:"original_filename"`
	ContentType      string   `json:"content_type,omitempty"`
	Description      string   `json:"description,omitempty"`
	Tags             []string `json:"tags"`
}

type UploadResult struct {
	Record       Record `json:"record"`
	Deduplicated bool   `json:"deduplicated"`
}
type ArtifactView struct {
	SHA256          string   `json:"sha256"`
	SizeBytes       int64    `json:"size_bytes"`
	ObjectCreatedAt string   `json:"object_created_at"`
	Records         []Record `json:"records"`
}
type VerifyResult struct {
	SHA256       string `json:"sha256"`
	Status       string `json:"status"`
	ExpectedSize int64  `json:"expected_size"`
	ActualSize   *int64 `json:"actual_size,omitempty"`
	ActualSHA256 string `json:"actual_sha256,omitempty"`
	Message      string `json:"message,omitempty"`
}
type DeleteResult struct {
	SHA256                 string `json:"sha256"`
	DeletedRecordID        string `json:"deleted_record_id"`
	RemainingActiveRecords int64  `json:"remaining_active_records"`
	UnreferencedAt         string `json:"unreferenced_at,omitempty"`
}
type GCResult struct {
	Eligible int      `json:"eligible"`
	Deleted  int      `json:"deleted"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}
type ListOptions struct {
	Limit                                    int
	Cursor, Tag, CreatedAfter, CreatedBefore string
}
type ListResult struct {
	Items      []Record `json:"items"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

type Store struct {
	db                   *db.DB
	cfg                  config.Config
	lifecycle            sync.RWMutex
	now                  func() time.Time
	beforeMetadataCommit func() error
}

func New(d *db.DB, c config.Config) (*Store, error) {
	s := &Store{db: d, cfg: c, now: time.Now}
	for _, p := range []string{c.StorageRoot, filepath.Join(c.StorageRoot, "staging"), filepath.Join(c.StorageRoot, "objects"), filepath.Join(c.StorageRoot, "trash"), filepath.Join(c.StorageRoot, "lost+found")} {
		if err := os.MkdirAll(p, 0o750); err != nil {
			return nil, err
		}
	}
	return s, nil
}
func NormalizeHash(v string) (string, error) {
	if !hashRE.MatchString(v) {
		return "", ErrInvalidHash
	}
	return strings.ToLower(v), nil
}
func expectedRel(sha string) string { return filepath.Join("objects", sha[:2], sha[2:4], sha) }
func (s *Store) expectedPath(sha string) string {
	return filepath.Join(s.cfg.StorageRoot, expectedRel(sha))
}
func nowText(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func (s *Store) ValidateMetadata(m *MetadataInput) error {
	if m.Filename == "" {
		m.Filename = "artifact.bin"
	}
	if len(m.Filename) > s.cfg.MaxFilenameBytes || strings.ContainsRune(m.Filename, '\x00') {
		return fmt.Errorf("%w: filename", ErrInvalidMetadata)
	}
	if len(m.Description) > s.cfg.MaxDescriptionBytes || strings.ContainsRune(m.Description, '\x00') {
		return fmt.Errorf("%w: description", ErrInvalidMetadata)
	}
	if len(m.ContentType) > 255 || strings.ContainsAny(m.ContentType, "\r\n\x00") {
		return fmt.Errorf("%w: content type", ErrInvalidMetadata)
	}
	if len(m.Tags) > s.cfg.MaxTags {
		return fmt.Errorf("%w: too many tags", ErrInvalidMetadata)
	}
	seen := map[string]bool{}
	out := m.Tags[:0]
	for _, t := range m.Tags {
		t = strings.TrimSpace(t)
		if t == "" || len(t) > s.cfg.MaxTagBytes || strings.ContainsRune(t, '\x00') {
			return fmt.Errorf("%w: tag", ErrInvalidMetadata)
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	m.Tags = out
	return nil
}
