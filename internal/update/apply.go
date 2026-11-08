package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	maxReleaseSize     = 1 << 20
	maxChecksumsSize   = 1 << 20
	maxArchiveSize     = 100 << 20
	checksumsAssetName = "checksums.txt"
)

type ApplyOptions struct {
	BaseURL string
	Repo    string
	Current string
	GOOS    string
	GOARCH  string
	Target  string
	Client  *http.Client
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func Apply(ctx context.Context, opts ApplyOptions) (string, error) {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	url := strings.TrimRight(opts.BaseURL, "/") + "/repos/" + opts.Repo + "/releases/latest"
	release, err := fetchRelease(ctx, client, url, opts.Current)
	if err != nil {
		return "", err
	}
	archiveName := AssetName(release.TagName, opts.GOOS, opts.GOARCH)
	archiveURL, checksumsURL := assetURLs(release.Assets, archiveName)
	if archiveURL == "" {
		return "", fmt.Errorf("update: release %s has no asset %q", release.TagName, archiveName)
	}
	if checksumsURL == "" {
		return "", fmt.Errorf("update: release %s has no %s asset", release.TagName, checksumsAssetName)
	}
	checksums, err := download(ctx, client, checksumsURL, opts.Current, maxChecksumsSize)
	if err != nil {
		return "", err
	}
	want, ok := checksumFor(checksums, archiveName)
	if !ok {
		return "", fmt.Errorf("update: %s has no entry for %s", checksumsAssetName, archiveName)
	}
	archive, err := download(ctx, client, archiveURL, opts.Current, maxArchiveSize)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return "", fmt.Errorf("update: checksum mismatch for %s: got %s, want %s", archiveName, got, want)
	}
	data, err := extractBinary(archive, opts.GOOS)
	if err != nil {
		return "", err
	}
	if err := ReplaceExecutable(opts.Target, data); err != nil {
		return "", fmt.Errorf("update: replace %s: %w", opts.Target, err)
	}
	return release.TagName, nil
}

func fetchRelease(ctx context.Context, client *http.Client, url, current string) (*githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "domsync/"+current)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("update: GET %s: unexpected status %s", url, resp.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseSize)).Decode(&release); err != nil {
		return nil, fmt.Errorf("update: GET %s: %w", url, err)
	}
	if release.TagName == "" {
		return nil, fmt.Errorf("update: GET %s: empty release tag", url)
	}
	return &release, nil
}

func assetURLs(assets []releaseAsset, archiveName string) (archiveURL, checksumsURL string) {
	for _, asset := range assets {
		switch asset.Name {
		case archiveName:
			archiveURL = asset.BrowserDownloadURL
		case checksumsAssetName:
			checksumsURL = asset.BrowserDownloadURL
		}
	}
	return archiveURL, checksumsURL
}

func download(ctx context.Context, client *http.Client, url, current string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "domsync/"+current)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("update: GET %s: unexpected status %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("update: GET %s: exceeds %d byte limit", url, limit)
	}
	return data, nil
}

func checksumFor(checksums []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(strings.TrimSuffix(line, "\r"))
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		return fields[0], true
	}
	return "", false
}

func extractBinary(archive []byte, goos string) ([]byte, error) {
	if goos == "windows" {
		return extractZip(archive, "domsync.exe")
	}
	return extractTarGz(archive, "domsync")
}

func extractTarGz(archive []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("update: read archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("update: read archive: %w", err)
		}
		cleaned, err := cleanEntryName(hdr.Name)
		if err != nil {
			return nil, err
		}
		if path.Base(cleaned) != binaryName || hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxArchiveSize+1))
		if err != nil {
			return nil, fmt.Errorf("update: read %s from archive: %w", binaryName, err)
		}
		if int64(len(data)) > maxArchiveSize {
			return nil, fmt.Errorf("update: %s exceeds %d byte limit", binaryName, maxArchiveSize)
		}
		return data, nil
	}
	return nil, fmt.Errorf("update: archive has no entry %q", binaryName)
}

func extractZip(archive []byte, binaryName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("update: read archive: %w", err)
	}
	for _, file := range zr.File {
		cleaned, err := cleanEntryName(file.Name)
		if err != nil {
			return nil, err
		}
		if path.Base(cleaned) != binaryName || file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("update: open %s in archive: %w", binaryName, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxArchiveSize+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("update: read %s from archive: %w", binaryName, err)
		}
		if int64(len(data)) > maxArchiveSize {
			return nil, fmt.Errorf("update: %s exceeds %d byte limit", binaryName, maxArchiveSize)
		}
		return data, nil
	}
	return nil, fmt.Errorf("update: archive has no entry %q", binaryName)
}

func cleanEntryName(name string) (string, error) {
	cleaned := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("update: archive entry %q escapes the archive root", name)
	}
	return cleaned, nil
}
