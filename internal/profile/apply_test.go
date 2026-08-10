package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"1api/internal/tools"
)

func TestRefreshNoActiveProfileIsNoop(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	// No EnsureDefault/Apply has run yet, so nothing is active.
	if err := s.Refresh(tool); err != nil {
		t.Fatalf("Refresh with no active profile should be a no-op, got %v", err)
	}
}

func TestRefreshCapturesLiveChangeIntoActiveProfile(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := mergedToolWithDisplay(dir)
	write(t, cfg, `{"model":"claude-haiku","effortLevel":"low"}`)

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}

	// Live /model change with no explicit save and no profile switch.
	write(t, cfg, `{"model":"claude-opus","effortLevel":"high"}`)
	if err := s.Refresh(tool); err != nil {
		t.Fatal(err)
	}

	model, effort := s.ProfileModelEffort(tool, DefaultName)
	if model != "claude-opus" || effort != "high" {
		t.Errorf("after Refresh, default's captured model/effort = %q/%q, want claude-opus/high", model, effort)
	}
}

func TestApplyRejectsInvalidName(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	if _, err := s.Apply(tool, "../escape"); err == nil {
		t.Error("expected error applying an invalid profile name")
	}
}

func TestApplyMissingProfile(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	if _, err := s.Apply(tool, "nonexistent"); err == nil {
		t.Error("expected error applying a profile that was never saved")
	}
}

func TestDriftNoActiveProfile(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	drift, err := s.Drift(tool)
	if err != nil || drift {
		t.Errorf("Drift with no active profile = %v, %v; want false, nil", drift, err)
	}
}

func TestOpenCodeApplySyncsOmoModels(t *testing.T) {
	liveHome := t.TempDir()
	t.Setenv("HOME", liveHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(liveHome, ".config"))

	// Live opencode + omo before any profile.
	ocDir := filepath.Join(liveHome, ".config", "opencode")
	if err := os.MkdirAll(ocDir, 0o700); err != nil {
		t.Fatal(err)
	}
	liveCfg := []byte(`{"model":"1api/alpha","provider":{"1api":{"models":{"alpha":{"name":"alpha"}},"options":{"baseURL":"https://a","apiKey":"ska"}}}}`)
	if err := os.WriteFile(filepath.Join(ocDir, "opencode.jsonc"), liveCfg, 0o600); err != nil {
		t.Fatal(err)
	}
	omoPath := filepath.Join(liveHome, ".omo", "omo.jsonc")
	if err := os.MkdirAll(filepath.Dir(omoPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(omoPath, []byte(`{
  "[opencode]": {
    "agents": {
      "explore": {"model": "charon/stale"},
      "sisyphus": {"model": "charon/stale"}
    }
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s := newStore(t)
	tool := tools.Find("opencode")
	if tool == nil {
		t.Fatal("opencode missing")
	}
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}

	// Profile "beta" snapshot with different model.
	prof := s.profDir("opencode", "beta")
	if err := os.MkdirAll(prof, 0o700); err != nil {
		t.Fatal(err)
	}
	betaCfg := []byte(`{"model":"1api/beta","provider":{"1api":{"models":{"beta":{"name":"beta"}},"options":{"baseURL":"https://b","apiKey":"skb"}}}}`)
	if err := os.WriteFile(filepath.Join(prof, "opencode.jsonc"), betaCfg, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(prof, Manifest{
		Label:   "beta",
		Present: map[string]bool{"opencode.jsonc": true},
		Spec:    &Spec{Endpoint: "https://b", Key: "skb", Model: "beta"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Apply(tool, "beta"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	raw, err := os.ReadFile(omoPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("omo json: %v\n%s", err, raw)
	}
	agents := root["[opencode]"].(map[string]any)["agents"].(map[string]any)
	if agents["explore"].(map[string]any)["model"] != "1api/beta" {
		t.Errorf("explore after switch = %#v", agents["explore"])
	}
	if agents["sisyphus"].(map[string]any)["model"] != "1api/beta" {
		t.Errorf("sisyphus after switch = %#v", agents["sisyphus"])
	}
}
