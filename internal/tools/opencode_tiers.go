package tools

import (
	"strings"

	"1api/internal/models"
)

// opencodeTier is a model role used when wiring OpenCode defaults and agents.
type opencodeTier string

const (
	tierLow  opencodeTier = "low"
	tierMid  opencodeTier = "mid"
	tierHigh opencodeTier = "high"
)

// opencodeTierModels is the resolved mid/low/high model ids (no "1api/" prefix).
type opencodeTierModels struct {
	Mid  string
	Low  string
	High string
}

// resolveOpenCodeTiers picks mid/low/high from primary + available ids.
func resolveOpenCodeTiers(primary string, available []string) opencodeTierModels {
	t := models.ResolveTiers(primary, available)
	return opencodeTierModels{Mid: t.Mid, Low: t.Low, High: t.High}
}

// modelNameTierHint maps common model-id tokens to low/mid/high; empty = unknown.
func modelNameTierHint(id string) opencodeTier {
	return opencodeTier(models.NameTierClass(id))
}

// agentTierClassifies an OpenCode agent name into low/mid/high.
// Names mirror common OpenCode + oh-my agent roles; unknown → mid.
func agentTierClass(name string) opencodeTier {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "":
		return tierMid
	case containsAny(n, "compaction", "explore", "librarian", "title", "summary", "small", "quick", "fast"):
		return tierLow
	case containsAny(n, "plan", "oracle", "architect", "review", "research", "deep", "ultrabrain",
		"prometheus", "momus", "metis", "sisyphus", "critique", "security"):
		return tierHigh
	default:
		return tierMid
	}
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func (t opencodeTierModels) forTier(tier opencodeTier) string {
	switch tier {
	case tierLow:
		return t.Low
	case tierHigh:
		return t.High
	default:
		return t.Mid
	}
}

// applyOpenCodeTierRouting writes model, small_model, and agent/agents model fields
// using mid/low/high. Only rewrites agent models that are empty or already 1api/*.
// Ensures agent.compaction exists when a low model is available.
func applyOpenCodeTierRouting(cfg map[string]any, tiers opencodeTierModels) {
	if tiers.Mid != "" {
		cfg["model"] = "1api/" + tiers.Mid
	}
	if tiers.Low != "" {
		cfg["small_model"] = "1api/" + tiers.Low
	}

	for _, key := range []string{"agent", "agents"} {
		raw, ok := cfg[key]
		if !ok || raw == nil {
			continue
		}
		agents, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for name, v := range agents {
			entry, ok := v.(map[string]any)
			if !ok {
				continue
			}
			cur, _ := entry["model"].(string)
			if cur != "" && !strings.HasPrefix(cur, "1api/") {
				continue // leave non-1api models alone
			}
			id := tiers.forTier(agentTierClass(name))
			if id == "" {
				continue
			}
			entry["model"] = "1api/" + id
			agents[name] = entry
		}
		cfg[key] = agents
	}

	// Ensure compaction is wired to low (OpenCode uses this for session compaction).
	if tiers.Low == "" {
		return
	}
	agents := subMap(cfg, "agent")
	comp, _ := agents["compaction"].(map[string]any)
	if comp == nil {
		comp = map[string]any{}
	}
	cur, _ := comp["model"].(string)
	if cur == "" || strings.HasPrefix(cur, "1api/") {
		comp["model"] = "1api/" + tiers.Low
		agents["compaction"] = comp
	}
}
