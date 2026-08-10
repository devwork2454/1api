# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository.
`1api` (module/binary; historically “charon”) is a small Go CLI that detects the
Codex, Claude Code, OpenCode, and Pi CLIs and switches each one's
**endpoint + credentials** between named profiles.

## Golden rule: this tool edits real user credentials

`1api` reads and writes live config for other tools (`~/.codex`, `~/.claude`,
`~/.config/opencode`, `~/.local/share/opencode`, `~/.pi/agent`, and for OpenCode
also `~/.omo/omo.jsonc` model fields) and the macOS Keychain. It also
**reads** `~/.claude.json` (`oauthAccount.emailAddress`) solely to name an
account-backup profile — that file is never written or snapshotted.

- **Never** run `1api add`, `1api switch`, `1api save`, or the interactive menu
  against your real `$HOME` while developing. Always sandbox:
  ```sh
  HOME=$(mktemp -d) go run ./cmd/1api status
  ```
- Tests must never touch real config. Use `t.Setenv("HOME", t.TempDir())` and
  `t.Setenv("XDG_CONFIG_HOME", t.TempDir())`. See `internal/tools/tools_test.go`
  and `internal/profile/store_test.go` for the pattern.
- Do not add tests that read or write the real Keychain. The keychain shell-out
  (`internal/secret/keychain_darwin.go`) is intentionally left uncovered.
- Preserve the safety guarantees: **atomic writes** (temp file + rename),
  `0600` on credential files / `0700` on dirs, and an **auto-backup before every
  switch**. Don't regress these.
- OpenCode `add`/`edit`/`switch`/`run` also **derive-patches** `~/.omo/omo.jsonc`
  agent/category `model` fields (`1api/*` or `charon/*` only) from the live
  opencode mid/low/high routing. Missing omo is a silent no-op — never create it
  in tests against a real HOME; sandbox `$HOME` and optional fixture under
  `$HOME/.omo/omo.jsonc` when covering this path.

### OpenCode omo sync (do not regress)

Implementation: `internal/tools/opencode_omo.go` (`SyncOpenCodeOmo` /
`SyncOpenCodeOmoAt` / `SeedAndSyncOpenCodeOmoAt`), tier map in
`opencode_tiers.go`, hooks in `opencode.go` `ApplyAuth`, `profile/apply.go`
`switchTo`, `profile/session.go` `MaterializeSession`.

| Invariant | Detail |
|-----------|--------|
| Path | `$HOME/.omo/omo.jsonc` only (via `home()`); not an Artifact under `~/.config/1api` |
| Trigger | After successful OpenCode `ApplyAuth`; after OpenCode `switchTo` restore; after `MaterializeSession` for opencode |
| Missing file | Return nil — never create or install oh-my |
| Rewrite set | Only empty / `1api/*` / legacy `charon/*` `model` strings under `"[opencode]".agents` and `.categories` |
| Preserve | All non-model keys; foreign provider models (e.g. `openai/…`) |
| Tiers | Reuse `resolveOpenCodeTiers` + `agentTierClass` / `categoryTierClass` |
| Pre-write verify | OpenCode `ApplyAuth` (unless `AuthSpec.SkipVerify` / `--no-verify`): `models.FilterReachable` (list + parallel chat per id) via `VerifyOpenCodeAuth`; only usable ids registered; all fail → `暂无可用模型`; fail closed |
| Session | Copy host omo into sandbox HOME when absent, then patch from sandbox opencode config |
| Tests | `opencode_omo_test.go`, `TestOpenCodeApplyAuthSyncsOmo`, `TestOpenCodeApplySyncsOmoModels`, session materialize assertions |

Do **not** snapshot full omo into profiles, rewrite skills/prompts, or touch
Codex/Claude/Pi for omo.

## Commands

```sh
make build      # build ./1api
make test       # go vet + go test -race ./...
make cover      # coverage summary
make lint       # golangci-lint run
make fmt        # gofmt -w .
make run        # build + open the interactive menu (sandbox your HOME first)
```

Always run `make fmt` and `make test` before finishing a change. CI
(`.github/workflows/ci.yml`) runs fmt-check, vet, `-race` tests, build, and
golangci-lint on Linux + macOS; keep all of them green.

## Architecture

```
cmd/1api/           CLI entrypoint (thin; no business logic)
  main.go           main, subcommand dispatch, usage
  commands.go       one cmd* func per subcommand + requireTool
  provider_cmd.go   provider / use subcommands
internal/artifact/  snapshot/restore primitives, no tool knowledge
  Artifact/Rotator/Merger/Peeker interfaces; FileArtifact,
  MergedFileArtifact, KeychainArtifact; AtomicWrite
internal/tools/   per-tool adapters
  tool.go           Tool struct, AuthSpec, registry (All/Find)
  providers.go      guards for managed provider "1api" (+ legacy "charon")
  edit.go           JSON/TOML load-merge-write helpers (preserve unknown keys)
  codex.go / claude.go / opencode.go / opencode_tiers.go / opencode_omo.go / pi.go
internal/provider/  central external API configs (endpoint/key/tiers/usable)
internal/profile/ snapshot store, split by concern:
  store.go (layout/config/name validation) · snapshot.go (Save/Add/Edit/EnsureDefault)
  apply.go (Apply/Undo/Drift/refresh) · backup.go (backups + prune) · manage.go (rm/mv/cp)
  provider.go (migrate + ApplyProvider) · session.go (MaterializeSession for 1api run)
internal/models/  fetch model lists + ResolveTiers + FilterReachable
internal/jsonc/   strip comments for OpenCode/omo JSONC
internal/secret/  masking + platform keychain (darwin vs. other build tags)
internal/tui/     bubbletea interactive menu
```

Layering (imports point left): `secret` ← `artifact` ← `tools`/`models` ← `provider` ← `profile` ← `cmd`/`tui`.
Profile and provider names are validated centrally (`validateName` / provider sanitize);
never join a user-supplied name into a path without it.

Data lives under `~/.config/1api/` (`$XDG_CONFIG_HOME` respected):
`providers/<name>/provider.json` (central endpoint+key+high/mid/low+usable),
`profiles/<tool>/<name>/` (snapshot files + `manifest.json`),
`backups/<tool>/<timestamp>/`, and `config.json` (active profile per tool,
`toolProvider` bindings, `providersMigrated` flag).
OpenCode oh-my models live separately at `~/.omo/omo.jsonc` (patched in place).

### Central providers (do not regress)

Implementation: `internal/provider` (CRUD + `FilterReachable` gates),
`internal/profile/provider.go` (migrate + `ApplyProvider` + bind),
CLI `provider` / `use`, TUI “Use provider…”.

| Invariant | Detail |
|-----------|--------|
| Path | `$XDG_CONFIG_HOME/1api/providers/<name>/provider.json` only |
| Once | External API credentials configured once; tools bind via `toolProvider` |
| Tiers | mid/low/high from `models.ResolveTiers`; set only if id ∈ usable (or live probe) |
| Verify | `provider add/edit` and `use` fail-closed unless `--no-verify` |
| Migrate | On open: API-proxy profile Specs → providers (offline, `needsVerify`); OAuth untouched |
| Compat | `1api add <tool>` upserts provider + binds tool + snapshots profile |
| Safety | 0600 files, atomic write; never log raw keys |

### How to add a new tool

1. Add `internal/tools/<tool>.go` returning a `*Tool` with: `Name`, `Title`,
   `Provider` (`openai`/`anthropic`), `DefaultEndpoint`, `Artifacts` (built from
   `internal/artifact` constructors), `Detected`, `Describe`, and `ApplyAuth`.
2. Register it in `All()` in `tool.go`.
3. Add a `TestXxxDescribeAndApply` in `tools_test.go` using a sandboxed `$HOME`.
   Everything else (store, CLI, TUI) is generic and needs no changes.

`ApplyAuth` must **merge** into existing config (use the `edit.go` helpers) so
unrelated user settings survive; it must not rewrite the file wholesale.

## Conventions

- Standard Go style: `gofmt`/`goimports`, tabs, error wrapping with `%w`,
  table-driven tests, small focused packages, exported identifiers documented.
- Keep `cmd/` thin — logic belongs in `internal/` packages so it stays testable.
- Never log or print full secrets; route them through `secret.Mask`.
- Prefer standard library. Current third-party deps are intentionally minimal:
  `bubbletea`/`bubbles`/`lipgloss` (TUI) and `pelletier/go-toml/v2` (Codex TOML).
  Discuss before adding more.
- Platform-specific code goes behind build tags (`_darwin.go` / `_other.go`),
  never runtime `runtime.GOOS` branching for the keychain.

## Testing expectations

- Any new behavior needs a test. Keep coverage on `internal/models`,
  `internal/profile`, and `internal/tools` from regressing.
- Use `httptest` for anything hitting the network (see `fetch_test.go`); never
  make real API calls in tests.
- The TUI is verified by compile + `go vet`; extract pure logic into testable
  helpers rather than testing the bubbletea event loop.

## Out of scope / do not do

- Don't commit built binaries, `dist/`, or coverage files (see `.gitignore`).
- Don't send config or secrets to any external service.
- Don't weaken file permissions or remove the pre-switch backup step.
