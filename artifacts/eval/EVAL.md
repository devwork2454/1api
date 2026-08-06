# EVAL — OpenCode ↔ OMO companion sync

## GOAL
为 OpenCode 设置 API provider/模型时同步 oh-my-openagent；未安装则跳过。

## 多维评估

| 维度 | 评级 | 说明 |
|------|------|------|
| D1 功能正确性 | OK | verify 绿；ApplyAuth 同步 OMO；缺省跳过；switch 经 AfterLiveChange 同步 |
| D2 架构一致性 | OK | OMO 非 Artifact；companion + AfterLiveChange；AGENTS.md 已注明 |
| D3 成本/安全闸门 | OK | 沙箱测试；原子写 0600；不创建缺失的 OMO；不碰真实 HOME |
| D4 可维护性 | OK | PROGRESS/harness/单测入口齐全 |
| D5 已知条件项 | OK | 未把「安装 OMO」伪造成必选；实机刷新需用户再跑 switch/add |

## 结论
无明显问题（无 Critical / Major）。

## Minor / 未尽
- 实机 `~/.omo/omo.jsonc` 需用户执行一次 OpenCode add/switch 才会被本逻辑刷新
