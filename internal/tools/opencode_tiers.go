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

// opencodeTierModels is the resolved mid/low/high model ids (no "1api/" prefix).
type opencodeTierModels struct {
	Mid  string
	Low  string
	High string
}

// resolveOpenCodeTiers picks mid/low/high from primary + available ids:
// exact mid/low/high → name capability hints → token contains → primary fallback.
func resolveOpenCodeTiers(primary string, available []string) opencodeTierModels {
	primary = strings.TrimSpace(primary)
	var ids []string
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, id := range available {
		add(id)
	}
	add(primary)

	pickExact := func(name string) string {
		if _, ok := seen[name]; ok {
			return name
		}
		return ""
	}
	pickContains := func(token string) string {
		token = strings.ToLower(token)
		var hit string
		for _, id := range ids {
			if strings.Contains(strings.ToLower(id), token) {
				if hit == "" || len(id) < len(hit) {
					hit = id
				}
			}
		}
		return hit
	}
	pickClass := func(want opencodeTier) string {
		var hit string
		for _, id := range ids {
			if modelNameTierHint(id) != want {
				continue
			}
			if hit == "" || len(id) < len(hit) {
				hit = id
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
	if mid == "" {
		for _, id := range ids {
			h := modelNameTierHint(id)
			if h == tierMid || h == "" {
				mid = id
				break
			}
		}
	}
	if mid == "" && len(ids) > 0 {
		mid = ids[0]
	}

	low := pickExact("low")
	if low == "" {
		low = pickClass(tierLow)
	}
	if low == "" {
		low = pickContains("low")
	}
	if low == "" {
		low = mid
	}

	high := pickExact("high")
	if high == "" {
		high = pickClass(tierHigh)
	}
	if high == "" {
		high = pickContains("high")
	}
	if high == "" {
		high = mid
	}

	return opencodeTierModels{Mid: mid, Low: low, High: high}
}

// modelNameTierHint maps common model-id tokens to low/mid/high; empty = unknown.
func modelNameTierHint(id string) opencodeTier {
	n := strings.ToLower(strings.TrimSpace(id))
	if n == "" {
		return ""
	}
	switch n {
	case "low", "small", "fast":
		return tierLow
	case "high", "large":
		return tierHigh
	case "mid", "medium", "default":
		return tierMid
	}
	lowTokens := []string{
		"flash", "mini", "nano", "lite", "haiku", "small", "fast", "8b", "7b", "9b",
		"tiny", "cheap", "light",
	}
	highTokens := []string{
		"opus", "ultra", "reasoning", "thinking", "heavy", "max", "pro", "o1", "o3",
		"r1", "reasoner", "premier", "sonnet-4", "gpt-5", "claude-4",
	}
	if containsAny(n, "mini", "nano", "flash", "haiku", "lite") {
		return tierLow
	}
	for _, t := range highTokens {
		if strings.Contains(n, t) {
			return tierHigh
		}
	}
	for _, t := range lowTokens {
		if strings.Contains(n, t) {
			return tierLow
		}
	}
	return ""
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
