package tui

import (
	"strings"
	"time"

	"1api/internal/models"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
)

const minLoadDuration = 1 * time.Second

type fetchedMsg struct {
	list []string
	err  error
}

type minLoadElapsedMsg struct{}

func fetchModelsCmd(provider, endpoint, key string) tea.Cmd {
	return func() tea.Msg {
		l, err := models.FilterReachable(models.Provider(provider), endpoint, key, models.FilterOptions{})
		return fetchedMsg{list: l, err: err}
	}
}

func filterModels(all []string, query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return all
	}
	matches := fuzzy.Find(q, all)
	out := make([]string, len(matches))
	for i, mt := range matches {
		out[i] = mt.Str
	}
	return out
}
