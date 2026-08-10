package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	omoRelPath     = ".omo/omo.jsonc"
	omoOpenCodeKey = "[opencode]"
)

// SyncOpenCodeOmo patches ~/.omo/omo.jsonc agent/category models from the live
// OpenCode config under the process home. Missing omo is a no-op success.
func SyncOpenCodeOmo() error {
	return SyncOpenCodeOmoAt(home())
}

// SyncOpenCodeOmoAt is SyncOpenCodeOmo for an explicit home directory (live or
// session sandbox). It never creates omo when absent.
func SyncOpenCodeOmoAt(homeDir string) error {
	if homeDir == "" {
		return fmt.Errorf("omo sync: empty home")
	}
	omoPath := filepath.Join(homeDir, omoRelPath)
	if _, err := os.Stat(omoPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cfgPath := opencodeConfigPathAt(homeDir)
	cfg, err := loadJSONMap(cfgPath)
	if err != nil {
		return fmt.Errorf("omo sync: read opencode config: %w", err)
	}
	tiers, prefix := openCodeTiersFromConfig(cfg)
	if tiers.Mid == "" || prefix == "" {
		return nil
	}
	return patchOmoModels(omoPath, tiers, prefix)
}

// SeedAndSyncOpenCodeOmoAt copies the host omo into homeDir when the sandbox has
// none, then patches models from that homeDir's OpenCode config. Used by session
// run. Host omo is read via home() at call time.
func SeedAndSyncOpenCodeOmoAt(homeDir string) error {
	if homeDir == "" {
		return fmt.Errorf("omo sync: empty home")
	}
	dst := filepath.Join(homeDir, omoRelPath)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		src := filepath.Join(home(), omoRelPath)
		if data, rerr := os.ReadFile(src); rerr == nil {
			if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
				return err
			}
			if werr := os.WriteFile(dst, data, 0o600); werr != nil {
				return werr
			}
		}
	}
	return SyncOpenCodeOmoAt(homeDir)
}

// opencodeConfigPathAt mirrors opencodeConfigPath for an explicit home.
func opencodeConfigPathAt(homeDir string) string {
	dir := filepath.Join(homeDir, ".config", "opencode")
	for _, name := range []string{"opencode.jsonc", "opencode.json"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dir, "opencode.jsonc")
}

// openCodeTiersFromConfig derives mid/low/high and the managed provider name.
func openCodeTiersFromConfig(cfg map[string]any) (opencodeTierModels, string) {
	prefix := managedProvider
	primary := ""
	if m, ok := cfg["model"].(string); ok {
		if p, id, ok := splitProviderModel(m); ok {
			if isManagedProvider(p) {
				prefix = p
			}
			primary = id
		} else if m != "" {
			primary = m
		}
	}
	if primary == "" {
		if m, ok := cfg["small_model"].(string); ok {
			primary = trimProviderPrefix(m)
		}
	}

	var ids []string
	if providers, ok := cfg["provider"].(map[string]any); ok {
		if prev, key := firstManagedProvider(providers); prev != nil {
			if key != "" && prefix == managedProvider {
				prefix = key
			}
			if models, ok := prev["models"].(map[string]any); ok {
				for id := range models {
					ids = append(ids, id)
				}
			}
		}
	}
	if primary != "" && !containsString(ids, primary) {
		ids = append(ids, primary)
	}
	return resolveOpenCodeTiers(primary, ids), prefix
}

func splitProviderModel(s string) (provider, model string, ok bool) {
	provider, model, found := strings.Cut(s, "/")
	if !found || provider == "" || model == "" {
		return "", "", false
	}
	return provider, model, true
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func isManagedModelRef(s string) bool {
	p, _, ok := splitProviderModel(s)
	return ok && isManagedProvider(p)
}

// patchOmoModels rewrites managed model fields under "[opencode]".agents/.categories.
func patchOmoModels(omoPath string, tiers opencodeTierModels, providerPrefix string) error {
	root, err := loadJSONMap(omoPath)
	if err != nil {
		return fmt.Errorf("omo sync: read %s: %w", omoPath, err)
	}
	oc, ok := root[omoOpenCodeKey].(map[string]any)
	if !ok || oc == nil {
		return nil
	}
	changed := false
	if agents, ok := oc["agents"].(map[string]any); ok {
		if rewriteOmoGroupModels(agents, providerPrefix, tiers, agentTierClass) {
			changed = true
		}
	}
	if cats, ok := oc["categories"].(map[string]any); ok {
		if rewriteOmoGroupModels(cats, providerPrefix, tiers, categoryTierClass) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeJSONMap(omoPath, root, 0o600)
}

func rewriteOmoGroupModels(group map[string]any, prefix string, tiers opencodeTierModels, classFn func(string) opencodeTier) bool {
	changed := false
	for name, raw := range group {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cur, _ := entry["model"].(string)
		if cur != "" && !isManagedModelRef(cur) {
			continue
		}
		id := tiers.forTier(classFn(name))
		if id == "" {
			continue
		}
		want := prefix + "/" + id
		if cur == want {
			continue
		}
		entry["model"] = want
		group[name] = entry
		changed = true
	}
	return changed
}

// categoryTierClass maps oh-my category names to low/mid/high.
func categoryTierClass(name string) opencodeTier {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "":
		return tierMid
	case containsAny(n, "quick", "unspecified-low", "writing"):
		return tierLow
	case containsAny(n, "ultrabrain", "deep", "unspecified-high", "artistry"):
		return tierHigh
	default:
		return tierMid
	}
}
