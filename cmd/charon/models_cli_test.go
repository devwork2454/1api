package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charon/internal/profile"
	"charon/internal/tools"
)

func TestDiscoverModelsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"m2"},{"id":"m1"}]}`))
	}))
	t.Cleanup(srv.Close)

	tool := tools.Find("opencode")
	got := discoverModels(tool, srv.URL, "sk-test")
	if len(got) != 2 || got[0] != "m1" || got[1] != "m2" {
		t.Fatalf("got %v", got)
	}
}

func TestDiscoverModelsFailureIsSoft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	// Capture stderr so the warning doesn't pollute test output.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	got := discoverModels(tools.Find("codex"), srv.URL, "sk-bad")
	_ = w.Close()
	os.Stderr = old
	buf := make([]byte, 512)
	n, _ := r.Read(buf)
	_ = r.Close()
	if got != nil {
		t.Fatalf("want nil models on failure, got %v", got)
	}
	if !strings.Contains(string(buf[:n]), "could not list models") {
		t.Fatalf("expected warning on stderr, got %q", string(buf[:n]))
	}
}

func TestAddProfileCLISeedsOpenCodeModels(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("USER", "tester")

	// Live OpenCode config so Detected() is true.
	cfgPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"$schema":"https://opencode.ai/config.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"alpha"},{"id":"beta"},{"id":"gamma"}]}`))
	}))
	t.Cleanup(srv.Close)

	store, err := profile.Open()
	if err != nil {
		t.Fatal(err)
	}
	tool := tools.Find("opencode")
	if err := store.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}

	// Simulate cmdAdd's discover + AddProfile path.
	all := discoverModels(tool, srv.URL+"/v1", "sk-cli")
	if len(all) != 3 {
		t.Fatalf("discover got %v", all)
	}
	if err := store.AddProfile(tool, "proxy", profile.Spec{
		Endpoint: srv.URL + "/v1",
		Key:      "sk-cli",
		Model:    "beta",
	}, all...); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(body, id) {
			t.Errorf("opencode config missing model %q:\n%s", id, body)
		}
	}
	if !strings.Contains(body, `"@ai-sdk/openai-compatible"`) && !strings.Contains(body, "@ai-sdk/openai-compatible") {
		t.Errorf("expected openai-compatible npm package in config:\n%s", body)
	}
}
