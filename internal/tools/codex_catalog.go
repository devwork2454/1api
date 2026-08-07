package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charon/internal/artifact"
)

// codexModelCatalogRel is the fixed catalog file charon maintains for custom
// Codex providers. Codex resolves unknown slugs via model_catalog_json; without
// it, custom models fall back to restrictive built-in metadata.
const codexModelCatalogRel = "charon-model-catalog.json"

func codexDir() string {
	return filepath.Join(home(), ".codex")
}

func codexConfigPath() string {
	return filepath.Join(codexDir(), "config.toml")
}

func codexModelCatalogPath() string {
	return filepath.Join(codexDir(), codexModelCatalogRel)
}

// codexContextWindow picks a conservative context window for a custom model.
func codexContextWindow(model string) int {
	if w := claudeContextWindow(model); w != 0 {
		return w
	}
	return 128_000
}

// codexModelCatalogEntry is one ModelsResponse.models[] item in the shape Codex
// deserializes (see openai/codex protocol ModelInfo). Required keys must be
// present even when null.
type codexModelCatalogEntry struct {
	Slug                       string         `json:"slug"`
	DisplayName                string         `json:"display_name"`
	Description                *string        `json:"description"`
	SupportedReasoningLevels   []any          `json:"supported_reasoning_levels"`
	ShellType                  string         `json:"shell_type"`
	Visibility                 string         `json:"visibility"`
	SupportedInAPI             bool           `json:"supported_in_api"`
	Priority                   int            `json:"priority"`
	AvailabilityNux            any            `json:"availability_nux"`
	Upgrade                    any            `json:"upgrade"`
	BaseInstructions           string         `json:"base_instructions"`
	SupportsReasoningSummaries bool           `json:"supports_reasoning_summaries"`
	SupportVerbosity           bool           `json:"support_verbosity"`
	DefaultVerbosity           any            `json:"default_verbosity"`
	ApplyPatchToolType         string         `json:"apply_patch_tool_type"`
	TruncationPolicy           map[string]any `json:"truncation_policy"`
	SupportsParallelToolCalls  bool           `json:"supports_parallel_tool_calls"`
	ExperimentalSupportedTools []any          `json:"experimental_supported_tools"`
	ContextWindow              int            `json:"context_window"`
	MaxContextWindow           int            `json:"max_context_window"`
	InputModalities            []string       `json:"input_modalities"`
}

func newCodexCatalogEntry(slug string, priority int) codexModelCatalogEntry {
	w := codexContextWindow(slug)
	return codexModelCatalogEntry{
		Slug:                       slug,
		DisplayName:                slug,
		Description:                nil,
		SupportedReasoningLevels:   []any{},
		ShellType:                  "shell_command",
		Visibility:                 "list",
		SupportedInAPI:             true,
		Priority:                   priority,
		AvailabilityNux:            nil,
		Upgrade:                    nil,
		BaseInstructions:           "You are a helpful coding agent running in a terminal.",
		SupportsReasoningSummaries: false,
		SupportVerbosity:           false,
		DefaultVerbosity:           nil,
		ApplyPatchToolType:         "freeform",
		TruncationPolicy:           map[string]any{"mode": "bytes", "limit": 10000},
		SupportsParallelToolCalls:  true,
		ExperimentalSupportedTools: []any{},
		ContextWindow:              w,
		MaxContextWindow:           w,
		InputModalities:            []string{"text"},
	}
}

// writeCodexModelCatalog writes a StaticModelsManager catalog for the given
// model ids (active first). Empty ids are skipped; at least one id is required.
func writeCodexModelCatalog(ids []string) (string, error) {
	seen := map[string]bool{}
	var models []codexModelCatalogEntry
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, newCodexCatalogEntry(id, len(models)))
	}
	if len(models) == 0 {
		return "", fmt.Errorf("codex model catalog: no model ids")
	}
	path := codexModelCatalogPath()
	body, err := json.MarshalIndent(map[string]any{"models": models}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := artifact.AtomicWrite(path, append(body, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// collectCodexCatalogIDs returns active model plus AllModels, active first.
func collectCodexCatalogIDs(active string, all []string) []string {
	out := make([]string, 0, 1+len(all))
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	add(active)
	for _, id := range all {
		add(id)
	}
	return out
}

// setCodexAppsEnabled merges features.apps without clobbering other feature tables.
func setCodexAppsEnabled(cfg map[string]any, enabled bool) {
	features := subMap(cfg, "features")
	if enabled {
		delete(features, "apps")
		if len(features) == 0 {
			delete(cfg, "features")
		}
		return
	}
	features["apps"] = false
}

// syncCodexCompanionFromLive keeps model_catalog_json + features.apps aligned
// with whether the live config points at the charon provider. Used after
// ApplyAuth and after profile switch/undo.
func syncCodexCompanionFromLive() error {
	cfg, err := loadTOMLMap(codexConfigPath())
	if err != nil {
		return err
	}
	provider, _ := cfg["model_provider"].(string)
	if provider != managedProvider {
		delete(cfg, "model_catalog_json")
		setCodexAppsEnabled(cfg, true)
		return writeTOMLMap(codexConfigPath(), cfg, 0o600)
	}
	model, _ := cfg["model"].(string)
	if ids := collectCodexCatalogIDs(model, nil); len(ids) > 0 {
		path, err := writeCodexModelCatalog(ids)
		if err != nil {
			return err
		}
		cfg["model_catalog_json"] = path
	} else {
		// No resolvable slug — drop a stale catalog pointer rather than leave it.
		delete(cfg, "model_catalog_json")
	}
	setCodexAppsEnabled(cfg, false)
	return writeTOMLMap(codexConfigPath(), cfg, 0o600)
}
