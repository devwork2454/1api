package tools

import (
	"strings"
)

// opencodeTier is a model role used when wiring OpenCode defaults and agents.
type opencodeTier string

const (
	tierLow  opencodeTier = "low"
	tierMid  opencodeTier = "mid"
	tierHigh opencodeTier = "high"
)

// opencodeTierModels is the resolved mid/low/high model ids (no "charon/" prefix).
type opencodeTierModels struct {
	Mid  string
	Low  string
	High string
}

// resolveOpenCodeTiers picks mid/low/high from the primary model and the available
// model id list. Preference order for each slot:
//  1. exact id "mid"/"low"/"high"
//  2. id equal to primary for mid; id containing the tier name
//  3. fall back to primary (so a single-model proxy still works)
func resolveOpenCodeTiers(primary string, available []string) opencodeTierModels {
	primary = strings.TrimSpace(primary)
	set := map[string]struct{}{}
	for _, id := range available {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	if primary != "" {
		set[primary] = struct{}{}
	}

	pickExact := func(name string) string {
		if _, ok := set[name]; ok {
			return name
		}
		return ""
	}
	pickContains := func(token string) string {
		token = strings.ToLower(token)
		var hit string
		for id := range set {
			if strings.Contains(strings.ToLower(id), token) {
				if hit == "" || len(id) < len(hit) {
					hit = id
				}
			}
		}
		return hit
	}

	mid := pickExact("mid")
	if mid == "" && primary != "" {
		mid = primary
	}
	if mid == "" {
		mid = pickContains("mid")
	}

	low := pickExact("low")
	if low == "" {
		low = pickContains("low")
	}
	if low == "" {
		low = mid
	}

	high := pickExact("high")
	if high == "" {
		high = pickContains("high")
	}
	if high == "" {
		high = mid
	}

	return opencodeTierModels{Mid: mid, Low: low, High: high}
}

// agentTierClassifies an OpenCode agent name into low/mid/high.
// Names mirror common OpenCode + oh-my agent roles; unknown → mid.
func agentTierClass(name string) opencodeTier {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "":
		return tierMid
	case containsAny(n, "compaction", "explore", "librarian", "title", "summary", "small", "quick", "fast",
		"writing", "unspecified-low") || strings.HasSuffix(n, "-low"):
		return tierLow
	case containsAny(n, "plan", "oracle", "architect", "review", "research", "deep", "ultrabrain",
		"prometheus", "momus", "metis", "sisyphus", "critique", "security", "artistry",
		"unspecified-high") || strings.HasSuffix(n, "-high"):
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
// using mid/low/high. Only rewrites agent models that are empty or already charon/*.
// Ensures agent.compaction exists when a low model is available.
func applyOpenCodeTierRouting(cfg map[string]any, tiers opencodeTierModels) {
	if tiers.Mid != "" {
		cfg["model"] = "charon/" + tiers.Mid
	}
	if tiers.Low != "" {
		cfg["small_model"] = "charon/" + tiers.Low
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
			if cur != "" && !strings.HasPrefix(cur, "charon/") {
				continue // leave non-charon models alone
			}
			id := tiers.forTier(agentTierClass(name))
			if id == "" {
				continue
			}
			entry["model"] = "charon/" + id
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
	if cur == "" || strings.HasPrefix(cur, "charon/") {
		comp["model"] = "charon/" + tiers.Low
		agents["compaction"] = comp
	}
}
