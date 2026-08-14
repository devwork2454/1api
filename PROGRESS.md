# 项目进度

## 当前任务

修复：添加 DeepSeek 供应商并选 Anthropic 协议时报错。

### 规格（代定）

- **根因**：`https://api.deepseek.com/anthropic` 无 `GET /v1/models`（404）；目录在主机 `GET /v1/models`；对话在 `POST /anthropic/v1/messages`。`FilterReachable` 只打 `{endpoint}/v1/models`，添加失败。
- **修复**：Anthropic wire 的 list/chat URL 增加回退（不改鉴权头）。
  - list：`{ep}/v1/models` 失败则试去 `/anthropic` 后的主机 `/v1/models`。
  - chat：`{ep}/v1/messages` 失败且 path 无 `/anthropic` 时试 `{origin}/anthropic/v1/messages`。
- **验收**：httptest 模拟 DeepSeek 分叉时 `Fetch` / `Probe` / `FilterReachable` 成功；`.harness/verify.sh` 为 0。
- **不做什么**：不改真实 HOME 里的 provider；测试不打真网；不为此单独打 tag。

### 规格（代定）

- **版本**：`v1.5.15-devwork1`（上一发布 tag 为 `v1.5.14-devwork1`）。
- **GitHub**：推 `1api/main` + 打 annotated tag，等 Release workflow 出 4 平台 tar.gz、`checksums.txt`、`install.sh`。
- **Gitee**：推同一 tag，创建 release，上传同名附件。
- **脚本**：`scripts/sync-gitee-release.sh` 从 GitHub 拉资产再挂到 Gitee（token 只读 `GITEE_TOKEN`）。
- **验收**：
  1. GitHub latest tag = `v1.5.15-devwork1`，6 个资产齐全。
  2. Gitee `/releases/latest` 的 `tag_name` = `v1.5.15-devwork1`，同 6 个资产可下载。
  3. `.harness/verify.sh` 仍为 0。
- **不做什么**：不改业务逻辑；不把 token 写入仓库；不同步更旧的 `v1.5.14` 附件（latest 已覆盖 update 路径）。

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

## 已完成（本任务）

- 根因已用真 DeepSeek 口验证：`GET /anthropic/v1/models` 404；`GET /v1/models` 200；`POST /anthropic/v1/messages` 200。
- Anthropic list/chat 增加 URL 回退；httptest 覆盖 Fetch / Probe / FilterReachable。
- `.harness/verify.sh` 通过。
- 独立终审 PASS。

## 进行中

（无）

## 未开始 / 已知问题

- GitHub Actions「Mirror to Gitee」仍失败；本次用本地 `git push gitee` 补齐了 main/tag。
- 本机已装的 `v1.5.13-devwork1-dirty` 需用户自己跑一次 `1api update` 才会换成新逻辑。
