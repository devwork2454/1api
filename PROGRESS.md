# 项目进度

## 当前任务

（无）

## 已完成（v1.5.19 发版）

- 修复：Pi 扩展 contextWindow 不再写死 128k——
  - 读 pi 自己的 `models-store.json`（pi.dev 远端目录缓存），与 pi footer/压缩同源；
  - 跨 provider 共享 live catalog 窗口（如 grok-4.5 ← x.ai 500k 共享给 arcdent）；
  - 内置已证实窗口表 `piBuiltinWindow`（deepseek-v4-*→1M、glm-5.2/z-ai→1M、grok-4.5→500k、grok-4.20→1M）；
  - live catalog fetch 并发 best-effort，失败静默回退，离线不阻塞写扩展。
- 修复：Pi 扩展 baseUrl 对 anthropic 端点（`…/anthropic`）转成 OpenAI 兼容 URL（`piOpenAIBaseURL`），修掉 404。
- 清理：删除 `models/probe.go` 未使用的 `fetchWithClient`（unused linter）。
- 测试：新增 `TestPiBuiltinWindow` / `TestPiReadStoredWindows` / `TestMergeWindows` / `TestPiCatalogWindows` / `TestPiOpenAIBaseURL` / `TestPiPrimaryWire`；`TestPiDescribeAndApply` 覆盖 store 场景。
- GitHub Release `v1.5.19-devwork1` 成功（6 资产，tag 指向 `2685792`；修正历史遗留 tag 指向）。
- Gitee release `798900`，6 个同名附件公开；latest = `v1.5.19-devwork1`（tag 同步修正）。
- 本地 `make install` → `1api v1.5.19-devwork1`（`~/.local/bin/1api` + charon 符号链接）。
- `.harness/verify.sh` 预检通过。

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
