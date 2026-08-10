package tools

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"1api/internal/models"
)

// VerifyOpenCodeAuth lists the endpoint, keeps only chat-reachable models,
// classifies mid/low/high from that set, and requires at least one usable model.
// Empty key skips the network and returns name-only tiers from candidates.
func VerifyOpenCodeAuth(endpoint, key, primary string, candidates []string) (tiers models.Tiers, reachable []string, err error) {
	primary = strings.TrimSpace(primary)
	candidates = cleanModelIDs(candidates)

	if strings.TrimSpace(key) == "" {
		return models.ResolveTiers(primary, candidates), candidates, nil
	}

	reachable, err = models.FilterReachable(models.OpenAI, endpoint, key, models.FilterOptions{
		Timeout:         90 * time.Second,
		PerModelTimeout: 12 * time.Second,
		Concurrency:     6,
		Candidates:      candidates,
	})
	if err != nil {
		if errors.Is(err, models.ErrNoUsableModels) {
			return models.Tiers{}, nil, models.ErrNoUsableModels
		}
		return models.Tiers{}, nil, err
	}
	if primary != "" && !containsString(reachable, primary) {
		return models.Tiers{}, reachable, fmt.Errorf(
			"primary model %q is not usable (not listed or chat failed); usable: %d", primary, len(reachable))
	}
	if primary == "" {
		primary = models.ResolveTiers("", reachable).Mid
	}
	tiers = models.ResolveTiers(primary, reachable)
	if tiers.Mid == "" {
		return tiers, reachable, models.ErrNoUsableModels
	}
	return tiers, reachable, nil
}

func cleanModelIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
