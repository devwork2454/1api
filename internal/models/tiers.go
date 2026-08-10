package models

import "strings"

// Tiers is the resolved mid/low/high model ids (no provider prefix).
type Tiers struct {
	Mid  string
	Low  string
	High string
}

// ResolveTiers picks mid/low/high from primary + available ids:
// exact mid/low/high → name capability hints → token contains → primary fallback.
func ResolveTiers(primary string, available []string) Tiers {
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
	pickClass := func(want tierClass) string {
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

	return Tiers{Mid: mid, Low: low, High: high}
}

type tierClass string

const (
	tierLow  tierClass = "low"
	tierMid  tierClass = "mid"
	tierHigh tierClass = "high"
)

// NameTierClass maps common model-id tokens to "low"/"mid"/"high"; empty = unknown.
func NameTierClass(id string) string {
	return string(modelNameTierHint(id))
}

// modelNameTierHint maps common model-id tokens to low/mid/high; empty = unknown.
func modelNameTierHint(id string) tierClass {
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

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
