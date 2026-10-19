package jobs

import (
	"testing"
	"time"

	"github.com/rangoDJ/meshmanager/internal/store"
)

func TestScanDue(t *testing.T) {
	now := time.Now()
	attempt := func(ago time.Duration) *store.Job { return &store.Job{CreatedAt: now.Add(-ago)} }

	cases := []struct {
		name string
		src  store.Source
		want bool
	}{
		{"never scanned", store.Source{Enabled: true}, true},
		{"disabled", store.Source{Enabled: false}, false},
		{"recent", store.Source{Enabled: true, LastScan: attempt(time.Hour)}, false},
		{"stale", store.Source{Enabled: true, LastScan: attempt(13 * time.Hour)}, true},
		{"exactly due", store.Source{Enabled: true, LastScan: attempt(12 * time.Hour)}, true},
	}
	for _, c := range cases {
		if got := scanDue(c.src, 12*time.Hour, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
