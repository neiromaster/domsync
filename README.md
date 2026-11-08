# domsync

`domsync` keeps a single domain list in sync between your laptop and an OpenClash rule-provider file on a router. It reads the local list and the provider file over SSH, merges them (the local entries win, router-only domains go into an `other` section), validates everything, then atomically rewrites each file that changed and optionally restarts OpenClash.

It never silently rewrites anything it cannot parse: an ambiguous local file or a malformed provider line stops the run before a single byte is written.

**Not supported in v1:** homeproxy, subscription URLs or any remote source other than the current provider file, SSH password authentication, `ProxyJump`, a fallback to the system `ssh`, IDN domains (use punycode), and cryptographic verification of updates inside the CLI. There is no daemon, watcher or scheduler; run `domsync` from cron or by hand.

## Install

Homebrew (cask tap):

```sh
brew install --cask neiromaster/tap/domsync
```

The cask works on macOS and, with Homebrew 6.0 or newer, on Linux.

npm (macOS, Linux and Windows):

```sh
npm i -g domsync
```

Install script (macOS and Linux):

```sh
curl -fsSL https://raw.githubusercontent.com/neiromaster/domsync/main/install.sh | sh
```

The script downloads the release archive and `checksums.txt`, verifies the SHA-256 checksum, and installs to `$DOMSYNC_INSTALL_DIR` (default `$HOME/.local/bin`). Set `DOMSYNC_VERSION` (for example `1.2.3` or `v1.2.3`) to pin a release instead of the latest one. It never uses `sudo`; if the destination is not writable it prints instructions instead. If `cosign` is installed it also verifies the Sigstore bundle, otherwise it prints the manual verification commands.

## Quick start

```sh
cp domsync.example.yaml domsync.yaml
cat > domains.txt <<'EOF'
[media]
+.example.com
+.example.net

[misc]
+.example.org
EOF
```

Edit `domsync.yaml` for your router (`target.host`, `target.user`, `target.ssh_key`, `target.path`), make sure the router's host key is already in `~/.ssh/known_hosts`, then run:

```sh
domsync
```

The config is looked up in this order: `--config <path>`, the `DOMSYNC_CONFIG` environment variable, then `./domsync.yaml` in the current directory. Relative paths inside the config (`source.domains`, `target.ssh_key`, `target.known_hosts`, `logging.file`) are resolved against the config file's directory; `~` and `~/...` are expanded to your home directory.

A first run needs the router's host key in `known_hosts`. If it is missing, the error includes the SHA-256 fingerprint and the exact `ssh-keyscan -H ... >> ~/.ssh/known_hosts` command to add it.

## OpenClash provider

Add this provider to the OpenClash configuration:

```yaml
rule-providers:
  domsync:
    type: file
    behavior: domain
    format: text
    path: ./rule_provider/domsync.txt
```

`format: text` is **mandatory**. Without it Mihomo parses the file as YAML, the provider ends up empty, and OpenClash gives no error.

`target.path` in `domsync.yaml` is the absolute path of the same file; OpenClash keeps provider files under `/etc/openclash/rule_provider/`.

`/etc/init.d/openclash reload` does **not** re-read provider files. Use `restart`, which is why the example config's post-write command is `/etc/init.d/openclash restart`. Any other command that genuinely reloads providers works too.

In the provider file, `# [section]` lines are comments for humans only: Mihomo ignores lines starting with `#` or `//`. domsync writes section headers only in that commented form, because a bare `[media]` line would be accepted by Mihomo as a literal domain and silently inflate the rule count. Comments are never preserved from the router: the file is regenerated from the local state.

## Local file format

```text
# A note about the whole file

[media]
+.bitru.org
+.nnmclub.to
-+.servarr.com

[misc]
+.example.org
+.servarr.com
```

### Sections

A section header is `[name]` on its own line, optionally with surrounding whitespace. Names must be non-empty, at most 64 characters, free of control characters, and unique. Section order follows first appearance in the local file, but the `target.other_section` section (default `other`) is always rendered last. Empty sections are dropped, together with their comments.

A file with no section headers is a flat list and stays flat; in that case no `other` section is ever created, even if router-only domains are adopted.

### Patterns

Every positive entry is a Mihomo domain pattern:

| Form | Meaning |
|---|---|
| `example.com` | that exact domain |
| `.example.com` | subdomains only |
| `*.example.com` | exactly one extra label |
| `+.example.com` | the domain and all subdomains |
| `*.*.example.com` | wildcards on two labels |

Rules enforced by the parser:

- `+` may appear only as the whole first label and only when at least one more label follows; `*` must occupy a whole label.
- Labels may contain ASCII letters, digits, `_` and `-`; a hyphen may not be the first or last character of a label.
- A slash is forbidden, a trailing dot is rejected, and non-ASCII is rejected with a punycode hint.
- Entries must start at column 0: leading or trailing whitespace is an error.
- The pattern is lowercased and otherwise kept verbatim. `example.com`, `.example.com`, `*.example.com` and `+.example.com` are different rules and domsync never canonicalizes one into another.

### Deletion and movement

A leading `-` is the only structural marker: `-` followed by any valid pattern is a deletion directive. A `+` or `*` inside the pattern is Mihomo syntax, not a sign.

- `-pattern` with no positive entry for the same pattern **deletes** it from the router and from the local snapshot.
- `-pattern` plus a positive entry for the same pattern in a **different** section **moves** it to that section. On the router only the section comment changes.
- `-pattern` and a positive entry in the **same** section is redundant: the positive entry wins and a warning is printed. With `--strict` it is an error.
- A `-pattern` that exists neither locally nor on the router produces a warning; `--strict` turns it into an error.

To delete a domain that still has a positive line, remove that positive line and add `-pattern`. While the positive line remains, domsync treats the pair as a move, not a deletion.

After a successful sync the `-` lines disappear from the local file: the domain has either been removed or is now written in its destination section. Duplicate positive patterns and duplicate `-` directives are errors.

### Comments

- `#` at the start of a line is a comment. Comments are local-only: they are kept in the local snapshot and are never written to the router.
- Comments attach to the current section and are rendered immediately after that section's header, in their original order. Comments before the first section stay at the top of the file.
- Inline comments are invalid. The line `+.example.com # note` is a validation error; put the note on its own line.
- `//` is not a local comment. A local line starting with `//` is a pattern and is rejected.
- Blank lines are ignored, a UTF-8 BOM is stripped, and CRLF is accepted.

### Output rules

Within each section, patterns are sorted; section order is otherwise preserved; the `other_section` is last. The local file is rewritten as a snapshot of the merged state, comments included. When the local file has sections, the router file contains `# [section]` headers followed by the same sorted patterns; a flat local file produces a flat router file. Router-only domains are added to `other_section`; if the local file already has that section it is reused, so a second one is never created.

An empty local file, or a plan whose final list would be empty, is an error: domsync refuses to wipe the router.

### Router file validation

When reading the provider file from the router, domsync ignores blank lines and lines starting with `#` or `//`. Every other line must be a valid pattern; otherwise the run fails with the file, line number and offending text, and nothing is written. Duplicate patterns are collapsed. A bare `[media]` header on the router is therefore an error, not a domain.

## Configuration reference

YAML parsing is strict: unknown fields, multiple YAML documents and an empty file are errors.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `version` | integer | required | Must be `1`. |
| `source.domains` | path | `domains.txt` | Local domain list, resolved relative to the config file. |
| `target.host` | string | required | Router host name or address. |
| `target.port` | integer | `22` | SSH port (1-65535). |
| `target.user` | string | required | SSH user. |
| `target.ssh_key` | path | empty | Private key (ed25519, ecdsa or rsa). If empty, `SSH_AUTH_SOCK` must provide an agent. Both are used when available. |
| `target.known_hosts` | path | `~/.ssh/known_hosts` | Must already exist and be a regular file. |
| `target.path` | string | required | Absolute path of the provider file on the router; a trailing `/` is rejected. |
| `target.other_section` | string | `other` | Section that receives router-only domains. |
| `target.mode` | string | `0644` | Octal permissions for the written provider file, at most `0777`. Quote it in YAML. |
| `target.backup` | boolean | `true` | Copy the previous provider file to the backup path before replacing it. |
| `target.backup_suffix` | string | `.domsync.bak` | Suffix appended to `target.path` for the backup file. |
| `target.post_write.run` | string | `on_change` | When to run `target.post_write.commands`: `on_change`, `always` or `never`. |
| `target.post_write.timeout` | duration string | `120s` | Timeout for each command. See the note below. |
| `target.post_write.commands` | list of strings | empty | Commands run sequentially over the existing SSH connection. |
| `update.check` | boolean | `true` | Check GitHub for a newer release at launch. `--no-update-check` overrides it. |
| `logging.file` | path | empty (disabled) | When set, one summary line is appended per run. |

`target.post_write.run` values:

- `on_change` - run the commands only when the router provider file changed.
- `always` - run them after every successful sync, including a run with no changes at all.
- `never` - never run them.

`--dry-run` never runs commands, and no commands run when an earlier step failed. A failing command stops the sequence and exits with code 5.

**The command timeout cannot be disabled.** `timeout` applies to each command separately. A value of `0s` is treated as "unset" and replaced by the built-in default of `120s`; negative values are rejected. Choose a large finite value if a command legitimately runs long.

**Host keys are always verified.** This is true for key-file authentication and for ssh-agent authentication alike. There is no trust-on-first-use mode.

Config validation runs before SSH is dialled: a missing key file, a missing `known_hosts`, or an empty `target.ssh_key` with `SSH_AUTH_SOCK` unset fails with exit code 1 before any connection is attempted.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success, including "no changes". Also `--help` and `--version`. |
| `1` | Usage or configuration error, local file read/write error, or log write error. |
| `2` | Validation error in the local list or the router provider file, including `--strict` warnings, and an empty final list. |
| `3` | SSH connection, authentication, host-key or remote read/write/verify error. |
| `4` | Update error, including the Homebrew/npm refusal. |
| `5` | A post-write command failed (non-zero exit or transport error). |

`--strict` only promotes local warnings (deletion target not found, redundant `+`/`-` in one section) to errors. Problems in the router file are always errors, and a failed launch-time update check never changes the sync exit code.

## Updates

On every ordinary run, if `update.check` is `true` and `--no-update-check` was not passed, domsync asks the GitHub releases API for the latest tag with a 2-second budget and an ETag cache under `os.UserCacheDir()/domsync/update.json`. When a newer version exists it prints one line to stderr after the report:

```text
domsync: доступна v1.4.0 (у вас v1.3.2). Обновление: domsync update
```

The notice is suppressed by `--quiet` and `--json`; in JSON the report carries an `update` object with `current`, `latest` and `available`. A network or API failure is silent, or a warning in `--verbose`; it never fails the sync.

Explicit update commands:

```sh
domsync update          # download, verify SHA-256 and replace the binary
domsync update --check  # check and report only; changes nothing
```

`domsync update --check` prints the current version and either the available version or `Обновлений нет`. `update.check: false` does not affect these explicit commands.

domsync does not replace binaries installed by Homebrew or npm. The install method is detected from the resolved executable path, and for those installs it prints this and exits with code 4 (the message itself is Russian):

```text
domsync установлен через Homebrew. Обновление: brew upgrade --cask domsync
domsync установлен через npm. Обновление: npm i -g domsync@latest
```

So update those installs with `brew upgrade --cask domsync` or `npm i -g domsync@latest`. The same refusal applies to `domsync update --check`, because install detection happens first. A standalone binary is replaced atomically after its archive has been checked against `checksums.txt`.

## Output

The default human report is Russian and looks like this for a run that changed both files:

```text
domsync: 14 доменов, 3 секции
  роутер: +1 −1, записан, бэкап /etc/openclash/rule_provider/domsync.txt.domsync.bak
  локально: +1 −1, перемещён 1
  команда: /etc/init.d/openclash restart — ok, 2.1s
```

- The header reports the total number of domains and non-empty sections.
- `роутер` and `локально` show added and removed counts with the U+2212 minus sign. The router line adds `записан` when the file was written and the backup path when a backup was made; the local line adds `перемещён N` for moved patterns.
- `без изменений` means that file was already identical.
- With `--dry-run`, each changed file is marked `будет записан (dry-run)`.
- Command results are `ok, <duration>` or `ошибка <exit code>`; a transport failure prints `ошибка: <message>`.
- Warnings print as `предупреждение: <message>`.

`--dry-run` still connects to the router and reads the current provider file; it skips both writes and all commands. It is not an offline mode.

- `--json` writes a single JSON object to stdout (errors are JSON as well), with English field names: `ok`, `changed`, `dry_run`, `total`, `sections`, `router`, `local`, `warnings`, `commands`, `update`, `error`.
- `--quiet` prints only errors; `--verbose` adds update-check failures to the warnings; `--strict` treats local warnings as errors.

## Security

- **Strict host key checking.** Every connection verifies the router key against `known_hosts` using `golang.org/x/crypto/ssh/knownhosts`, whether authentication uses a key file or the ssh-agent. Unknown or changed keys abort the run with the SHA-256 fingerprint and a `ssh-keyscan` hint. TOFU does not exist in v1.
- **Updates in v1 rely on SHA-256 only.** The binary checks the downloaded archive against `checksums.txt` from the release and extracts only the expected executable, rejecting path traversal. GoReleaser also publishes a cosign keyless signature bundle (`checksums.txt.sigstore.json`) and GitHub build attestations; these are for manual verification, not checked by the CLI. To verify a release by hand:

  ```sh
  curl -fsSL -O https://github.com/neiromaster/domsync/releases/latest/download/checksums.txt
  curl -fsSL -O https://github.com/neiromaster/domsync/releases/latest/download/checksums.txt.sigstore.json
  cosign verify-blob --bundle checksums.txt.sigstore.json \
    --certificate-identity-regexp '^https://github.com/neiromaster/domsync/' \
    --certificate-oidc-issuer-regexp '^https://token\.actions\.githubusercontent\.com$' \
    checksums.txt
  ```

- **`install.sh` verifies SHA-256 before installing** and refuses to continue if neither `sha256sum` nor `shasum` is available. It also verifies the cosign bundle when `cosign` is installed.
- **Remote writes are atomic and verified.** Contents go over stdin, not the command line. A temporary file is renamed over the target, the previous file is optionally copied to the backup path, symlinks are resolved so the link keeps pointing at the updated file, and the result is read back and compared byte for byte. A directory target is an error.
- SSH password authentication, `ProxyJump` and the system `ssh` binary are not used.

## Development

```sh
go test ./...
./scripts/check-coverage.sh
go tool golangci-lint run
go tool govulncheck ./...
```

Fuzzing:

```sh
go test ./internal/domainlist -run '^$' -fuzz FuzzParseLocal -fuzztime 30s
go test ./internal/domainlist -run '^$' -fuzz FuzzParseRemote -fuzztime 30s
```

`lefthook install` enables the pre-commit hook (gofmt, golangci-lint, tests).

## Release

Tagging `v*` runs `.github/workflows/release.yml`. Maintainers need:

- An `NPM_TOKEN` repository secret. `.github/scripts/publish-npm.sh` requires it to publish the `domsync` launcher and the six `@neiromaster/domsync-<os>-<cpu>` platform packages; the npm job also has `id-token: write` and publishes with provenance.
- A `neiromaster/homebrew-tap` repository and a `HOMEBREW_TAP_TOKEN` secret with write access to it, used by the cask publisher.
- A **public** repository. The Scorecard workflow sets `publish_results: true`, which requires public visibility.

GoReleaser builds the archives, `checksums.txt`, the cosign bundle and SBOMs, publishes the GitHub release and the Homebrew cask; the release workflow adds build provenance attestations and then publishes the npm packages.

## License

MIT
