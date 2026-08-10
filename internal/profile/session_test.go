package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"1api/internal/tools"
)

func TestMaterializeSessionCodexIsolatesFromLive(t *testing.T) {
	liveHome := t.TempDir()
	t.Setenv("HOME", liveHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(liveHome, ".config"))

	liveCfg := filepath.Join(liveHome, ".codex", "config.toml")
	liveAuth := filepath.Join(liveHome, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(liveCfg), 0o700); err != nil {
		t.Fatal(err)
	}
	// Marker content that must survive a session materialize.
	liveCfgBody := []byte("model = \"live-only\"\n")
	liveAuthBody := []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-live"}`)
	if err := os.WriteFile(liveCfg, liveCfgBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(liveAuth, liveAuthBody, 0o600); err != nil {
		t.Fatal(err)
	}

	// Build a profile dir the way snapshot would: artifacts + manifest.
	s := newStore(t)
	tool := tools.Find("codex")
	if tool == nil {
		t.Fatal("codex tool missing")
	}
	// Capture live as a named profile via Save path would work, but construct
	// a minimal snapshot to avoid ApplyAuth side effects.
	prof := s.profDir("codex", "work")
	if err := os.MkdirAll(prof, 0o700); err != nil {
		t.Fatal(err)
	}
	snapCfg := []byte("model = \"work-model\"\nmodel_provider = \"1api\"\n")
	snapAuth := []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-work"}`)
	if err := os.WriteFile(filepath.Join(prof, "config.toml"), snapCfg, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prof, "auth.json"), snapAuth, 0o600); err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		Label:   "work",
		Present: map[string]bool{"config.toml": true, "auth.json": true},
	}
	if err := writeManifest(prof, m); err != nil {
		t.Fatal(err)
	}

	sandbox := t.TempDir()
	if err := s.MaterializeSession(tool, "work", sandbox); err != nil {
		t.Fatalf("MaterializeSession: %v", err)
	}

	// Live untouched.
	got, err := os.ReadFile(liveCfg)
	if err != nil || string(got) != string(liveCfgBody) {
		t.Fatalf("live config changed: %q err=%v", got, err)
	}
	got, err = os.ReadFile(liveAuth)
	if err != nil || string(got) != string(liveAuthBody) {
		t.Fatalf("live auth changed: %q err=%v", got, err)
	}

	// Sandbox has profile contents.
	got, err = os.ReadFile(filepath.Join(sandbox, ".codex", "config.toml"))
	if err != nil || string(got) != string(snapCfg) {
		t.Fatalf("sandbox config = %q err=%v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(sandbox, ".codex", "auth.json"))
	if err != nil || string(got) != string(snapAuth) {
		t.Fatalf("sandbox auth = %q err=%v", got, err)
	}

}

func TestMaterializeSessionOpenCodePaths(t *testing.T) {
	liveHome := t.TempDir()
	t.Setenv("HOME", liveHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(liveHome, ".config"))

	hostOmo := filepath.Join(liveHome, ".omo", "omo.jsonc")
	if err := os.MkdirAll(filepath.Dir(hostOmo), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostOmo, []byte(`{
  "[opencode]": {
    "agents": {"explore": {"model": "charon/stale"}, "sisyphus": {"model": "charon/stale"}}
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s := newStore(t)
	tool := tools.Find("opencode")
	if tool == nil {
		t.Fatal("opencode tool missing")
	}
	prof := s.profDir("opencode", "proxy")
	if err := os.MkdirAll(prof, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := []byte(`{"model":"1api/gpt","provider":{"1api":{"models":{"gpt":{"name":"gpt"}},"options":{"baseURL":"https://x"}}}}`)
	auth := []byte(`{}`)
	if err := os.WriteFile(filepath.Join(prof, "opencode.jsonc"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prof, "auth.json"), auth, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(prof, Manifest{
		Label:   "proxy",
		Present: map[string]bool{"opencode.jsonc": true, "auth.json": true},
	}); err != nil {
		t.Fatal(err)
	}

	sandbox := t.TempDir()
	if err := s.MaterializeSession(tool, "proxy", sandbox); err != nil {
		t.Fatalf("MaterializeSession: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandbox, ".config", "opencode", "opencode.jsonc")); err != nil {
		t.Fatalf("missing opencode config in sandbox: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandbox, ".local", "share", "opencode", "auth.json")); err != nil {
		t.Fatalf("missing opencode auth in sandbox: %v", err)
	}
	sbOmo := filepath.Join(sandbox, ".omo", "omo.jsonc")
	raw, err := os.ReadFile(sbOmo)
	if err != nil {
		t.Fatalf("sandbox omo missing: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	agents := root["[opencode]"].(map[string]any)["agents"].(map[string]any)
	if agents["explore"].(map[string]any)["model"] != "1api/gpt" {
		t.Errorf("sandbox explore model = %#v", agents["explore"])
	}
	if agents["sisyphus"].(map[string]any)["model"] != "1api/gpt" {
		t.Errorf("sandbox sisyphus model = %#v", agents["sisyphus"])
	}
}

func TestMaterializeSessionRejectsUnsupported(t *testing.T) {
	s := newStore(t)
	claude := tools.Find("claude")
	if claude == nil {
		t.Fatal("claude missing")
	}
	err := s.MaterializeSession(claude, "default", t.TempDir())
	if err == nil {
		t.Fatal("expected error for claude session")
	}
}

func TestSessionEnvOpenCodeSetsXDG(t *testing.T) {
	env, err := tools.SessionEnv("opencode", "/tmp/sb")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"HOME=/tmp/sb":                       true,
		"XDG_CONFIG_HOME=/tmp/sb/.config":    true,
		"XDG_DATA_HOME=/tmp/sb/.local/share": true,
	}
	for _, e := range env {
		delete(want, e)
	}
	if len(want) != 0 {
		t.Fatalf("missing env entries: %v (have %v)", want, env)
	}
}
