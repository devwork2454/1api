package profile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"1api/internal/artifact"
	"1api/internal/provider"
	"1api/internal/tools"
)

func TestMigrateProvidersOnce(t *testing.T) {
	s := newStore(t)
	// Seed a fake profile with Spec without going through ApplyAuth network.
	tool := &tools.Tool{
		Name:            "codex",
		Title:           "Codex",
		Provider:        "openai",
		DefaultEndpoint: "https://api.openai.com/v1",
		Artifacts:       nil,
	}
	dir := s.profDir(tool.Name, "openrouter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		Label:     "openrouter",
		CreatedAt: time.Now(),
		Present:   map[string]bool{},
		Spec: &Spec{
			Endpoint: "https://openrouter.ai/api/v1",
			Key:      "sk-or-test",
			Model:    "openai/gpt-4",
		},
	}
	if err := writeManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := s.setActive(tool.Name, "openrouter"); err != nil {
		t.Fatal(err)
	}

	if err := s.MigrateProvidersOnce(); err != nil {
		t.Fatal(err)
	}
	if err := s.MigrateProvidersOnce(); err != nil {
		t.Fatal(err)
	}

	ps, err := s.ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	if !ps.Exists("openrouter") {
		t.Fatalf("providers: %v", ps.List())
	}
	rec, err := ps.Get("openrouter")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Key != "sk-or-test" || rec.Endpoint != "https://openrouter.ai/api/v1" {
		t.Fatalf("rec %+v", rec)
	}
	if !rec.NeedsVerify {
		t.Fatal("migrated provider should need verify")
	}
	if s.ActiveProvider("codex") != "openrouter" {
		t.Fatalf("binding %q", s.ActiveProvider("codex"))
	}
	if !s.readConfig().ProvidersMigrated {
		t.Fatal("flag not set")
	}
}

func TestApplyProviderWritesAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	cfg := filepath.Join(home, "tool.cfg")
	tool := &tools.Tool{
		Name:  "applier",
		Title: "Applier",
		Detected: func() bool {
			_, err := os.Stat(cfg)
			return err == nil
		},
		Artifacts: []artifact.Artifact{artifact.NewFile("config", cfg, 0o600)},
		ApplyAuth: func(a tools.AuthSpec) error {
			return os.WriteFile(cfg, []byte(a.Endpoint+"|"+a.Key+"|"+a.Model), 0o600)
		},
	}
	_ = os.WriteFile(cfg, []byte("old"), 0o600)

	s := newStore(t)
	ps, err := s.ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Upsert(provider.Spec{
		Name: "demo", Endpoint: "https://e/v1", Key: "sk", Model: "m1", SkipVerify: true,
	}, provider.UpsertOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := s.ApplyProvider(tool, "demo", true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cfg)
	if string(got) != "https://e/v1|sk|m1" {
		t.Fatalf("cfg %q", got)
	}
	if s.ActiveProvider("applier") != "demo" {
		t.Fatalf("bind %q", s.ActiveProvider("applier"))
	}
}
