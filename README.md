<p align="center">
  <img src=".github/assets/vikey-logo.png" alt="Vikey" width="88">
</p>

<h1 align="center">Vikey Installer</h1>

<p align="center">
  One command to set up <b>Claude Code</b>, <b>Codex</b>, <b>opencode</b> and <b>Hermes</b> with your Vikey API key.<br>
  macOS, Linux and Windows. Missing agents are installed for you.
</p>

<p align="center">
  <a href="https://github.com/Vikey-AI/vikey-installer/actions/workflows/ci.yml"><img src="https://github.com/Vikey-AI/vikey-installer/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Vikey-AI/vikey-installer/releases/latest"><img src="https://img.shields.io/github/v/release/Vikey-AI/vikey-installer?label=release&color=e3262f" alt="Latest release"></a>
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-555" alt="macOS, Linux, Windows">
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> &nbsp;|&nbsp;
  <a href="#supported-agents">Agents</a> &nbsp;|&nbsp;
  <a href="#commands">Commands</a> &nbsp;|&nbsp;
  <a href="#is-it-safe">Is it safe?</a> &nbsp;|&nbsp;
  <a href="#troubleshooting">Troubleshooting</a>
</p>

---

## Quick start

**1. Get your install command.** Open [API Keys](https://vikey.ai/dashboard/api-keys) in the Vikey dashboard and press **Pasang** (Install) next to a key. A new key shows the command right away.

**2. Paste it into a terminal.**

<table>
<tr><th>macOS and Linux</th><th>Windows (PowerShell)</th></tr>
<tr>
<td>

```bash
curl -fsSL https://app.vikey.ai/i/YOURCODE | sh
```

</td>
<td>

```powershell
irm https://app.vikey.ai/i/YOURCODE.ps1 | iex
```

</td>
</tr>
</table>

**3. Tick the agents you want and press Enter.** Done. Open a new terminal and start your agent as usual:

```bash
claude      # or: codex, opencode, hermes
```

> [!NOTE]
> `YOURCODE` is a **one-time code**, not your API key. It works once and expires after 15 minutes, so a command left in your shell history is useless to anyone else. Need it on another computer? Press **Pasang** again for a fresh command.

## What you will see

The installer runs in your terminal. Use the arrow keys and Space to choose, Enter to continue. Its screens are in Indonesian, like the Vikey dashboard.

```text
Vikey Setup
Atur coding agent di perangkat ini supaya memakai Vikey.

› Memeriksa coding agent di perangkat ini
┃ Pilih coding agent yang mau memakai Vikey
┃ Spasi untuk pilih atau batal pilih, Enter untuk lanjut.
┃ > [✓] Claude Code   terpasang, versi 2.1.326
┃   [✓] Codex         terpasang, versi 0.169.0, juga Codex Desktop
┃   [ ] opencode      belum terpasang, akan dipasang
┃   [✓] Hermes        terpasang, versi 0.23.2

✓ Terhubung dengan API key Laptop kantor (vk-1a2b3c4d...9f3e)
✓ Claude Code memakai Vikey
✓ Codex memakai Vikey
✓ Hermes memakai Vikey
✓ Koneksi ke Vikey berhasil
```

| On screen | Meaning |
| --- | --- |
| `terpasang, versi 2.1.326` | Installed, version 2.1.326 |
| `belum terpasang, akan dipasang` | Not installed yet, it will be installed |
| `sudah memakai Vikey` | Already set up with Vikey |
| `juga Codex Desktop` | The desktop app shares these settings too |

## Supported agents

| Agent | If it is missing | Where Vikey is set up |
| --- | --- | --- |
| **Claude Code** | Installed with Anthropic's official installer | `~/.claude/settings.json`, also used by the VS Code and JetBrains extensions and Claude Desktop |
| **Codex** (CLI and Desktop) | Installed with OpenAI's official installer | `~/.codex/config.toml` |
| **opencode** | Installed with opencode's official installer (Windows: npm or Scoop) | `~/.config/opencode/opencode.json` |
| **Hermes** (CLI and Desktop) | Installed with Nous Research's official installer | Hermes settings, via `hermes config set` |

Only the Vikey part of each file changes. Your other settings, comments and providers stay as they are.

### Systems

| System | Architectures |
| --- | --- |
| macOS 12 or newer | Apple Silicon, Intel |
| Linux | x86_64, ARM64 |
| Windows 10 and 11 (PowerShell 5.1 or 7) | x64, ARM64 |

## Token Plan or pay as you go

If your Vikey account has an active **Token Plan**, the installer asks which one your agents should use:

- **Token Plan** uses your plan's quota with the `vikey/plan` model.
- **Pay as you go** charges your balance per use and gives access to every model.

Without a Token Plan, pay as you go is used and nothing is asked.

## Already signed in to Claude?

If Claude Code on your computer is signed in with a Claude subscription, the installer asks first:

- **Make Vikey the default.** `claude` uses Vikey until you run `vikey uninstall`.
- **Use Vikey only through `vikey claude`.** `claude` keeps your subscription, `vikey claude` starts Claude Code with Vikey. Your Claude Code settings are not touched.

## Commands

The installer leaves a small `vikey` command on your computer (in `~/.vikey/bin`).

| Command | What it does |
| --- | --- |
| `vikey doctor` | Shows each agent, whether it uses Vikey, and checks your connection |
| `vikey setup` | Set up again, for example after installing another agent. Reuses your saved key, no new code needed |
| `vikey uninstall` | Puts every agent back exactly as it was before Vikey |
| `vikey claude` | Starts Claude Code with Vikey without changing its settings |
| `vikey version` | Prints the version |

<details>
<summary><b>Flags for scripts and automation</b></summary>

<br>

```bash
vikey setup --yes --tools claude,codex --use payg
```

| Flag | Meaning |
| --- | --- |
| `--yes` | Do not ask anything, use the defaults |
| `--tools claude,codex,opencode,hermes` | Agents to set up, or `all` |
| `--use plan` / `--use payg` | Token Plan or pay as you go |
| `--claude-mode default` / `separate` | Make Vikey the default for Claude Code, or use `vikey claude` only |
| `--no-install` | Only set up agents that are already installed |
| `--code CODE` | The one-time code from the dashboard |

`vikey uninstall --yes --tools codex` undoes one agent without asking.

</details>

## What it changes on your computer

- **Agent settings.** Only the Vikey entries listed [above](#supported-agents). A copy of every file is saved in `~/.vikey/backups` before the first change.
- **Your API key** is saved in `~/.vikey/credentials.json` and in each agent's settings, readable only by your user account. Desktop apps need the key in their settings because they cannot see variables exported in a shell.
- **PATH.** `~/.vikey/bin` (and any agent folder that was missing) is added for new terminals: a clearly marked block in `~/.zshrc` or `~/.bashrc`, or your user Path on Windows.

`vikey uninstall` reverses all of it and restores each file to what it was.

## Is it safe?

- **Your API key never appears in a URL.** The command carries a one-time code. The installer trades it for your key once, over HTTPS, and the code stops working.
- **Every download is checked.** The `vikey` program comes from this repository's [releases](https://github.com/Vikey-AI/vikey-installer/releases), and its SHA-256 checksum is compared with the one Vikey pins before anything runs.
- **Agents come from their makers.** Missing agents are installed with their vendors' own installers: `claude.ai/install.sh`, `chatgpt.com/codex/install.sh`, `opencode.ai/install` and `hermes-agent.nousresearch.com/install.sh` (or the `.ps1` versions on Windows). The exact command is printed before it runs.
- **Nothing is hidden.** Read the bootstrap scripts in [`bootstrap/`](bootstrap) and the rest of the source here. Releases are built from this source by GitHub Actions.

Found a security problem? Please report it privately with **Report a vulnerability** on the [Security tab](https://github.com/Vikey-AI/vikey-installer/security), not in a public issue.

## Troubleshooting

| You see | Why | Fix |
| --- | --- | --- |
| `Kode pemasangan tidak berlaku` | The code was used or is older than 15 minutes | Press **Pasang** in the dashboard for a new command |
| `command not found: claude` right after setup | This terminal has not read the new PATH | Open a new terminal |
| Codex Desktop still uses the old settings | The app was open during setup | Quit Codex Desktop completely, then open it again |
| opencode cannot be installed on Windows | It needs Node.js or Scoop there | Run `npm install -g opencode-ai` (or use WSL), then `vikey setup` |
| An agent answers with `401` | The key was turned off or deleted | Check the key in the dashboard, then run the install again |
| Anything else | | Run `vikey doctor` and share its output with Vikey support |

Prefer to set things up by hand? Follow the guides at [docs.vikey.ai](https://docs.vikey.ai/en/agents/claude-code/).

## Build from source

You need [Go](https://go.dev/dl/) 1.27 or newer.

```bash
git clone https://github.com/Vikey-AI/vikey-installer.git
cd vikey-installer
go build -o vikey ./cmd/vikey
./vikey setup        # asks for the code from the dashboard
```

Or install it straight away:

```bash
go install github.com/Vikey-AI/vikey-installer/cmd/vikey@latest
```

Run the tests with `go test ./...`. Every push also runs the real one-line install on Ubuntu, macOS and Windows against a mock server, with each agent answering through the installed key (see [`.github/workflows/ci.yml`](.github/workflows/ci.yml)). Maintainers: see [docs/RELEASING.md](docs/RELEASING.md).
