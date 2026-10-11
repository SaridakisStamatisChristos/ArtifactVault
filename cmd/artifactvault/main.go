package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/cli"
	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/config"
	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/db"
	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/httpapi"
	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/metrics"
	"github.com/SaridakisStamatisChristos/ArtitactVault/internal/store"
)

func level(v string) slog.Level {
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
func logger(c config.Config) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level(c.LogLevel)}))
}
func serve() error {
	c, e := config.FromEnv()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(c.DBPath), 0o750); e != nil {
		return e
	}
	l := logger(c)
	d, e := db.Open(c.DBPath)
	if e != nil {
		return e
	}
	defer d.Close()
	if e = d.MigrateUp(); e != nil {
		return e
	}
	st, e := store.New(d, c)
	if e != nil {
		return e
	}
	if e = st.Recover(); e != nil {
		l.Warn("startup_recovery_partial", "error", e.Error())
	}
	met := &metrics.Metrics{}
	api := httpapi.New(st, c, l, met)
	srv := httpapi.HTTPServer(c.Bind, api.Handler())
	errc := make(chan error, 1)
	go func() {
		l.Info("server_start", "bind", c.Bind, "db", c.DBPath, "storage", c.StorageRoot)
		e := srv.ListenAndServe()
		if e != nil && e != http.ErrServerClosed {
			errc <- e
		} else {
			errc <- nil
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case e = <-errc:
		return e
	case <-sig:
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
func migrate(args []string) error {
	c, e := config.FromEnv()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(c.DBPath), 0o750); e != nil {
		return e
	}
	d, e := db.Open(c.DBPath)
	if e != nil {
		return e
	}
	defer d.Close()
	if len(args) == 0 || args[0] == "up" {
		return d.MigrateUp()
	}
	if args[0] == "down" {
		return d.MigrateDownOne()
	}
	if args[0] == "version" {
		v, e := d.CurrentVersion()
		if e == nil {
			fmt.Println(v)
		}
		return e
	}
	return fmt.Errorf("migrate expects up, down, or version")
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if e := serve(); e != nil {
			fmt.Fprintln(os.Stderr, "error:", e)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if e := migrate(os.Args[2:]); e != nil {
			fmt.Fprintln(os.Stderr, "error:", e)
			os.Exit(1)
		}
		return
	}
	os.Exit(cli.New().Run(os.Args[1:]))
}
