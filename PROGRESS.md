# 项目进度

## GOAL
修复 charon 配置 Codex 自定义 API 后启动告警：`codex_apps` MCP 401（ChatGPT token）与自定义模型 `Model metadata … not found`。

### 验收标准
- [x] `charon` ApplyAuth（codex）写入 `model_catalog_json` + `~/.codex/charon-model-catalog.json`，slug 与 `model` 一致
- [x] ApplyAuth / switch 后对自定义 provider 设置 `features.apps = false`，避免 `codex_apps` 用失效 ChatGPT token 握手
- [x] UseOfficialAuth / 切回非 charon provider 时清除 `model_catalog_json`，并去掉 charon 写入的 `features.apps`
- [x] 仅改 charon 相关键；保留用户 `[features].code_mode` 等其它设置
- [x] `bash .harness/verify.sh` 退出 0
- [x] 多维评估无 Critical/Major

### 代定决策
- 根因拆分：① `codex_apps` 走 ChatGPT OAuth（与自定义 Bearer 无关）；② 自定义 slug 不在 Codex 内置目录 → fallback metadata
- 目录文件固定路径：`$HOME/.codex/charon-model-catalog.json`（0600）
- catalog 含 active model + `AllModels`（若有）；字段用 Codex MVP schema + `context_window`（Claude 200K，其它 128K）+ `apply_patch_tool_type=freeform`
- `features.apps=false` 仅在 `model_provider=charon` 时写入；不把整个 `features` 纳入 profile ownedKeys
- `model_catalog_json` 纳入 MergedTOML ownedKeys，便于 switch 正确增删
- AfterLiveChange：按 live `model_provider` 同步 catalog / apps（覆盖 switch/undo）
- verify.sh：追加 Codex catalog/apps 相关 `-run` 过滤（只增强）

## 已完成
- `internal/tools/codex_catalog.go` + ApplyAuth/UseOfficialAuth/AfterLiveChange 挂钩
- 单测 `TestCodexApplyAuthWritesModelCatalogAndDisablesApps` / `TestCodexAfterLiveChangeSyncsCompanion`
- live `~/.codex` 已同步；smoke：无 MCP/metadata 告警，模型可回复
- `bash .harness/verify.sh` 通过；EVAL 无 Critical/Major

## 进行中
（无）

## 未开始 / 已知问题
- `auth.json` ChatGPT refresh token 仍 invalidated；若要用官方 Apps 需 `codex login`。自定义 API 推理不受影响，但 Codex 可能仍打 refresh 失败日志。

## 阻塞
（无）
