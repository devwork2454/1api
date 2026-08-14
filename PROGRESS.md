# 项目进度

## 当前任务

`1api update` 默认先拉 Gitee，失败再拉 GitHub。

### 规格（代定）

- **目标**：去掉 locale / GitHub 2s 探测选源；无覆盖时恒为 Gitee → GitHub。
- **保留**：`CHARON_UPDATE_URL` 单源覆盖；`CHARON_UPDATE_SOURCE=github|gh|global` 仍可强制 GitHub 优先。
- **文档**：usage 与 README 安装顺序与代码一致（Gitee 先）。
- **不做什么**：不打新 tag / 不发布；不同步 Gitee release 附件；不改 install.sh 下载顺序（已是 Gitee 先）。
- **验收**：
  1. 空 `CHARON_UPDATE_SOURCE` 时 `preferGiteeUpdate() == true`，`updateInstallURL` 含 `gitee.com/wbff/1api`。
  2. `CHARON_UPDATE_SOURCE=github` 仍走 GitHub URL。
  3. `looksLikeChinaEnv` / `githubQuickReachable` 不再参与选源。
  4. `.harness/verify.sh` 退出码 0。

## 已完成

- 核对发布：GitHub `v1.5.14-devwork1` 已发布；Gitee latest 仍为 `v1.5.13-devwork1`。
- 核对 `update`：已发布版本按 locale/TZ + GitHub 可达性选主源，**不是**默认 Gitee。
- `preferGiteeUpdate` 默认恒为 true；仅 `CHARON_UPDATE_SOURCE=github|gh|global` 翻转。
- 删除 locale / TCP 探测选源；usage 与 README 改为 Gitee 先。
- `.harness/verify.sh`：gofmt + vet + update 测试 + `go test -race ./...` 通过。

## 进行中

（无）

## 未开始 / 已知问题

- Gitee 发行版附件落后一版；`update` 改 Gitee 优先后，在未同步附件前会装到 `v1.5.13`。
- 本改动需新版本发布后，已安装用户才会拿到新的 `update` 行为。
