// Package settings holds the app settings edited on the Settings page. They live in
// DATA_DIR/config.json, which the API and every worker read, so a change applies
// everywhere without a restart. Missing keys fall back to defaults.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const FileName = "config.json"

type Settings struct {
	// Files hashed in parallel during a scan.
	HashConcurrency int `json:"hash_concurrency"`
	// Rescan every enabled source this often; 0 means only on demand.
	ScanIntervalHours int `json:"scan_interval_hours"`
	// Edge length of new thumbnails in pixels.
	ThumbSize int `json:"thumb_size"`
	// Index video files alongside models.
	IndexVideos bool `json:"index_videos"`
}

func Defaults() Settings {
	return Settings{
		HashConcurrency:   4,
		ScanIntervalHours: 12,
		ThumbSize:         384,
		IndexVideos:       false,
	}
}

// Validate returns a user-facing problem with the values, or nil.
func (s Settings) Validate() error {
	switch {
	case s.HashConcurrency < 1 || s.HashConcurrency > 32:
		return errors.New("hash_concurrency must be between 1 and 32")
	case s.ScanIntervalHours < 0 || s.ScanIntervalHours > 720:
		return errors.New("scan_interval_hours must be between 0 and 720")
	case s.ThumbSize < 128 || s.ThumbSize > 1024:
		return errors.New("thumb_size must be between 128 and 1024")
	}
	return nil
}

func (s Settings) ScanInterval() time.Duration {
	return time.Duration(s.ScanIntervalHours) * time.Hour
}

// Store reads and writes the settings file, re-reading it whenever it changes on disk
// (another process saved it, or someone edited it by hand).
type Store struct {
	path string

	mu      sync.Mutex
	current Settings
	modTime time.Time
	size    int64
	loaded  bool
}

func NewStore(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, FileName), current: Defaults()}
}

func (s *Store) Path() string { return s.path }

// Get returns the current settings. It never fails: a missing file means defaults,
// and an unreadable or invalid file keeps the last good values.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.current, s.loaded = Defaults(), false
		return s.current
	}
	if err != nil {
		slog.Warn("settings: cannot stat file, using last known values", "path", s.path, "err", err)
		return s.current
	}
	if s.loaded && st.ModTime().Equal(s.modTime) && st.Size() == s.size {
		return s.current
	}

	v, err := readFile(s.path)
	if err != nil {
		slog.Warn("settings: ignoring invalid file, using last known values", "path", s.path, "err", err)
	} else {
		s.current = v
	}
	s.modTime, s.size, s.loaded = st.ModTime(), st.Size(), true
	return s.current
}

func readFile(path string) (Settings, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, err
	}
	v := Defaults() // keys missing from the file keep their defaults
	if err := json.Unmarshal(b, &v); err != nil {
		return Settings{}, err
	}
	if err := v.Validate(); err != nil {
		return Settings{}, err
	}
	return v, nil
}

// Save validates and writes the settings atomically.
func (s *Store) Save(v Settings) error {
	if err := v.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeAtomic(s.path, v); err != nil {
		return fmt.Errorf("save settings to %s: %w", s.path, err)
	}
	s.current, s.loaded = v, false // re-stat on next Get
	return nil
}

// EnsureFile writes the defaults if no settings file exists yet, so it can be found
// and edited by hand.
func (s *Store) EnsureFile() error {
	if _, err := os.Stat(s.path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.Save(Defaults())
}

func writeAtomic(path string, v Settings) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
