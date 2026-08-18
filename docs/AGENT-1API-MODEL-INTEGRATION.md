# Agent × 1api 模型衔接指南

> **读者**：接入本机 AI 栈的 Agent / 扩展作者 / 宿主实现者（OpenCode、Pi/aiia、Codex、Claude Code，或自研客户端）。  
> **目标**：模型配置与 **1api** 无缝衔接——不重复管 key、不写死供应商、分档语义一致、换网关零改业务代码。  
> **1api 是什么**：本机 CLI，把各工具的 **endpoint + credentials + 可用模型** 在「中央供应商」与「工具绑定」之间统一切换；**运行时请求不经过 1api 进程**。

---

## 0. 30 秒心智模型

```
┌─────────────────────────────────────────────────────────┐
│  1api = 控制面（配一次供应商，绑到各工具配置文件）          │
│  ~/.config/1api/providers/<name>/provider.json            │
│       mid / low / high / usable[] / endpoint / key        │
└───────────────────────────┬─────────────────────────────┘
                            │ 1api use <tool> <provider>
                            ▼
┌─────────────────────────────────────────────────────────┐
│  各工具自己的配置（1api 写入，Agent 只读）                  │
│  OpenCode: opencode.jsonc + ~/.omo/omo.jsonc              │
│  Pi:       ~/.pi/agent/extensions/1api.ts + settings.json │
│  Codex / Claude: 各自 auth + model 字段                   │
└───────────────────────────┬─────────────────────────────┘
                            │ Agent / CLI 启动后
                            ▼
┌─────────────────────────────────────────────────────────┐
│  运行时：读配置里的 provider + model id → 直连网关 API      │
│  不再调用 1api，也不再 ResolveTiers（除非你自己实现路由）   │
└─────────────────────────────────────────────────────────┘
```

| 角色 | 负责 | 不负责 |
|------|------|--------|
| **1api** | 供应商 CRUD、探针 usable、mid/low/high、写入各工具配置、备份/切换 | 代发 chat、业务 Agent 逻辑 |
| **接入 Agent** | 读已写好的 model 字段、按任务选档（若工具支持）、工具/安全/记忆等 | 长期保存 API key、私自覆盖 `1api.ts` / 托管 provider 块 |

**一句话**：Agent 把 1api 当「本机模型配置的真源搬运工」；自己只消费配置，不拥有密钥。

---

## 1. 硬规则（违反即无法无缝）

1. **禁止**在 Agent 仓库、skill、extension 中硬编码 API key / 长期 endpoint。  
2. **禁止**整文件覆盖工具配置（会冲掉 1api 写入的 provider / defaultModel）。只 **merge** 自己命名空间下的键。  
3. **禁止**对「真实云厂商 / 网关 API」直接请求 `model=low|mid|high|medium|reasoning` 等**档位别名**——除非中间有反代把别名映射成真实 id。  
4. **禁止**把 OpenCode 的 `1api/<id>` 字符串原样塞进 Pi/Codex（格式不同，见 §3）。  
5. **换供应商后**必须假设配置文件已变：新开会话或重启常驻宿主；不要缓存旧 baseUrl/key/model 列表。  
6. **只改托管引用**：OpenCode/omo 里仅空、`1api/*`、旧版 `charon/*` 会被 1api 重写；`openai/*` 等外来引用不动——Agent 若要跟档，应使用托管前缀。  
7. **开发 1api 本身时**禁止对真实 `$HOME` 执行 `use/switch/add`；用 `HOME=$(mktemp -d)`。接入 Agent 的集成测试同理，勿污染用户凭证。

---

## 2. 中央供应商与三档语义

### 2.1 真源路径

```text
$XDG_CONFIG_HOME/1api/providers/<name>/provider.json
# 通常即 ~/.config/1api/providers/<name>/provider.json
```

关键字段（概念）：

| 字段 | 含义 |
|------|------|
| `endpoint` | OpenAI/Anthropic 兼容 base URL |
| `key` | API key（磁盘 0600，勿日志打印） |
| `wire` | `openai` \| `anthropic` |
| `mid` | 默认 / 主模型真实 id |
| `low` | 便宜 / 快 / 探索档真实 id |
| `high` | 重推理 / 架构 / 评审档真实 id |
| `usable` | 探针通过、允许使用的 id 列表 |

### 2.2 档位如何产生

`ResolveTiers(primary, usable)` 大致顺序：

1. 列表中恰有 id 名为 `mid` / `low` / `high`  
2. 名称启发式（如 `flash|mini|haiku`→low，`opus|pro|o1|r1|thinking`→high）  
3. 用户在 `provider add/edit` 或 TUI 中显式指定  
4. 找不到的档 **回退到 mid**（单模型供应商时三档同一 id，属正常）

**Agent 含义**：

- 「走 low」= 使用 **当前绑定供应商的 `low` 真实 id**，不是字符串 `"low"`。  
- 单模型时 low=mid=high，UI 仍可能显示档名——**显示档名 ≠ 请求别名**。

### 2.3 人机常用命令（Agent 可提示用户执行，或自身在允许时 shell 调用）

```sh
1api provider ls
1api provider models <name>      # usable + mid/low/high
1api provider verify <name>      # 重新探针
1api use <tool> <provider>       # 绑定并写入该工具配置
1api status                      # 各工具当前绑定与模型摘要
1api verify <tool>               # 按工具侧规则检查
```

`<tool>` ∈ `opencode` | `pi` | `codex` | `claude`。

---

## 3. 各工具配置契约（Agent 必须按宿主读取）

### 3.1 对照总表

| 工具 | 1api 写入位置 | 默认模型 | 三档自动路由 | Agent 应读的 model 形态 |
|------|---------------|----------|--------------|-------------------------|
| **OpenCode** | `~/.config/opencode/opencode.json(c)`；`~/.omo/omo.jsonc`（若存在） | `model` = mid | **有**：按 agent/category **名**写入 | `1api/<真实id>` 或旧 `charon/<真实id>` |
| **Pi** | `~/.pi/agent/extensions/1api.ts`；`settings.json` 的 default* | `defaultModel` = mid | **无**预写；列表在 extension | `provider=1api` + **裸** `真实id` |
| **Codex** | `~/.codex/config.toml` + `auth.json` | `model` = mid | 无 | 工具内当前 model 字段 |
| **Claude Code** | `~/.claude/settings.json`（+ macOS Keychain OAuth 等） | mid 相关字段 | 无 | 工具内当前 model 字段 |

### 3.2 OpenCode / oh-my-openagent

**写入结果（概念）**：

```jsonc
// opencode.jsonc
{
  "model": "1api/<mid>",
  "small_model": "1api/<low>",
  "provider": {
    "1api": {
      "options": { "baseURL": "<endpoint>" },
      "models": { "<id>": { "name": "<id>" } /* usable 全集 */ }
    }
  },
  "agent": {
    "explore": { "model": "1api/<low>" },
    "compaction": { "model": "1api/<low>" }
    // high 名 → 1api/<high>；其余 → mid
  }
}
```

```jsonc
// ~/.omo/omo.jsonc — 仅当文件已存在；1api 从不创建
{
  "[opencode]": {
    "agents": {
      "explore": { "model": "1api/<low>" },
      "sisyphus": { "model": "1api/<high>" }
    },
    "categories": {
      "quick": { "model": "1api/<low>" },
      "deep": { "model": "1api/<high>" }
    }
  }
}
```

**名字 → 档（1api 写配置时使用；运行时不再算）**：

| 档 | Agent 名关键字（含子串） | Category 名关键字 |
|----|--------------------------|-------------------|
| **low** | `explore`, `librarian`, `compaction`, `quick`, `fast`, `title`, `summary`, `small` | `quick`, `unspecified-low`, `writing` |
| **high** | `sisyphus`, `oracle`, `plan`, `deep`, `ultrabrain`, `prometheus`, `momus`, `metis`, `architect`, `review`, `research`, `security`, `critique` | `ultrabrain`, `deep`, `unspecified-high`, `artistry` |
| **mid** | 其它未知名 | 其它（含 `visual-engineering` 等） |

**OpenCode 侧 Agent 应做**：

1. 子代理 / category 的 `model` 使用 **`1api/...` 或 `charon/...`**，不要写死 `openai/gpt-4` 若希望跟随 `1api use`。  
2. 发起请求时按 OpenCode 解析：`provider` + `modelId`（去掉前缀后的 id）。  
3. 需要「探索用小模型」：保证 agent **名字**命中 low 关键字，或显式 `model` 指向当前 low 的托管 ref；下次 `1api use/switch` 会按名重写托管 ref。  
4. **不要**在运行时再发明一套与上表冲突的 low/mid/high 字符串协议（除非只用于 UI 标签）。

### 3.3 Pi（含 aiia 等基于 Pi 的宿主）

**写入结果（概念）**：

```ts
// ~/.pi/agent/extensions/1api.ts — 1api 独占维护
pi.registerProvider("1api", {
  name: "1api",
  baseUrl: "<endpoint>",
  apiKey: "<key>",
  api: "openai-completions",
  models: [ { id: "<真实id>", name: "<真实id>", /* ... */ }, ... ]
});
```

```jsonc
// ~/.pi/agent/settings.json（1api 只碰 default*；其它键保留）
{
  "defaultProvider": "1api",
  "defaultModel": "<mid 真实id>",
  "packages": [ "…/your-agent/pi-agent" ]  // 由业务 install 写入，1api 不删
}
```

**Pi 侧 Agent 应做**：

1. **消费** `defaultProvider` + `defaultModel`；模型 id 必须是 `1api.ts` 的 `models[].id` 之一。  
2. 请求形态是 **provider 与 model 分离**：`provider=1api`，`model=<裸id>`——**不是** `1api/<id>` 单字符串。  
3. 业务扩展（安全、记忆、质量门）放在 **package / extensions**，**不要**重写 `1api.ts`。  
4. 修改 `settings.json` 时 **merge**，保留 `defaultProvider` / `defaultModel`。  
5. 若实现运行时路由（如按复杂度改 model）：  
   - 默认仅对 **local-proxy / 认档位别名的反代** 输出 `low|medium|high|…`；  
   - 对 1api 直连真实网关时，应改写为 **provider.json 中的真实 mid/low/high id**，或保持 `defaultModel`；  
   - 可用环境变量显式控制（示例名，以各项目为准）：`ROUTER_ENABLED`、`ROUTER_FORCE_MODEL`。  
6. 换 `1api use pi …` 后 **重启** 常驻 host / 新开 `pi` 会话。

### 3.4 Codex / Claude Code

- 1api 绑定后默认 **只有 mid** 写入当前模型相关字段。  
- Agent **无** OpenCode 式按角色自动分档；要强/弱模型需用户在 CLI 内切换或你读 usable 列表后显式设置（若 API 允许）。  
- 不要假设 omo 或 `1api.ts` 存在。

---

## 4. 推荐接入流程（通用）

### 4.1 用户侧一次性

```sh
# 1. 安装 1api，配置中央供应商
1api provider add --name <prov> --endpoint <url> --key <key>
# 或 provider verify / TUI 调整 mid low high

# 2. 绑定当前要用的工具
1api use opencode <prov>
1api use pi <prov>
# …

# 3. 再启动 Agent / 宿主
```

### 4.2 Agent 启动时自检（建议实现）

按宿主做**只读**检查，失败时用自然语言提示用户跑 1api，而不是静默假成功：

| 检查 | OpenCode | Pi |
|------|----------|-----|
| 托管 provider 存在 | `provider.1api` 或 `provider.charon` | extension `1api.ts` 可解析且含 baseUrl |
| 默认模型非空 | `model` 或 omo 主 agent | `defaultProvider==1api` 且 `defaultModel` 非空 |
| 模型在目录中 | id ∈ provider.models | id ∈ models[].id |
| 可选 | omo 存在则 agents 的托管 model 可读 | `packages` 含本 Agent 包路径 |

伪代码：

```text
on_agent_start:
  cfg = read_host_config()
  if not cfg.has_managed_provider():
    tell_user("请先执行: 1api use <tool> <provider>")
    return degraded_or_block
  model = cfg.current_model_id()  # 已是真实 id 或可拆出 id
  if model not in cfg.usable_ids():
    tell_user("默认模型不在可用列表，请 1api provider verify && 1api use …")
  log_info(provider, model)  # 打真实 id，勿只打 "low"
```

### 4.3 运行中选模型

| 意图 | OpenCode / oh-my | Pi / 自研 |
|------|------------------|-----------|
| 默认主会话 | 用配置 `model`（mid） | `defaultModel`（mid） |
| 便宜 / 搜索 / 压缩 | 用已写入的 low 角色或 `small_model` | 显式选 usable 中的 low id，或反代别名 |
| 重推理 / 规划 | 用已写入的 high 角色 | 显式选 high id |
| 用户指定 | 尊重用户/会话覆盖 | 同上 |
| 强制调试 | 用户改配置或 `1api use` | `ROUTER_FORCE_MODEL=<真实id>` 等 |

### 4.4 切换供应商（无缝）

```text
用户或运维: 1api use <tool> <other-provider>
Agent: 不改代码；重启或新会话；自检通过后继续
```

业务逻辑应依赖 **档位角色**（explore vs oracle）或 **能力**（fast vs strong），而不是依赖某个供应商的具体型号字符串（除非做了能力探测）。

---

## 5. 显示名 vs 请求名（防坑）

| 层 | 可能看到 | 实际请求 |
|----|----------|----------|
| UI / 文档 / router 日志 | `low` / `mid` / `high` | 应为 `usable` 中的真实 id |
| OpenCode 配置 | `1api/grok-4.5` | 网关 body.model = `grok-4.5` |
| Pi 配置 | provider `1api` + model `grok-4.5` | body.model = `grok-4.5` |
| 错误示范 | 配置或请求 `model=low` 打到阿里云/xAI | 上游不认 → 失败 |

**验收标准**：日志中的 **request model id** 必须属于 `1api provider models <name>` 的 usable（或官方 OAuth 目录），而不是档位英文名——除非目标 endpoint 明确实现了别名。

---

## 6. 与业务扩展共存（Pi packages / OpenCode plugins）

```text
✅  1api 写：provider、key、模型目录、默认 mid、（OpenCode）角色 model 托管 ref
✅  Agent 写：packages、hooks、tools、skills、safety、自有 settings 键
❌  Agent 写：1api.ts 全文、opencode provider.1api 的 key/baseURL、伪造 usable
❌  双方：在 git 中提交含 key 的配置副本
```

Merge 设置示例（Pi）：

```jsonc
// 只合并业务键，保留 defaultProvider / defaultModel
{
  "enableSkillCommands": false,
  "packages": [" /absolute/or/stable/path/to/pi-agent"]
}
```

---

## 7. 安全与权限

- 凭证文件权限：密钥类 **0600**，目录 **0700**（1api 默认如此，Agent 勿改宽）。  
- 日志、trajectory、issue 上报：**禁止**完整 key；需要时仅 mask 后几位。  
- 不把 `provider.json` / `1api.ts` 拷进项目仓库。  
- 多租户 / 共享机器：按用户 HOME 隔离；Agent 不要读他人 HOME。  
- 合规：路由与 header 注入仅限合法用途；**不做**越狱/绕过供应商安全策略的改写。

---

## 8. 故障排查清单

| 现象 | 优先查 |
|------|--------|
| 401 / 鉴权失败 | `1api status`；是否未 `use`；host 是否未重启 |
| model not found | 请求 id 是否在 usable；是否误传 `low` 别名；OpenCode 是否少了 provider 前缀解析 |
| 一直像「同一个模型」 | 供应商 usable 是否仅 1 个 id（三档回退 mid） |
| omo 没跟着变 | `~/.omo/omo.jsonc` 是否不存在（1api 不创建）；model 是否外来前缀未被改 |
| Pi 扩展双份 / 行为怪 | `settings.packages` 是否含临时 worktree 重复路径 |
| 与文档 low 不一致 | 分清 UI 档名 vs 配置中真实 id（§5） |
| 旧 `charon` 字样 | 兼容旧前缀；新写入多为 `1api`；以当前文件为准 |

```sh
1api status
1api provider models <name>
# OpenCode
grep -E '"model"|small_model' ~/.config/opencode/opencode.jsonc | head
# Pi（勿 cat 出 key；只查结构）
jq '{defaultProvider, defaultModel, packages}' ~/.pi/agent/settings.json
```

---

## 9. 给 Agent 的最小实现清单（可粘贴进 AGENTS.md）

```text
[模型 / 1api]
1. 不存储、不提交 API key；不覆盖 1api 托管的 provider 配置文件。
2. 启动时只读宿主配置；缺 1api 绑定时提示用户: 1api use <tool> <provider>。
3. 请求使用的 model id 必须来自当前 usable / 托管 models 列表。
4. 需要 cheap/strong 时：
   - OpenCode: 依赖已写入的 agent/category model（1api/*），或按角色名约定。
   - Pi: 使用真实 mid/low/high id，或仅对别名反代做档位路由。
5. 禁止对直连真实上游发送 model=low|mid|high 等别名。
6. 日志打印真实 model id；档位名仅可作附加标签。
7. 用户切换供应商后重启常驻进程；不缓存跨供应商的 endpoint/model 列表。
8. 业务 settings 只 merge 自有键；保留 defaultProvider/defaultModel/model 托管字段。
9. 集成测试使用临时 HOME，不触碰真实 ~/.config/1api 与真实密钥文件。
10. 不把 1api 当 HTTP 中<|tool_call_begin|>；chat 直连配置中的 baseUrl。
```

---

## 10. 术语表

| 术语 | 含义 |
|------|------|
| **provider（1api）** | 中央命名的一份 endpoint+key+档位 |
| **provider（OpenCode/Pi）** | 工具配置里的模型供应商条目，托管名一般为 `1api` |
| **mid / low / high** | 1api 三档；值为**真实模型 id** |
| **usable** | 探针可用的模型 id 集合 |
| **托管 ref** | OpenCode 下 `1api/<id>` 或遗留 `charon/<id>` |
| **档位别名** | 字面量 `low`/`medium`/… 作为 model 字段；仅当上游或反代认识时可用 |
| **Apply / use** | 1api 把某 provider 写入某工具实时配置 |

---

## 11. 文档维护

| 项 | 说明 |
|----|------|
| 实现真源 | 本仓库 `internal/provider`、`internal/models/tiers.go`、`internal/tools/opencode*.go`、`internal/tools/pi.go` |
| 用户功能说明 | 根目录 `README.md` |
| 贡献与安全 | `AGENTS.md`（沙箱 HOME、勿弱化权限） |
| 本文角色 | **跨工具 Agent 接入契约**；工具专有扩展细节见各产品 ARCHITECTURE |

变更 1api 写入格式或分档关键字时，应同步更新本文 §3 与 §9。

---

## 12. 副本使用建议

- 放在 1api 仓库：`docs/AGENT-1API-MODEL-INTEGRATION.md`（本文件）。  
- 接入方（如 aiia）可在 `AGENTS.md` 或 `docs/` **链接本文**，或摘录 §1 + §3.3 + §9，避免两份长期分叉。  
- 对用户可见的一句话：**「模型与密钥用 1api 配；Agent 只负责怎么用模型。」**
