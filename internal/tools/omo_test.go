package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCodeApplyAuthSyncsOMO(t *testing.T) {
	home := sandboxHome(t)
	ocPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, ocPath, `{"$schema":"https://opencode.ai/config.json"}`)

	omoPath := filepath.Join(home, ".omo", "omo.jsonc")
	writeFile(t, omoPath, `{
  "$schema": "https://example.com/omo.schema.json",
  "[opencode]": {
    "agents": {
      "sisyphus": {"model": "google/old-high", "skills": ["keep-me"]},
      "explore": {"model": "google/old-low", "prompt_append": "keep"},
      "atlas": {"model": "google/old-mid"}
    },
    "categories": {
      "quick": {"model": "google/old-low"},
      "deep": {"model": "google/old-high"},
      "visual-engineering": {"model": "google/old-mid"}
    },
    "team_mode": {"enabled": false}
  }
}`)

	c := Find("opencode")
	if err := c.ApplyAuth(AuthSpec{
		Endpoint:  "https://proxy.example/v1",
		Key:       "sk-omo",
		Model:     "mid",
		AllModels: []string{"low", "mid", "high"},
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(omoPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse omo: %v\n%s", err, data)
	}
	sec := cfg["[opencode]"].(map[string]any)
	agents := sec["agents"].(map[string]any)
	if agents["sisyphus"].(map[string]any)["model"] != "charon/high" {
		t.Errorf("sisyphus = %#v", agents["sisyphus"])
	}
	if agents["explore"].(map[string]any)["model"] != "charon/low" {
		t.Errorf("explore = %#v", agents["explore"])
	}
	if agents["atlas"].(map[string]any)["model"] != "charon/mid" {
		t.Errorf("atlas = %#v", agents["atlas"])
	}
	// Non-model keys preserved.
	skills, _ := agents["sisyphus"].(map[string]any)["skills"].([]any)
	if len(skills) != 1 || skills[0] != "keep-me" {
		t.Errorf("skills not preserved: %#v", agents["sisyphus"])
	}
	if agents["explore"].(map[string]any)["prompt_append"] != "keep" {
		t.Errorf("prompt_append not preserved: %#v", agents["explore"])
	}
	cats := sec["categories"].(map[string]any)
	if cats["quick"].(map[string]any)["model"] != "charon/low" {
		t.Errorf("quick = %#v", cats["quick"])
	}
	if cats["deep"].(map[string]any)["model"] != "charon/high" {
		t.Errorf("deep = %#v", cats["deep"])
	}
	if cats["visual-engineering"].(map[string]any)["model"] != "charon/mid" {
		t.Errorf("visual-engineering = %#v", cats["visual-engineering"])
	}
	tm, _ := sec["team_mode"].(map[string]any)
	if tm["enabled"] != false {
		t.Errorf("team_mode should be preserved: %#v", tm)
	}
}

func TestOpenCodeApplyAuthSkipsMissingOMO(t *testing.T) {
	home := sandboxHome(t)
	ocPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, ocPath, `{}`)

	c := Find("opencode")
	if err := c.ApplyAuth(AuthSpec{
		Endpoint: "https://proxy.example/v1",
		Key:      "sk-no-omo",
		Model:    "mid",
	}); err != nil {
		t.Fatalf("ApplyAuth without OMO must succeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".omo")); !os.IsNotExist(err) {
		t.Fatalf("must not create ~/.omo when absent: err=%v", err)
	}
}

func TestSyncOMOFromOpenCodeLive(t *testing.T) {
	home := sandboxHome(t)
	ocPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, ocPath, `{
  "model": "charon/mid",
  "small_model": "charon/low",
  "provider": {
    "charon": {
      "models": {
        "low": {"name": "low"},
        "mid": {"name": "mid"},
        "high": {"name": "high"}
      }
    }
  }
}`)
	omoPath := filepath.Join(home, ".omo", "omo.jsonc")
	writeFile(t, omoPath, `{
  "[opencode]": {
    "agents": {
      "oracle": {"model": "stale"},
      "librarian": {"model": "stale"}
    },
    "categories": {
      "ultrabrain": {"model": "stale"}
    }
  }
}`)

	if err := SyncOMOFromOpenCodeLive(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(omoPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	sec := cfg["[opencode]"].(map[string]any)
	agents := sec["agents"].(map[string]any)
	if agents["oracle"].(map[string]any)["model"] != "charon/high" {
		t.Errorf("oracle = %#v", agents["oracle"])
	}
	if agents["librarian"].(map[string]any)["model"] != "charon/low" {
		t.Errorf("librarian = %#v", agents["librarian"])
	}
	cats := sec["categories"].(map[string]any)
	if cats["ultrabrain"].(map[string]any)["model"] != "charon/high" {
		t.Errorf("ultrabrain = %#v", cats["ultrabrain"])
	}
}

func TestSyncOMOFlatLayout(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.jsonc"), `{
  "model": "charon/gemini-mid",
  "provider": {"charon": {"models": {
    "gemini-low": {}, "gemini-mid": {}, "gemini-high": {}
  }}}
}`)
	omoPath := filepath.Join(home, ".omo", "omo.json")
	writeFile(t, omoPath, `{
  "agents": {"sisyphus": {"model": "old"}},
  "categories": {"writing": {"model": "old"}}
}`)

	if err := SyncOMOFromOpenCodeLive(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(omoPath)
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	// resolveOpenCodeTiers: mid=primary gemini-mid; low contains "low"; high contains "high"
	agents := cfg["agents"].(map[string]any)
	if agents["sisyphus"].(map[string]any)["model"] != "charon/gemini-high" {
		t.Errorf("sisyphus = %#v", agents["sisyphus"])
	}
	cats := cfg["categories"].(map[string]any)
	if cats["writing"].(map[string]any)["model"] != "charon/gemini-low" {
		t.Errorf("writing = %#v", cats["writing"])
	}
}
