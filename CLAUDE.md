# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`1api` (binary; historically charon) is a Go CLI that detects the Codex, Claude
Code, OpenCode, and Pi CLIs and switches each one's **endpoint + credentials**
between named profiles. It reads and writes real user credential files
(`~/.codex`, `~/.claude`, `~/.config/opencode`, `~/.pi/agent`, and for OpenCode
also `model` fields in `~/.omo/omo.jsonc`) and the macOS Keychain.

## Read AGENTS.md first

`AGENTS.md` is the canonical agent guide for this repo. Its rules are
**non-negotiable** and override any general instinct — read it before changing
anything. The most important, restated because a mistake here corrupts real
credentials:

### Golden rule: sandbox `$HOME` when running charon

Never run `charon`, its subcommands, or the interactive menu against your real
`$HOME` while developing — it will back up and rewrite your real Codex/Claude/OpenCode/Pi
auth. Always sandbox:

```sh
HOME=$(mktemp -d) go run ./cmd/1api status
```

Tests must do the same: `t.Setenv("HOME", t.TempDir())` and
`t.Setenv("XDG_CONFIG_HOME", t.TempDir())`. See `internal/tools/tools_test.go` and
`internal/profile/store_test.go` for the pattern. Never add a test that touches the
real Keychain (`internal/secret/keychain_darwin.go` is intentionally uncovered).

`~/.claude.json` is **read-only** (used only to read `oauthAccount.emailAddress` to
name an account-backup profile). Never write or snapshot it.

### Safety guarantees to preserve

Every change must keep these intact — do not regress them:
- **Atomic writes** (temp file + `rename`, via `artifact.AtomicWrite`).
- **`0600`** on credential files, **`0700`** on dirs.
- **Auto-backup before every switch / add / undo** (`internal/profile/backup.go`).
- **Merge, never rewrite** config files (use `internal/tools/edit.go` helpers) so
  unrelated user settings survive.
- **Non-destructive provider entry**: only the managed `1api` provider (legacy
  name `charon` still recognized) is touched, never hand-authored providers
  (see `internal/tools/providers.go`).
- Secrets are never logged — route them through `secret.Mask`.
- **OpenCode omo sync**: after OpenCode ApplyAuth / switch restore / session
  materialize, derive-patch `~/.omo/omo.jsonc` model fields only
  (`internal/tools/opencode_omo.go`). Missing file = no-op; never create omo;
  never snapshot full omo into profiles. Full invariants: [AGENTS.md](AGENTS.md).

## Commands

```sh
make build      # build ./1api for the current platform
make test       # go vet + go test -race ./...
make cover      # coverage summary
make lint       # golangci-lint run (v2 config in .golangci.yml)
make fmt        # gofmt -w .
make run        # build + open the interactive menu (sandbox HOME first!)
```

Run a single test / package:

```sh
go test -race -run TestXxx ./internal/profile/
go test -race ./internal/tools/...        # one package
```

Always run `make fmt && make test` before finishing a change. CI
(`.github/workflows/ci.yml`) runs fmt-check, vet, `-race` tests, build, and
golangci-lint on Linux + macOS; keep all green. Requires Go 1.24+.

## Architecture

Layering (imports point left, never the reverse):

```
secret  ←  artifact  ←  tools  ←  profile  ←  cmd / tui
```

- **`cmd/1api/`** — thin entrypoint. `main.go` dispatches subcommands;
  `commands.go` has one `cmd*` func per subcommand. Keep it thin — business logic
  belongs in `internal/` so it stays testable. On startup it calls
  `store.EnsureDefault(t)` for every detected tool (captures the pristine config as
  the reserved `default` profile on first sight — the always-available revert point).
- **`internal/artifact/`** — snapshot/restore primitives, no tool knowledge.
  `Artifact` interface (`Read/Write/Remove/ID`) with `FileArtifact`,
  `MergedFileArtifact` (JSON or TOML; swaps only `ownedKeys`, preserves the rest via
  `Merge`), and `KeychainArtifact` (macOS). Optional interfaces `Rotator` (live
  contents can change outside a switch — e.g. OAuth tokens — store refreshes its
  snapshot before leaving the profile), `Merger`, and `Peeker` (surface model/effort
  from a stored snapshot without applying it). `AtomicWrite` lives here.
- **`internal/tools/`** — per-tool adapters. `tool.go` defines the `Tool` struct
  (`Name`, `Title`, `Provider` = `"openai"`/`"anthropic"` wire format,
  `DefaultEndpoint`, `Artifacts`, `Detected`, `Describe`, `ApplyAuth`) and the
  `All()`/`Find()` registry. One file per tool (`codex.go`, `claude.go`,
  `opencode.go`, `pi.go`). `edit.go` has the JSON/TOML load-merge-write helpers
  (preserve unknown keys); `providers.go` guards the shared `charon` provider entry.
- **`internal/profile/`** — the snapshot store, split by concern:
  `store.go` (on-disk layout, `config.json`, central `validateName`/`sanitizeProfileName`
  — **never join a user-supplied name into a path without `validateName`**),
  `snapshot.go` (Save/Add/Edit/EnsureDefault), `apply.go` (Apply/Undo/refresh/Drift),
  `backup.go` (timestamped backups + prune), `manage.go` (rm/mv/cp).
- **`internal/models/`** — fetch model lists from a provider API (`GET /v1/models`,
  `Authorization: Bearer` for OpenAI-style, `x-api-key` for Anthropic). Use
  `httptest` in tests; never make real API calls.
- **`internal/secret/`** — masking (`Mask`) + platform keychain behind build tags
  (`keychain_darwin.go` / `keychain_other.go`). Never use runtime `runtime.GOOS`
  branching for the keychain — use build tags.
- **`internal/tui/`** — bubbletea interactive menu. Verified by compile + `go vet`;
  extract pure logic into testable helpers rather than testing the event loop.

### Data layout

`~/.config/charon/` (`$XDG_CONFIG_HOME` respected), all `0700`/`0600`:
- `profiles/<tool>/<name>/` — snapshot files + `manifest.json`.
- `backups/<tool>/<timestamp>/` — auto-backup before every switch/add/undo; keeps
  newest 10 per tool (`charon prune <tool> --keep N`).
- `config.json` — active profile per tool.

`default` is a reserved profile name (captured automatically, never overwritten, not
editable, not renamable).

### Adding a new tool

1. Add `internal/tools/<tool>.go` returning a `*Tool` (build `Artifacts` from
   `internal/artifact` constructors; implement `Detected`, `Describe`, `ApplyAuth`).
   `ApplyAuth` must **merge** into existing config, never rewrite wholesale.
2. Register it in `All()` in `tool.go`.
3. Add a `TestXxxDescribeAndApply` in `tools_test.go` using a sandboxed `$HOME`.
   Everything else (store, CLI, TUI) is generic and needs no changes.

## Conventions

- Standard Go style: `gofmt`/`goimports`, tabs, error wrapping with `%w`,
  table-driven tests, small focused packages, exported identifiers documented.
- Prefer the standard library. Third-party deps are intentionally minimal:
  `bubbletea`/`bubbles`/`lipgloss` (TUI), `pelletier/go-toml/v2` (Codex TOML),
  `sahilm/fuzzy` (TUI fuzzy finder). Discuss before adding more.
- Don't commit built binaries, `dist/`, or coverage files (see `.gitignore`).

## 闭环工作协议
- 每个任务按 autonomy-harness:closed-loop 技能执行。
- 会话开始：先读 PROGRESS.md 和 git log 恢复状态。
- 完成判定以 .harness/verify.sh 为准，验证不过不得宣告完成。
- 每个可验证子目标完成即 git commit，并同步更新 PROGRESS.md。
