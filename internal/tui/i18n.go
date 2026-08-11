package tui

import "1api/internal/profile"

type msgID int

const (
	msgMainTitle msgID = iota
	msgProviders
	msgProvidersDesc
	msgBindings
	msgBindingsDesc
	msgSettings
	msgSettingsDesc
	msgAddProvider
	msgAddProviderDesc
	msgNoProviders
	msgProvBound
	msgProvUnused
	msgProvStale
	msgDeleteConfirm
	msgDeleteBlocked
	msgDeleted
	msgCancelled
	msgLang
	msgLangDesc
	msgSkin
	msgSkinDesc
	msgSkinTeal
	msgSkinMono
	msgSkinWarm
	msgLangZH
	msgLangEN
	msgPickProvider
	msgPickMid
	msgPickLow
	msgPickHigh
	msgSameAsMid
	msgBoundOK
	msgBindingTitle
	msgNotInstalled
	msgNoApply
	msgEndpoint
	msgAPIKey
	msgWire
	msgWireOpenAI
	msgWireAnthropic
	msgName
	msgFetching
	msgEnterContinue
	msgEscBack
	msgEscQuit
	msgEnterOpen
	msgKeyDelete
	msgHelpChoose
	msgStepOf
	msgBusyBind
	msgBusyVerify
	msgSelectTool
	msgUseCurrentTiers
	msgKeyEdit
	msgEditProvider
	msgEditKeepKey
	msgEdited
	msgBusyEdit
)

var zh = map[msgID]string{
	msgMainTitle:       "1API — 主菜单",
	msgProviders:       "供应商",
	msgProvidersDesc:   "列表 · 新增 · e 编辑 · d 删除未占用",
	msgKeyEdit:         "e 编辑",
	msgEditProvider:    "编辑供应商",
	msgEditKeepKey:     "留空则保留原 Key",
	msgEdited:          "已更新供应商 %s",
	msgBusyEdit:        "正在保存供应商…",
	msgBindings:        "工具绑定",
	msgBindingsDesc:    "查看并切换各 CLI 绑定的供应商与模型档位",
	msgSettings:        "设置",
	msgSettingsDesc:    "语言 · 皮肤",
	msgAddProvider:     "＋ 新增供应商…",
	msgAddProviderDesc: "填写 endpoint 与 key，配置一次",
	msgNoProviders:     "暂无供应商 — 选「新增供应商」",
	msgProvBound:       "已绑定",
	msgProvUnused:      "未使用",
	msgProvStale:       "待校验",
	msgDeleteConfirm:   "删除供应商 %q？此操作不可撤销。",
	msgDeleteBlocked:   "供应商 %q 正被工具使用，无法删除",
	msgDeleted:         "已删除供应商 %s",
	msgCancelled:       "已取消",
	msgLang:            "语言",
	msgLangDesc:        "界面语言",
	msgSkin:            "皮肤",
	msgSkinDesc:        "主题配色",
	msgSkinTeal:        "青绿（默认）",
	msgSkinMono:        "单色",
	msgSkinWarm:        "暖色",
	msgLangZH:          "中文",
	msgLangEN:          "English",
	msgPickProvider:    "选择供应商 · %s",
	msgPickMid:         "选择 mid（主模型）",
	msgPickLow:         "选择 low（轻量）",
	msgPickHigh:        "选择 high（重量）",
	msgSameAsMid:       "（与 mid 相同）",
	msgBoundOK:         "已绑定 %s → %s",
	msgBindingTitle:    "工具绑定",
	msgNotInstalled:    "未安装",
	msgNoApply:         "此工具不支持供应商绑定",
	msgEndpoint:        "API 基址（可留空用默认）",
	msgAPIKey:          "API Key（输入时隐藏）",
	msgWire:            "协议格式",
	msgWireOpenAI:      "OpenAI 兼容",
	msgWireAnthropic:   "Anthropic",
	msgName:            "供应商名称",
	msgFetching:        "正在拉取可用模型…",
	msgEnterContinue:   "enter 继续",
	msgEscBack:         "esc 返回",
	msgEscQuit:         "esc 退出",
	msgEnterOpen:       "enter 打开",
	msgKeyDelete:       "d 删除",
	msgHelpChoose:      "enter 选择",
	msgStepOf:          "步骤 %d / %d · %s",
	msgBusyBind:        "正在绑定…",
	msgBusyVerify:      "正在校验…",
	msgSelectTool:      "选择工具以绑定供应商",
	msgUseCurrentTiers: "使用当前 mid/low/high 直接绑定",
}

var en = map[msgID]string{
	msgMainTitle:       "1API — menu",
	msgProviders:       "Providers",
	msgProvidersDesc:   "List · add · e edit · d delete unused",
	msgKeyEdit:         "e edit",
	msgEditProvider:    "Edit provider",
	msgEditKeepKey:     "blank keeps current key",
	msgEdited:          "updated provider %s",
	msgBusyEdit:        "saving provider…",
	msgBindings:        "Tool bindings",
	msgBindingsDesc:    "Bind CLIs to providers and set model tiers",
	msgSettings:        "Settings",
	msgSettingsDesc:    "Language · skin",
	msgAddProvider:     "＋ Add provider…",
	msgAddProviderDesc: "endpoint + key once",
	msgNoProviders:     "No providers — choose Add provider",
	msgProvBound:       "bound",
	msgProvUnused:      "unused",
	msgProvStale:       "stale",
	msgDeleteConfirm:   "Delete provider %q? This cannot be undone.",
	msgDeleteBlocked:   "provider %q is in use by a tool",
	msgDeleted:         "deleted provider %s",
	msgCancelled:       "cancelled",
	msgLang:            "Language",
	msgLangDesc:        "UI language",
	msgSkin:            "Skin",
	msgSkinDesc:        "Color theme",
	msgSkinTeal:        "Teal (default)",
	msgSkinMono:        "Mono",
	msgSkinWarm:        "Warm",
	msgLangZH:          "中文",
	msgLangEN:          "English",
	msgPickProvider:    "Choose provider · %s",
	msgPickMid:         "Choose mid (primary)",
	msgPickLow:         "Choose low (light)",
	msgPickHigh:        "Choose high (heavy)",
	msgSameAsMid:       "(same as mid)",
	msgBoundOK:         "bound %s → %s",
	msgBindingTitle:    "Tool bindings",
	msgNotInstalled:    "not installed",
	msgNoApply:         "tool does not support provider bind",
	msgEndpoint:        "API base URL (blank = default)",
	msgAPIKey:          "API key (hidden)",
	msgWire:            "Wire format",
	msgWireOpenAI:      "OpenAI-compatible",
	msgWireAnthropic:   "Anthropic",
	msgName:            "Provider name",
	msgFetching:        "Fetching usable models…",
	msgEnterContinue:   "enter continue",
	msgEscBack:         "esc back",
	msgEscQuit:         "esc quit",
	msgEnterOpen:       "enter open",
	msgKeyDelete:       "d delete",
	msgHelpChoose:      "enter choose",
	msgStepOf:          "Step %d of %d · %s",
	msgBusyBind:        "binding…",
	msgBusyVerify:      "verifying…",
	msgSelectTool:      "Select a tool to bind a provider",
	msgUseCurrentTiers: "Bind with current mid/low/high",
}

func tr(lang string, id msgID) string {
	if lang == profile.LangEN {
		if s, ok := en[id]; ok {
			return s
		}
	}
	if s, ok := zh[id]; ok {
		return s
	}
	return en[id]
}
