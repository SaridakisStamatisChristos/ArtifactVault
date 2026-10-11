package db

import (
	"path/filepath"
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
	if e != nil || v != 1 {
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
	if e != nil || v != 0 {
		t.Fatalf("version after down = %d, %v", v, e)
	}
	if e = d.MigrateUp(); e != nil {
		t.Fatal(e)
	}
	v, _ = d.CurrentVersion()
	if v != 1 {
		t.Fatalf("version after second up = %d", v)
	}
}
