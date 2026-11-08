package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func applyServer(t *testing.T, archive []byte, checksums string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"domsync_1.2.3_darwin_arm64.tar.gz","browser_download_url":"` + "http://" + r.Host + `/dl/archive"},{"name":"checksums.txt","browser_download_url":"http://` + r.Host + `/dl/checksums"}]}`))
		case "/dl/checksums":
			_, _ = w.Write([]byte(checksums))
		case "/dl/archive":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestApplyVerifiesAndReplaces(t *testing.T) {
	archive := tarGz(t, map[string]string{"domsync": "new-binary", "README.md": "x"})
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  domsync_1.2.3_darwin_arm64.tar.gz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"domsync_1.2.3_darwin_arm64.tar.gz","browser_download_url":"` + "http://" + r.Host + `/dl/archive"},{"name":"checksums.txt","browser_download_url":"http://` + r.Host + `/dl/checksums"}]}`))
		case "/dl/checksums":
			_, _ = w.Write([]byte(checksums))
		case "/dl/archive":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "domsync")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Apply(context.Background(), ApplyOptions{
		BaseURL: srv.URL, Repo: "o/r", Current: "v1.0.0",
		GOOS: "darwin", GOARCH: "arm64", Target: target, Client: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "v1.2.3" {
		t.Fatalf("version = %q", got)
	}
	body, _ := os.ReadFile(target)
	if string(body) != "new-binary" {
		t.Fatalf("binary = %q", body)
	}
}

func TestApplyChecksumMismatch(t *testing.T) {
	archive := tarGz(t, map[string]string{"domsync": "new-binary"})
	other := sha256.Sum256([]byte("not the archive"))
	checksums := hex.EncodeToString(other[:]) + "  domsync_1.2.3_darwin_arm64.tar.gz\n"
	srv := applyServer(t, archive, checksums)
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "domsync")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Apply(context.Background(), ApplyOptions{
		BaseURL: srv.URL, Repo: "o/r", Current: "v1.0.0",
		GOOS: "darwin", GOARCH: "arm64", Target: target, Client: srv.Client(),
	})
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if got != "" {
		t.Fatalf("version = %q, want empty", got)
	}
	body, _ := os.ReadFile(target)
	if string(body) != "old" {
		t.Fatalf("target modified on mismatch: %q", body)
	}
}

func TestApplyRejectsTraversal(t *testing.T) {
	archive := tarGz(t, map[string]string{"../evil": "payload"})
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  domsync_1.2.3_darwin_arm64.tar.gz\n"
	srv := applyServer(t, archive, checksums)
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "domsync")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(context.Background(), ApplyOptions{
		BaseURL: srv.URL, Repo: "o/r", Current: "v1.0.0",
		GOOS: "darwin", GOARCH: "arm64", Target: target, Client: srv.Client(),
	})
	if err == nil {
		t.Fatal("expected traversal error")
	}
	body, _ := os.ReadFile(target)
	if string(body) != "old" {
		t.Fatalf("target modified on traversal: %q", body)
	}
}
