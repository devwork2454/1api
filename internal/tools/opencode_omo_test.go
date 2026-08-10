package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeOmoFixture(t *testing.T, home string) string {
	t.Helper()
	p := filepath.Join(home, ".omo", "omo.jsonc")
	writeFile(t, p, `{
  "[opencode]": {
    "agents": {
      "explore": {"model": "charon/old-low", "description": "keep"},
      "sisyphus": {"model": "charon/old-high"},
      "atlas": {"model": "charon/old-mid"},
      "custom": {"model": "openai/gpt-4"}
    },
    "categories": {
      "quick": {"model": "charon/old-low"},
      "deep": {"model": "charon/old-high"},
      "visual-engineering": {"model": "charon/old-mid"}
    }
  }
}`)
	return p
}

func writeOpenCodeWithTiers(t *testing.T, home, primary string, ids []string) {
	t.Helper()
	models := map[string]any{}
	for _, id := range ids {
		models[id] = map[string]any{"name": id}
	}
	cfg := map[string]any{
		"model":       "1api/" + primary,
		"small_model": "1api/" + primary,
		"provider": map[string]any{
			"1api": map[string]any{
				"name":    "1api",
				"npm":     "@ai-sdk/openai-compatible",
				"options": map[string]any{"baseURL": "https://x/v1", "apiKey": "sk-x"},
				"models":  models,
			},
		},
	}
	// Prefer explicit low/mid/high when present
	if containsString(ids, "low") {
		cfg["small_model"] = "1api/low"
	}
	if containsString(ids, "mid") {
		cfg["model"] = "1api/mid"
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.jsonc"), string(data))
}

func readOmoModels(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("omo not pure JSON: %v\n%s", err, data)
	}
	oc := root["[opencode]"].(map[string]any)
	out := map[string]string{}
	for _, group := range []string{"agents", "categories"} {
		g, _ := oc[group].(map[string]any)
		for name, raw := range g {
			entry := raw.(map[string]any)
			out[group+"."+name] = entry["model"].(string)
		}
	}
	return out
}

func TestSyncOpenCodeOmoMissingIsNoop(t *testing.T) {
	home := sandboxHome(t)
	writeOpenCodeWithTiers(t, home, "mid", []string{"low", "mid", "high"})
	if err := SyncOpenCodeOmoAt(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".omo", "omo.jsonc")); !os.IsNotExist(err) {
		t.Fatalf("must not create omo: %v", err)
	}
}

func TestSyncOpenCodeOmoRewritesManagedPreservesForeign(t *testing.T) {
	home := sandboxHome(t)
	omo := writeOmoFixture(t, home)
	writeOpenCodeWithTiers(t, home, "mid", []string{"low", "mid", "high"})

	if err := SyncOpenCodeOmoAt(home); err != nil {
		t.Fatal(err)
	}
	got := readOmoModels(t, omo)
	// explore → low, sisyphus → high, atlas → mid
	if got["agents.explore"] != "1api/low" {
		t.Errorf("explore = %q", got["agents.explore"])
	}
	if got["agents.sisyphus"] != "1api/high" {
		t.Errorf("sisyphus = %q", got["agents.sisyphus"])
	}
	if got["agents.atlas"] != "1api/mid" {
		t.Errorf("atlas = %q", got["agents.atlas"])
	}
	if got["agents.custom"] != "openai/gpt-4" {
		t.Errorf("foreign model must survive: %q", got["agents.custom"])
	}
	if got["categories.quick"] != "1api/low" {
		t.Errorf("quick = %q", got["categories.quick"])
	}
	if got["categories.deep"] != "1api/high" {
		t.Errorf("deep = %q", got["categories.deep"])
	}
	if got["categories.visual-engineering"] != "1api/mid" {
		t.Errorf("visual = %q", got["categories.visual-engineering"])
	}
	// description preserved
	data, _ := os.ReadFile(omo)
	var root map[string]any
	_ = json.Unmarshal(data, &root)
	agents := root["[opencode]"].(map[string]any)["agents"].(map[string]any)
	if agents["explore"].(map[string]any)["description"] != "keep" {
		t.Error("non-model fields must survive")
	}
}

func TestSyncOpenCodeOmoSingleModelFillsAllTiers(t *testing.T) {
	home := sandboxHome(t)
	omo := writeOmoFixture(t, home)
	writeOpenCodeWithTiers(t, home, "deepseek-v4-flash-0731", []string{"deepseek-v4-flash-0731"})
	if err := SyncOpenCodeOmoAt(home); err != nil {
		t.Fatal(err)
	}
	got := readOmoModels(t, omo)
	want := "1api/deepseek-v4-flash-0731"
	for _, k := range []string{"agents.explore", "agents.sisyphus", "agents.atlas", "categories.quick", "categories.deep"} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
}

func TestSyncOpenCodeOmoKeepsCharonPrefixWhenLiveUsesIt(t *testing.T) {
	home := sandboxHome(t)
	omo := writeOmoFixture(t, home)
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.jsonc"), `{
  "model": "charon/only",
  "provider": {
    "charon": {
      "models": {"only": {"name": "only"}},
      "options": {"baseURL": "https://x/v1", "apiKey": "sk"}
    }
  }
}`)
	if err := SyncOpenCodeOmoAt(home); err != nil {
		t.Fatal(err)
	}
	got := readOmoModels(t, omo)
	if got["agents.atlas"] != "charon/only" {
		t.Errorf("atlas = %q, want charon/only", got["agents.atlas"])
	}
}

func TestCategoryTierClass(t *testing.T) {
	cases := map[string]opencodeTier{
		"quick":              tierLow,
		"unspecified-low":    tierLow,
		"writing":            tierLow,
		"deep":               tierHigh,
		"ultrabrain":         tierHigh,
		"unspecified-high":   tierHigh,
		"artistry":           tierHigh,
		"visual-engineering": tierMid,
		"":                   tierMid,
	}
	for name, want := range cases {
		if got := categoryTierClass(name); got != want {
			t.Errorf("categoryTierClass(%q)=%q want %q", name, got, want)
		}
	}
}

func TestOpenCodeApplyAuthSyncsOmo(t *testing.T) {
	home := sandboxHome(t)
	omo := writeOmoFixture(t, home)
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.jsonc"), `{
  "$schema": "https://opencode.ai/config.json",
  "theme": "keep"
}`)
	c := Find("opencode")
	if err := c.ApplyAuth(AuthSpec{
		Endpoint:   "https://proxy.example/v1",
		Key:        "sk-omo",
		Model:      "mid",
		AllModels:  []string{"low", "mid", "high"},
		SkipVerify: true,
	}); err != nil {
		t.Fatal(err)
	}
	got := readOmoModels(t, omo)
	if got["agents.explore"] != "1api/low" || got["agents.sisyphus"] != "1api/high" {
		t.Fatalf("ApplyAuth did not sync omo: %#v", got)
	}
	if got["agents.custom"] != "openai/gpt-4" {
		t.Errorf("foreign preserved: %q", got["agents.custom"])
	}
}
