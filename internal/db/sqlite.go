package db

/*
#cgo pkg-config: sqlite3
#include <sqlite3.h>
#include <stdlib.h>
static int bind_text_transient(sqlite3_stmt *s, int i, const char *v, int n) { return sqlite3_bind_text(s,i,v,n,SQLITE_TRANSIENT); }
static int bind_blob_transient(sqlite3_stmt *s, int i, const void *v, int n) { return sqlite3_bind_blob(s,i,v,n,SQLITE_TRANSIENT); }
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

type DB struct {
	ptr *C.sqlite3
	mu  sync.Mutex
}
type Rows struct {
	stmt *C.sqlite3_stmt
	db   *DB
	done bool
}
type Tx struct {
	db     *DB
	active bool
}

func Open(path string) (*DB, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var p *C.sqlite3
	flags := C.SQLITE_OPEN_READWRITE | C.SQLITE_OPEN_CREATE | C.SQLITE_OPEN_FULLMUTEX
	if rc := C.sqlite3_open_v2(cpath, &p, C.int(flags), nil); rc != C.SQLITE_OK {
		if p != nil {
			msg := C.GoString(C.sqlite3_errmsg(p))
			C.sqlite3_close(p)
			return nil, errors.New(msg)
		}
		return nil, fmt.Errorf("sqlite open rc=%d", int(rc))
	}
	d := &DB{ptr: p}
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, e := d.Exec(q); e != nil {
			d.Close()
			return nil, e
		}
	}
	return d, nil
}
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ptr == nil {
		return nil
	}
	rc := C.sqlite3_close(d.ptr)
	if rc != C.SQLITE_OK {
		return fmt.Errorf("sqlite close rc=%d", int(rc))
	}
	d.ptr = nil
	return nil
}
func (d *DB) Ping() error { _, e := d.Exec("SELECT 1"); return e }

func bind(stmt *C.sqlite3_stmt, args []any) error {
	for i, a := range args {
		idx := C.int(i + 1)
		var rc C.int
		switch v := a.(type) {
		case nil:
			rc = C.sqlite3_bind_null(stmt, idx)
		case string:
			cs := C.CString(v)
			rc = C.bind_text_transient(stmt, idx, cs, C.int(len(v)))
			C.free(unsafe.Pointer(cs))
		case []byte:
			if len(v) == 0 {
				rc = C.bind_blob_transient(stmt, idx, nil, 0)
			} else {
				rc = C.bind_blob_transient(stmt, idx, unsafe.Pointer(&v[0]), C.int(len(v)))
			}
		case int:
			rc = C.sqlite3_bind_int64(stmt, idx, C.sqlite3_int64(v))
		case int64:
			rc = C.sqlite3_bind_int64(stmt, idx, C.sqlite3_int64(v))
		case bool:
			if v {
				rc = C.sqlite3_bind_int(stmt, idx, 1)
			} else {
				rc = C.sqlite3_bind_int(stmt, idx, 0)
			}
		default:
			return fmt.Errorf("unsupported sqlite bind type %T", a)
		}
		if rc != C.SQLITE_OK {
			return fmt.Errorf("sqlite bind rc=%d", int(rc))
		}
	}
	return nil
}
func prepare(p *C.sqlite3, q string, args ...any) (*C.sqlite3_stmt, error) {
	cq := C.CString(q)
	defer C.free(unsafe.Pointer(cq))
	var s *C.sqlite3_stmt
	if rc := C.sqlite3_prepare_v2(p, cq, -1, &s, nil); rc != C.SQLITE_OK {
		return nil, fmt.Errorf("sqlite prepare: %s", C.GoString(C.sqlite3_errmsg(p)))
	}
	if e := bind(s, args); e != nil {
		C.sqlite3_finalize(s)
		return nil, e
	}
	return s, nil
}
func execUnlocked(p *C.sqlite3, q string, args ...any) (int64, error) {
	s, e := prepare(p, q, args...)
	if e != nil {
		return 0, e
	}
	defer C.sqlite3_finalize(s)
	for {
		rc := C.sqlite3_step(s)
		if rc == C.SQLITE_ROW {
			continue
		}
		if rc != C.SQLITE_DONE {
			return 0, fmt.Errorf("sqlite step: %s", C.GoString(C.sqlite3_errmsg(p)))
		}
		return int64(C.sqlite3_changes(p)), nil
	}
}
func (d *DB) Exec(q string, args ...any) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ptr == nil {
		return 0, errors.New("database closed")
	}
	return execUnlocked(d.ptr, q, args...)
}
func (d *DB) Query(q string, args ...any) (*Rows, error) {
	d.mu.Lock()
	if d.ptr == nil {
		d.mu.Unlock()
		return nil, errors.New("database closed")
	}
	s, e := prepare(d.ptr, q, args...)
	if e != nil {
		d.mu.Unlock()
		return nil, e
	}
	return &Rows{stmt: s, db: d}, nil
}
func (r *Rows) Next() bool {
	if r.done {
		return false
	}
	rc := C.sqlite3_step(r.stmt)
	if rc == C.SQLITE_ROW {
		return true
	}
	r.done = true
	return false
}
func (r *Rows) Err() error {
	if !r.done {
		return nil
	}
	rc := C.sqlite3_errcode(r.db.ptr)
	if rc == C.SQLITE_OK || rc == C.SQLITE_DONE || rc == C.SQLITE_ROW {
		return nil
	}
	return errors.New(C.GoString(C.sqlite3_errmsg(r.db.ptr)))
}
func (r *Rows) Text(i int) string {
	p := C.sqlite3_column_text(r.stmt, C.int(i))
	if p == nil {
		return ""
	}
	return C.GoString((*C.char)(unsafe.Pointer(p)))
}
func (r *Rows) Int64(i int) int64 { return int64(C.sqlite3_column_int64(r.stmt, C.int(i))) }
func (r *Rows) Null(i int) bool   { return C.sqlite3_column_type(r.stmt, C.int(i)) == C.SQLITE_NULL }
func (r *Rows) Close() error {
	if r.stmt != nil {
		C.sqlite3_finalize(r.stmt)
		r.stmt = nil
	}
	if r.db != nil {
		r.db.mu.Unlock()
		r.db = nil
	}
	return nil
}
func (d *DB) WithTx(fn func(*Tx) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ptr == nil {
		return errors.New("database closed")
	}
	if _, e := execUnlocked(d.ptr, "BEGIN IMMEDIATE"); e != nil {
		return e
	}
	tx := &Tx{db: d, active: true}
	if e := fn(tx); e != nil {
		execUnlocked(d.ptr, "ROLLBACK")
		tx.active = false
		return e
	}
	if _, e := execUnlocked(d.ptr, "COMMIT"); e != nil {
		execUnlocked(d.ptr, "ROLLBACK")
		tx.active = false
		return e
	}
	tx.active = false
	return nil
}
func (t *Tx) Exec(q string, args ...any) (int64, error) {
	if !t.active {
		return 0, errors.New("transaction inactive")
	}
	return execUnlocked(t.db.ptr, q, args...)
}
func (t *Tx) QueryOne(q string, args []any, scan func(*Rows) error) error {
	if !t.active {
		return errors.New("transaction inactive")
	}
	s, e := prepare(t.db.ptr, q, args...)
	if e != nil {
		return e
	}
	r := &Rows{stmt: s}
	defer C.sqlite3_finalize(s)
	rc := C.sqlite3_step(s)
	if rc == C.SQLITE_DONE {
		return ErrNotFound
	}
	if rc != C.SQLITE_ROW {
		return errors.New(C.GoString(C.sqlite3_errmsg(t.db.ptr)))
	}
	return scan(r)
}

var ErrNotFound = errors.New("not found")
