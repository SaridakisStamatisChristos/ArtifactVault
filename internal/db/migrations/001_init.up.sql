CREATE TABLE objects (
  sha256 TEXT PRIMARY KEY CHECK(length(sha256)=64),
  size_bytes INTEGER NOT NULL CHECK(size_bytes>=0),
  created_at TEXT NOT NULL,
  stored_relpath TEXT NOT NULL UNIQUE,
  unreferenced_at TEXT NULL
);
CREATE TABLE artifact_records (
  id TEXT PRIMARY KEY,
  sha256 TEXT NOT NULL REFERENCES objects(sha256) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  original_filename TEXT NOT NULL,
  content_type TEXT NULL,
  description TEXT NULL,
  deleted_at TEXT NULL
);
CREATE INDEX idx_artifact_records_sha_active ON artifact_records(sha256, deleted_at);
CREATE INDEX idx_artifact_records_created ON artifact_records(created_at DESC, id DESC);
CREATE TABLE tags (
  record_id TEXT NOT NULL REFERENCES artifact_records(id) ON DELETE CASCADE,
  tag TEXT NOT NULL,
  PRIMARY KEY(record_id, tag)
);
CREATE INDEX idx_tags_tag_record ON tags(tag, record_id);
