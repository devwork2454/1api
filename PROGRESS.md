# 项目进度

## 当前任务

发布 `v1.5.18-devwork1`：模型目录解析最大上下文长度，provider 持久化后写入 Codex/Pi。

### 规格

- **版本**：`v1.5.18-devwork1`（上一发布 tag 为 `v1.5.17-devwork1`）。
- **内容**：`a2ac2fa` 从 `/v1/models` 解析 context window；`provider.Record.contextWindows`；Codex/Pi ApplyAuth 优先目录值。
- **GitHub**：推 `1api/main` + annotated tag，等 Release 出 6 个资产。
- **Gitee**：推同一 tag，`scripts/sync-gitee-release.sh` 挂同名附件。
- **验收**：
  1. GitHub latest tag = `v1.5.18-devwork1`，6 个资产齐全。
  2. Gitee `/releases/latest` 的 `tag_name` = `v1.5.18-devwork1`，同 6 个资产可下载。
  3. `.harness/verify.sh` 为 0。
- **回滚点**：`v1.5.17-devwork1`。
- **不做什么**：不提交 `.agent/`、`patch_pi*.js`、`1api_test` 等未跟踪杂项。

## 已完成

- 核对发布：GitHub `v1.5.14-devwork1` 已发布；Gitee latest 仍为 `v1.5.13-devwork1`。
- 核对 `update`：已发布版本按 locale/TZ + GitHub 可达性选主源，**不是**默认 Gitee。
- `preferGiteeUpdate` 默认恒为 true；仅 `CHARON_UPDATE_SOURCE=github|gh|global` 翻转。
- 删除 locale / TCP 探测选源；usage 与 README 改为 Gitee 先。
- `.harness/verify.sh`：gofmt + vet + update 测试 + `go test -race ./...` 通过。
- 独立终审（autonomy-harness:verifier）PASS。
- 发布 `v1.5.15-devwork1`：
  - GitHub Release workflow 成功（6 资产）。
  - Gitee tag + release `796309`，6 个同名附件公开 200。
  - 两边 `/releases/latest` 均为 `v1.5.15-devwork1`。
  - CI on main 成功。

## 已完成（DeepSeek Anthropic 回退）

- 根因已用真 DeepSeek 口验证：`GET /anthropic/v1/models` 404；`GET /v1/models` 200；`POST /anthropic/v1/messages` 200。
- Anthropic list/chat 增加 URL 回退；httptest 覆盖 Fetch / Probe / FilterReachable。
- `.harness/verify.sh` 通过。
- 独立终审 PASS。

## 已完成（v1.5.16 发版）

- GitHub Release `v1.5.16-devwork1` 成功（6 资产）。
- Gitee release `796374`，6 个同名附件公开 200。
- 两边 latest = `v1.5.16-devwork1`。
- CI on main 成功。
- `.harness/verify.sh` 预检通过。

## 已完成（v1.5.17 发版）

- GitHub Release `v1.5.17-devwork1` 成功（6 资产，tag 指向 `037cc05`）。
- Gitee release `796987`，6 个同名附件公开 200。
- 两边 latest = `v1.5.17-devwork1`。
- CI on main 成功（lint + ubuntu/macos test）。
- `.harness/verify.sh` 预检通过。
- 发版前补了 `pi.go` lint（`*Ids` → `*IDs`，删除未用常量），并在 0 下载时重打同一 tag。
- 独立终审（autonomy-harness:verifier）PASS。

## 进行中

- 发布 `v1.5.18-devwork1`（context window 解析）。

## 未开始 / 已知问题

- GitHub Actions「Mirror to Gitee」仍失败；本次用本地 `git push gitee` 补齐 main/tag。
