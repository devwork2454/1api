<h1 align="center">Charon</h1>

<p align="center">
  <em>Ferry your AI tools between endpoints.</em>
</p>

<p align="center">
  <a href="https://github.com/devwork2454/1api/releases/latest"><img src="https://img.shields.io/github/v/release/devwork2454/1api?style=flat-square&color=6c47ff" alt="Latest Release"></a>
  <a href="https://github.com/devwork2454/1api/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/devwork2454/1api/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/devwork2454/1api?style=flat-square" alt="MIT License"></a>
  <a href="https://github.com/devwork2454/1api/issues"><img src="https://img.shields.io/github/issues/devwork2454/1api?style=flat-square" alt="Open Issues"></a>
</p>

Charon is a tiny Go CLI that detects the **Codex**, **Claude Code**,
**OpenCode**, and **Pi** CLIs and switches each one's **endpoint + credentials**
between named profiles. Every profile is a full snapshot of that tool's auth
surface, so it works for both API-key logins and OAuth/ChatGPT sessions — and
switching away and back is always clean and reversible.

<p align="center">
  <img src="https://raw.githubusercontent.com/mingtheanlay/1api/main/assets/screenshot.png" alt="Charon interactive menu" width="80%">
</p>

## Features

- **One command, four tools.** Manage Codex, Claude Code, OpenCode, and Pi from
  a single interactive menu or a scriptable CLI.
- **Named profiles.** Snapshot each tool's full auth surface and hop between
  endpoints/keys instantly.
- **Model discovery.** Add a profile from just an endpoint + key; Charon fetches
  the model list and lets you pick one.
- **Safe by default.** Every switch is backed up first, writes are atomic, and an
  auto-captured `default` profile means you can always revert.
- **Non-destructive.** Charon only ever touches its own `1api` provider entry
  in each tool's config, never your hand-authored providers.

## Supported tools

| Tool | Endpoint | Credentials |
|------|----------|-------------|
| **Codex** | `~/.codex/config.toml` (`model_provider` → `base_url`) | `~/.codex/auth.json` |
| **Claude Code** | `~/.claude/settings.json` (`env.ANTHROPIC_BASE_URL`) | `settings.json` env key **or** macOS Keychain `Claude Code-credentials` |
| **OpenCode** | `~/.config/opencode/opencode.json` (`provider.*.options.baseURL`) | `~/.local/share/opencode/auth.json` |
| **Pi** | `~/.pi/agent/extensions/1api.ts` (`pi.registerProvider("1api", ...)`) | `~/.pi/agent/auth.json` |

## Supported platforms

| OS | Status | Notes |
|----|--------|-------|
| **macOS** (darwin) | ✅ Fully supported | Reads/writes Claude Code's OAuth token via the macOS Keychain (`security`). Primary tested platform. |
| **Linux** | ✅ Supported | File-based profiles for all tools work. Keychain access is a no-op — Claude OAuth credentials are read from `~/.claude` files instead. |
| **Windows** | ⚠️ Untested | Builds; paths resolve under `%USERPROFILE%`. Keychain is a no-op. Not yet verified. |

Keychain support is compiled in per-platform (`keychain_darwin.go` vs.
`keychain_other.go`), so non-macOS builds simply skip it.

## Installation

### curl (Linux & macOS)

No Go needed — downloads the prebuilt binary for your platform, verifies its
checksum, and installs to `~/.local/bin`:

```sh
curl -fsSL https://github.com/devwork2454/1api/releases/latest/download/install.sh | sh
```

> Prepend `PREFIX=/usr/local` to install system-wide, or `VERSION=v1.2.3` to pin a release.
> This line installs from **this fork's** releases (includes local fixes such as
> OpenCode active-model registration). Upstream official installs use
> `mingtheanlay/1api` instead.

<details>
<summary><b>Other methods</b> — manual binary · build from source · upstream</summary>

**Pre-built binary** — grab your platform's archive from the
[Releases page](https://github.com/devwork2454/1api/releases/latest)
(`1api_{darwin,linux}_{amd64,arm64}.tar.gz`) and verify it against the included
`checksums.txt`:

```sh
curl -L https://github.com/devwork2454/1api/releases/latest/download/1api_linux_amd64.tar.gz | tar xz
sudo mv 1api /usr/local/bin/
```

**From source** — requires Go 1.24+:

```sh
make install                      # build + install to ~/.local/bin (PREFIX to override)
go build -o 1api ./cmd/1api   # or just build here
```

</details>

## Usage

### Interactive menu

Run `1api` with no arguments to open an arrow-key menu: pick a tool, then
switch, add, edit, or delete profiles. Quit any time with `ctrl+c`.

### CLI reference

```sh
1api                       # interactive arrow-key menu
1api status                # show each tool's active profile, endpoint, and auth (--json)
1api ls <tool>             # list saved profiles (--json)
1api save <tool> [name]    # snapshot current live config (omit name to use the logged-in account)
1api models <tool>         # list models offered by an API (--key [--endpoint])
1api add <tool>            # add + activate a profile (--name --key [--endpoint --model])
1api edit <tool> <p>       # change a profile's endpoint/key/model (--name to rename)
1api rename <tool> <o> <n> # rename a saved profile
1api cp <tool> <src> <dst> # duplicate a saved profile
1api switch <tool> <p>     # apply a saved profile (backs up current first)
1api run <tool> <p> [--]   # start tool with profile in a temp HOME (codex/opencode)
1api alias <tool> <p>      # print a shell function wrapping 1api run
1api restore <tool>        # revert to the auto-captured default
1api undo <tool>           # revert to the most recent pre-switch backup
1api prune <tool>          # delete old backups, keeping the newest (--keep N, default 10)
1api rm <tool> <p>         # delete a profile
1api completion <shell>    # print a bash/zsh/fish completion script
```

`status` and `ls` accept `--json` for scripting and editor integrations. `status`
also flags **`(modified)`** next to a tool whose live config changed outside Charon
(e.g. a fresh `claude login`), so a stale active profile is easy to spot.

### Shell completions

Completions ship in the release archives and are installed automatically via
Homebrew. To enable them manually:

```sh
# bash — add to ~/.bashrc
source <(1api completion bash)
# zsh — add to ~/.zshrc (ensure `compinit` runs)
source <(1api completion zsh)
# fish
1api completion fish | source
```

They complete subcommands, tool names, and — for `switch`/`edit`/`rename`/`cp`/`rm`
— saved profile names.

## Adding & editing profiles

### From an endpoint + key (with model discovery)

In the menu, drill into a tool and choose **＋ Add new profile…**. The wizard:

1. asks for the **API base URL** (leave blank to accept the provider default;
   real values are never prefilled),
2. asks for the **API key** (hidden input),
3. **fetches the model list** from that endpoint (`GET /v1/models`, using
   `Authorization: Bearer` for OpenAI-style APIs and `x-api-key` for Anthropic),
4. lets you **pick a model** (or skip), then
5. names the profile — writing the endpoint/key/model into the tool's live config
   and switching to it.

### Backing up a logged-in account

Already signed in to Codex or Claude Code with a real account? Charon can snapshot
that session and **name the profile after the account** automatically:

```sh
codex login              # sign in as your work account
1api save codex        # → saves & activates profile "you@work.com"

codex login              # sign in as a second account
1api save codex        # → saves & activates profile "you@personal.com"

1api switch codex you@work.com   # hop back instantly
```

In the menu, drill into a tool and press **`b`** on a profile to back it up. What
happens depends on the profile:

- **A logged-in account** (the `default` login or any OAuth snapshot — no
  editable endpoint/key) is captured and **named after its account email**
  automatically. The email is read from the tool's own config — Codex's
  `id_token`, Claude Code's `~/.claude.json` — purely to name the profile; that
  file is only ever read, never modified. These login backups are **not editable**
  (there's no endpoint/key to change); re-running `b` refreshes the snapshot.
- **An API-proxy profile** (endpoint + key) is **duplicated**: Charon prompts for
  a name, pre-filled with the next free `name-2`, validates it isn't a duplicate,
  and the copy is a normal profile you can **edit and delete**.

An API-key login has no account, so `1api save` still expects an explicit name.

### Editing an existing profile

Press **`e`** on a profile to open its edit form, showing the current **Name**,
**URL**, **Token** (masked), and **Model**. Press **`e`** on any field to change
it — selecting **Model** re-fetches the endpoint's model list so you can pick a
new one. Press **`esc`** to save your changes and switch to the profile; renaming
is handled automatically. The auto-captured **`default`** profile and login
backups (which have no endpoint/key) are protected and cannot be edited.

### Non-interactively

```sh
1api models codex --endpoint https://openrouter.ai/api/v1 --key sk-...
1api add    codex --name openrouter --endpoint https://openrouter.ai/api/v1 \
                    --key sk-... --model openai/gpt-5.5
```

Each tool gets a dedicated `1api` provider entry written into its own config
format (Codex `[model_providers.1api]`, Claude `env.ANTHROPIC_*`, OpenCode an
`@ai-sdk/openai-compatible` provider, Pi a `pi.registerProvider("1api", ...)`
extension), so switching away and back is clean.

A typical flow: log into a tool normally, `1api save codex work-key`; log into a
different endpoint/key, `1api save codex proxy`; then hop between them with
`1api switch codex work-key` — or just run `1api` and pick from the menu.
`restore` always returns to the pristine config captured the first time Charon ran.

## How it works

- **Storage:** `~/.config/1api/` (`$XDG_CONFIG_HOME` respected).
  - `profiles/<tool>/<name>/` — snapshot files + `manifest.json`.
  - `backups/<tool>/<timestamp>/` — auto-backup taken before every switch, add,
    or undo. `1api undo` reverts to the newest; the last 10 per tool are kept
    (tune with `1api prune <tool> --keep N`).
  - `config.json` — active profile per tool.
- **`default`** is captured automatically the first time a detected tool is seen,
  so reverting is always possible and it is never overwritten.
- Writes are **atomic** (temp file → `rename`), and credential files/dirs are
  mode `0600`/`0700`.

## Security

Profiles are stored **unencrypted** on disk (mode `0600`), including any OAuth
token copied out of the macOS Keychain. Keep `~/.config/1api` private; a future
version may push secrets back into the Keychain instead.

## Project layout

```
cmd/1api/          entrypoint + subcommands
internal/artifact/   snapshot/restore primitives (Artifact interface + implementations)
internal/tools/      per-tool adapters (codex, claude, opencode, pi)
internal/profile/    snapshot store (split by concern: snapshot, apply, backup, manage)
internal/tui/        bubbletea interactive menu
internal/secret/     masking + macOS keychain access
```

## Development

```sh
make test    # go vet + go test -race ./...
make cover   # coverage summary
make lint    # golangci-lint run
make fmt     # gofmt -w .
```

CI (`.github/workflows/ci.yml`) runs formatting checks, vet, race tests, build,
and golangci-lint on Linux and macOS. Contributor and agent conventions —
including the rule to **always sandbox `HOME` when testing so real credentials
are never touched** — live in [AGENTS.md](AGENTS.md).

## Roadmap

- Optional `--verify` post-switch auth ping to confirm credentials actually work.
- Windows Keychain / Credential Manager support.
- Support for more AI CLI tools.

## Contributing

**PRs and issues are very welcome.** This is an early project with plenty of room
to grow — your ideas and bug reports genuinely shape where it goes next.

- 🐛 **Found a bug?** [Open an issue](https://github.com/mingtheanlay/1api/issues/new) with the tool name, OS, and expected vs. actual behavior.
- 💡 **Have an idea?** [Start a discussion](https://github.com/mingtheanlay/1api/issues/new) — new tool support, UX tweaks, anything is fair game.
- 🔧 **Sending a fix or feature?** Fork → branch → PR. Run `make fmt && make test` before pushing. See [AGENTS.md](AGENTS.md) for the conventions.

No contribution is too small — a typo fix is as appreciated as a new feature.

## License

Released under the [MIT License](LICENSE).
