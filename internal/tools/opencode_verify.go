package tools

import (
	"fmt"
	"strings"
	"time"

	"1api/internal/models"
)

// VerifyOpenCodeAuth checks endpoint+key connectivity, intersects candidates with
// the live catalog, classifies mid/low/high, and smoke-tests chat on the mid model.
// Empty key skips the network and returns name-only tiers from candidates.
func VerifyOpenCodeAuth(endpoint, key, primary string, candidates []string) (tiers opencodeTierModels, reachable []string, err error) {
	primary = strings.TrimSpace(primary)
	candidates = cleanModelIDs(candidates)

	if strings.TrimSpace(key) == "" {
		return resolveOpenCodeTiers(primary, candidates), candidates, nil
	}

	list, err := models.Probe(models.OpenAI, endpoint, key, models.ProbeOptions{
		Timeout:  25 * time.Second,
		SkipChat: true,
	})
	if err != nil {
		return opencodeTierModels{}, nil, err
	}
	reachable = list.Models
	if len(candidates) > 0 {
		reachable = intersectModels(candidates, list.Models)
		if len(reachable) == 0 {
			return opencodeTierModels{}, list.Models, fmt.Errorf(
				"none of the selected models are on the live endpoint (live has %d)", len(list.Models))
		}
	}
	if primary != "" && !containsString(reachable, primary) {
		if containsString(list.Models, primary) {
			reachable = append([]string{primary}, reachable...)
		} else {
			return opencodeTierModels{}, reachable, fmt.Errorf(
				"primary model %q not offered by endpoint", primary)
		}
	}
	if primary == "" && len(reachable) > 0 {
		primary = resolveOpenCodeTiers("", reachable).Mid
	}
	tiers = resolveOpenCodeTiers(primary, reachable)
	if tiers.Mid == "" {
		return tiers, reachable, fmt.Errorf("could not resolve a mid model from live list")
	}
	if _, err := models.Probe(models.OpenAI, endpoint, key, models.ProbeOptions{
		Timeout:   25 * time.Second,
		ChatModel: tiers.Mid,
	}); err != nil {
		return tiers, reachable, fmt.Errorf("mid model %q not usable: %w", tiers.Mid, err)
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

func intersectModels(want, live []string) []string {
	set := map[string]struct{}{}
	for _, id := range live {
		set[id] = struct{}{}
	}
	var out []string
	for _, id := range want {
		if _, ok := set[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
