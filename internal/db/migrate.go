package db

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type migration struct {
	version  int
	up, down string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	m := map[int]*migration{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		parts := strings.Split(e.Name(), ".")
		if len(parts) != 3 {
			continue
		}
		prefix := strings.SplitN(parts[0], "_", 2)[0]
		v, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("bad migration name %s", e.Name())
		}
		b, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		x := m[v]
		if x == nil {
			x = &migration{version: v}
			m[v] = x
		}
		if parts[1] == "up" {
			x.up = string(b)
		} else if parts[1] == "down" {
			x.down = string(b)
		}
	}
	vs := make([]int, 0, len(m))
	for v := range m {
		vs = append(vs, v)
	}
	sort.Ints(vs)
	out := make([]migration, 0, len(vs))
	for _, v := range vs {
		if m[v].up == "" {
			return nil, fmt.Errorf("migration %d missing up", v)
		}
		out = append(out, *m[v])
	}
	return out, nil
}

func (d *DB) EnsureMigrationTable() error {
	_, e := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`)
	return e
}
func (d *DB) CurrentVersion() (int, error) {
	if err := d.EnsureMigrationTable(); err != nil {
		return 0, err
	}
	r, e := d.Query(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`)
	if e != nil {
		return 0, e
	}
	defer r.Close()
	if !r.Next() {
		return 0, fmt.Errorf("migration version query returned no row")
	}
	return int(r.Int64(0)), nil
}

func execScriptTx(tx *Tx, script string) error {
	for _, q := range strings.Split(script, ";") {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if _, e := tx.Exec(q); e != nil {
			return fmt.Errorf("migration statement failed: %w", e)
		}
	}
	return nil
}
func (d *DB) MigrateUp() error {
	ms, e := loadMigrations()
	if e != nil {
		return e
	}
	if e = d.EnsureMigrationTable(); e != nil {
		return e
	}
	cur, e := d.CurrentVersion()
	if e != nil {
		return e
	}
	for _, m := range ms {
		if m.version <= cur {
			continue
		}
		if e := d.WithTx(func(tx *Tx) error {
			if e := execScriptTx(tx, m.up); e != nil {
				return e
			}
			_, e := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,strftime('%Y-%m-%dT%H:%M:%fZ','now'))`, m.version)
			return e
		}); e != nil {
			return fmt.Errorf("apply migration %d: %w", m.version, e)
		}
	}
	return nil
}
func (d *DB) MigrateDownOne() error {
	ms, e := loadMigrations()
	if e != nil {
		return e
	}
	cur, e := d.CurrentVersion()
	if e != nil {
		return e
	}
	if cur == 0 {
		return nil
	}
	var target *migration
	for i := range ms {
		if ms[i].version == cur {
			target = &ms[i]
			break
		}
	}
	if target == nil || target.down == "" {
		return fmt.Errorf("migration %d has no rollback", cur)
	}
	return d.WithTx(func(tx *Tx) error {
		if e := execScriptTx(tx, target.down); e != nil {
			return e
		}
		_, e := tx.Exec(`DELETE FROM schema_migrations WHERE version=?`, cur)
		return e
	})
}
