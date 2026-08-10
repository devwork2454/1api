package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

type statusLevel int

const (
	statusInfo statusLevel = iota
	statusOK
	statusErr
)

const (
	skipModel       = "\x00nomodel"
	sepSentinel     = "\x00sep"
	exampleEndpoint = "https://api.example.com/v1"
)

type item struct {
	title, desc string
	value       string
	active      bool
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

var (
	keyBack   = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyChoose = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose"))
	keyFilter = key.NewBinding(key.WithKeys("\x00filter"), key.WithHelp("type", "search"))
)

type providerWiz struct {
	endpoint, key, wire, name, model string
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
