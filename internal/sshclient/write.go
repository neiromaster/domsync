package sshclient

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"strings"
)

const (
	defaultWriteMode    fs.FileMode = 0o644
	defaultBackupSuffix             = ".domsync.bak"
)

type WriteOptions struct {
	Mode         fs.FileMode
	Backup       bool
	BackupSuffix string
}

type WriteResult struct {
	BackupPath string
}

func (c *Client) WriteFileAtomic(ctx context.Context, path string, data []byte, opts WriteOptions) (WriteResult, error) {
	mode := opts.Mode.Perm()
	if mode == 0 {
		mode = defaultWriteMode
	}
	backup := ""
	if opts.Backup {
		suffix := opts.BackupSuffix
		if suffix == "" {
			suffix = defaultBackupSuffix
		}
		backup = path + suffix
	}
	ctx, cancel := withFileTimeout(ctx)
	defer cancel()
	result, err := c.runWithStdin(ctx, writeScript(path, backup, mode), bytes.NewReader(data), 0)
	if err != nil {
		return WriteResult{}, fmt.Errorf("write %s: %w", path, err)
	}
	if result.ExitCode != 0 {
		return WriteResult{}, fmt.Errorf("write %s: exit %d: %s", path, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	written, err := c.ReadFile(ctx, path)
	if err != nil {
		return WriteResult{}, fmt.Errorf("verify %s: %w", path, err)
	}
	if !bytes.Equal(written, data) {
		return WriteResult{}, fmt.Errorf("verify %s: read back %d bytes, wrote %d: %w", path, len(written), len(data), ErrVerify)
	}
	return WriteResult{BackupPath: strings.TrimSpace(result.Stdout)}, nil
}

func writeScript(path, backup string, mode fs.FileMode) string {
	return fmt.Sprintf(`set -eu
path=%s
backup=%s
dir=$(dirname "$path")
tmp="$dir/.domsync.$$.tmp"
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
chmod %04o "$tmp"
if [ -f "$path" ]; then
  cp -p "$path" "$backup" && printf '%%s\n' "$backup"
fi
mv "$tmp" "$path"
`, shellQuote(path), shellQuote(backup), mode)
}
