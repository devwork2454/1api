package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"1api/internal/artifact"
	"1api/internal/models"
	"1api/internal/provider"
)

// Pi has no static provider-config file: providers are registered by TypeScript
// extensions (pi.registerProvider(...)) auto-loaded from ~/.pi/agent/extensions.
// 1api owns one such extension, 1api.ts, wrapping a JSON blob so it can be
// round-tripped without a TS parser; see the 1api:config markers in
// piExtensionContent.

// piConfigRE matches both the current "1api" provider name and the legacy
// pre-rename "charon" name, so a config written by an older version is still read.
var piConfigRE = regexp.MustCompile(`(?s)pi\.registerProvider\("(?:1api|charon)",\s*({.*?})\);`)

// piModel is one entry of a pi provider's "models" array.
type piModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Reasoning     bool     `json:"reasoning"`
	Input         []string `json:"input"`
	Cost          piCost   `json:"cost"`
	ContextWindow int      `json:"contextWindow"`
	MaxTokens     int      `json:"maxTokens"`
}

type piCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// piProviderConfig is the object literal passed to pi.registerProvider.
type piProviderConfig struct {
	Name    string    `json:"name"`
	BaseURL string    `json:"baseUrl"`
	APIKey  string    `json:"apiKey"`
	API     string    `json:"api"`
	Models  []piModel `json:"models"`
}

// piEscapeValue escapes "$" and "!", which pi's apiKey/headers fields treat as
// env-var interpolation ("$VAR", "${VAR}") and command execution ("!cmd") markers,
// so a literal key/header value containing either is never misinterpreted.
func piEscapeValue(s string) string {
	s = strings.ReplaceAll(s, "$", "$$")
	s = strings.ReplaceAll(s, "!", "$!")
	return s
}

// piBuiltinWindow returns a verified context window for well-known models whose
// own catalogs don't report one (e.g. DeepSeek's /v1/models returns ids only,
// and third-party gateways like Aliyun/NVIDIA proxy them without metadata). Only
// values confirmed from the model vendor's catalog are listed; unknown models
// return 0 and fall through to the generic default.
func piBuiltinWindow(model string) int {
	lower := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(lower, "deepseek-v4-"):
		// DeepSeek V4 family: 1M (vendor /models reports ids only; pi.dev catalog
		// confirms deepseek-v4-flash/pro = 1M, snapshot variants share it).
		return 1_000_000
	case strings.Contains(lower, "glm-5.2") || strings.Contains(lower, "glm-5.3") || strings.Contains(lower, "glm-5."):
		// Zhipu GLM-5 family: 1M per vendor docs.
		return 1_000_000
	case strings.Contains(lower, "ark-code"):
		// Volcengine Ark Coding: 200k
		return 200_000
	case strings.Contains(lower, "grok-4.5"):
		// xAI grok-4.5: 500k (confirmed from api.x.ai catalog context_length).
		return 500_000
	case strings.Contains(lower, "grok-4.20"):
		// xAI grok-4.20: 1M (confirmed from api.x.ai catalog context_length).
		return 1_000_000
	}
	return 0
}

// piContextWindow mirrors claudeContextWindow's Claude-model special-case, plus a
// generic default for everything else. known overrides when the catalog reported a window.
func piContextWindow(model string, known map[string]int) int {
	if known != nil {
		if w := known[model]; w > 0 {
			return w
		}
	}
	if w := piBuiltinWindow(model); w != 0 {
		return w
	}
	if w := claudeContextWindow(model); w != 0 {
		return w
	}
	return 128_000
}

// piFetchInfo is swappable in tests; production uses models.FetchInfo. Catalog
// lookups are best-effort and never block writing the extension on failure.
var piFetchInfo = models.FetchInfo

// piReadStoredWindows loads id → contextWindow from pi's cached remote catalog
// (models-store.json, populated by pi from https://pi.dev). Some gateways (e.g.
// DeepSeek) don't report windows on their own /v1/models, so this is the only
// reliable source for those models' real windows — and it keeps 1api's extension
// consistent with what pi itself displays and compacts against.
func piReadStoredWindows(agentDir string) map[string]int {
	data, err := os.ReadFile(filepath.Join(agentDir, "models-store.json"))
	if err != nil {
		return nil
	}
	var store map[string]struct {
		Models []struct {
			ID            string `json:"id"`
			ContextWindow int    `json:"contextWindow"`
		} `json:"models"`
	}
	if json.Unmarshal(data, &store) != nil {
		return nil
	}
	out := map[string]int{}
	for _, prov := range store {
		for _, m := range prov.Models {
			if m.ID != "" && m.ContextWindow > 0 {
				out[m.ID] = m.ContextWindow
			}
		}
	}
	return out
}

// mergeWindows returns base plus any positive windows from fill that base lacks.
// base (freshest source, e.g. a just-probed catalog) wins over fill (pi's cached
// remote catalog).
func mergeWindows(base, fill map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range fill {
		if v > 0 {
			out[k] = v
		}
	}
	for k, v := range base {
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

// piWindowsMissing reports whether any id lacks a positive window in known.
func piWindowsMissing(ids []string, known map[string]int) bool {
	for _, id := range ids {
		if id == "" {
			continue
		}
		if known == nil || known[id] <= 0 {
			return true
		}
	}
	return false
}

// piOpenAIBaseURL converts a provider endpoint to the OpenAI-compatible base URL
// pi's "openai-completions" API needs. Anthropic-style endpoints (e.g. DeepSeek's
// https://api.deepseek.com/anthropic) serve only /v1/messages; the OpenAI
// completions route lives on the host root, so strip the /v1 and /anthropic path
// suffixes. Endpoints that aren't Anthropic-style pass through unchanged (the
// /v1 suffix is kept).
func piOpenAIBaseURL(endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if !strings.Contains(base, "/anthropic") {
		return base
	}
	base = strings.TrimSuffix(base, "/v1")
	base = strings.TrimRight(base, "/")
	base = strings.TrimSuffix(base, "/anthropic")
	return strings.TrimRight(base, "/")
}

// piPrimaryWire guesses the models-list wire format for the primary "1api"
// provider. Profile auth specs don't carry the wire flag; anthropic-style
// endpoints (e.g. DeepSeek's https://api.deepseek.com/anthropic) need the
// Anthropic list URL and x-api-key headers.
func piPrimaryWire(endpoint string) models.Provider {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(base, "/anthropic") || strings.Contains(base, "/anthropic/") {
		return models.Anthropic
	}
	return models.OpenAI
}

// piOtherWire returns the opposite wire format, for retrying a primary provider
// whose endpoint shape misled piPrimaryWire.
func piOtherWire(w models.Provider) models.Provider {
	if w == models.Anthropic {
		return models.OpenAI
	}
	return models.Anthropic
}

// piCatalogWindows best-effort merges fresh context windows from the live model
// catalog into known (stored catalog / prior parse). Freshly reported windows
// win; known values survive for ids the endpoint didn't report (proxy aliases
// like high/mid/low). A failure returns known unchanged — offline must never
// block writing the pi extension.
func piCatalogWindows(ids []string, endpoint, key string, wire models.Provider, known map[string]int) map[string]int {
	out := known
	if out == nil {
		out = map[string]int{}
	}
	// Fast path: every target id already has a window — no network needed.
	if !piWindowsMissing(ids, out) {
		return out
	}
	if infos, err := piFetchInfo(wire, endpoint, key); err == nil {
		for _, m := range infos {
			if m.ID != "" && m.ContextWindow > 0 {
				out[m.ID] = m.ContextWindow
			}
		}
	}
	return out
}

// piFetchJob is one provider's best-effort context-window lookup.
type piFetchJob struct {
	name     string
	ids      []string
	endpoint string
	key      string
	wire     models.Provider
	known    map[string]int
}

// piFetchAllWindows runs all catalog lookups concurrently (each is bounded by
// models.FetchInfo's timeout) and returns a name → windows map.
func piFetchAllWindows(jobs []piFetchJob) map[string]map[string]int {
	out := make(map[string]map[string]int, len(jobs))
	if len(jobs) == 0 {
		return out
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, job := range jobs {
		wg.Add(1)
		go func(j piFetchJob) {
			defer wg.Done()
			w := piCatalogWindows(j.ids, j.endpoint, j.key, j.wire, j.known)
			mu.Lock()
			out[j.name] = w
			mu.Unlock()
		}(job)
	}
	wg.Wait()
	return out
}

// piBuildModels turns a list of model ids into pi model entries.
func piBuildModels(ids []string, known map[string]int) []piModel {
	models := make([]piModel, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		models = append(models, piModel{
			ID:            id,
			Name:          id,
			Input:         []string{"text", "image"},
			ContextWindow: piContextWindow(id, known),
			MaxTokens:     8192,
		})
	}
	return models
}

// piExtensionContent renders 1api's extension .ts file for cfg.
func piExtensionContent(cfgs []piProviderConfig) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  // 1api:config
`)
	for _, cfg := range cfgs {
		body, err := json.MarshalIndent(cfg, "  ", "  ")
		if err != nil {
			return nil, err
		}
		b.WriteString("  pi.registerProvider(\"")
		b.WriteString(cfg.Name)
		b.WriteString("\", ")
		b.Write(body)
		b.WriteString(");\n")
	}
	b.WriteString("  // 1api:config:end\n}\n")
	return []byte(b.String()), nil
}

// piParseExtension extracts the provider config JSON from a previously written
// 1api.ts, "" fields / nil models if the file is absent or unrecognized.
func piParseExtension(data []byte) (piProviderConfig, bool) {
	m := piConfigRE.FindSubmatch(data)
	if m == nil {
		return piProviderConfig{}, false
	}
	var cfg piProviderConfig
	if json.Unmarshal(m[1], &cfg) != nil {
		return piProviderConfig{}, false
	}
	return cfg, true
}

// piReadExtension loads the pi extension, trying the current on-disk name first
// and falling back to the legacy pre-rename name so old installs keep working
// until the next switch rewrites 1api.ts.
func piReadExtension(extensionPath, legacyExtensionPath string) ([]byte, error) {
	if data, err := os.ReadFile(extensionPath); err == nil {
		return data, nil
	}
	return os.ReadFile(legacyExtensionPath)
}

// newPi describes the pi coding agent: providers are registered via a TypeScript
// extension (~/.pi/agent/extensions/1api.ts); model/effort defaults live in
// ~/.pi/agent/settings.json; OAuth-based provider logins (unrelated to 1api's
// key-based provider) persist in ~/.pi/agent/auth.json.
func newPi() *Tool {
	dir := filepath.Join(home(), ".pi", "agent")
	settingsPath := filepath.Join(dir, "settings.json")
	authPath := filepath.Join(dir, "auth.json")
	extensionPath := filepath.Join(dir, "extensions", "1api.ts")
	// legacyExtensionPath is the pre-rename on-disk name; old installs still carry
	// charon.ts, so reads fall back to it until the next switch rewrites 1api.ts.
	legacyExtensionPath := filepath.Join(dir, "extensions", "charon.ts")

	return &Tool{
		Name:            "pi",
		Title:           "Pi",
		Provider:        "openai",
		DefaultEndpoint: "https://api.openai.com/v1",
		Artifacts: []artifact.Artifact{
			// Other settings.json fields (theme, extensions list, shell, ...) are CLI
			// preferences, not per-profile auth — preserved live. defaultModel and
			// defaultThinkingLevel switch with the profile, matching Claude Code/Codex.
			artifact.NewMergedJSONFile("settings.json", settingsPath, 0o600,
				"defaultProvider", "defaultModel", "defaultThinkingLevel").
				WithDisplay("defaultModel", "defaultThinkingLevel"),
			artifact.NewFile("1api.ts", extensionPath, 0o600),      // 1api owns this extension file outright
			artifact.NewRotatingFile("auth.json", authPath, 0o600), // OAuth provider logins; pi refreshes them in place
		},
		ApplyAuth: func(a AuthSpec) error {
			// Preserve the previously-registered model list when this call doesn't bring
			// its own (rename, key rotation, CLI --model) — otherwise pi's /model picker
			// would collapse down to just the single current model.
			var existingModels []string
			if data, err := piReadExtension(extensionPath, legacyExtensionPath); err == nil {
				if prev, ok := piParseExtension(data); ok {
					for _, m := range prev.Models {
						existingModels = append(existingModels, m.ID)
					}
				}
			}

			var tierIDs []string
			if a.High != "" {
				tierIDs = append(tierIDs, a.High)
			}
			if a.Model != "" {
				tierIDs = append(tierIDs, a.Model)
			}
			if a.Low != "" {
				tierIDs = append(tierIDs, a.Low)
			}

			var uniqueIDs []string
			seen := map[string]bool{}
			for _, id := range tierIDs {
				if id != "" && !seen[id] {
					seen[id] = true
					uniqueIDs = append(uniqueIDs, id)
				}
			}

			ids := uniqueIDs
			if len(ids) == 0 {
				ids = a.AllModels
			}
			if len(ids) == 0 {
				ids = existingModels
			}
			if len(ids) == 0 && a.Model != "" {
				ids = []string{a.Model}
			}

			// Best-effort: resolve fresh context windows for each provider's models so
			// pi's footer shows the real window and auto-compaction waits for it,
			// instead of the 128k fallback. Sources, freshest first: the stored
			// catalog (just probed), pi's cached remote catalog (models-store.json —
			// the only source for gateways like DeepSeek whose /v1/models reports no
			// window), then a live catalog fetch. Offline failures fall back to the
			// heuristic without blocking the write.
			stored := piReadStoredWindows(dir)
			mainWire := piPrimaryWire(a.Endpoint)
			mainWindows := piCatalogWindows(ids, a.Endpoint, a.Key, mainWire, mergeWindows(a.ContextWindows, stored))
			if piWindowsMissing(ids, mainWindows) {
				// Endpoint shape may have misled the wire guess; try the other format.
				mainWindows = piCatalogWindows(ids, a.Endpoint, a.Key, piOtherWire(mainWire), mainWindows)
			}
			cfg := piProviderConfig{
				Name:    "1api",
				BaseURL: piOpenAIBaseURL(a.Endpoint),
				APIKey:  piEscapeValue(a.Key),
				API:     "openai-completions",
			}

			var cfgs []piProviderConfig
			cfgs = append(cfgs, cfg)

			base := os.Getenv("XDG_CONFIG_HOME")
			if base == "" {
				h, _ := os.UserHomeDir()
				base = filepath.Join(h, ".config")
			}
			providerRoot := filepath.Join(base, "1api")
			var recs []struct {
				name     string
				endpoint string
				key      string
				wire     string
				ids      []string
				known    map[string]int
			}
			if ps, err := provider.OpenAt(providerRoot); err == nil {
				for _, pName := range ps.List() {
					if rec, err := ps.Get(pName); err == nil {

						var usableIDs []string
						if rec.High != "" {
							usableIDs = append(usableIDs, rec.High)
						}
						if rec.Mid != "" {
							usableIDs = append(usableIDs, rec.Mid)
						}
						if rec.Low != "" {
							usableIDs = append(usableIDs, rec.Low)
						}

						var uniqIDs []string
						uSeen := map[string]bool{}
						for _, id := range usableIDs {
							if id != "" && !uSeen[id] {
								uSeen[id] = true
								uniqIDs = append(uniqIDs, id)
							}
						}

						if len(uniqIDs) == 0 {
							uniqIDs = rec.Usable
						}
						if len(uniqIDs) == 0 && rec.Mid != "" {
							uniqIDs = []string{rec.Mid}
						}

						recs = append(recs, struct {
							name     string
							endpoint string
							key      string
							wire     string
							ids      []string
							known    map[string]int
						}{
							name:     "1api-" + pName,
							endpoint: rec.Endpoint,
							key:      rec.Key,
							wire:     rec.Wire,
							ids:      uniqIDs,
							known:    mergeWindows(rec.ContextWindows, stored),
						})
					}
				}
			}

			jobs := make([]piFetchJob, 0, len(recs))
			for _, r := range recs {
				jobs = append(jobs, piFetchJob{
					name:     r.name,
					ids:      r.ids,
					endpoint: r.endpoint,
					key:      r.key,
					wire:     models.Provider(r.wire),
					known:    r.known,
				})
			}
			windowsByName := piFetchAllWindows(jobs)

			// Share windows across providers: the same model id served by different
			// gateways has the same real window (e.g. grok-4.5 via api.x.ai reports
			// 500k, so the arcdent gateway's grok-4.5 gets it too). Fetched windows
			// win over pi's cached remote catalog.
			shared := map[string]int{}
			for _, w := range windowsByName {
				for k, v := range w {
					if v > 0 {
						shared[k] = v
					}
				}
			}
			for k, v := range mainWindows {
				if v > 0 {
					shared[k] = v
				}
			}
			for k, v := range stored {
				if v > 0 {
					if _, ok := shared[k]; !ok {
						shared[k] = v
					}
				}
			}

			// Each provider's own stored catalog is freshest; shared fills the gaps.
			cfgs[0].Models = piBuildModels(ids, mergeWindows(a.ContextWindows, shared))

			for _, r := range recs {
				cfgs = append(cfgs, piProviderConfig{
					Name:    r.name,
					BaseURL: piOpenAIBaseURL(r.endpoint),
					APIKey:  piEscapeValue(r.key),
					API:     "openai-completions",
					Models:  piBuildModels(r.ids, mergeWindows(r.known, shared)),
				})
			}

			content, err := piExtensionContent(cfgs)
			if err != nil {
				return fmt.Errorf("render pi extension: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(extensionPath), 0o700); err != nil {
				return err
			}
			if err := artifact.AtomicWrite(extensionPath, content, 0o600); err != nil {
				return err
			}

			s, err := loadJSONMap(settingsPath)
			if err != nil {
				return err
			}
			s["defaultProvider"] = "1api"
			if a.Model != "" {
				s["defaultModel"] = a.Model
			}
			return writeJSONMap(settingsPath, s, 0o600)
		},
		Detected: func() bool {
			return detected("pi", settingsPath, authPath, extensionPath)
		},
		Describe: func() (Info, error) {
			var info Info

			if data, err := os.ReadFile(settingsPath); err == nil {
				var s struct {
					DefaultProvider      string `json:"defaultProvider"`
					DefaultModel         string `json:"defaultModel"`
					DefaultThinkingLevel string `json:"defaultThinkingLevel"`
				}
				if json.Unmarshal(data, &s) == nil {
					info.Model = s.DefaultModel
					info.Effort = s.DefaultThinkingLevel

					if s.DefaultProvider == "1api" || s.DefaultProvider == "charon" || s.DefaultProvider == "" {
						if data, err := piReadExtension(extensionPath, legacyExtensionPath); err == nil {
							if cfg, ok := piParseExtension(data); ok {
								info.Endpoint = cfg.BaseURL
								if cfg.APIKey != "" {
									info.Secret, info.AuthMode = cfg.APIKey, "api"
								}
							}
						}
					}
				}
			}

			// Otherwise fall back to an OAuth-based provider login (pi's /login).
			if info.AuthMode == "" {
				if data, err := os.ReadFile(authPath); err == nil {
					var auth map[string]json.RawMessage
					if json.Unmarshal(data, &auth) == nil && len(auth) > 0 {
						names := make([]string, 0, len(auth))
						for name := range auth {
							names = append(names, name)
						}
						sort.Strings(names)
						info.AuthMode = "oauth"
						info.Account = names[0]
					}
				}
			}

			return info.withDefaults("(provider default)"), nil
		},
	}
}
