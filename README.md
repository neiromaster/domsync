# domsync

`domsync` keeps local sectioned domain lists in sync with OpenClash/Mihomo rule-provider files on your servers. One config lists any number of servers, and each server lists any number of **pairs**: one local domain list and one provider file. For every pair `domsync` reads the local list and the provider file over SSH, merges them (the local entries win, provider-only domains go into an `other` section), validates everything, then atomically rewrites each file that changed and optionally runs per-server post-write commands.

Pairs are independent: a dead server or a broken pair is reported and the rest still run. It never silently rewrites anything it cannot parse: an ambiguous local file or a malformed provider line stops that pair before a single byte is written.

**Not supported in v1:** homeproxy, subscription URLs or any remote source other than the current provider file, SSH password authentication, `ProxyJump`, a fallback to the system `ssh`, IDN domains (use punycode), and cryptographic verification of updates inside the CLI. One local list cannot be shared by two pairs (cross-sync), servers and pairs cannot be selected on the command line, and execution is sequential. There is no daemon, watcher or scheduler; run `domsync` from cron or by hand.

## Install

Homebrew (cask tap):

```sh
brew install --cask neiromaster/tap/domsync
```

The cask works on macOS and, with Homebrew 6.0 or newer, on Linux.

npm (macOS, Linux and Windows):

```sh
npm i -g @domsync/cli
```

Go (any platform with a Go toolchain, built from source):

```sh
go install github.com/neiromaster/domsync/cmd/domsync@latest
```

`go install` places the binary in `$GOBIN` (or `$HOME/go/bin`). The toolchain version is pinned in `go.mod`.

Install script (macOS and Linux):

```sh
curl -fsSL https://github.com/neiromaster/domsync/releases/latest/download/install.sh | sh
```

The script downloads the release archive and `checksums.txt`, verifies the SHA-256 checksum, and installs to `$DOMSYNC_INSTALL_DIR` (default `$HOME/.local/bin`). Set `DOMSYNC_VERSION` (for example `1.2.3` or `v1.2.3`) to pin a release instead of the latest one. It never uses `sudo`; if the destination is not writable it prints instructions instead. If `cosign` is installed it also verifies the Sigstore bundle, otherwise it prints the manual verification commands.

## Quick start

```sh
cp domsync.example.yaml domsync.yaml
mkdir -p domains
cat > domains/media.txt <<'EOF'
[media]
+.example.com
+.example.net
EOF
cat > domains/ai.txt <<'EOF'
[ai]
+.example.org
EOF
```

Edit `domsync.yaml`: in the `router` server set `host`, `user`, `ssh_key` and the `target` of each file. The example also lists a second server, `vps-1`, to show the shape; delete that entry or fix its settings before running. Set `enabled: false` instead to park it. Make sure each server's host key is already in `known_hosts`, then run:

```sh
domsync
```

The config is looked up in this order: `--config <path>`, the `DOMSYNC_CONFIG` environment variable, then `./domsync.yaml` in the current directory. Relative paths inside the config (`servers[].ssh_key`, `servers[].known_hosts`, `servers[].files[].source`, `logging.file`) are resolved against the config file's directory; `~` and `~/...` are expanded to your home directory.

A first run needs each server's host key in `known_hosts`. If it is missing, the error includes the SHA-256 fingerprint and the exact `ssh-keyscan -H ... >> ~/.ssh/known_hosts` command to add it.

## OpenClash provider

Add one provider per synced file to the OpenClash configuration:

```yaml
rule-providers:
  media:
    type: file
    behavior: domain
    format: text
    path: ./rule_provider/media.txt
```

`format: text` is **mandatory**. Without it Mihomo parses the file as YAML, the provider ends up empty, and OpenClash gives no error.

Each pair's `target` in `domsync.yaml` is the absolute path of the same file; OpenClash keeps provider files under `/etc/openclash/rule_provider/`.

`/etc/init.d/openclash reload` does **not** re-read provider files. Use `restart`, which is why the example config's post-write command is `/etc/init.d/openclash restart`. Any other command that genuinely reloads providers works too.

In the provider file, `# [section]` lines are comments for humans only: Mihomo ignores lines starting with `#` or `//`. domsync writes section headers only in that commented form, because a bare `[media]` line would be accepted by Mihomo as a literal domain and silently inflate the rule count. Comments are never preserved from the server: the file is regenerated from the local state.

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

A section header is `[name]` on its own line, optionally with surrounding whitespace. Names must be non-empty, at most 64 characters, free of control characters, and unique. Section order follows first appearance in the local file, but the pair's `other_section` section (default `other`) is always rendered last. Empty sections are dropped, together with their comments.

A file with no section headers is a flat list and stays flat; in that case no `other` section is ever created, even if provider-only domains are adopted.

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

- `-pattern` with no positive entry for the same pattern **deletes** it from the provider file and from the local snapshot.
- `-pattern` plus a positive entry for the same pattern in a **different** section **moves** it to that section. On the server only the section comment changes.
- `-pattern` and a positive entry in the **same** section is redundant: the positive entry wins and a warning is printed. With `--strict` it is an error.
- A `-pattern` that exists neither locally nor in the provider file produces a warning; `--strict` turns it into an error.

To delete a domain that still has a positive line, remove that positive line and add `-pattern`. While the positive line remains, domsync treats the pair as a move, not a deletion.

After a successful sync the `-` lines disappear from the local file: the domain has either been removed or is now written in its destination section. Duplicate positive patterns and duplicate `-` directives are errors.

### Comments

- `#` at the start of a line is a comment. Comments are local-only: they are kept in the local snapshot and are never written to the provider file.
- Comments attach to the current section and are rendered immediately after that section's header, in their original order. Comments before the first section stay at the top of the file.
- Inline comments are invalid. The line `+.example.com # note` is a validation error; put the note on its own line.
- `//` is not a local comment. A local line starting with `//` is a pattern and is rejected.
- Blank lines are ignored, a UTF-8 BOM is stripped, and CRLF is accepted.

### Output rules

Within each section, patterns are sorted; section order is otherwise preserved; the `other_section` is last. The local file is rewritten as a snapshot of the merged state, comments included. When the local file has sections, the provider file contains `# [section]` headers followed by the same sorted patterns; a flat local file produces a flat provider file. Provider-only domains are added to `other_section`; if the local file already has that section it is reused, so a second one is never created.

A missing local file is treated as an empty list, so the first run bootstraps the local snapshot from the provider file: every provider-only domain is adopted and written locally. An empty local file with an empty provider file is a no-op. A plan whose final list would be empty while the provider file is not is an error: domsync refuses to wipe the provider file.

Concurrent runs are refused. A run holds an exclusive lock on a hidden `.<name>.lock` file next to the config, where `<name>` is the config file name without its extension, from the moment the configuration is loaded until every write and post-write command has finished, so a cron job and a manual run cannot interleave their reads and writes. For `domsync.yaml` that file is `.domsync.lock`; two configs with different file names get different locks and can run at the same time. On Windows the file is also marked with the hidden attribute, since a leading dot alone does not hide it there. The lock file stays on disk after the run; the lock itself is released by the operating system when the process exits, including after a crash.

### Provider file validation

When reading the provider file from a server, domsync ignores blank lines and lines starting with `#` or `//`. Every other line must be a valid pattern; otherwise that pair fails with the file, line number and offending text, and nothing is written for it. Duplicate patterns are collapsed. A bare `[media]` header on the server is therefore an error, not a domain.

## Configuration reference

YAML parsing is strict: unknown fields, multiple YAML documents and an empty file are errors.

Top-level keys:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `version` | integer | required | Must be `2`; a `version: 1` config is rejected. |
| `servers` | list | required | One or more servers; at least one entry. |
| `update.check` | boolean | `true` | Check GitHub for a newer release at launch. `--no-update-check` overrides it. |
| `logging.file` | path | empty (disabled) | When set, one summary line is appended on every non-dry-run run that reaches the end, including runs with failed pairs or servers. |

Server keys:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `name` | string | required | Unique across servers, `[a-z0-9][a-z0-9_-]*`, at most 32 characters. Used in reports and logs. |
| `enabled` | boolean | `true` | `false` skips the server entirely, `validate` included. See below. |
| `host` | string | required | Server host name or address. |
| `port` | integer | `22` | SSH port (1-65535). |
| `user` | string | required | SSH user. |
| `ssh_key` | path | empty | Private key (ed25519, ecdsa or rsa). If empty, `SSH_AUTH_SOCK` must provide an agent. Both are used when available. |
| `known_hosts` | path | `~/.ssh/known_hosts` | Must already exist and be a regular file. |
| `post_write.run` | string | `on_change` | When to run `post_write.commands`: `on_change`, `always` or `never`. |
| `post_write.timeout` | duration string | `120s` | Timeout for each command. See the note below. |
| `post_write.commands` | list of strings | empty | Commands run sequentially over the existing SSH connection, after the server's pairs. |
| `files` | list | required | One or more pairs; at least one entry, even when the server is disabled. |

File (pair) keys:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `source` | path | required in version 2 | Local domain list, resolved relative to the config file. A regular file or absent; absent means an empty list. Must be unique across the whole config. |
| `target` | string | required | Absolute path of the provider file on that server; a trailing `/` is rejected. Must be unique within the server; the same target on two servers is fine. |
| `other_section` | string | `other` | Section that receives provider-only domains. |
| `mode` | string | `0644` | Octal permissions for the written provider file, between `0001` and `0777`. Quote it in YAML. |
| `backup` | boolean | `true` | Copy the previous provider file to the backup path before replacing it. |
| `backup_suffix` | string | `.domsync.bak` | Suffix appended to `target` for the backup file. |

Two pairs may not share a `source` (resolved absolute path): both would rewrite it, so the config is rejected naming both positions. Two pairs on the same server may not share a `target`; the same path on two different servers is fine. Relative paths resolve against the config file's directory and `~` expands to your home, as described in Quick start.

`post_write.run` values:

- `on_change` - run the commands when at least one pair on this server changed something, on the server or locally.
- `always` - run them after every non-dry-run sync of the server, including a run with no changes and a run in which a pair failed.
- `never` - never run them.

`--dry-run` never runs commands, and a server whose connection failed runs none. A command failure is recorded and does not undo any write; the sequence stops at the first failing command.

`enabled: false` parks a server: `sync` and `validate` skip it entirely, and the report marks it `skipped`. Every shape check still applies to a disabled server — required fields, name syntax and uniqueness, `port`, `mode`, `other_section`, `post_write.run`, absolute `target`, unique `source` across the whole config, and unique `target` within the server — so a typo in a parked server is a config error, not a surprise on the day it is switched on. Only filesystem and environment checks are skipped: `ssh_key` exists, `known_hosts` exists, `SSH_AUTH_SOCK` when `ssh_key` is empty, and `source` being a regular file or absent. That makes `enabled: false` a legitimate way to park a server whose key is not on this machine yet.

**The command timeout cannot be disabled.** `timeout` applies to each command separately. A value of `0s` is treated as "unset" and replaced by the built-in default of `120s`; negative values are rejected. Choose a large finite value if a command legitimately runs long.

**Host keys are always verified.** This is true for key-file authentication and for ssh-agent authentication alike. There is no trust-on-first-use mode.

Config validation runs before SSH is dialled: a missing key file, a missing `known_hosts`, an empty `ssh_key` with `SSH_AUTH_SOCK` unset, or a `source` that is not a regular file fails with exit code 1 before any connection is attempted. Disabled servers are exempt from these filesystem checks.

## Validation

`domsync validate` loads the configuration, walks every enabled server's pairs and parses each local list without opening an SSH connection. It covers everything `domsync` checks before dialling — the config schema, path resolution, cross-pair uniqueness, the existence of each server's `ssh_key` and `known_hosts`, and every local list's syntax — and nothing that needs a server. Disabled servers are skipped and reported as `skipped`.

Validation continues past a broken pair: it is reported and the remaining pairs are still checked.

```sh
domsync validate
domsync --json validate
```

Exit codes follow the table below: `1` for a configuration or read error and `2` for an invalid local list. When several pairs fail, the exit code is the code of the first failure in config order (server order, then file order).

With `--json` the output is one object with `ok` and `servers`; each server carries `name`, `ok`, `skipped` and `files`, and each file carries `source`, `ok`, `total`, `sections` and, on failure, an `error` object with the exit code and message. A config-level error produces `ok: false` with an `error` object and exits 1 before any pair is examined. Without `--json` the human output prints the resolved config path, then a block per server (`name`, then each local file with its counts) and the usual summary line (`domsync: N доменов, M секций`); a failed file prints `ошибка: ...`.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success, including "no changes". Also `--help` and `--version`. |
| `1` | Usage or configuration error, local file read/write error, log write error, or another run holding the config lock. |
| `2` | Validation error in a local list or a provider file, including `--strict` warnings, and an empty final list. |
| `3` | SSH connection, authentication, host-key or remote read/write/verify error. |
| `4` | Update error, including the Homebrew/npm refusal; uninstall cancelled or left to a package manager. |
| `5` | A post-write command failed (non-zero exit or transport error). |

When several things fail, the exit code is the code of the first failure in execution order: server order, then file order. A server that cannot be dialled counts at its own position, before its pairs, so a connection error beats a later pair failure. A failure in one pair or one server never stops the rest.

`--strict` only promotes local warnings (deletion target not found, redundant `+`/`-` in one section) to errors, and fails only that pair. Problems in a provider file are always errors, and a failed launch-time update check never changes the sync exit code.

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
domsync установлен через npm. Обновление: npm i -g @domsync/cli@latest
```

So update those installs with `brew upgrade --cask domsync` or `npm i -g @domsync/cli@latest`. The same refusal applies to `domsync update --check`, because install detection happens first. A standalone binary is replaced atomically after its archive has been checked against `checksums.txt`.

## Uninstall

`domsync uninstall` removes what the installation created: the binary and the cache directory (`os.UserCacheDir()/domsync`, which holds the update cache). It never touches your config, domain lists or log file.

```sh
domsync uninstall             # asks: Удалить domsync и созданные им файлы? [y/N]
domsync uninstall --yes       # no question, for scripts
domsync --dry-run uninstall   # print what would be removed and change nothing
```

Enter, or anything other than `y`, `yes`, `д` or `да`, cancels and exits with code 4. Without a terminal the question cannot be asked, so `--yes` is required.

The install method is detected the same way as for updates. Homebrew and npm installs are removed through the package manager: domsync runs `brew uninstall --cask --zap domsync` or `npm uninstall -g @domsync/cli` itself and then removes the cache directory. The cask declares a `zap` stanza for that directory, so `brew uninstall --cask --zap domsync` is just as complete without this command. On Windows a running npm package cannot delete itself, so the command prints the npm command and exits 4; the same happens when `brew` or `npm` is not on `PATH`, except the cache directory is still removed.

The empty hidden `.<name>.lock` beside a config (`.domsync.lock` for `domsync.yaml`) is a run lock, not an install artifact; it goes away with the config.

With `--json` the output is the usual envelope, with `removed` listing the paths and `dry_run` set for `--dry-run`.

## Output

The default human report is Russian and looks like this for a run that changed one pair:

```text
router (192.168.1.1)
  domains/media.txt → /etc/openclash/rule_provider/media.txt
    роутер: +2 −1, записан, бэкап /etc/openclash/rule_provider/media.txt.domsync.bak
    локально: +1, перемещён 1
  domains/ai.txt → /etc/openclash/rule_provider/ai.txt: без изменений
  команда: /etc/init.d/openclash restart — ok, 0.8s
vps-1: пропущен (disabled)
domsync: 42 домена, 7 секций
```

- Each server block starts with its name and host, followed by one line per pair. A skipped server is a single `name: пропущен (disabled)` line; a server that failed to connect prints its name and the error, with the number of pairs that were not attempted.
- A pair with no changes on either side stays on one line with `без изменений`; a failed pair prints `ошибка: <message>` on its own line.
- `роутер` and `локально` show added and removed counts with the U+2212 minus sign. The router line adds `записан` only when the file was actually written, and the backup path when a backup was made; the local line adds `перемещён N` for moved patterns.
- The final line reports the total number of domains and non-empty sections across every pair that ran. A successful run without changes says `domsync: изменений нет (N доменов, M секций)`.
- With `--dry-run`, each changed file is marked `будет записан (dry-run)`.
- Command results are `ok, <duration>` or `ошибка <exit code>`; a transport failure prints `ошибка: <message>`.
- Warnings print as `предупреждение: <message>`.

`--dry-run` still connects to every enabled server and reads the current provider files; it skips both writes and all commands. It is not an offline mode.

- `--json` writes a single JSON object to stdout; errors after argument parsing are JSON as well. Top-level fields are `ok`, `changed`, `dry_run`, `total`, `sections`, `servers`, `warnings`, `update` and `error`; each server carries `name`, `host`, `ok`, `changed`, `skipped`, `skipped_pairs`, `files`, `commands` and `error`; each file carries `source`, `target`, `ok`, `changed`, `total`, `sections`, `router`, `local`, `warnings` and `error`.
- `--quiet` prints only errors; `--verbose` adds update-check failures to the warnings; `--strict` treats local warnings as errors.

## Security

- **Strict host key checking.** Every connection verifies the server key against `known_hosts` using `golang.org/x/crypto/ssh/knownhosts`, whether authentication uses a key file or the ssh-agent. An unknown key aborts with its SHA-256 fingerprint and a `ssh-keyscan` hint; a changed key aborts naming both the presented and the expected fingerprints. TOFU does not exist in v1. The host key type is negotiated from the algorithms recorded for the host in `known_hosts`, so a host with only one recorded key type is not rejected merely because the server also offers another.
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

- **No npm token.** The `npm` job requests `id-token: write` and `.github/scripts/publish-npm.sh` publishes the `@domsync/cli` launcher and the six `@domsync/<os>-<cpu>` platform packages through [trusted publishing](https://docs.npmjs.com/trusted-publishers/): npm exchanges the GitHub OIDC token for a short-lived credential and attaches provenance. Each of the seven package names needs a trusted publisher configured on npmjs.com (`neiromaster` / `domsync` / `release.yml`, no environment), or on the command line with `npm trust github <package> --file release.yml --repo neiromaster/domsync --allow-publish`.
  npm only accepts a trusted publisher for a package that already exists, so the first release is published once by a maintainer: run `npm login` (2FA), then, after the GitHub release exists, `GITHUB_TOKEN=$(gh auth token) GITHUB_REF_NAME=<tag> GITHUB_REPOSITORY=neiromaster/domsync ./.github/scripts/publish-npm.sh`. After that, tag pushes publish without credentials. The script generates provenance only inside GitHub Actions, so the bootstrap version is published without it.
- A `neiromaster/homebrew-tap` repository and a `HOMEBREW_TAP_TOKEN` secret with write access to it, used by the cask publisher.
- A **public** repository. The Scorecard workflow sets `publish_results: true`, which requires public visibility.

GoReleaser builds the archives, `checksums.txt`, the cosign bundle and SBOMs, publishes the GitHub release and the Homebrew cask; the release workflow adds build provenance attestations and then publishes the npm packages.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup, commit conventions and the pull request workflow. Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## License

MIT
