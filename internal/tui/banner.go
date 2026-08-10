package tui

import "github.com/charmbracelet/lipgloss"

// bannerHeight is rows reserved above the main-menu list (title line + blank).
const bannerHeight = 2

var bannerStyle = lipgloss.NewStyle().Foreground(colorBrand).Bold(true)

// banner returns a plain product name line for the main menu.
func banner(version string) string {
	line := "ONE API"
	if version != "" {
		line += "  ·  v" + version
	}
	return bannerStyle.Render(line) + "  " +
		hintStyle.Render("· ↑/↓ move · ? help · ctrl+c quit")
}
