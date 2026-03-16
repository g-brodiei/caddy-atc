package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	repo        = "g-brodiei/caddy-atc"
	cacheTTL    = 24 * time.Hour
	httpTimeout = 5 * time.Second
)

// CheckResult holds the outcome of a version check.
type CheckResult struct {
	LatestVersion  string
	CurrentVersion string
	UpdateAvail    bool
}

type cacheEntry struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

func (c *cacheEntry) isStale() bool {
	return time.Since(c.CheckedAt) > cacheTTL
}

func cachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".caddy-atc", "update-check.json")
}

func loadCache(path string) (*cacheEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var c cacheEntry
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, nil // treat corrupt cache as missing
	}
	return &c, nil
}

func saveCache(path string, entry *cacheEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0700)
	os.WriteFile(path, data, 0600)
}

// fetchLatestVersion queries the GitHub API for the latest release tag.
func fetchLatestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("no tag_name in response")
	}
	return release.TagName, nil
}

// isNewer returns true if latest is a higher semver than current.
// Both should be in "vMAJOR.MINOR.PATCH" format.
func isNewer(latest, current string) bool {
	l := parseVersion(latest)
	c := parseVersion(current)
	if l == nil || c == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		// Strip any suffix after digits (e.g., "0-rc1")
		for j, c := range p {
			if c < '0' || c > '9' {
				p = p[:j]
				break
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}

// CheckAsync starts a version check in the background and returns a channel.
// The channel receives a result (or nil if skipped/failed). Non-blocking.
func CheckAsync(currentVersion string) <-chan *CheckResult {
	ch := make(chan *CheckResult, 1)
	if currentVersion == "dev" {
		ch <- nil
		return ch
	}

	go func() {
		result := checkVersion(currentVersion)
		ch <- result
	}()

	return ch
}

// CheckSync performs a blocking version check.
func CheckSync(currentVersion string) *CheckResult {
	if currentVersion == "dev" {
		return nil
	}
	return checkVersion(currentVersion)
}

func checkVersion(currentVersion string) *CheckResult {
	cp := cachePath()
	if cp == "" {
		return nil
	}

	// Try cache first
	cached, _ := loadCache(cp)
	if cached != nil && !cached.isStale() {
		return &CheckResult{
			LatestVersion:  cached.LatestVersion,
			CurrentVersion: currentVersion,
			UpdateAvail:    isNewer(cached.LatestVersion, currentVersion),
		}
	}

	// Fetch from GitHub
	latest, err := fetchLatestVersion()
	if err != nil {
		return nil // silent failure
	}

	// Update cache
	saveCache(cp, &cacheEntry{
		LatestVersion: latest,
		CheckedAt:     time.Now(),
	})

	return &CheckResult{
		LatestVersion:  latest,
		CurrentVersion: currentVersion,
		UpdateAvail:    isNewer(latest, currentVersion),
	}
}
