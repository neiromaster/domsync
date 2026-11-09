# Security Policy

## Supported versions

Only the latest release receives security fixes. Upgrade with `brew upgrade --cask domsync`, `npm i -g @domsync/cli@latest`, or `domsync update` before reporting an issue that may already be fixed.

## Reporting a vulnerability

Do not open a public issue. Use GitHub's private vulnerability reporting:

<https://github.com/neiromaster/domsync/security/advisories/new>

Include the affected version (`domsync --version`), the platform, the config with secrets redacted, and the smallest reproduction you can construct. You will get an acknowledgement within seven days, and a fix or a written assessment as soon as the report is triaged.

## Scope

Reports about the following are especially useful:

- host key verification in `internal/sshclient` (known-host matching, algorithm negotiation, agent authentication);
- archive extraction and checksum verification in `internal/update` (path traversal, symlink handling, atomic replacement);
- `install.sh` (checksum bypass, unsafe install directory handling);
- parsing of the local list and the router provider file (`internal/domainlist`) leading to a write the user did not ask for.

Known and documented limitations are not vulnerabilities by themselves: updates are verified with SHA-256 only (cosign bundles and build attestations exist for manual verification), SSH password authentication, `ProxyJump` and the system `ssh` binary are unsupported, and the CLI has no trust-on-first-use mode. See the Security section of the README for the full list.

## Release integrity

Every release publishes `checksums.txt`, a cosign keyless signature bundle, GitHub build provenance attestations, and SBOMs. Verification commands are in the README.
