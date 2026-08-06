package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// omoConfigPath returns the existing OMO config under ~/.omo, or "" if absent.
// Detection is file-based: missing oh-my-openagent means we skip companion sync.
func omoConfigPath() string {
	dir := filepath.Join(home(), ".omo")
	for _, name := range []string{"omo.jsonc", "omo.json"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// SyncOMOFromOpenCodeLive reads the live OpenCode config, resolves mid/low/high,
// and rewrites OMO agent/category models to charon/<id>. No-op when OMO is not
// installed or OpenCode has no usable charon models.
func SyncOMOFromOpenCodeLive() error {
	path := omoConfigPath()
	if path == "" {
		return nil
	}
	cfg, err := loadJSONMap(opencodeConfigPath())
	if err != nil {
		return fmt.Errorf("omo sync: read opencode config: %w", err)
	}
	tiers, ok := tiersFromOpenCodeConfig(cfg)
	if !ok {
		return nil
	}
	return syncOMOTiers(tiers)
}

// syncOMOTiers updates ~/.omo agents/categories model fields from resolved tiers.
// Skips entirely when OMO is not installed.
func syncOMOTiers(tiers opencodeTierModels) error {
	path := omoConfigPath()
	if path == "" {
		return nil
	}
	if tiers.Mid == "" && tiers.Low == "" && tiers.High == "" {
		return nil
	}
	cfg, err := loadJSONMap(path)
	if err != nil {
		return fmt.Errorf("omo sync: read %s: %w", path, err)
	}
	section := omoOpenCodeSection(cfg)
	if section == nil {
		return nil
	}
	applyOMOTierRouting(section, tiers)
	if err := writeJSONMap(path, cfg, 0o600); err != nil {
		return fmt.Errorf("omo sync: write %s: %w", path, err)
	}
	return nil
}

// omoOpenCodeSection returns the map that holds agents/categories.
// Prefer the harness block "[opencode]"; fall back to a flat top-level layout.
func omoOpenCodeSection(cfg map[string]any) map[string]any {
	if sec, ok := cfg["[opencode]"].(map[string]any); ok {
		return sec
	}
	_, hasAgents := cfg["agents"]
	_, hasCategories := cfg["categories"]
	if hasAgents || hasCategories {
		return cfg
	}
	return nil
}

// applyOMOTierRouting sets model on every agent/category entry to charon/<tier-id>.
// Preserves all other keys (skills, prompt_append, …).
func applyOMOTierRouting(section map[string]any, tiers opencodeTierModels) {
	for _, key := range []string{"agents", "categories"} {
		raw, ok := section[key]
		if !ok || raw == nil {
			continue
		}
		entries, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for name, v := range entries {
			entry, ok := v.(map[string]any)
			if !ok {
				continue
			}
			id := tiers.forTier(agentTierClass(name))
			if id == "" {
				continue
			}
			entry["model"] = "charon/" + id
			entries[name] = entry
		}
		section[key] = entries
	}
}

// tiersFromOpenCodeConfig derives mid/low/high from a live OpenCode config map.
// ok is false when there is no primary model and no charon model list to use.
func tiersFromOpenCodeConfig(cfg map[string]any) (opencodeTierModels, bool) {
	primary := strings.TrimPrefix(stringVal(cfg["model"]), "charon/")
	var ids []string
	if provider, ok := cfg["provider"].(map[string]any); ok {
		if charon, ok := provider["charon"].(map[string]any); ok {
			if models, ok := charon["models"].(map[string]any); ok {
				for id := range models {
					ids = append(ids, id)
				}
			}
		}
	}
	if primary == "" && len(ids) == 0 {
		return opencodeTierModels{}, false
	}
	return resolveOpenCodeTiers(primary, ids), true
}

func stringVal(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
