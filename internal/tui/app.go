// Package tui is the interactive bubbletea menu for 1api.
package tui

import (
	"fmt"
	"strings"
	"time"

	"1api/internal/profile"
	"1api/internal/provider"
	"1api/internal/secret"
	"1api/internal/tools"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type aView int

const (
	aViewMenu aView = iota
	aViewProviders
	aViewBindings
	aViewSettings
	aViewConfirmDel
	aViewAddEndpoint
	aViewAddKey
	aViewAddWire
	aViewAddName
	aViewFetching
	aViewPickModel // mid after add
	aViewPickProv
	aViewPickMid
	aViewPickLow
	aViewPickHigh
	aViewSetLang
	aViewSetSkin
	aViewEditEndpoint
	aViewEditKey
	aViewEditWire
)

const (
	aMenuProv      = "\x00menu:prov"
	aMenuBind      = "\x00menu:bind"
	aMenuSet       = "\x00menu:set"
	aAddSentinel   = "\x00addprov"
	aLangSentinel  = "\x00setlang"
	aSkinSentinel  = "\x00setskin"
	aProvPrefix    = "\x00prov:"
	aToolPrefix    = "\x00tool:"
	aWireOpenAI    = provider.WireOpenAI
	aWireAnthropic = provider.WireAnthropic
	aSameAsMid     = "\x00sameasmids"
	aUseTiers      = "\x00usetiers"
)

type app struct {
	store   *profile.Store
	version string
	lang    string
	view    aView
	list    list.Model
	input   textinput.Model
	spinner spinner.Model

	allTools       []*tools.Tool
	wiz            providerWiz
	provName       string
	toolName       string
	mid, low, high string
	allModels      []string
	modelFilter    string
	delTarget      string
	editName       string
	editOrigKey    string

	pending       *fetchedMsg
	fetchStart    time.Time
	loadingMsg    string
	busy          bool
	status        string
	statusLvl     statusLevel
	width, height int
}

func newApp(store *profile.Store, version string) app {
	applySkin(store.UISkin())
	l := newBaseList()
	ti := textinput.New()
	ti.CharLimit = 200
	ti.PromptStyle = ti.PromptStyle.Foreground(colorAccent)
	ti.PlaceholderStyle = ti.PlaceholderStyle.Foreground(colorMuted)
	ti.Cursor.Style = ti.Cursor.Style.Foreground(colorAccent)
	ti.Cursor.SetMode(cursor.CursorStatic)

	m := app{
		store:    store,
		version:  version,
		lang:     store.UILang(),
		view:     aViewMenu,
		list:     l,
		input:    ti,
		spinner:  newSpinner(),
		allTools: tools.All(),
	}
	m.loadMenu()
	return m
}

func (m *app) t(id msgID) string { return tr(m.lang, id) }

func (m *app) setStatus(level statusLevel, msg string) {
	m.status, m.statusLvl = msg, level
}

func (m *app) clearStatus() {
	m.status, m.statusLvl = "", statusInfo
}

func (m *app) setHelp(bs ...key.Binding) {
	m.list.AdditionalShortHelpKeys = func() []key.Binding { return bs }
	m.list.AdditionalFullHelpKeys = m.list.AdditionalShortHelpKeys
}

func (m *app) resize() {
	header := 1
	if m.view == aViewMenu {
		header = bannerHeight + 1
	}
	h := m.height - header
	if h < 3 {
		h = 3
	}
	m.list.SetSize(m.width, h)
}

func (m *app) startInput(ph string, secretMode bool) {
	m.startInputValue(ph, "", secretMode)
}

func (m *app) startInputValue(ph, value string, secretMode bool) {
	m.input.SetValue(value)
	m.input.Placeholder = ph
	if secretMode {
		m.input.EchoMode = textinput.EchoPassword
		m.input.EchoCharacter = '•'
	} else {
		m.input.EchoMode = textinput.EchoNormal
	}
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *app) isInput() bool {
	switch m.view {
	case aViewAddEndpoint, aViewAddKey, aViewAddName, aViewEditEndpoint, aViewEditKey:
		return true
	}
	return false
}

func (m *app) findTool(name string) *tools.Tool {
	for _, t := range m.allTools {
		if t.Name == name {
			return t
		}
	}
	return nil
}

func (m *app) toolsBoundTo(prov string) []string {
	var names []string
	for _, t := range m.allTools {
		if m.store.ActiveProvider(t.Name) == prov {
			names = append(names, t.Name)
		}
	}
	return names
}

func (m *app) skipSep(before int) {
	it, ok := m.list.SelectedItem().(item)
	if !ok || it.value != sepSentinel {
		return
	}
	if m.list.Index() >= before {
		m.list.CursorDown()
	} else {
		m.list.CursorUp()
	}
}

func (m *app) loadMenu() {
	items := []list.Item{
		item{title: m.t(msgProviders), desc: m.t(msgProvidersDesc), value: aMenuProv},
		item{title: m.t(msgBindings), desc: m.t(msgBindingsDesc), value: aMenuBind},
		item{title: m.t(msgSettings), desc: m.t(msgSettingsDesc), value: aMenuSet},
	}
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	m.list.Title = m.t(msgMainTitle)
	m.setHelp(
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", m.t(msgEnterOpen))),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", m.t(msgEscQuit))),
	)
}

func (m *app) loadProviders() {
	var items []list.Item
	ps, err := m.store.ProviderStore()
	n := 0
	if err != nil {
		m.setStatus(statusErr, err.Error())
	} else {
		for _, name := range ps.List() {
			r, err := ps.Get(name)
			if err != nil {
				continue
			}
			n++
			bound := m.toolsBoundTo(name)
			desc := r.Endpoint + " · mid=" + orDash(r.Mid)
			if r.NeedsVerify {
				desc += " · " + m.t(msgProvStale)
			}
			if len(bound) > 0 {
				desc += " · " + m.t(msgProvBound) + ": " + strings.Join(bound, ", ")
			} else {
				desc += " · " + m.t(msgProvUnused)
			}
			items = append(items, item{title: name, desc: desc, value: aProvPrefix + name})
		}
	}
	if len(items) > 0 {
		items = append(items, item{value: sepSentinel})
	}
	items = append(items, item{title: m.t(msgAddProvider), desc: m.t(msgAddProviderDesc), value: aAddSentinel})
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	m.list.Title = m.t(msgProviders)
	m.setHelp(
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", m.t(msgHelpChoose))),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", m.t(msgKeyEdit))),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", m.t(msgKeyDelete))),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", m.t(msgEscBack))),
	)
	if n == 0 && m.status == "" {
		m.setStatus(statusInfo, m.t(msgNoProviders))
	}
}

func (m *app) loadBindings() {
	var items []list.Item
	for _, t := range m.allTools {
		desc := m.t(msgNotInstalled)
		if t.Detected != nil && t.Detected() {
			p := m.store.ActiveProvider(t.Name)
			if p == "" {
				p = "—"
			}
			mid := "—"
			if ps, err := m.store.ProviderStore(); err == nil {
				if r, err := ps.Get(p); err == nil {
					mid = orDash(r.Mid)
					if r.Low != "" || r.High != "" {
						mid = fmt.Sprintf("mid=%s low=%s high=%s", orDash(r.Mid), orDash(r.Low), orDash(r.High))
					}
				}
			}
			desc = "provider: " + p + " · " + mid
		}
		items = append(items, item{title: t.Title, desc: desc, value: aToolPrefix + t.Name})
	}
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	m.list.Title = m.t(msgBindingTitle)
	m.setHelp(
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", m.t(msgHelpChoose))),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", m.t(msgEscBack))),
	)
	if m.status == "" {
		m.setStatus(statusInfo, m.t(msgSelectTool))
	}
}

func (m *app) loadSettings() {
	items := []list.Item{
		item{title: m.t(msgLang), desc: m.t(msgLangDesc) + " · " + m.lang, value: aLangSentinel},
		item{title: m.t(msgSkin), desc: m.t(msgSkinDesc) + " · " + m.store.UISkin(), value: aSkinSentinel},
	}
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	m.list.Title = m.t(msgSettings)
	m.setHelp(
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", m.t(msgHelpChoose))),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", m.t(msgEscBack))),
	)
}

func (m *app) loadLangPicker() {
	items := []list.Item{
		item{title: m.t(msgLangZH), value: profile.LangZH, active: m.lang == profile.LangZH},
		item{title: m.t(msgLangEN), value: profile.LangEN, active: m.lang == profile.LangEN},
	}
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	m.list.Title = m.t(msgLang)
	m.setHelp(keyChoose, keyBack)
}

func (m *app) loadSkinPicker() {
	skin := m.store.UISkin()
	items := []list.Item{
		item{title: m.t(msgSkinTeal), value: profile.SkinTeal, active: skin == profile.SkinTeal},
		item{title: m.t(msgSkinMono), value: profile.SkinMono, active: skin == profile.SkinMono},
		item{title: m.t(msgSkinWarm), value: profile.SkinWarm, active: skin == profile.SkinWarm},
	}
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	m.list.Title = m.t(msgSkin)
	m.setHelp(keyChoose, keyBack)
}

func (m *app) loadWirePicker() {
	items := []list.Item{
		item{title: m.t(msgWireOpenAI), desc: "Bearer · /v1/models", value: aWireOpenAI, active: m.wiz.wire == aWireOpenAI},
		item{title: m.t(msgWireAnthropic), desc: "x-api-key · Messages", value: aWireAnthropic, active: m.wiz.wire == aWireAnthropic},
	}
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	title := m.t(msgWire)
	if m.view == aViewEditWire && m.editName != "" {
		title = m.t(msgEditProvider) + " · " + m.editName + " · " + title
	}
	m.list.Title = title
	m.setHelp(keyChoose, keyBack)
	for i, raw := range items {
		if it, ok := raw.(item); ok && it.value == m.wiz.wire {
			m.list.Select(i)
			break
		}
	}
}

func (m *app) loadProvidersForTool(toolName string) {
	var items []list.Item
	ps, err := m.store.ProviderStore()
	active := m.store.ActiveProvider(toolName)
	if err == nil {
		for _, name := range ps.List() {
			r, err := ps.Get(name)
			if err != nil {
				continue
			}
			title := name
			if name == active {
				title = "✓ " + title
			}
			desc := r.Endpoint + " · mid=" + orDash(r.Mid)
			items = append(items, item{title: title, desc: desc, value: aProvPrefix + name, active: name == active})
		}
	}
	if len(items) == 0 {
		m.setStatus(statusInfo, m.t(msgNoProviders))
	}
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	m.list.Title = fmt.Sprintf(m.t(msgPickProvider), toolName)
	m.setHelp(keyChoose, keyBack)
}

func (m *app) showModelPick(titleID msgID, includeSame bool) {
	current := m.currentTierModel()
	filtered := filterModels(m.allModels, m.modelFilter)
	items := make([]list.Item, 0, len(filtered)+3)
	selIdx := -1
	for _, id := range filtered {
		title, active := markSelected(id, current)
		if active {
			selIdx = len(items)
		}
		items = append(items, item{title: title, value: id, active: active})
	}
	if includeSame {
		if len(items) > 0 {
			items = append(items, item{value: sepSentinel})
		}
		sameActive := current != "" && current == m.mid && (m.view == aViewPickLow || m.view == aViewPickHigh)
		sameTitle := m.t(msgSameAsMid)
		if sameActive {
			sameTitle = "✓ " + sameTitle
			if selIdx < 0 {
				selIdx = len(items)
			}
		}
		items = append(items, item{title: sameTitle, value: aSameAsMid, active: sameActive})
	}
	if m.view == aViewPickProv || false {
		_ = aUseTiers
	}
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	if selIdx >= 0 {
		m.list.Select(selIdx)
	}
	m.list.Title = m.t(titleID)
	if m.modelFilter != "" {
		m.list.Title += " · " + m.modelFilter
	}
	m.setHelp(keyChoose, keyFilter, keyBack)
}

func (m *app) currentTierModel() string {
	switch m.view {
	case aViewPickMid:
		return m.mid
	case aViewPickLow:
		return m.low
	case aViewPickHigh:
		return m.high
	default:
		return ""
	}
}

func (m app) Init() tea.Cmd { return nil }

func (m app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil
	case fetchedMsg:
		if elapsed := time.Since(m.fetchStart); elapsed < minLoadDuration {
			m.pending = &msg
			return m, tea.Tick(minLoadDuration-elapsed, func(time.Time) tea.Msg { return minLoadElapsedMsg{} })
		}
		return m.applyFetched(msg)
	case minLoadElapsedMsg:
		if m.pending == nil {
			return m, nil
		}
		r := *m.pending
		m.pending = nil
		return m.applyFetched(r)
	case busyDoneMsg:
		m.busy = false
		m.status = msg.text
		m.statusLvl = msg.level
		switch msg.kind {
		case busyBind:
			m.view = aViewBindings
			m.toolName, m.provName = "", ""
			m.mid, m.low, m.high = "", "", ""
			m.loadBindings()
		case busyFetchTiers:
			if msg.level == statusErr {
				m.view = aViewPickProv
				m.loadProvidersForTool(m.toolName)
				return m, nil
			}
			return m.openTierPickFromCache()
		case busyEditProv:
			m.editName, m.editOrigKey = "", ""
			m.wiz = providerWiz{}
			m.view = aViewProviders
			m.loadProviders()
		default:
			if m.view == aViewProviders {
				m.loadProviders()
			}
		}
		return m, nil
	case spinner.TickMsg:
		if m.busy || m.view == aViewFetching {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		if m.isInput() {
			return m.updateInput(msg)
		}
		if m.view == aViewConfirmDel {
			return m.updateConfirmDel(msg)
		}
		if m.view == aViewPickModel || m.view == aViewPickMid || m.view == aViewPickLow || m.view == aViewPickHigh {
			return m.updateModelPick(msg)
		}
		switch msg.String() {
		case "esc":
			return m.onEsc()
		case "enter":
			return m.onEnter()
		case "d":
			if m.view == aViewProviders {
				if it, ok := m.list.SelectedItem().(item); ok && strings.HasPrefix(it.value, aProvPrefix) {
					name := strings.TrimPrefix(it.value, aProvPrefix)
					if bound := m.toolsBoundTo(name); len(bound) > 0 {
						m.setStatus(statusErr, fmt.Sprintf(m.t(msgDeleteBlocked), name))
						return m, nil
					}
					m.delTarget = name
					m.view = aViewConfirmDel
					return m, nil
				}
			}
		case "e":
			if m.view == aViewProviders {
				if it, ok := m.list.SelectedItem().(item); ok && strings.HasPrefix(it.value, aProvPrefix) {
					return m.beginEditProvider(strings.TrimPrefix(it.value, aProvPrefix))
				}
			}
		}
	}
	before := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	switch m.view {
	case aViewProviders, aViewPickModel, aViewPickMid, aViewPickLow, aViewPickHigh:
		m.skipSep(before)
	}
	return m, cmd
}

func (m app) onEsc() (tea.Model, tea.Cmd) {
	switch m.view {
	case aViewMenu:
		return m, tea.Quit
	case aViewProviders, aViewBindings, aViewSettings:
		m.view = aViewMenu
		m.clearStatus()
		m.loadMenu()
	case aViewSetLang, aViewSetSkin:
		m.view = aViewSettings
		m.clearStatus()
		m.loadSettings()
	case aViewPickProv:
		m.view = aViewBindings
		m.toolName, m.provName = "", ""
		m.clearStatus()
		m.loadBindings()
	case aViewPickMid, aViewPickLow, aViewPickHigh:
		m.view = aViewPickProv
		m.mid, m.low, m.high = "", "", ""
		m.clearStatus()
		m.loadProvidersForTool(m.toolName)
	case aViewAddWire:
		m.view = aViewAddKey
		m.startInput(m.t(msgAPIKey), true)
		return m, textinput.Blink
	case aViewPickModel:
		m.view = aViewAddName
		m.startInput(m.t(msgName), false)
		return m, textinput.Blink
	case aViewAddEndpoint, aViewAddKey, aViewAddName:
		m.view = aViewProviders
		m.wiz = providerWiz{}
		m.setStatus(statusInfo, m.t(msgCancelled))
		m.loadProviders()
	case aViewEditEndpoint, aViewEditKey, aViewEditWire:
		m.view = aViewProviders
		m.editName, m.editOrigKey = "", ""
		m.wiz = providerWiz{}
		m.setStatus(statusInfo, m.t(msgCancelled))
		m.loadProviders()
	case aViewConfirmDel:
		m.delTarget = ""
		m.view = aViewProviders
		m.loadProviders()
	}
	return m, nil
}

func (m app) onEnter() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok || it.value == sepSentinel {
		return m, nil
	}
	switch m.view {
	case aViewMenu:
		switch it.value {
		case aMenuProv:
			m.view = aViewProviders
			m.clearStatus()
			m.loadProviders()
		case aMenuBind:
			m.view = aViewBindings
			m.clearStatus()
			m.loadBindings()
		case aMenuSet:
			m.view = aViewSettings
			m.clearStatus()
			m.loadSettings()
		}
	case aViewProviders:
		if it.value == aAddSentinel {
			m.wiz = providerWiz{wire: provider.WireOpenAI}
			m.view = aViewAddEndpoint
			m.startInput(exampleEndpoint, false)
			return m, textinput.Blink
		}
		// enter on provider is no-op (list/query); bind is under 工具绑定
		return m, nil
	case aViewBindings:
		if !strings.HasPrefix(it.value, aToolPrefix) {
			return m, nil
		}
		name := strings.TrimPrefix(it.value, aToolPrefix)
		t := m.findTool(name)
		if t == nil || t.ApplyAuth == nil {
			m.setStatus(statusInfo, m.t(msgNoApply))
			return m, nil
		}
		if t.Detected != nil && !t.Detected() {
			m.setStatus(statusInfo, t.Title+" — "+m.t(msgNotInstalled))
			return m, nil
		}
		m.toolName = name
		m.view = aViewPickProv
		m.clearStatus()
		m.loadProvidersForTool(name)
	case aViewPickProv:
		if !strings.HasPrefix(it.value, aProvPrefix) {
			return m, nil
		}
		m.provName = strings.TrimPrefix(it.value, aProvPrefix)
		return m.beginTierPickRefresh()
	case aViewSettings:
		switch it.value {
		case aLangSentinel:
			m.view = aViewSetLang
			m.loadLangPicker()
		case aSkinSentinel:
			m.view = aViewSetSkin
			m.loadSkinPicker()
		}
	case aViewSetLang:
		_ = m.store.SetUILang(it.value)
		m.lang = it.value
		m.view = aViewSettings
		m.loadSettings()
	case aViewSetSkin:
		_ = m.store.SetUISkin(it.value)
		applySkin(it.value)
		m.view = aViewSettings
		m.loadSettings()
		m.list.Styles.Title = titleStyle
	case aViewAddWire:
		m.wiz.wire = it.value
		m.view = aViewAddName
		m.startInput(m.t(msgName), false)
		return m, textinput.Blink
	case aViewEditWire:
		m.wiz.wire = it.value
		return m.finishEdit()
	case aViewPickModel:
		if it.value == skipModel {
			m.wiz.model = ""
		} else {
			m.wiz.model = it.value
		}
		return m.finishAdd(true)
	}
	return m, nil
}

// beginTierPickRefresh probes the provider for a fresh usable catalog, then
// opens mid/low/high pickers. Avoids the stale single-mid usable set left by
// offline add / skip-verify paths.
func (m app) beginTierPickRefresh() (tea.Model, tea.Cmd) {
	ps, err := m.store.ProviderStore()
	if err != nil {
		m.setStatus(statusErr, err.Error())
		return m, nil
	}
	r, err := ps.Get(m.provName)
	if err != nil {
		m.setStatus(statusErr, err.Error())
		return m, nil
	}
	// Multi-model catalog already on disk — skip network (even if marked
	// needsVerify; that flag only means tiers may be stale, not the id list).
	if len(r.Usable) > 1 {
		return m.openTierPick(r)
	}

	provName := m.provName
	store := m.store
	m.busy = true
	m.spinner = newSpinner()
	m.setStatus(statusInfo, m.t(msgFetching))
	return m, tea.Batch(m.spinner.Tick, runBusy(func() busyDoneMsg {
		ps, err := store.ProviderStore()
		if err != nil {
			return busyDoneMsg{level: statusErr, text: err.Error(), kind: busyFetchTiers}
		}
		// Always re-list + probe so mid/low/high show the full reachable set.
		refreshed, err := ps.Refresh(provName, provider.UpsertOptions{})
		if err != nil {
			// Fall back to whatever is cached so the user can still bind.
			cur, gerr := ps.Get(provName)
			if gerr != nil || len(cur.Usable) == 0 {
				return busyDoneMsg{level: statusErr, text: err.Error(), kind: busyFetchTiers}
			}
			return busyDoneMsg{
				level: statusInfo,
				text:  fmt.Sprintf("%s · %v", tr(store.UILang(), msgProvStale), err),
				kind:  busyFetchTiers,
			}
		}
		_ = refreshed
		return busyDoneMsg{
			level: statusOK,
			text:  tr(store.UILang(), msgFetching),
			kind:  busyFetchTiers,
		}
	}))
}

func (m app) openTierPickFromCache() (tea.Model, tea.Cmd) {
	ps, err := m.store.ProviderStore()
	if err != nil {
		m.setStatus(statusErr, err.Error())
		return m, nil
	}
	r, err := ps.Get(m.provName)
	if err != nil {
		m.setStatus(statusErr, err.Error())
		return m, nil
	}
	return m.openTierPick(r)
}

func (m app) openTierPick(r provider.Record) (tea.Model, tea.Cmd) {
	m.allModels = append([]string{}, r.Usable...)
	if len(m.allModels) == 0 {
		for _, id := range []string{r.Mid, r.Low, r.High} {
			if id != "" {
				m.allModels = append(m.allModels, id)
			}
		}
		if len(m.allModels) == 0 {
			m.allModels = []string{"default"}
		}
	}
	m.modelFilter = ""
	m.mid, m.low, m.high = r.Mid, r.Low, r.High
	m.view = aViewPickMid
	m.showModelPick(msgPickMid, false)
	return m, nil
}

func (m app) updateModelPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.onEsc()
	case "enter":
		it, ok := m.list.SelectedItem().(item)
		if !ok || it.value == sepSentinel {
			return m, nil
		}
		return m.commitModelPick(it.value)
	case "backspace":
		if m.modelFilter != "" {
			r := []rune(m.modelFilter)
			m.modelFilter = string(r[:len(r)-1])
			m.rebuildModelList()
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.modelFilter += string(msg.Runes)
		m.rebuildModelList()
		return m, nil
	}
	before := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.skipSep(before)
	return m, cmd
}

func (m *app) rebuildModelList() {
	switch m.view {
	case aViewPickModel:
		m.showProvAddModels()
	case aViewPickMid:
		m.showModelPick(msgPickMid, false)
	case aViewPickLow, aViewPickHigh:
		id := msgPickLow
		if m.view == aViewPickHigh {
			id = msgPickHigh
		}
		m.showModelPick(id, true)
	}
}

func (m app) commitModelPick(val string) (tea.Model, tea.Cmd) {
	switch m.view {
	case aViewPickModel:
		if val == skipModel {
			m.wiz.model = ""
		} else {
			m.wiz.model = val
		}
		return m.finishAdd(true)
	case aViewPickMid:
		m.mid = val
		m.modelFilter = ""
		m.view = aViewPickLow
		m.showModelPick(msgPickLow, true)
	case aViewPickLow:
		if val == aSameAsMid {
			m.low = m.mid
		} else {
			m.low = val
		}
		m.modelFilter = ""
		m.view = aViewPickHigh
		m.showModelPick(msgPickHigh, true)
	case aViewPickHigh:
		if val == aSameAsMid {
			m.high = m.mid
		} else {
			m.high = val
		}
		return m.applyBindWithTiers()
	}
	return m, nil
}

func (m app) applyBindWithTiers() (tea.Model, tea.Cmd) {
	tool := m.findTool(m.toolName)
	if tool == nil {
		return m, nil
	}
	provName := m.provName
	mid, low, high := m.mid, m.low, m.high
	store := m.store
	m.busy = true
	m.spinner = newSpinner()
	m.setStatus(statusInfo, m.t(msgBusyBind))
	return m, runBusy(func() busyDoneMsg {
		ps, err := store.ProviderStore()
		if err != nil {
			return busyDoneMsg{level: statusErr, text: err.Error(), kind: busyBind}
		}
		if _, err := ps.SetTiers(provName, mid, low, high, provider.UpsertOptions{SkipVerify: true}); err != nil {
			return busyDoneMsg{level: statusErr, text: err.Error(), kind: busyBind}
		}
		if err := store.ApplyProvider(tool, provName, false); err != nil {
			return busyDoneMsg{level: statusErr, text: err.Error(), kind: busyBind}
		}
		return busyDoneMsg{
			level: statusOK,
			text:  fmt.Sprintf(tr(store.UILang(), msgBoundOK), tool.Name, provName),
			kind:  busyBind,
		}
	})
}

func (m app) beginEditProvider(name string) (tea.Model, tea.Cmd) {
	ps, err := m.store.ProviderStore()
	if err != nil {
		m.setStatus(statusErr, err.Error())
		return m, nil
	}
	r, err := ps.Get(name)
	if err != nil {
		m.setStatus(statusErr, err.Error())
		return m, nil
	}
	m.editName = name
	m.editOrigKey = r.Key
	m.wiz = providerWiz{
		endpoint: r.Endpoint,
		key:      r.Key,
		wire:     r.Wire,
		name:     name,
		model:    r.Mid,
	}
	m.view = aViewEditEndpoint
	m.clearStatus()
	m.startInputValue(m.t(msgEndpoint), r.Endpoint, false)
	return m, textinput.Blink
}

func (m app) finishEdit() (tea.Model, tea.Cmd) {
	name := m.editName
	endpoint := m.wiz.endpoint
	key := m.wiz.key
	wire := m.wiz.wire
	model := m.wiz.model
	store := m.store
	toolsAll := m.allTools
	m.busy = true
	m.spinner = newSpinner()
	m.setStatus(statusInfo, m.t(msgBusyEdit))
	return m, runBusy(func() busyDoneMsg {
		ps, err := store.ProviderStore()
		if err != nil {
			return busyDoneMsg{level: statusErr, text: err.Error(), kind: busyEditProv}
		}
		r, err := ps.Upsert(provider.Spec{
			Name: name, Endpoint: endpoint, Key: key, Wire: wire, Model: model,
		}, provider.UpsertOptions{})
		if err != nil {
			return busyDoneMsg{level: statusErr, text: err.Error(), kind: busyEditProv}
		}
		var applied []string
		for _, t := range toolsAll {
			if store.ActiveProvider(t.Name) != name {
				continue
			}
			if t.ApplyAuth == nil {
				continue
			}
			if err := store.ApplyProvider(t, name, false); err != nil {
				return busyDoneMsg{
					level: statusErr,
					text:  fmt.Sprintf("%s: %v", t.Name, err),
					kind:  busyEditProv,
				}
			}
			applied = append(applied, t.Name)
		}
		text := fmt.Sprintf(tr(store.UILang(), msgEdited), r.Name)
		if len(applied) > 0 {
			text += " · " + strings.Join(applied, ", ")
		}
		text += fmt.Sprintf(" · usable=%d", len(r.Usable))
		return busyDoneMsg{level: statusOK, text: text, kind: busyEditProv}
	})
}

func (m app) updateConfirmDel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		name := m.delTarget
		if bound := m.toolsBoundTo(name); len(bound) > 0 {
			m.setStatus(statusErr, fmt.Sprintf(m.t(msgDeleteBlocked), name))
		} else if ps, err := m.store.ProviderStore(); err != nil {
			m.setStatus(statusErr, err.Error())
		} else if err := ps.Remove(name); err != nil {
			m.setStatus(statusErr, err.Error())
		} else {
			m.setStatus(statusOK, fmt.Sprintf(m.t(msgDeleted), name))
		}
		m.delTarget = ""
		m.view = aViewProviders
		m.loadProviders()
	case "n", "N", "esc":
		m.delTarget = ""
		m.view = aViewProviders
		m.setStatus(statusInfo, m.t(msgCancelled))
		m.loadProviders()
	}
	return m, nil
}

func (m app) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m.onEsc()
	case tea.KeyEnter:
		return m.commitInput()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m app) commitInput() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(m.input.Value())
	switch m.view {
	case aViewAddEndpoint:
		if err := tools.ValidateEndpoint(val); err != nil {
			m.setStatus(statusErr, err.Error())
			return m, nil
		}
		m.wiz.endpoint = val
		m.view = aViewAddKey
		m.clearStatus()
		m.startInput(m.t(msgAPIKey), true)
		return m, textinput.Blink
	case aViewAddKey:
		if err := tools.ValidateKey(val); err != nil {
			m.setStatus(statusErr, err.Error())
			return m, nil
		}
		m.wiz.key = val
		m.view = aViewAddWire
		m.clearStatus()
		m.loadWirePicker()
		return m, nil
	case aViewAddName:
		if val == "" {
			m.setStatus(statusErr, "name is required")
			return m, nil
		}
		if val == profile.DefaultName {
			m.setStatus(statusErr, fmt.Sprintf("%q is reserved", profile.DefaultName))
			return m, nil
		}
		m.wiz.name = val
		return m, m.beginFetch()
	case aViewEditEndpoint:
		if err := tools.ValidateEndpoint(val); err != nil {
			m.setStatus(statusErr, err.Error())
			return m, nil
		}
		m.wiz.endpoint = val
		m.view = aViewEditKey
		m.clearStatus()
		ph := m.t(msgEditKeepKey)
		if m.editOrigKey != "" {
			ph = secret.Mask(m.editOrigKey) + " · " + m.t(msgEditKeepKey)
		}
		m.startInput(ph, true)
		return m, textinput.Blink
	case aViewEditKey:
		if val == "" {
			m.wiz.key = m.editOrigKey
		} else {
			if err := tools.ValidateKey(val); err != nil {
				m.setStatus(statusErr, err.Error())
				return m, nil
			}
			m.wiz.key = val
		}
		m.view = aViewEditWire
		m.clearStatus()
		m.loadWirePicker()
		for i, raw := range m.list.Items() {
			if it, ok := raw.(item); ok && it.value == m.wiz.wire {
				m.list.Select(i)
				break
			}
		}
		return m, nil
	}
	return m, nil
}

func (m *app) beginFetch() tea.Cmd {
	m.view = aViewFetching
	m.fetchStart = time.Now()
	m.pending = nil
	m.loadingMsg = m.t(msgFetching)
	m.spinner = newSpinner()
	wire := m.wiz.wire
	if wire == "" {
		wire = provider.WireOpenAI
	}
	return tea.Batch(m.spinner.Tick, fetchModelsCmd(wire, m.wiz.endpoint, m.wiz.key))
}

func (m app) applyFetched(msg fetchedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || len(msg.list) == 0 {
		m.wiz.model = ""
		return m.finishAdd(true)
	}
	m.view = aViewPickModel
	m.allModels = msg.list
	m.modelFilter = ""
	m.showProvAddModels()
	return m, nil
}

func (m *app) showProvAddModels() {
	filtered := filterModels(m.allModels, m.modelFilter)
	items := make([]list.Item, 0, len(filtered)+2)
	for _, id := range filtered {
		items = append(items, item{title: id, value: id})
	}
	items = append(items, item{value: sepSentinel})
	items = append(items, item{title: "(skip)", value: skipModel})
	m.list.SetDelegate(themedDelegate())
	m.list.SetItems(items)
	m.list.Title = m.t(msgPickMid)
	m.setHelp(keyChoose, keyFilter, keyBack)
}

func (m app) finishAdd(skipVerify bool) (tea.Model, tea.Cmd) {
	ps, err := m.store.ProviderStore()
	if err != nil {
		m.setStatus(statusErr, err.Error())
		m.view = aViewProviders
		m.loadProviders()
		return m, nil
	}
	// Prefer the full catalog from the just-completed fetch (m.allModels).
	// Without it, SkipVerify would store only the chosen mid and bind would
	// show a single-model list.
	var catalog []string
	if len(m.allModels) > 0 {
		catalog = append([]string{}, m.allModels...)
		skipVerify = true // already probed via fetchModelsCmd
	}
	r, err := ps.Upsert(provider.Spec{
		Name: m.wiz.name, Endpoint: m.wiz.endpoint, Key: m.wiz.key,
		Wire: m.wiz.wire, Model: m.wiz.model, Usable: catalog, SkipVerify: skipVerify,
	}, provider.UpsertOptions{SkipVerify: skipVerify})
	if err != nil {
		m.setStatus(statusErr, err.Error())
		m.view = aViewProviders
		m.loadProviders()
		return m, nil
	}
	msg := fmt.Sprintf("%s · mid=%s · %d models", r.Name, orDash(r.Mid), len(r.Usable))
	if r.NeedsVerify {
		msg += " · " + m.t(msgProvStale)
	}
	m.setStatus(statusOK, msg)
	m.wiz = providerWiz{}
	m.allModels = nil
	m.view = aViewProviders
	m.loadProviders()
	return m, nil
}

func (m app) View() string {
	switch m.view {
	case aViewConfirmDel:
		body := "\n" + titleStyle.Render(m.t(msgProviders)) +
			"\n\n" + warnStyle.Render(fmt.Sprintf(m.t(msgDeleteConfirm), m.delTarget)) +
			"\n\n" + hintStyle.Render("y · n / esc")
		return body
	case aViewFetching:
		return "\n" + titleStyle.Render(m.t(msgAddProvider)) + "\n\n" +
			promptStyle.Render(m.spinner.View()+m.loadingMsg)
	case aViewAddEndpoint, aViewAddKey, aViewAddName:
		step := 1
		label := m.t(msgEndpoint)
		switch m.view {
		case aViewAddKey:
			step, label = 2, m.t(msgAPIKey)
		case aViewAddName:
			step, label = 4, m.t(msgName)
		}
		body := "\n" + titleStyle.Render(m.t(msgAddProvider)) + "\n" +
			stepStyle.Render(fmt.Sprintf(m.t(msgStepOf), step, 5, label)) + "\n\n" +
			promptStyle.Render(label) + "\n\n  " + m.input.View() +
			"\n\n" + hintStyle.Render(m.t(msgEnterContinue)+" · "+m.t(msgEscBack))
		if line := statusRender(m.statusLvl, m.status); line != "" {
			body += "\n" + line
		}
		return body
	case aViewEditEndpoint, aViewEditKey:
		step := 1
		label := m.t(msgEndpoint)
		if m.view == aViewEditKey {
			step, label = 2, m.t(msgAPIKey)
		}
		title := m.t(msgEditProvider)
		if m.editName != "" {
			title += " · " + m.editName
		}
		body := "\n" + titleStyle.Render(title) + "\n" +
			stepStyle.Render(fmt.Sprintf(m.t(msgStepOf), step, 3, label)) + "\n\n" +
			promptStyle.Render(label) + "\n\n  " + m.input.View() +
			"\n\n" + hintStyle.Render(m.t(msgEnterContinue)+" · "+m.t(msgEscBack))
		if line := statusRender(m.statusLvl, m.status); line != "" {
			body += "\n" + line
		}
		return body
	}
	out := m.list.View()
	if m.view == aViewMenu {
		out = banner(m.version) + "\n\n" + out
	}
	if m.busy {
		out += "\n" + promptStyle.Render(m.spinner.View()+m.status)
	} else if line := statusRender(m.statusLvl, m.status); line != "" {
		out += "\n" + line
	}
	return out
}
