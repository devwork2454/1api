# 项目进度

## 当前任务

发布 `v1.5.15-devwork1`：GitHub GoReleaser + 同步同名附件到 Gitee，让 `1api update` 默认 Gitee 能装到含 Gitee-first 逻辑的版本。

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

## 进行中

（无）

## 未开始 / 已知问题

- Gitee 发行版附件落后一版；`update` 改 Gitee 优先后，在未同步附件前会装到 `v1.5.13`。
- 本改动需新版本发布后，已安装用户才会拿到新的 `update` 行为。
