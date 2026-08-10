package tui

import (
	"1api/internal/profile"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Run starts the unified interactive menu (providers / bindings / settings).
func Run(store *profile.Store, version string) error {
	_, err := tea.NewProgram(newApp(store, version), tea.WithAltScreen()).Run()
	return err
}

func newBaseList() list.Model {
	l := list.New(nil, themedDelegate(), 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.InfiniteScrolling = true
	l.KeyMap.Quit.SetEnabled(false)
	l.Styles.Title = titleStyle
	l.Styles.TitleBar = l.Styles.TitleBar.Padding(0, 0, 1, 0)
	l.Styles.HelpStyle = l.Styles.HelpStyle.Foreground(colorMuted)
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Foreground(colorMuted)
	l.Styles.ArabicPagination = lipgloss.NewStyle().Foreground(colorMuted)
	l.Styles.NoItems = lipgloss.NewStyle().Foreground(colorMuted)
	l.Help.Styles.ShortKey = l.Help.Styles.ShortKey.Foreground(colorAccent)
	l.Help.Styles.ShortDesc = l.Help.Styles.ShortDesc.Foreground(colorMuted)
	l.Help.Styles.ShortSeparator = l.Help.Styles.ShortSeparator.Foreground(colorMuted)
	l.Help.Styles.FullKey = l.Help.Styles.FullKey.Foreground(colorAccent)
	l.Help.Styles.FullDesc = l.Help.Styles.FullDesc.Foreground(colorMuted)
	l.Help.Styles.FullSeparator = l.Help.Styles.FullSeparator.Foreground(colorMuted)
	l.Help.Styles.Ellipsis = l.Help.Styles.Ellipsis.Foreground(colorMuted)
	return l
}
