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
- **Central providers.** Configure each external API (endpoint + key) **once**,
  pick high/mid/low from reachable models, then bind any tool with
  `1api use <tool> <provider>` — no re-entering credentials per CLI.
- **Drop-in for `charon`.** `make install` also links `charon` → `1api`. On first
  run, profiles under `~/.config/charon` are merged into `~/.config/1api` (no
  overwrite of existing names; legacy dir kept) and API-proxy specs are
  fingerprint-deduped into the central provider list.
- **Model discovery.** Add a profile from just an endpoint + key; Charon fetches
  the model list and lets you pick one.
- **Safe by default.** Every switch is backed up first, writes are atomic, and an
  auto-captured `default` profile means you can always revert.
- **Non-destructive.** Charon only ever touches its own `1api` provider entry
  in each tool's config, never your hand-authored providers.
- **OpenCode + oh-my.** When you add/edit/switch/run an OpenCode profile, Charon
  also realigns `~/.omo/omo.jsonc` agent/category `model` fields to the same
  mid/low/high routing (only `1api/*` and legacy `charon/*` refs). Missing omo is
  a silent no-op.
- **Only usable models.** Lists and OpenCode apply keep models that pass a 1-token
  chat probe (parallel). Unreachable ids are hidden; if none work → `暂无可用模型`.
  `add`/`edit` fail closed; `--no-verify` / `models --no-probe` skip probes.

## Supported tools

| Tool | Endpoint | Credentials | Extra |
|------|----------|-------------|-------|
| **Codex** | `~/.codex/config.toml` (`model_provider` → `base_url`) | `~/.codex/auth.json` | — |
| **Claude Code** | `~/.claude/settings.json` (`env.ANTHROPIC_BASE_URL`) | `settings.json` env key **or** macOS Keychain `Claude Code-credentials` | — |
| **OpenCode** | `~/.config/opencode/opencode.json(c)` (`provider.*.options.baseURL`) | `~/.local/share/opencode/auth.json` | Derive-patches `~/.omo/omo.jsonc` models (see below) |
| **Pi** | `~/.pi/agent/extensions/1api.ts` (`pi.registerProvider("1api", ...)`) | `~/.pi/agent/auth.json` | — |

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
> The installer tries **GitHub releases first**, then falls back to the **Gitee**
> mirror (`GITEE_REPO`, default same `owner/1api`) if GitHub is unreachable.
> See [docs/GITEE.md](docs/GITEE.md) for mirror setup. Upstream official installs
> use `mingtheanlay/1api` instead.


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

Run `1api` with no arguments. The menu opens **immediately** (no New/Classic
picker). Default language is **Chinese**; switch to English under Settings.

| Menu | What it does |
|------|----------------|
| **供应商 / Providers** | List providers, add one (endpoint + key), delete with **`d`** only when no tool uses it |
| **工具绑定 / Tool bindings** | Per-tool bound provider; pick a tool → provider → set **high / mid / low** models → apply |
| **设置 / Settings** | Language (`zh` / `en`) and skin (`teal` / `mono` / `warm`) |

Navigation: **`esc`** goes back one screen; on the root menu it **quits**.
**`ctrl+c`** always quits. Bind/apply shows a short busy status so the UI does
not look frozen. Preferences live in `config.json` as `uiLang` and `uiSkin`.

### CLI reference

```sh
1api                       # interactive arrow-key menu
1api status                # show each tool's active profile, endpoint, and auth (--json)
1api ls <tool>             # list saved profiles (--json)
1api save <tool> [name]    # snapshot current live config (omit name to use the logged-in account)
1api models <tool>         # list chat-usable models (--key [--endpoint] [--no-probe])
1api verify <tool>         # probe every model; show mid/low/high + usable set
1api provider ls           # list central providers (--json)
1api provider add          # add provider once (--name --key [--endpoint --wire --model --low --high] [--no-verify])
1api provider edit <n>     # edit provider tiers/endpoint/key [--no-verify]
1api provider rm <n>       # delete a provider
1api provider verify <n>   # re-probe usable models + refresh mid/low/high
1api provider models <n>   # show usable set and tier assignment
1api use <tool> <provider> # bind tool → provider and apply [--no-verify]
1api add    <tool>            # add provider + bind tool (--name --key [--endpoint --model] [--no-verify])
1api edit   <tool> <p>       # change endpoint/key/model (--name; OpenCode: [--no-verify])
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

### OpenCode + oh-my-openagent (`omo.jsonc`)

If you use [oh-my-openagent](https://github.com/code-yeongyu/oh-my-openagent),
sub-agent models live in `~/.omo/omo.jsonc` under `"[opencode]".agents` /
`categories`, not only in `opencode.jsonc`. Charon keeps those in step when
OpenCode profiles change:

| Command | What happens to omo |
|---------|---------------------|
| `1api add opencode` / `edit` | After writing OpenCode config, rewrite managed omo `model` fields |
| `1api switch` / `undo` / `restore` | After restoring the profile snapshot, re-derive omo from live OpenCode config |
| `1api run opencode <profile>` | Seed host omo into the temp HOME (if present), then patch models for the sandbox |

**Rules (by design):**

- **Source of truth** is the live OpenCode config (`model` / `small_model` /
  managed `provider.1api` or legacy `provider.charon` model map). omo is
  *derived*, not snapshotted into profiles.
- **Before write** (`add`/`edit` ApplyAuth): probe `GET …/models` and a 1-token
  chat on the resolved **mid** model. Fail closed (no config/omo write) if the
  key or mid model is unusable. Classify mid/low/high from the **live** id list
  (exact `mid`/`low`/`high`, then name hints like `flash`/`mini`→low,
  `opus`/`pro`/`r1`→high, then primary fallback). `--no-verify` skips the probe.
  `1api verify opencode` runs the same check without writing.
- Only `model` strings that are empty, `1api/…`, or legacy `charon/…` are
  rewritten. Foreign refs (e.g. `openai/gpt-4`) and non-model keys
  (`description`, `skills`, `prompt_append`, …) are left alone.
- Agents/categories are mapped to mid/low/high the same way OpenCode agent
  routing is (e.g. `explore`/`librarian`/`quick` → low; `sisyphus`/`oracle`/
  `deep` → high; default mid). A single-model endpoint fills all three tiers.
- If `~/.omo/omo.jsonc` does not exist, Charon does **nothing** (does not
  install or create oh-my).
- Rewrites use indented JSON (same as OpenCode config writes); JSONC comments
  in omo may be dropped.

```sh
1api switch opencode aliyun   # opencode.jsonc + omo models follow aliyun
1api switch opencode grok-api # both follow grok-api again
```

## How it works

- **Storage:** `~/.config/1api/` (`$XDG_CONFIG_HOME` respected).
  - `providers/<name>/` — central API configs (`provider.json`: endpoint, key,
    wire, mid/low/high, usable models). Configure once; bind tools with `use`.
  - `profiles/<tool>/<name>/` — snapshot files + `manifest.json`.
  - `backups/<tool>/<timestamp>/` — auto-backup taken before every switch, add,
    or undo. `1api undo` reverts to the newest; the last 10 per tool are kept
    (tune with `1api prune <tool> --keep N`).
  - `config.json` — active profile per tool, `toolProvider` bindings, migration flag.
- **Migration:** On first run after upgrade, API-proxy profile specs are imported
  into central providers (offline; marked stale until `provider verify` / `use`).
- **`default`** is captured automatically the first time a detected tool is seen,
  so reverting is always possible and it is never overwritten.
- Writes are **atomic** (temp file → `rename`), and credential files/dirs are
  mode `0600`/`0700`.
- **OpenCode omo:** not stored under `~/.config/1api/`; patched in place at
  `~/.omo/omo.jsonc` after apply/switch/run (see above).

## Security

Profiles are stored **unencrypted** on disk (mode `0600`), including any OAuth
token copied out of the macOS Keychain. Keep `~/.config/1api` private; a future
version may push secrets back into the Keychain instead.

## Project layout

```
cmd/1api/            entrypoint + subcommands
internal/artifact/   snapshot/restore primitives (Artifact interface + implementations)
internal/tools/      per-tool adapters (codex, claude, opencode + tiers/omo, pi)
internal/profile/    snapshot store (split by concern: snapshot, apply, backup, manage, session)
internal/tui/        bubbletea interactive menu
internal/secret/     masking + macOS keychain access
internal/models/     provider model-list fetch
internal/jsonc/      JSON-with-comments decode for OpenCode/omo configs
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

- Optional post-`switch` verify (OpenCode `add`/`edit` already probe before write).
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
