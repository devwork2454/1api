# EVAL — Codex custom provider companion sync

## GOAL
修复 charon 配置 Codex 自定义 API 后：`codex_apps` 401 与 `Model metadata not found`。

## 维度

| 维度 | 评级 | 说明 |
|------|------|------|
| D1 功能正确性 | OK | verify 绿；live smoke：无 codex_apps / metadata 告警，模型回复 pong |
| D2 架构一致性 | OK | 仅 charon provider + owned `model_catalog_json`；`features.apps` 经 AfterLiveChange 同步，不整表快照 |
| D3 成本/安全闸门 | OK | 密钥仍 0600/原子写；catalog 无密钥；未触碰 auth.json |
| D4 可维护性 | OK | PROGRESS/verify 增强；单测覆盖 ApplyAuth/UseOfficialAuth/AfterLiveChange |
| D5 已知条件项 | OK | ChatGPT `refresh_token_invalidated` 日志保留为条件项（需用户 `codex login`），非本 GOAL 伪装完成 |

## 结论
无明显问题（无 Critical / Major）。

## Minor / 未尽
- live Codex 仍可能打印 `Failed to refresh token`（auth.json ChatGPT 会话已失效）；不影响自定义 API 推理。
