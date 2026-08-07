package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOpenCodeTiersExactIds(t *testing.T) {
	got := resolveOpenCodeTiers("mid", []string{"low", "mid", "high", "other"})
	if got.Mid != "mid" || got.Low != "low" || got.High != "high" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveOpenCodeTiersFallbackToPrimary(t *testing.T) {
	got := resolveOpenCodeTiers("only-model", []string{"only-model"})
	if got.Mid != "only-model" || got.Low != "only-model" || got.High != "only-model" {
		t.Fatalf("single model should fill all tiers: %+v", got)
	}
}

func TestResolveOpenCodeTiersContains(t *testing.T) {
	got := resolveOpenCodeTiers("gpt-mid", []string{"gpt-low", "gpt-mid", "gpt-high-pro"})
	if got.Low != "gpt-low" || got.Mid != "gpt-mid" || got.High != "gpt-high-pro" {
		t.Fatalf("got %+v", got)
	}
}

func TestAgentTierClass(t *testing.T) {
	cases := map[string]opencodeTier{
		"compaction": tierLow,
		"explore":    tierLow,
		"librarian":  tierLow,
		"build":      tierMid,
		"atlas":      tierMid,
		"oracle":     tierHigh,
		"prometheus": tierHigh,
		"sisyphus":   tierHigh,
		"":           tierMid,
	}
	for name, want := range cases {
		if got := agentTierClass(name); got != want {
			t.Errorf("agentTierClass(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestApplyOpenCodeTierRouting(t *testing.T) {
	cfg := map[string]any{
		"agent": map[string]any{
			"build":      map[string]any{"model": "1api/old"},
			"compaction": map[string]any{"model": "1api/old-low"},
			"oracle":     map[string]any{"model": "openai/gpt-4"}, // non-1api: leave
		},
	}
	applyOpenCodeTierRouting(cfg, opencodeTierModels{Mid: "mid", Low: "low", High: "high"})
	if cfg["model"] != "1api/mid" {
		t.Errorf("model = %v", cfg["model"])
	}
	if cfg["small_model"] != "1api/low" {
		t.Errorf("small_model = %v", cfg["small_model"])
	}
	agents := cfg["agent"].(map[string]any)
	if agents["build"].(map[string]any)["model"] != "1api/mid" {
		t.Errorf("build = %#v", agents["build"])
	}
	if agents["compaction"].(map[string]any)["model"] != "1api/low" {
		t.Errorf("compaction = %#v", agents["compaction"])
	}
	if agents["oracle"].(map[string]any)["model"] != "openai/gpt-4" {
		t.Errorf("non-1api oracle must be preserved: %#v", agents["oracle"])
	}
}

func TestOpenCodeApplyAuthTierRoutingLive(t *testing.T) {
	home := sandboxHome(t)
	jsoncPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, jsoncPath, `{
  "$schema": "https://opencode.ai/config.json",
  "theme": "keep-me",
  "agent": {
    "build": {"model": "1api/stale"},
    "compaction": {"model": "1api/stale-low"},
    "oracle": {"model": "1api/stale-high"}
  },
  "provider": {
    "myllm": {"name": "mine"}
  }
}`)

	c := Find("opencode")
	err := c.ApplyAuth(AuthSpec{
		Endpoint:  "https://proxy.example/v1",
		Key:       "sk-tier",
		Model:     "mid",
		AllModels: []string{"low", "mid", "high"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Theme      string `json:"theme"`
		Model      string `json:"model"`
		SmallModel string `json:"small_model"`
		Agent      map[string]struct {
			Model string `json:"model"`
		} `json:"agent"`
		Provider map[string]struct {
			Options struct {
				BaseURL string `json:"baseURL"`
				APIKey  string `json:"apiKey"`
			} `json:"options"`
			Models map[string]any `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("live not pure JSON: %v\n%s", err, data)
	}

	if cfg.Theme != "keep-me" {
		t.Errorf("theme = %q, want keep-me", cfg.Theme)
	}
	if cfg.Model != "1api/mid" {
		t.Errorf("model = %q, want 1api/mid", cfg.Model)
	}
	if cfg.SmallModel != "1api/low" {
		t.Errorf("small_model = %q, want 1api/low", cfg.SmallModel)
	}
	if cfg.Agent["build"].Model != "1api/mid" {
		t.Errorf("agent.build = %q", cfg.Agent["build"].Model)
	}
	if cfg.Agent["compaction"].Model != "1api/low" {
		t.Errorf("agent.compaction = %q", cfg.Agent["compaction"].Model)
	}
	if cfg.Agent["oracle"].Model != "1api/high" {
		t.Errorf("agent.oracle = %q, want 1api/high", cfg.Agent["oracle"].Model)
	}
	p, ok := cfg.Provider["1api"]
	if !ok {
		t.Fatal("1api provider missing")
	}
	if p.Options.BaseURL != "https://proxy.example/v1" || p.Options.APIKey != "sk-tier" {
		t.Errorf("options = %#v", p.Options)
	}
	for _, id := range []string{"low", "mid", "high"} {
		if _, ok := p.Models[id]; !ok {
			t.Errorf("models map missing %q: %#v", id, p.Models)
		}
	}
	if _, ok := cfg.Provider["myllm"]; !ok {
		t.Error("user provider myllm must survive")
	}

	// EnsureDefault path: Describe sees mid.
	info, err := c.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Model != "mid" || info.Endpoint != "https://proxy.example/v1" {
		t.Errorf("Describe = %#v", info)
	}
}

func TestOpenCodeApplyAuthCreatesCompactionWhenMissing(t *testing.T) {
	home := sandboxHome(t)
	jsoncPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, jsoncPath, `{"$schema":"https://opencode.ai/config.json"}`)

	c := Find("opencode")
	if err := c.ApplyAuth(AuthSpec{
		Endpoint:  "https://x/v1",
		Key:       "sk-x",
		Model:     "mid",
		AllModels: []string{"low", "mid", "high"},
	}); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		SmallModel string `json:"small_model"`
		Agent      map[string]struct {
			Model string `json:"model"`
		} `json:"agent"`
	}
	data, _ := os.ReadFile(jsoncPath)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SmallModel != "1api/low" {
		t.Errorf("small_model = %q", cfg.SmallModel)
	}
	if cfg.Agent["compaction"].Model != "1api/low" {
		t.Errorf("auto compaction = %#v", cfg.Agent)
	}
}
