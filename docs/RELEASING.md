# Releasing

For Vikey maintainers.

## How the pieces fit

1. The Vikey dashboard asks the Vikey panel for a one-time code and shows
   `curl -fsSL https://app.vikey.ai/i/<code> | sh` (or the `.ps1` command).
2. The panel answers that URL with [`bootstrap/install.sh`](../bootstrap/install.sh)
   or [`install.ps1`](../bootstrap/install.ps1), filled in with the code, the
   pinned release version and the SHA-256 of each binary.
3. The script downloads `vikey-<os>-<arch>` from this repository's release,
   checks the checksum and runs `vikey setup --code <code>`.
4. `vikey setup` trades the code for the API key and the agent settings
   (`POST /api/install/exchange`) and configures the agents.

The panel decides models and base URLs, this repository decides where each
agent keeps them. The panel serves only the release it has pinned, so a new
tag here changes nothing for users until the panel is updated.

## Steps

1. Make sure CI is green on `main` (unit tests and the one-line install on
   Ubuntu, macOS and Windows).
2. Tag and push:

   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

   The release workflow builds the six binaries and publishes them with
   `checksums.txt`, the bootstrap templates and `release.json`.
3. In the panel repository, pin the release and deploy:

   ```bash
   node scripts/sync-cli-release.js 0.1.0
   ```

   This checks the templates against `checksums.txt` and writes the pinned
   version, checksums and templates. Commit and deploy them. Until a release
   is pinned the dashboard hides every install button.

## Editing the bootstrap scripts

The scripts in `bootstrap/` run in people's shells straight from a pipe:

- Keep `install.sh` POSIX `sh` (dash on Debian and Ubuntu), no bash-isms.
- Keep `install.ps1` inside one `& { ... }` block and never call `exit`, it
  would close the person's PowerShell window.
- Placeholders `__CODE__`, `__VERSION__`, `__PANEL_URL__`, `__DOWNLOAD_BASE__`,
  `__ASSET_CASES__` and `__ASSET_HASH__` are filled in by the panel, which
  checks every value against a strict pattern first.

`tools/mockapi` serves the scripts the same way for local tests:

```bash
go build -o dist/vikey-darwin-arm64 ./cmd/vikey
go run ./tools/mockapi -addr 127.0.0.1:18080 -dl dist -bootstrap bootstrap
curl -fsSL http://127.0.0.1:18080/i/E2ETEST1 | VIKEY_SETUP_ARGS="--yes --no-install" HOME=/tmp/vikey-home sh
```

Use a throwaway `HOME` as above so your own agent settings stay untouched.
