package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCheckAvailable(t *testing.T) {
	var etagHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			etagHits++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.10.0", "prerelease": false})
	}))
	defer srv.Close()
	c := &Checker{BaseURL: srv.URL, Repo: "o/r", Client: srv.Client(), CachePath: filepath.Join(t.TempDir(), "cache.json")}
	info, err := c.Check(context.Background(), "v1.9.0")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Latest != "v1.10.0" {
		t.Fatalf("info = %+v", info)
	}
	info, err = c.Check(context.Background(), "v1.9.0")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || etagHits != 1 {
		t.Fatalf("second = %+v, etagHits=%d", info, etagHits)
	}
}

func TestCheckSameVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.9.0"})
	}))
	defer srv.Close()
	c := &Checker{BaseURL: srv.URL, Repo: "o/r", Client: srv.Client(), CachePath: filepath.Join(t.TempDir(), "c.json")}
	info, err := c.Check(context.Background(), "v1.9.0")
	if err != nil || info.Available {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
}

func TestCheckBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{")) }))
	defer srv.Close()
	c := &Checker{BaseURL: srv.URL, Repo: "o/r", Client: srv.Client(), CachePath: filepath.Join(t.TempDir(), "c.json")}
	if _, err := c.Check(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("expected error")
	}
}
