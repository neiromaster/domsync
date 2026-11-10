package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neiromaster/domsync/internal/version"
	"golang.org/x/mod/semver"
)

type Info struct {
	Current   string
	Latest    string
	Available bool
}

type Checker struct {
	BaseURL   string
	Repo      string
	Client    *http.Client
	CachePath string
}

type cacheEntry struct {
	ETag string `json:"etag"`
	Tag  string `json:"tag"`
}

var defaultClient = &http.Client{Timeout: 2 * time.Second}

func CacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "domsync"), nil
}

func NewChecker() *Checker {
	c := &Checker{
		BaseURL: "https://api.github.com",
		Repo:    "neiromaster/domsync",
		Client:  defaultClient,
	}
	if dir, err := CacheDir(); err == nil {
		c.CachePath = filepath.Join(dir, "update.json")
	}
	return c
}

func (c *Checker) Check(ctx context.Context, current string) (Info, error) {
	info := Info{Current: current}
	if current == "" || current == "dev" {
		return info, nil
	}
	latest, err := c.LatestTag(ctx)
	if err != nil {
		return info, err
	}
	info.Latest = latest
	info.Available = semver.Compare(canonical(latest), canonical(current)) > 0
	return info, nil
}

func (c *Checker) LatestTag(ctx context.Context) (string, error) {
	cache := c.loadCache()
	url := c.releasesURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "domsync/"+version.Version)
	if cache.ETag != "" {
		req.Header.Set("If-None-Match", cache.ETag)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNotModified:
		_, _ = io.Copy(io.Discard, resp.Body)
		if cache.Tag == "" {
			return "", fmt.Errorf("update: GET %s: 304 without a cached tag", url)
		}
		return cache.Tag, nil
	case http.StatusOK:
		var release struct {
			TagName string `json:"tag_name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return "", err
		}
		if release.TagName == "" {
			return "", fmt.Errorf("update: GET %s: empty release tag", url)
		}
		c.saveCache(cacheEntry{ETag: resp.Header.Get("ETag"), Tag: release.TagName})
		return release.TagName, nil
	default:
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("update: GET %s: unexpected status %s", url, resp.Status)
	}
}

func (c *Checker) httpClient() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultClient
}

func (c *Checker) releasesURL() string {
	return strings.TrimRight(c.BaseURL, "/") + "/repos/" + c.Repo + "/releases/latest"
}

func (c *Checker) loadCache() cacheEntry {
	if c.CachePath == "" {
		return cacheEntry{}
	}
	data, err := os.ReadFile(c.CachePath)
	if err != nil {
		return cacheEntry{}
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return cacheEntry{}
	}
	return entry
}

func (c *Checker) saveCache(entry cacheEntry) {
	if c.CachePath == "" {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	dir := filepath.Dir(c.CachePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".update-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(name, c.CachePath)
}

func canonical(v string) string {
	if v == "" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}
