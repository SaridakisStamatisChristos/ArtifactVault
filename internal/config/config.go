package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DBPath              string
	StorageRoot         string
	Bind                string
	MaxFilenameBytes    int
	MaxDescriptionBytes int
	MaxTags             int
	MaxTagBytes         int
	MaxArtifactBytes    int64
	GCGrace             time.Duration
	StagingTTL          time.Duration
	LogLevel            string
}

func Default() Config {
	return Config{
		DBPath: "./data/artifactvault.db", StorageRoot: "./data/storage", Bind: "127.0.0.1:8080",
		MaxFilenameBytes: 1024, MaxDescriptionBytes: 16 * 1024, MaxTags: 32, MaxTagBytes: 128,
		MaxArtifactBytes: 0, GCGrace: 24 * time.Hour, StagingTTL: 24 * time.Hour, LogLevel: "info",
	}
}

func FromEnv() (Config, error) {
	c := Default()
	if v := os.Getenv("ARTIFACTVAULT_DB_PATH"); v != "" {
		c.DBPath = v
	}
	if v := os.Getenv("ARTIFACTVAULT_STORAGE_ROOT"); v != "" {
		c.StorageRoot = v
	}
	if v := os.Getenv("ARTIFACTVAULT_BIND"); v != "" {
		c.Bind = v
	}
	if v := os.Getenv("ARTIFACTVAULT_LOG_LEVEL"); v != "" {
		c.LogLevel = strings.ToLower(v)
	}
	ints := []struct {
		name string
		dst  *int
	}{{"ARTIFACTVAULT_MAX_FILENAME_BYTES", &c.MaxFilenameBytes}, {"ARTIFACTVAULT_MAX_DESCRIPTION_BYTES", &c.MaxDescriptionBytes}, {"ARTIFACTVAULT_MAX_TAGS", &c.MaxTags}, {"ARTIFACTVAULT_MAX_TAG_BYTES", &c.MaxTagBytes}}
	for _, x := range ints {
		if v := os.Getenv(x.name); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 0 {
				return c, fmt.Errorf("%s must be a non-negative integer", x.name)
			}
			*x.dst = n
		}
	}
	if v := os.Getenv("ARTIFACTVAULT_MAX_ARTIFACT_BYTES"); v != "" {
		n, e := strconv.ParseInt(v, 10, 64)
		if e != nil || n < 0 {
			return c, fmt.Errorf("ARTIFACTVAULT_MAX_ARTIFACT_BYTES must be non-negative")
		}
		c.MaxArtifactBytes = n
	}
	durations := []struct {
		name string
		dst  *time.Duration
	}{{"ARTIFACTVAULT_GC_GRACE", &c.GCGrace}, {"ARTIFACTVAULT_STAGING_TTL", &c.StagingTTL}}
	for _, x := range durations {
		if v := os.Getenv(x.name); v != "" {
			d, e := time.ParseDuration(v)
			if e != nil || d < 0 {
				return c, fmt.Errorf("%s must be a non-negative duration", x.name)
			}
			*x.dst = d
		}
	}
	if c.DBPath == "" || c.StorageRoot == "" || c.Bind == "" {
		return c, fmt.Errorf("database path, storage root, and bind must be non-empty")
	}
	c.DBPath = filepath.Clean(c.DBPath)
	c.StorageRoot = filepath.Clean(c.StorageRoot)
	return c, nil
}
