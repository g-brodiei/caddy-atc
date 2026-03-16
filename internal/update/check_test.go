package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadCache_Missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "update-check.json")
	c, err := loadCache(path)
	if err != nil {
		t.Fatalf("loadCache() error = %v", err)
	}
	if c != nil {
		t.Errorf("expected nil cache for missing file, got %+v", c)
	}
}

func TestLoadCache_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "update-check.json")
	data := `{"latest_version":"v0.5.0","checked_at":"2026-03-16T12:00:00Z"}`
	os.WriteFile(path, []byte(data), 0644)

	c, err := loadCache(path)
	if err != nil {
		t.Fatalf("loadCache() error = %v", err)
	}
	if c.LatestVersion != "v0.5.0" {
		t.Errorf("expected v0.5.0, got %s", c.LatestVersion)
	}
}

func TestCacheIsStale(t *testing.T) {
	fresh := &cacheEntry{
		LatestVersion: "v0.5.0",
		CheckedAt:     time.Now().Add(-1 * time.Hour),
	}
	if fresh.isStale() {
		t.Error("1-hour-old cache should not be stale")
	}

	stale := &cacheEntry{
		LatestVersion: "v0.5.0",
		CheckedAt:     time.Now().Add(-25 * time.Hour),
	}
	if !stale.isStale() {
		t.Error("25-hour-old cache should be stale")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v0.6.0", "v0.5.0", true},
		{"v0.5.0", "v0.5.0", false},
		{"v0.4.0", "v0.5.0", false},
		{"v1.0.0", "v0.9.9", true},
		{"v0.5.1", "v0.5.0", true},
	}
	for _, tt := range tests {
		got := isNewer(tt.latest, tt.current)
		if got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}
