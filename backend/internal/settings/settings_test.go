package settings

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestDefaultsWhenMissing(t *testing.T) {
	s := NewStore(t.TempDir())
	if got := s.Get(); got != Defaults() {
		t.Fatalf("got %+v, want defaults", got)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	want := Settings{HashConcurrency: 8, ScanIntervalHours: 0, ThumbSize: 512, IndexVideos: true}
	if err := NewStore(dir).Save(want); err != nil {
		t.Fatal(err)
	}
	// A different store (another process) sees the saved values.
	if got := NewStore(dir).Get(); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	var onDisk map[string]any
	b, _ := os.ReadFile(NewStore(dir).Path())
	if err := json.Unmarshal(b, &onDisk); err != nil || onDisk["thumb_size"] != 512.0 {
		t.Fatalf("file content: %s", b)
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := os.WriteFile(s.Path(), []byte(`{"thumb_size": 256}`), 0o644); err != nil {
		t.Fatal(err)
	}
	want := Defaults()
	want.ThumbSize = 256
	if got := s.Get(); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPicksUpExternalEdits(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Save(Defaults()); err != nil {
		t.Fatal(err)
	}
	_ = s.Get()

	if err := os.WriteFile(s.Path(), []byte(`{"hash_concurrency": 2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(s.Path(), future, future)
	if got := s.Get().HashConcurrency; got != 2 {
		t.Fatalf("hash_concurrency = %d, want 2 after external edit", got)
	}
}

func TestInvalidFileKeepsLastGood(t *testing.T) {
	s := NewStore(t.TempDir())
	good := Defaults()
	good.ThumbSize = 512
	if err := s.Save(good); err != nil {
		t.Fatal(err)
	}
	_ = s.Get()

	for _, bad := range []string{`{not json`, `{"thumb_size": 5}`} {
		_ = os.WriteFile(s.Path(), []byte(bad), 0o644)
		future := time.Now().Add(time.Hour)
		_ = os.Chtimes(s.Path(), future, future)
		if got := s.Get(); got != good {
			t.Errorf("after %q: got %+v, want last good %+v", bad, got, good)
		}
	}
}

func TestValidate(t *testing.T) {
	bad := []Settings{
		{HashConcurrency: 0, ThumbSize: 384},
		{HashConcurrency: 4, ThumbSize: 64},
		{HashConcurrency: 4, ThumbSize: 384, ScanIntervalHours: -1},
	}
	for _, v := range bad {
		if v.Validate() == nil {
			t.Errorf("%+v should be invalid", v)
		}
		if NewStore(t.TempDir()).Save(v) == nil {
			t.Errorf("Save(%+v) should fail", v)
		}
	}
}

func TestEnsureFile(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	custom := Defaults()
	custom.HashConcurrency = 9
	if err := s.Save(custom); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	if got := NewStore(t.TempDir()); got == nil {
		t.Fatal("unreachable")
	}
	if got := s.Get().HashConcurrency; got != 9 {
		t.Fatalf("EnsureFile overwrote an existing file: hash_concurrency = %d", got)
	}
}
