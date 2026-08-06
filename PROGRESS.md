# 项目进度

## GOAL
为 charon 在配置 OpenCode 的 API provider/模型时，同步更新 oh-my-openagent（OMO）的 agents/categories 模型；未安装 OMO 则跳过。

### 验收标准
- [x] `charon add/edit opencode`（ApplyAuth）在存在 `~/.omo/omo.jsonc` 时，将 `[opencode].agents` / `categories` 的 `model` 写成与 OpenCode tier 一致的 `charon/<id>`
- [x] `charon switch opencode <profile>` 恢复 OpenCode 后同样同步 OMO（若已安装）
- [x] 无 `~/.omo/omo.jsonc|omo.json` 时 ApplyAuth/switch 不报错、不创建 OMO 文件
- [x] 仅改 `model` 字段，保留 skills/prompt_append 等其它键
- [x] `bash .harness/verify.sh` 退出 0

### 代定决策
- OMO 检测：仅看 `$HOME/.omo/omo.jsonc` 或 `omo.json` 是否存在
- 模型写入：`charon/<resolved-tier-id>`（与 OpenCode tier routing 同一套 mid/low/high）
- 覆盖策略：重写 agents/categories 下全部 `model`；其它键不动
- 配置节：优先 `[opencode]`，否则顶层 `agents`/`categories`
- 不把 OMO 纳入 profile Artifacts；通过 `Tool.AfterLiveChange` 在 switch/undo 后同步
- verify.sh 增强：追加 OMO 相关测试 `-run` 过滤（只增强不弱化）
- 一并纳入：`--key-env` 与 `models/` 前缀规范化

## 已完成
- `internal/tools/omo.go` + 单测；OpenCode ApplyAuth / AfterLiveChange 挂钩
- `internal/profile/apply.go` 调用 AfterLiveChange
- AGENTS.md 注明可选写入 `~/.omo/omo.jsonc`
- `bash .harness/verify.sh` 通过

## 进行中
（无）

## 未开始 / 已知问题
- 未改真实 `$HOME` 下的 omo（开发规范）；用户侧需再跑一次 `charon switch/add opencode …` 才会刷新实机 OMO
- 仓库内 `.omo/` 归档笔记、`CLAUDE.md` 为本地文件，未纳入本提交

## 阻塞
（无）
