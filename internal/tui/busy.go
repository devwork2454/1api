package tui

import tea "github.com/charmbracelet/bubbletea"

type busyDoneMsg struct {
	level statusLevel
	text  string
	kind  busyKind
}

type busyKind int

const (
	busyNone busyKind = iota
	busyVerify
	busyBind
	busyBindMulti
	// busyFetchTiers: bind flow finished refreshing usable catalog.
	busyFetchTiers
	// busyEditProv: provider edit finished (upsert + optional re-apply).
	busyEditProv
)

func runBusy(fn func() busyDoneMsg) tea.Cmd {
	return func() tea.Msg {
		return fn()
	}
}
