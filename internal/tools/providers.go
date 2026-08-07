package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The codex and opencode adapters register their endpoint + key as a provider
// entry named "1api" inside a config block that also holds user-authored
// providers. These guards make that write refuse to touch anything but ours.

// managedProvider is the only provider entry 1api owns; all others are the user's.
const managedProvider = "1api"

// legacyManagedProvider is the provider name 1api used before the rename from
// "charon". Configs written by older versions still carry this key; both names
// are treated as 1api's own entry so a rename never orphans an existing setup.
const legacyManagedProvider = "charon"

// isManagedProvider reports whether name refers to 1api's own provider entry,
// under either its current name or the legacy pre-rename name.
func isManagedProvider(name string) bool {
	return name == managedProvider || name == legacyManagedProvider
}

// firstManagedProvider returns the 1api-owned provider entry stored under either
// its current or legacy name, preferring the current name. It reports the key it
// was found under so callers can overwrite the same slot.
func firstManagedProvider(providers map[string]any) (map[string]any, string) {
	for _, name := range []string{managedProvider, legacyManagedProvider} {
		if v, ok := providers[name].(map[string]any); ok {
			return v, name
		}
	}
	return nil, ""
}

// trimProviderPrefix strips a leading "1api/" (or legacy "charon/") model prefix,
// so a config written by either version yields the bare model id.
func trimProviderPrefix(s string) string {
	if v, ok := strings.CutPrefix(s, managedProvider+"/"); ok {
		return v
	}
	return strings.TrimPrefix(s, legacyManagedProvider+"/")
}

// snapshotProviders records the JSON of every provider but 1api's, for later comparison.
func snapshotProviders(providers map[string]any) map[string]string {
	snap := map[string]string{}
	for name, v := range providers {
		if isManagedProvider(name) {
			continue
		}
		b, _ := json.Marshal(v)
		snap[name] = string(b)
	}
	return snap
}

// ensureOnlyManagedChanged errors if the write would delete or edit any user provider.
func ensureOnlyManagedChanged(original map[string]string, providers map[string]any) error {
	for name, want := range original {
		v, ok := providers[name]
		if !ok {
			return fmt.Errorf("refusing to write config: would delete provider %q", name)
		}
		b, _ := json.Marshal(v)
		if string(b) != want {
			return fmt.Errorf("refusing to write config: would modify provider %q", name)
		}
	}
	return nil
}
