package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationUpgradeRollbackUpgrade(t *testing.T) {
	d, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	if e = d.MigrateUp(); e != nil {
		t.Fatal(e)
	}
	v, e := d.CurrentVersion()
	if e != nil || v != 2 {
		t.Fatalf("version after up = %d, %v", v, e)
	}
	r, e := d.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name='objects'`)
	if e != nil {
		t.Fatal(e)
	}
	if !r.Next() {
		t.Fatal("objects table missing")
	}
	r.Close()
	if e = d.MigrateDownOne(); e != nil {
		t.Fatal(e)
	}
	v, e = d.CurrentVersion()
	if e != nil || v != 1 {
		t.Fatalf("version after first down = %d, %v", v, e)
	}
	if e = d.MigrateDownOne(); e != nil {
		t.Fatal(e)
	}
	v, e = d.CurrentVersion()
	if e != nil || v != 0 {
		t.Fatalf("version after second down = %d, %v", v, e)
	}
	if e = d.MigrateUp(); e != nil {
		t.Fatal(e)
	}
	v, _ = d.CurrentVersion()
	if v != 2 {
		t.Fatalf("version after second up = %d", v)
	}
}

func TestMigrationCanonicalizesLegacyTimestamps(t *testing.T) {
	d, e := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	if e = d.MigrateUp(); e != nil {
		t.Fatal(e)
	}
	if e = d.MigrateDownOne(); e != nil {
		t.Fatal(e)
	}

	sha := strings.Repeat("a", 64)
	rel := "objects/aa/aa/" + sha
	if _, e = d.Exec(`INSERT INTO objects(sha256,size_bytes,created_at,stored_relpath,unreferenced_at) VALUES(?,?,?,?,?)`, sha, 1, "2026-10-11T00:00:00Z", rel, "2026-10-11T00:00:00.1Z"); e != nil {
		t.Fatal(e)
	}
	if _, e = d.Exec(`INSERT INTO artifact_records(id,sha256,created_at,original_filename,content_type,description,deleted_at) VALUES(?,?,?,?,?,?,?)`, "00000000-0000-4000-8000-000000000001", sha, "2026-10-11T00:00:00.1Z", "legacy.bin", nil, nil, "2026-10-11T00:00:00Z"); e != nil {
		t.Fatal(e)
	}
	if e = d.MigrateUp(); e != nil {
		t.Fatal(e)
	}

	r, e := d.Query(`SELECT created_at,unreferenced_at FROM objects WHERE sha256=?`, sha)
	if e != nil {
		t.Fatal(e)
	}
	if !r.Next() {
		r.Close()
		t.Fatal("object missing after timestamp migration")
	}
	created, unref := r.Text(0), r.Text(1)
	r.Close()
	if created != "2026-10-11T00:00:00.000000000Z" || unref != "2026-10-11T00:00:00.100000000Z" {
		t.Fatalf("object timestamps not canonicalized: created=%q unref=%q", created, unref)
	}

	r, e = d.Query(`SELECT created_at,deleted_at FROM artifact_records WHERE sha256=?`, sha)
	if e != nil {
		t.Fatal(e)
	}
	if !r.Next() {
		r.Close()
		t.Fatal("record missing after timestamp migration")
	}
	recordCreated, deleted := r.Text(0), r.Text(1)
	r.Close()
	if recordCreated != "2026-10-11T00:00:00.100000000Z" || deleted != "2026-10-11T00:00:00.000000000Z" {
		t.Fatalf("record timestamps not canonicalized: created=%q deleted=%q", recordCreated, deleted)
	}
}
