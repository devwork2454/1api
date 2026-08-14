# 项目进度

## 当前任务

发布 `v1.5.17-devwork1`：把 Pi 全量 provider 注入与主档过滤打进 GitHub + Gitee，让 `1api update` 能装到该版。

### 规格（代定）

- **版本**：`v1.5.17-devwork1`（上一发布 tag 为 `v1.5.16-devwork1`）。
- **内容**：`5b90180` Pi 注入全部中央供应商为 `1api-[name]`；`874fed6` 只注入 High/Mid/Low 主档，避免 `/model` 被整表刷屏。
- **GitHub**：推 `1api/main` + annotated tag，等 Release 出 6 个资产。
- **Gitee**：推同一 tag，`scripts/sync-gitee-release.sh` 挂同名附件。
- **验收**：
  1. GitHub latest tag = `v1.5.17-devwork1`，6 个资产齐全。
  2. Gitee `/releases/latest` 的 `tag_name` = `v1.5.17-devwork1`，同 6 个资产可下载。
  3. `.harness/verify.sh` 为 0。
- **回滚点**：`v1.5.16-devwork1`。
- **不做什么**：不改业务逻辑；不把 token 写入仓库；不同步更旧附件；不提交 `.agent/`、`patch_pi*.js` 等未跟踪杂项。

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

（无）

## 未开始 / 已知问题

- GitHub Actions「Mirror to Gitee」仍失败；本次用本地 `git push gitee` 补齐了 main/tag。
- 本机旧二进制需用户跑一次 `1api update` 才会升到含 Pi 注入的 `v1.5.17`。
