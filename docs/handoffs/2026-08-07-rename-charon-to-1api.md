# Handoff: charon → 1api 全面改名（进行中，编译未通过）

- **Source tool / session:** Codex（fastctx 环境）
- **Written at:** 2026-08-07 15:00 CST
- **Cwd:** `/home/zakza/project/research/charon`
- **Branch:** `main`（dirty，大量未提交改动）
- **Git snapshot:** 见下方「Git 状态快照」
- **For next agent:** 继续完成改名收尾

## Goal

把项目从 `charon` 整体改名为 `1api`：模块名、二进制名、usage、配置目录
`~/.config/charon`→`~/.config/1api`、provider 标记 `"charon"`→`"1api"`（保留旧值兼容），
并让 `make fmt` + `make test` 全绿。

- **Acceptance criteria:** `go build ./...` 通过；`make test` 全绿；grep 不到残留的
  `charon`（除 providers.go/pi.go 中有意保留的 legacy 常量与注释外）。

## Done（已完成）

- [x] 目录重命名：`cmd/charon` → `cmd/1api`（git 识别为 rename）
- [x] 全局替换 Go 源码 + Makefile + .goreleaser.yaml + README + install.sh 中
      `charon` → `1api`（`module 1api` 已生效）
- [x] 修正非法标识符：`charonDelegate`→`apiDelegate`（`1api` 不能做 Go 标识符）
- [x] **provider 兼容逻辑**已写入 `internal/tools/providers.go`：
      `legacyManagedProvider = "charon"`、`isManagedProvider`、`firstManagedProvider`、
      `trimProviderPrefix`（对 `1api/` 与 `charon/` 前缀都剥离）
- [x] **opencode.go** 兼容：读旧 provider 用 `firstManagedProvider`；model 前缀用
      `trimProviderPrefix`；Describe 同时查 `managedProvider` 与 `legacyManagedProvider`
- [x] **pi.go** 兼容：`piConfigRE` 正则同时匹配 `1api`/`charon`；新增
      `piReadExtension` 读取时优先 `1api.ts`、回退 `charon.ts`；`defaultProvider`
      同时接受 `"1api"`/`"charon"`/`""`
- [x] **codex.go** 无需改：Describe 用 `cfg.ModelProvider`（配置里存的值）作 key，
      天然兼容新旧名

## Not done（未完成 / 阻塞）

- [ ] **`cmd/1api/commands.go:498` 语法错误未修复** —— 上一轮 sed 时把引号吃掉：
      `fmt.Println(1API binary uninstalled successfully.")` 缺开头引号，
      应改为 `fmt.Println("1API binary uninstalled successfully.")`
- [ ] 修复后需重新 `go build ./...` / `make test` 验证全绿
- [ ] `make fmt` 尚未执行
- [ ] 未检查 `_test.go` 中 provider 断言是否与新名一致（如 `tools_test.go`、
      `opencode_tiers_test.go` 里 `"charon"` provider 断言，需确认已随全局替换更新）
- [ ] `.goreleaser.yaml` 的 `release.github.name` 仍可能指向 `charon`，需确认改为 `1api`
- [ ] 未提交任何改动（全部在工作区）

## Git 状态快照

```text
M  .goreleaser.yaml, Makefile, README.md, go.mod, install.sh
RM cmd/charon/* -> cmd/1api/*  （7 个文件 rename）
M  internal/artifact/*, internal/jsonc/*, internal/profile/*,
   internal/tools/*, internal/tui/*  （约 30 个文件）
?? .omo/, CLAUDE.md   （未跟踪，勿动）
```

## Key paths

| Path | Why it matters |
|------|----------------|
| `cmd/1api/commands.go:498` | 当前唯一语法错误，需修引号 |
| `internal/tools/providers.go` | 新增的兼容函数所在（legacy 常量、firstManagedProvider 等）|
| `internal/tools/opencode.go` | provider/model 前缀兼容改动 |
| `internal/tools/pi.go` | 新旧扩展名回退 + defaultProvider 兼容 |
| `internal/tui/theme.go` | `charonDelegate`→`apiDelegate` |
| `go.mod` | `module 1api` |
| `Makefile` | `BINARY := 1api` |

## Decisions（不要随意撤销）

- `1api` 作为模块名/导入路径合法（Go 模块路径无「数字开头」限制，已实测编译通过）；
  但**不能作为 Go 类型/函数名**，故 `charonDelegate`→`apiDelegate`
- provider 标记写入用新名 `"1api"`，读取时兼容旧 `"charon"`，避免旧配置被当未知 provider
- 残留 `charon` 仅保留在 providers.go/pi.go 的 legacy 常量与注释里，属有意为之

## Next steps（有序）

1. 修复 `cmd/1api/commands.go:498` 的引号 → `fmt.Println("1API binary uninstalled successfully.")`
2. `export GOCACHE=/tmp/gocache && GOFLAGS=-buildvcs=false go build ./...` 确认编译通过
3. 检查 `.goreleaser.yaml` 的 `release.github.name` 是否需从 `charon` 改 `1api`
4. 检查 `*_test.go` 中 provider 字符串断言（`"charon"`）是否已全局替换为 `"1api"`，
   若有漏网需同步兼容或更新断言
5. `make fmt`
6. `make test`（沙箱 HOME，勿碰真实配置）
7. 全部绿后提交并推送到 fork（`origin` = `devwork2454/charon`，改名后地址需更新）

## Verify

```bash
export GOCACHE=/tmp/gocache
GOFLAGS=-buildvcs=false go build ./...      # 应 exit 0
make test                                     # 应全绿
# 残留 charon 应只在 providers.go / pi.go 的 legacy 处
grep -rn 'charon' --include="*.go" . | grep -v 'legacy\|charon.ts\|charon/\|//'
```

## Warnings

- 本文件是未经验证的历史记录；接手的 agent 需先核对仓库实际状态，repo 优先。
- 工具会改动真实用户凭据（`~/.codex` 等），**测试/开发必须沙箱 $HOME**，禁止对真实配置跑 `add/switch/save`。
- 保留原子写入、0600 权限、切换前自动备份等安全保证，勿回归。
- 未提交的 `.omo/`、`CLAUDE.md` 属工作区既有文件，不要动。
- 改完务必 `make fmt` + `make test` 全绿再提交。
