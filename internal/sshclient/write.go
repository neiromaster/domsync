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
	backupPath := strings.TrimSpace(result.Stdout)
	written, err := c.ReadFile(ctx, path)
	if err != nil {
		return WriteResult{BackupPath: backupPath}, fmt.Errorf("verify %s: %w", path, err)
	}
	if !bytes.Equal(written, data) {
		return WriteResult{BackupPath: backupPath}, fmt.Errorf("verify %s: read back %d bytes, wrote %d: %w", path, len(written), len(data), ErrVerify)
	}
	return WriteResult{BackupPath: backupPath}, nil
}

func writeScript(path, backup string, mode fs.FileMode) string {
	return fmt.Sprintf(`set -eu
path=%s
backup=%s
target=$(readlink -f "$path" 2>/dev/null || printf '%%s' "$path")
if [ -d "$target" ]; then echo "domsync: target is a directory" >&2; exit 1; fi
tmp="$(dirname "$target")/.domsync.$$.tmp"
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
chmod %04o "$tmp"
if [ -n "$backup" ] && [ -e "$target" ]; then
  cp -p "$target" "$backup"
fi
mv "$tmp" "$target"
if [ -n "$backup" ] && [ -e "$backup" ]; then printf '%%s\n' "$backup"; fi
`, shellQuote(path), shellQuote(backup), mode)
}
