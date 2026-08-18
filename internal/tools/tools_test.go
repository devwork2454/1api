package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"

	"1api/internal/artifact"
	"1api/internal/models"
)

// sandboxHome points HOME (and USER) at a temp dir so tool paths resolve there
// and no real user config is touched.
func sandboxHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USER", "tester")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindUnknown(t *testing.T) {
	if Find("nope") != nil {
		t.Error("expected nil for unknown tool")
	}
}

func TestCodexDescribeAndApply(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "model = \"gpt-5.5\"\n")
	writeFile(t, filepath.Join(home, ".codex", "auth.json"), `{"auth_mode":"chatgpt","tokens":{"access_token":"a"}}`)

	c := Find("codex")
	if !c.Detected() {
		t.Fatal("codex should be detected")
	}
	info, _ := c.Describe()
	// A ChatGPT OAuth login (auth_mode "chatgpt" on disk) surfaces as "oauth".
	if info.AuthMode != "oauth" {
		t.Errorf("authMode = %q, want oauth", info.AuthMode)
	}

	err := c.ApplyAuth(AuthSpec{Endpoint: "https://proxy/v1", Key: "sk-k123456789", Model: "gpt-5.5", SkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	info, _ = c.Describe()
	if info.Endpoint != "https://proxy/v1" || info.AuthMode != "api" || info.Secret != "sk-k123456789" {
		t.Errorf("after apply: %#v", info)
	}
	// The key must be embedded inline in config.toml, and the ChatGPT OAuth
	// tokens in auth.json must be left untouched.
	cfgData, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if !strings.Contains(string(cfgData), "experimental_bearer_token") {
		t.Error("config.toml missing inline bearer token")
	}
	var auth map[string]any
	data, _ := os.ReadFile(filepath.Join(home, ".codex", "auth.json"))
	_ = json.Unmarshal(data, &auth)
	if auth["auth_mode"] != "chatgpt" || auth["tokens"] == nil {
		t.Error("apply must not modify auth.json (ChatGPT OAuth)")
	}
}

func TestDetectedWithoutAuth(t *testing.T) {
	home := sandboxHome(t)
	t.Setenv("PATH", t.TempDir())

	for _, tc := range []struct {
		tool   string
		config string
	}{
		{tool: "codex", config: filepath.Join(home, ".codex", "config.toml")},
		{tool: "claude", config: filepath.Join(home, ".claude", "settings.json")},
		{tool: "opencode", config: filepath.Join(home, ".config", "opencode", "opencode.jsonc")},
		{tool: "pi", config: filepath.Join(home, ".pi", "agent", "settings.json")},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			writeFile(t, tc.config, "")
			if !Find(tc.tool).Detected() {
				t.Errorf("%s should be detected via config without auth", tc.tool)
			}
			if err := os.RemoveAll(filepath.Dir(tc.config)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEmptyConfigDirectoriesAreNotDetected(t *testing.T) {
	home := sandboxHome(t)
	t.Setenv("PATH", t.TempDir())

	for _, dir := range []string{
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".config", "opencode"),
		filepath.Join(home, ".pi", "agent"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"codex", "claude", "opencode", "pi"} {
		if Find(name).Detected() {
			t.Errorf("%s should not be detected via an empty config directory", name)
		}
	}
}

func TestCodexPinsClaudeContextWindow(t *testing.T) {
	home := sandboxHome(t)
	// A prior Claude profile pinned the window; switching to an OpenAI model
	// (which Codex already sizes from its catalog) must clear it.
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "model_context_window = 200000\n")

	c := Find("codex")
	window := func() (int64, bool) {
		var cfg struct {
			Window *int64 `toml:"model_context_window"`
		}
		data, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
		_ = toml.Unmarshal(data, &cfg)
		if cfg.Window == nil {
			return 0, false
		}
		return *cfg.Window, true
	}

	// A Claude model routed through the custom provider gets its window pinned
	// so Codex stops overrunning it with the 272K fallback.
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gw/v1", Key: "sk-k123456789", Model: "claude-opus-4-7", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	if w, ok := window(); !ok || w != 200000 {
		t.Errorf("claude model should pin model_context_window=200000, got %d (set=%v)", w, ok)
	}

	// Catalog-reported window wins over Claude heuristic (e.g. 1M gateway).
	if err := c.ApplyAuth(AuthSpec{
		Endpoint: "https://gw/v1", Key: "sk-k123456789", Model: "claude-opus-4-7", SkipVerify: true,
		ContextWindows: map[string]int{"claude-opus-4-7": 1_000_000},
	}); err != nil {
		t.Fatal(err)
	}
	if w, ok := window(); !ok || w != 1_000_000 {
		t.Errorf("catalog window should pin 1000000, got %d (set=%v)", w, ok)
	}

	// Non-Claude with catalog window is still pinned (gateway model).
	if err := c.ApplyAuth(AuthSpec{
		Endpoint: "https://gw/v1", Key: "sk-k123456789", Model: "deepseek-v4", SkipVerify: true,
		ContextWindows: map[string]int{"deepseek-v4": 65536},
	}); err != nil {
		t.Fatal(err)
	}
	if w, ok := window(); !ok || w != 65536 {
		t.Errorf("catalog non-claude should pin 65536, got %d (set=%v)", w, ok)
	}

	// Switching to a model Codex knows must drop the stale pin.
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gw/v1", Key: "sk-k123456789", Model: "gpt-5.5", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	if w, ok := window(); ok {
		t.Errorf("non-claude model must clear model_context_window, got %d", w)
	}
}

func TestCodexKeepsUserProvider(t *testing.T) {
	home := sandboxHome(t)
	// The user has their own custom Codex provider already configured.
	writeFile(t, filepath.Join(home, ".codex", "config.toml"),
		"[model_providers.myllm]\nname = \"mine\"\nbase_url = \"https://mine/v1\"\n")

	c := Find("codex")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gw/v1", Key: "sk-k123456789", Model: "gpt-x", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		ModelProviders map[string]struct {
			Name    string `toml:"name"`
			BaseURL string `toml:"base_url"`
		} `toml:"model_providers"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if p, ok := cfg.ModelProviders["myllm"]; !ok || p.BaseURL != "https://mine/v1" {
		t.Errorf("1api altered or removed the user's provider 'myllm': %#v", cfg.ModelProviders["myllm"])
	}
	if _, ok := cfg.ModelProviders["1api"]; !ok {
		t.Error("1api provider not added")
	}
}

func TestClaudeDescribeAndApply(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"theme":"dark"}`)

	c := Find("claude")
	if !c.Detected() {
		t.Fatal("claude should be detected via settings.json")
	}
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://api.anthropic.com", Key: "sk-ant-123456789", Model: "claude-opus-4-8", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	info, _ := c.Describe()
	if info.AuthMode != "api" || info.Model != "claude-opus-4-8" {
		t.Errorf("after apply: %#v", info)
	}
	var s struct {
		Theme string            `json:"theme"`
		Env   map[string]string `json:"env"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if s.Theme != "dark" {
		t.Error("apply dropped existing 'theme' setting")
	}
	if s.Env["ANTHROPIC_API_KEY"] != "sk-ant-123456789" {
		t.Errorf("stock endpoint should set ANTHROPIC_API_KEY, got env=%v", s.Env)
	}
	// The stock Anthropic endpoint must NOT set ANTHROPIC_BASE_URL — doing so
	// makes Claude Code treat it as a third-party gateway and disable connectors.
	if _, ok := s.Env["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("stock endpoint must not set ANTHROPIC_BASE_URL, got env=%v", s.Env)
	}
}

// TestClaudeGatewaySetsBothAuthEnvs locks in dual auth env keys for custom
// gateways: x-api-key-only gateways (e.g. OpenCode Go /v1/messages) reject
// Bearer, so ANTHROPIC_API_KEY must be written alongside ANTHROPIC_AUTH_TOKEN.
func TestClaudeGatewaySetsBothAuthEnvs(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"theme":"dark"}`)
	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://opencode.ai/zen/go/v1", Key: "sk-gw-123456789", Model: "kimi-k3", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	var s struct {
		Env map[string]string `json:"env"`
	}
	_ = json.Unmarshal(data, &s)
	if s.Env["ANTHROPIC_BASE_URL"] != "https://opencode.ai/zen/go" {
		t.Errorf("gateway base URL = %q", s.Env["ANTHROPIC_BASE_URL"])
	}
	if s.Env["ANTHROPIC_AUTH_TOKEN"] != "sk-gw-123456789" {
		t.Errorf("gateway should keep ANTHROPIC_AUTH_TOKEN, got env=%v", s.Env)
	}
	if s.Env["ANTHROPIC_API_KEY"] != "sk-gw-123456789" {
		t.Errorf("gateway should also set ANTHROPIC_API_KEY for x-api-key gateways, got env=%v", s.Env)
	}
	if s.Env["ANTHROPIC_MODEL"] != "kimi-k3" {
		t.Errorf("gateway model = %q", s.Env["ANTHROPIC_MODEL"])
	}
}

// TestClaudeSettingsMergeOwnsModelAndEffortButNotTheme locks in which settings.json
// keys switch per profile (so each account remembers its own model/effort) versus
// which stay a live, account-independent preference (theme).
func TestClaudeSettingsMergeOwnsModelAndEffortButNotTheme(t *testing.T) {
	sandboxHome(t)
	c := Find("claude")
	var merger artifact.Merger
	for _, a := range c.Artifacts {
		if a.ID() == "settings.json" {
			merger = a.(artifact.Merger)
		}
	}
	if merger == nil {
		t.Fatal("settings.json artifact should implement artifact.Merger")
	}

	snapshot := []byte(`{"model":"claude-haiku","effortLevel":"low","theme":"dark"}`)
	live := []byte(`{"model":"claude-opus","effortLevel":"medium","theme":"light"}`)

	merged, err := merger.Merge(snapshot, live)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "claude-haiku" {
		t.Errorf("model = %v, want claude-haiku (each profile keeps its own model)", got["model"])
	}
	if got["effortLevel"] != "low" {
		t.Errorf("effortLevel = %v, want low (each profile keeps its own effort)", got["effortLevel"])
	}
	if got["theme"] != "light" {
		t.Errorf("theme = %v, want light (theme is a live preference, not per-profile)", got["theme"])
	}
}

func TestClaudeApplyClearsStaleBaseURL(t *testing.T) {
	home := sandboxHome(t)
	// A previously-applied custom-gateway profile left a base URL behind.
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"env":{"ANTHROPIC_BASE_URL":"https://gateway.example/v1","ANTHROPIC_AUTH_TOKEN":"sk-gw-old"}}`)

	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://api.anthropic.com", Key: "sk-ant-123456789", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	var s struct {
		Env map[string]string `json:"env"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if _, ok := s.Env["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("switching to stock endpoint must clear stale ANTHROPIC_BASE_URL, got env=%v", s.Env)
	}
	if _, ok := s.Env["ANTHROPIC_AUTH_TOKEN"]; ok {
		t.Errorf("switching to stock endpoint must clear stale ANTHROPIC_AUTH_TOKEN, got env=%v", s.Env)
	}
}

func TestClaudeApplyApprovesAPIKey(t *testing.T) {
	home := sandboxHome(t)
	key := "sk-ant-abcdefghij0123456789" // last claudeKeyIDLen chars => "hij0123456789" padded
	id := key[len(key)-claudeKeyIDLen:]
	// Simulate a prior "No" that left this key disabled in Claude Code.
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"customApiKeyResponses":{"approved":[],"disabled":["`+id+`"]}}`)

	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://api.anthropic.com", Key: key, SkipVerify: true}); err != nil {
		t.Fatal(err)
	}

	var s struct {
		Resp struct {
			Approved []string `json:"approved"`
			Disabled []string `json:"disabled"`
		} `json:"customApiKeyResponses"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)

	if !contains(s.Resp.Approved, id) {
		t.Errorf("key id %q should be approved, got %v", id, s.Resp.Approved)
	}
	if contains(s.Resp.Disabled, id) {
		t.Errorf("key id %q must be removed from disabled, got %v", id, s.Resp.Disabled)
	}
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

func TestClaudeCustomEndpointUsesBearer(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{}`)

	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gateway.example/v1", Key: "sk-gw-123456789", Model: "some-model", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	var s struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if s.Env["ANTHROPIC_AUTH_TOKEN"] != "sk-gw-123456789" {
		t.Errorf("custom endpoint should keep ANTHROPIC_AUTH_TOKEN, got env=%v", s.Env)
	}
	if s.Env["ANTHROPIC_API_KEY"] != "sk-gw-123456789" {
		t.Errorf("custom endpoint should also set ANTHROPIC_API_KEY for x-api-key gateways, got env=%v", s.Env)
	}
	// Claude Code appends "/v1/messages" to the base URL, so a trailing "/v1"
	// (common in OpenAI-style gateway docs) is stripped to avoid "/v1/v1".
	if s.Env["ANTHROPIC_BASE_URL"] != "https://gateway.example" {
		t.Errorf("custom endpoint should set normalized ANTHROPIC_BASE_URL, got env=%v", s.Env)
	}
	if s.Env["ANTHROPIC_API_KEY"] != "sk-gw-123456789" {
		t.Error("custom endpoint should also set ANTHROPIC_API_KEY for x-api-key gateways")
	}
	if !bytes.Contains(data, []byte("customApiKeyResponses")) {
		t.Error("gateway key should be pre-approved in customApiKeyResponses")
	}
	// A custom gateway's model must go through ANTHROPIC_MODEL, not the
	// top-level "model" selector (which Claude Code validates against its
	// built-in catalog and rejects for unknown gateway models).
	if s.Env["ANTHROPIC_MODEL"] != "some-model" {
		t.Errorf("custom endpoint model should be in ANTHROPIC_MODEL, got env=%v", s.Env)
	}
	if s.Model != "" {
		t.Errorf("custom endpoint must not set top-level model, got %q", s.Model)
	}
}

func TestNormalizeClaudeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.reii.site/v1":  "https://api.reii.site",
		"https://api.reii.site/v1/": "https://api.reii.site",
		"https://api.reii.site":     "https://api.reii.site",
		"https://api.reii.site/":    "https://api.reii.site",
		"https://gw.example/v1beta": "https://gw.example/v1beta",
	}
	for in, want := range cases {
		if got := normalizeClaudeBaseURL(in); got != want {
			t.Errorf("normalizeClaudeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClaudeApplyClearsStaleTopLevelModel(t *testing.T) {
	home := sandboxHome(t)
	// A prior default-endpoint session left a top-level "opus" alias behind.
	// On a custom gateway that alias resolves to a model the gateway can't
	// serve, producing "model may not exist / no access".
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"opus"}`)

	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gateway.example/v1", Key: "sk-gw-123456789", Model: "gateway-model", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	var s struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if s.Model != "" {
		t.Errorf("switching to custom endpoint must clear stale top-level model, got %q", s.Model)
	}
	if s.Env["ANTHROPIC_MODEL"] != "gateway-model" {
		t.Errorf("custom endpoint model should be in ANTHROPIC_MODEL, got env=%v", s.Env)
	}
}

func TestOpenCodeDescribeAndApply(t *testing.T) {
	home := sandboxHome(t)
	authPath := filepath.Join(home, ".local", "share", "opencode", "auth.json")
	writeFile(t, authPath, `{"opencode":{"type":"api","key":"sk-existing"}}`)

	c := Find("opencode")
	if !c.Detected() {
		t.Fatal("opencode should be detected via auth.json")
	}
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://openrouter.ai/api/v1", Key: "sk-or-123456789", Model: "x/y", High: "x/z", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}

	// Verify Describe returns the correct model.
	info, err := c.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Model != "x/y" {
		t.Errorf("Describe model = %q, want %q", info.Model, "x/y")
	}

	// The provider (with key in options.apiKey and a models map) must be written
	// into the config. With no existing config file, 1api defaults to the
	// current opencode.jsonc name; auth.json must keep its existing login.
	var cfg struct {
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
		Provider        map[string]struct {
			Options struct {
				BaseURL string `json:"baseURL"`
				APIKey  string `json:"apiKey"`
			} `json:"options"`
			Models map[string]any `json:"models"`
		} `json:"provider"`
	}
	jsoncPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	cfgData, _ := os.ReadFile(jsoncPath)
	_ = json.Unmarshal(cfgData, &cfg)
	p, ok := cfg.Provider["1api"]
	if !ok {
		t.Fatal("provider '1api' not written to opencode.jsonc")
	}
	if p.Options.APIKey != "sk-or-123456789" {
		t.Errorf("apiKey not in options: %#v", p.Options)
	}
	if len(p.Models) == 0 {
		t.Error("models map is empty; models won't appear in opencode")
	}
	var auth map[string]any
	data, _ := os.ReadFile(authPath)
	_ = json.Unmarshal(data, &auth)
	if auth["opencode"] == nil {
		t.Error("apply must not drop existing login 'opencode' in auth.json")
	}

	// Manually write reasoningEffort to the config and check if Describe reads it.
	cfg.ReasoningEffort = "medium"
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsoncPath, cfgBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err = c.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Effort != "medium" {
		t.Errorf("Describe effort = %q, want %q", info.Effort, "medium")
	}

	// Manually write an agents block to test nested parsing of model and effort
	agentConfig := `{
		"$schema": "https://opencode.ai/config.json",
		"agents": {
			"coder": {
				"model": "1api/deepseek-v4",
				"reasoningEffort": "high"
			}
		}
	}`
	if err := os.WriteFile(jsoncPath, []byte(agentConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err = c.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Model != "deepseek-v4" {
		t.Errorf("Describe nested model = %q, want %q", info.Model, "deepseek-v4")
	}
	if info.Effort != "high" {
		t.Errorf("Describe nested effort = %q, want %q", info.Effort, "high")
	}
}

// TestOpenCodeAppliesModelRegistersIt reproduces the CLI path (no fetched model
// list) applying a new model id while the 1api provider already lists a
// different one: the active model must always be registered in the provider's
// models map, otherwise cfg["model"] points at a model opencode can't select and
// it silently falls back to its own default.
func TestOpenCodeAppliesModelRegistersIt(t *testing.T) {
	home := sandboxHome(t)
	jsoncPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	// Live config already has a 1api provider with one previously-registered model.
	writeFile(t, jsoncPath, `{
  "$schema": "https://opencode.ai/config.json",
  "model": "1api/deepseek-v4-flash",
  "provider": {
    "1api": {
      "name": "1api",
      "npm": "@ai-sdk/openai-compatible",
      "options": {"baseURL": "https://yunwu.ai", "apiKey": "sk-old"},
      "models": {"deepseek-v4-flash": {"name": "deepseek-v4-flash"}}
    }
  }
}`)

	c := Find("opencode")
	// CLI add/edit path: AllModels is empty, only a.Model is set to a new id.
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://yunwu.ai", Key: "sk-new", Model: "grok-4.5", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		Model    string `json:"model"`
		Provider map[string]struct {
			Models map[string]any `json:"models"`
		} `json:"provider"`
	}
	cfgData, _ := os.ReadFile(jsoncPath)
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "1api/grok-4.5" {
		t.Fatalf("model = %q, want %q", cfg.Model, "1api/grok-4.5")
	}
	p, ok := cfg.Provider["1api"]
	if !ok {
		t.Fatal("provider '1api' missing")
	}
	if _, ok := p.Models["grok-4.5"]; !ok {
		t.Errorf("active model %q not registered in provider models map %#v; opencode can't select it",
			"grok-4.5", p.Models)
	}
	// The previously-registered model is preserved (non-destructive).
	if _, ok := p.Models["deepseek-v4-flash"]; !ok {
		t.Errorf("previously-registered model %q was dropped from models map", "deepseek-v4-flash")
	}
}

func TestOpenCodeEditsExistingJsoncInPlace(t *testing.T) {
	home := sandboxHome(t)
	dir := filepath.Join(home, ".config", "opencode")
	jsonc := filepath.Join(dir, "opencode.jsonc")
	// The user already has their own provider configured in opencode.jsonc.
	writeFile(t, jsonc, `{"$schema":"https://opencode.ai/config.json","provider":{"myllm":{"name":"mine"}}}`)

	c := Find("opencode")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gw/v1", Key: "sk-abc", Model: "gpt-x", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}

	// 1api must edit the existing opencode.jsonc, not write a second
	// opencode.json that opencode would ignore.
	if _, err := os.Stat(filepath.Join(dir, "opencode.json")); err == nil {
		t.Error("1api wrote a stray opencode.json instead of editing opencode.jsonc")
	}

	var cfg struct {
		Provider map[string]json.RawMessage `json:"provider"`
	}
	data, _ := os.ReadFile(jsonc)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	// The user's original provider must survive untouched alongside 1api's.
	if _, ok := cfg.Provider["myllm"]; !ok {
		t.Error("1api removed the user's original provider 'myllm'")
	}
	if _, ok := cfg.Provider["1api"]; !ok {
		t.Error("1api provider not added to opencode.jsonc")
	}
}

func TestEnsureOnlyManagedChanged(t *testing.T) {
	provider := map[string]any{
		"myllm": map[string]any{"name": "mine"},
		"1api":  map[string]any{"name": "1api"},
	}
	original := snapshotProviders(provider) // captures myllm only

	// Updating only 1api is allowed.
	provider["1api"] = map[string]any{"name": "1api", "options": map[string]any{"baseURL": "x"}}
	if err := ensureOnlyManagedChanged(original, provider); err != nil {
		t.Errorf("changing only 1api must be allowed, got %v", err)
	}

	// Editing the user's provider is refused.
	edited := map[string]any{"myllm": map[string]any{"name": "hijacked"}}
	if err := ensureOnlyManagedChanged(original, edited); err == nil {
		t.Error("editing an original provider must be refused")
	}

	// Deleting the user's provider is refused.
	deleted := map[string]any{"1api": map[string]any{"name": "1api"}}
	if err := ensureOnlyManagedChanged(original, deleted); err == nil {
		t.Error("deleting an original provider must be refused")
	}
}

func TestNotDetectedInEmptyHome(t *testing.T) {
	sandboxHome(t)
	t.Setenv("PATH", t.TempDir())
	for _, name := range []string{"codex", "opencode", "pi"} {
		if Find(name).Detected() {
			t.Errorf("%s should not be detected in empty HOME", name)
		}
	}
}

func TestPiDescribeAndApply(t *testing.T) {
	home := sandboxHome(t)
	dir := filepath.Join(home, ".pi", "agent")
	writeFile(t, filepath.Join(dir, "settings.json"), `{}`)

	// Stub the live catalog as empty (like DeepSeek, whose /v1/models reports no
	// windows) and seed pi's cached remote catalog so ApplyAuth resolves the real
	// windows from models-store.json instead of the 128k placeholder.
	stubPiFetchInfo(t, func(models.Provider, string, string) ([]models.ModelInfo, error) {
		return []models.ModelInfo{}, nil
	})
	writeFile(t, filepath.Join(dir, "models-store.json"), `{
  "deepseek": {
    "models": [
      {"id": "x/y", "name": "x/y", "contextWindow": 200000},
      {"id": "x/z", "name": "x/z", "contextWindow": 1000000}
    ]
  }
}`)

	c := Find("pi")
	if !c.Detected() {
		t.Fatal("pi should be detected via settings.json")
	}
	if err := c.ApplyAuth(AuthSpec{
		Endpoint:   "https://openrouter.ai/api/v1",
		Key:        "sk-or-123456789",
		Model:      "x/y",
		High:       "x/z",
		AllModels:  []string{"x/y", "x/z"},
		SkipVerify: true,
	}); err != nil {
		t.Fatal(err)
	}

	info, err := c.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Model != "x/y" {
		t.Errorf("Describe model = %q, want %q", info.Model, "x/y")
	}
	if info.Endpoint != "https://openrouter.ai/api/v1" || info.Secret != "sk-or-123456789" || info.AuthMode != "api" {
		t.Errorf("Describe info = %+v, want endpoint/key/api set", info)
	}

	extensionPath := filepath.Join(dir, "extensions", "1api.ts")
	data, err := os.ReadFile(extensionPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, ok := piParseExtension(data)
	if !ok {
		t.Fatal("could not parse 1api.ts extension back out")
	}
	if len(cfg.Models) != 2 {
		t.Errorf("models = %v, want 2 entries", cfg.Models)
	}
	// Context windows must come from pi's cached catalog (models-store.json), not
	// the 128k placeholder.
	windows := map[string]int{}
	for _, m := range cfg.Models {
		windows[m.ID] = m.ContextWindow
	}
	if windows["x/y"] != 200_000 || windows["x/z"] != 1_000_000 {
		t.Errorf("extension context windows = %v, want x/y=200000 x/z=1000000", windows)
	}

	settingsPath := filepath.Join(dir, "settings.json")
	var s map[string]any
	sd, _ := os.ReadFile(settingsPath)
	_ = json.Unmarshal(sd, &s)
	if s["defaultProvider"] != "1api" || s["defaultModel"] != "x/y" {
		t.Errorf("settings.json = %v, want defaultProvider/defaultModel set to 1api/x/y", s)
	}

	// A rename/key-rotation call without AllModels must preserve the previously
	// registered model list rather than collapsing the /model picker to one entry.
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://openrouter.ai/api/v1", Key: "sk-or-999", Model: "x/y", High: "x/z", SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(extensionPath)
	cfg, _ = piParseExtension(data)
	if len(cfg.Models) != 2 {
		t.Errorf("models after re-apply = %v, want preserved 2 entries", cfg.Models)
	}

	// A live-set defaultThinkingLevel (a CLI preference, e.g. via pi's /settings)
	// must survive being merged back in by the profile store, matching Codex/OpenCode.
	s["defaultThinkingLevel"] = "high"
	sd, _ = json.Marshal(s)
	if err := os.WriteFile(settingsPath, sd, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err = c.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Effort != "high" {
		t.Errorf("Describe effort = %q, want %q", info.Effort, "high")
	}
}

// stubPiFetchInfo swaps the live-catalog fetcher for the duration of a test so
// ApplyAuth resolves context windows without touching the network.
func stubPiFetchInfo(t *testing.T, fn func(models.Provider, string, string) ([]models.ModelInfo, error)) {
	t.Helper()
	old := piFetchInfo
	piFetchInfo = fn
	t.Cleanup(func() { piFetchInfo = old })
}

func TestPiCatalogWindows(t *testing.T) {
	t.Run("known covering all ids skips the network", func(t *testing.T) {
		calls := 0
		stubPiFetchInfo(t, func(models.Provider, string, string) ([]models.ModelInfo, error) {
			calls++
			return nil, errors.New("must not be called")
		})
		known := map[string]int{"a": 1000, "b": 2000}
		got := piCatalogWindows([]string{"a", "b"}, "https://x/v1", "k", models.OpenAI, known)
		if calls != 0 {
			t.Errorf("catalog fetched %d times, want 0 (fast path)", calls)
		}
		if got["a"] != 1000 || got["b"] != 2000 {
			t.Errorf("windows = %v, want a=1000 b=2000", got)
		}
	})

	t.Run("live catalog fills gaps and overrides stale known values", func(t *testing.T) {
		stubPiFetchInfo(t, func(models.Provider, string, string) ([]models.ModelInfo, error) {
			return []models.ModelInfo{
				{ID: "a", ContextWindow: 5000},
				{ID: "c", ContextWindow: 3000},
			}, nil
		})
		got := piCatalogWindows([]string{"a", "b", "c"}, "https://x/v1", "k", models.OpenAI, map[string]int{"a": 1000, "b": 2000})
		if got["a"] != 5000 {
			t.Errorf("a = %d, want 5000 (live catalog wins)", got["a"])
		}
		if got["b"] != 2000 {
			t.Errorf("b = %d, want 2000 (known survives for unreported ids)", got["b"])
		}
		if got["c"] != 3000 {
			t.Errorf("c = %d, want 3000 (gap filled)", got["c"])
		}
	})

	t.Run("fetch failure keeps known unchanged", func(t *testing.T) {
		stubPiFetchInfo(t, func(models.Provider, string, string) ([]models.ModelInfo, error) {
			return nil, errors.New("offline")
		})
		known := map[string]int{"a": 1000}
		got := piCatalogWindows([]string{"a", "b"}, "https://x/v1", "k", models.OpenAI, known)
		if got["a"] != 1000 {
			t.Errorf("a = %d, want 1000", got["a"])
		}
		if _, ok := got["b"]; ok {
			t.Errorf("b unexpectedly present: %v", got)
		}
	})

	t.Run("nil known falls back to empty map", func(t *testing.T) {
		stubPiFetchInfo(t, func(models.Provider, string, string) ([]models.ModelInfo, error) {
			return nil, errors.New("offline")
		})
		got := piCatalogWindows([]string{"a"}, "https://x/v1", "k", models.OpenAI, nil)
		if len(got) != 0 {
			t.Errorf("windows = %v, want empty", got)
		}
	})
}

func TestPiReadStoredWindows(t *testing.T) {
	dir := t.TempDir()
	// Missing file → nil.
	if got := piReadStoredWindows(dir); got != nil {
		t.Errorf("missing file = %v, want nil", got)
	}
	writeFile(t, filepath.Join(dir, "models-store.json"), `{
  "deepseek": {
    "models": [
      {"id": "deepseek-v4-flash", "name": "x", "contextWindow": 1000000},
      {"id": "deepseek-v4-pro", "name": "y", "contextWindow": 0}
    ]
  },
  "google": {
    "models": [{"id": "gemini-3.1", "name": "g", "contextWindow": 1000000}]
  }
}`)
	got := piReadStoredWindows(dir)
	if got["deepseek-v4-flash"] != 1_000_000 {
		t.Errorf("deepseek-v4-flash = %d, want 1000000", got["deepseek-v4-flash"])
	}
	if _, ok := got["deepseek-v4-pro"]; ok {
		t.Errorf("zero-window model should be skipped: %v", got)
	}
	if got["gemini-3.1"] != 1_000_000 {
		t.Errorf("gemini-3.1 = %d, want 1000000", got["gemini-3.1"])
	}
	// Corrupt file → nil.
	writeFile(t, filepath.Join(dir, "models-store.json"), `{not json`)
	if got := piReadStoredWindows(dir); got != nil {
		t.Errorf("corrupt file = %v, want nil", got)
	}
}

func TestMergeWindows(t *testing.T) {
	got := mergeWindows(map[string]int{"a": 1000, "b": 2000}, map[string]int{"b": 0, "c": 3000})
	if got["a"] != 1000 || got["b"] != 2000 || got["c"] != 3000 {
		t.Errorf("merge = %v, want a=1000 b=2000 c=3000", got)
	}
	if got := mergeWindows(nil, nil); len(got) != 0 {
		t.Errorf("nil merge = %v, want empty", got)
	}
}

func TestPiOpenAIBaseURL(t *testing.T) {
	cases := []struct {
		endpoint string
		want     string
	}{
		{"https://api.deepseek.com/anthropic", "https://api.deepseek.com"},
		{"https://api.deepseek.com/anthropic/v1", "https://api.deepseek.com"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"http://127.0.0.1:4000/v1", "http://127.0.0.1:4000/v1"},
		{"https://llm.example.com/compatible-mode/v1", "https://llm.example.com/compatible-mode/v1"},
	}
	for _, tc := range cases {
		if got := piOpenAIBaseURL(tc.endpoint); got != tc.want {
			t.Errorf("piOpenAIBaseURL(%q) = %q, want %q", tc.endpoint, got, tc.want)
		}
	}
}

func TestPiBuiltinWindow(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"deepseek-v4-flash", 1_000_000},
		{"deepseek-v4-pro", 1_000_000},
		{"deepseek-v4-flash-0731", 1_000_000},
		{"glm-5.2", 1_000_000},
		{"z-ai/glm-5.2", 1_000_000},
		{"grok-4.5", 500_000},
		{"grok-4.20", 1_000_000},
		{"claude-sonnet-4.5", 0}, // Claude handled by claudeContextWindow, not builtin
		{"unknown-model-xyz", 0},
	}
	for _, tc := range cases {
		if got := piBuiltinWindow(tc.model); got != tc.want {
			t.Errorf("piBuiltinWindow(%q) = %d, want %d", tc.model, got, tc.want)
		}
	}
	// known overrides builtin; builtin beats the 128k default.
	if w := piContextWindow("glm-5.2", map[string]int{"glm-5.2": 200_000}); w != 200_000 {
		t.Errorf("known should override builtin, got %d", w)
	}
	if w := piContextWindow("deepseek-v4-flash-0731", nil); w != 1_000_000 {
		t.Errorf("builtin should beat default, got %d", w)
	}
	if w := piContextWindow("claude-opus-4-8", nil); w != 200_000 {
		t.Errorf("claude heuristic should apply, got %d", w)
	}
}

func TestPiPrimaryWire(t *testing.T) {
	cases := []struct {
		endpoint string
		want     models.Provider
	}{
		{"https://api.deepseek.com/anthropic", models.Anthropic},
		{"https://api.deepseek.com/anthropic/v1", models.Anthropic},
		{"https://api.openai.com/v1", models.OpenAI},
		{"http://127.0.0.1:4000/v1", models.OpenAI},
		{"https://openrouter.ai/api/v1", models.OpenAI},
	}
	for _, tc := range cases {
		if got := piPrimaryWire(tc.endpoint); got != tc.want {
			t.Errorf("piPrimaryWire(%q) = %v, want %v", tc.endpoint, got, tc.want)
		}
	}
	if piOtherWire(models.Anthropic) != models.OpenAI || piOtherWire(models.OpenAI) != models.Anthropic {
		t.Error("piOtherWire should return the opposite format")
	}
}

// TestOpenCodeApplyAuthWithJSONCComments ensures live opencode.jsonc with // comments
// still receives ApplyAuth overwrites (regression: pure json.Unmarshal failed silently
// or aborted, leaving manual config in place while only the profile store updated).
func TestOpenCodeApplyAuthWithJSONCComments(t *testing.T) {
	home := sandboxHome(t)
	jsoncPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, jsoncPath, `{
  "$schema": "https://opencode.ai/config.json",
  "theme": "manual-theme",
  "model": "1api/old",
  "provider": {
    "1api": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "1api",
      "options": {"baseURL": "https://old.example/v1", "apiKey": "sk-old"},
      "models": {"old": {"name": "old"}}
    }
  },
  "agent": {
    "compaction": {
      // OpenCode comment that breaks encoding/json
      "model": "1api/low"
    }
  }
}`)

	c := Find("opencode")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://new.example/v1", Key: "sk-new", Model: "fresh", SkipVerify: true}); err != nil {
		t.Fatalf("ApplyAuth with JSONC comments: %v", err)
	}

	var cfg struct {
		Theme    string `json:"theme"`
		Model    string `json:"model"`
		Provider map[string]struct {
			Options struct {
				BaseURL string `json:"baseURL"`
				APIKey  string `json:"apiKey"`
			} `json:"options"`
			Models map[string]any `json:"models"`
		} `json:"provider"`
	}
	data, err := os.ReadFile(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("live file should be pure JSON after ApplyAuth: %v\n%s", err, data)
	}
	if cfg.Model != "1api/fresh" {
		t.Errorf("model = %q, want 1api/fresh", cfg.Model)
	}
	p, ok := cfg.Provider["1api"]
	if !ok {
		t.Fatal("1api provider missing")
	}
	if p.Options.BaseURL != "https://new.example/v1" || p.Options.APIKey != "sk-new" {
		t.Errorf("provider options not overwritten: %#v", p.Options)
	}
	if _, ok := p.Models["fresh"]; !ok {
		t.Errorf("fresh model not registered: %#v", p.Models)
	}
	// Non-owned preference keys are not ApplyAuth's job; theme may be lost on rewrite
	// of the whole map via writeJSONMap (ApplyAuth loads full map then writes).
	// theme should survive load-merge because loadJSONMap keeps all keys.
	if cfg.Theme != "manual-theme" {
		t.Errorf("theme = %q, want manual-theme preserved", cfg.Theme)
	}

	info, err := c.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Model != "fresh" || info.Endpoint != "https://new.example/v1" {
		t.Errorf("Describe after JSONC apply: %#v", info)
	}
}
