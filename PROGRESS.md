# 项目进度

## 当前任务

（无）

## 已完成（v1.5.18 发版）

- 功能：`/v1/models` 解析 context window；`provider.Record.contextWindows`；Codex/Pi 优先目录值。
- GitHub Release `v1.5.18-devwork1` 成功（6 资产，tag 指向 `8a035e0`）。
- Gitee release `798875`，6 个同名附件公开 200；latest = `v1.5.18-devwork1`。
- 本地 `make install` → `1api v1.5.18-devwork1`（`~/.local/bin/1api`）。
- `.harness/verify.sh` 预检通过。

## 已完成（v1.5.17 发版）

- GitHub Release `v1.5.17-devwork1` 成功（6 资产，tag 指向 `037cc05`）。
- Gitee release `796987`，6 个同名附件公开 200。
- 两边 latest 曾为 `v1.5.17-devwork1`。
- CI on main 成功；`.harness/verify.sh` 预检通过。

## 进行中

（无）

## 未开始 / 已知问题

- GitHub Actions「Mirror to Gitee」仍失败；本次用带 token 的 `git push` 补齐 main/tag。
