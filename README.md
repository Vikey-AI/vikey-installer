# Vikey CLI

Sets up coding agents to use [Vikey](https://vikey.ai): Claude Code, Codex,
opencode and Hermes, on macOS, Linux and Windows. Missing agents are
installed with their vendor's official installer first.

People don't run this repo directly. The Vikey dashboard shows a one-line
command for an API key:

```bash
curl -fsSL https://app.vikey.ai/i/<code> | sh        # macOS, Linux
irm https://app.vikey.ai/i/<code>.ps1 | iex          # Windows PowerShell
```

`<code>` is a one-time code (15 minutes, single use), never the API key.
The script ([bootstrap/](bootstrap)) downloads the `vikey` binary from this
repo's releases, checks its SHA-256 and runs `vikey setup --code <code>`,
which trades the code for the key once.

## Commands

| Command | What it does |
| --- | --- |
| `vikey setup` | Pick agents, install missing ones, point them at Vikey. With an active Token Plan it asks Token Plan or pay as you go |
| `vikey doctor` | Installed agents, whether they use Vikey, key check |
| `vikey uninstall` | Put every agent's settings back |
| `vikey claude` | Claude Code with Vikey through `--settings`, for people who keep a Claude subscription as default |

`vikey setup --yes --tools claude,codex --use payg` runs without questions.

## Where settings go

| Agent | File | How |
| --- | --- | --- |
| Claude Code | `~/.claude/settings.json` | `env`, `model`, `modelOverrides` keys, comments kept |
| Codex | `~/.codex/config.toml` | Marked blocks at the top and bottom, the key in `experimental_bearer_token` so Codex Desktop works without shell variables. Conflicting lines are commented out in place |
| opencode | `~/.config/opencode/opencode.json[c]` | `provider.vikey`, `model`, `small_model`, comments kept |
| Hermes | `~/.hermes` (Windows `%LOCALAPPDATA%\hermes`) | `hermes config set`, the key in Hermes's `.env` |

Every change records the old value in `~/.vikey/state.json`, so uninstall
restores the files exactly. Copies of changed files go to `~/.vikey/backups`.
The panel decides models and base URLs (vikey-api `config/cliInstall.js`),
this repo decides where each agent keeps them.

## Development

```bash
go test ./...
go run ./tools/mockapi -addr 127.0.0.1:18080 -dl dist -bootstrap bootstrap   # fake panel and inference API
```

CI runs the real one-line install on Ubuntu, macOS and Windows against the
mock, with every agent answering through the installed key, then doctor and
uninstall (`.github/workflows/ci.yml`, weekly too). Tag `vX.Y.Z` to release,
then run `node scripts/sync-cli-release.js X.Y.Z` in vikey-api.
