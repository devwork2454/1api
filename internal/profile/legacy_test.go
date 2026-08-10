package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportLegacyCharon_copiesProfilesAndMigratesProviders(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// Seed legacy charon tree with two tools sharing the same key/ep (dedupe)
	// and one unique key.
	legacy := filepath.Join(xdg, "charon")
	writeLegacyProfile(t, legacy, "opencode", "grok-api", Spec{
		Endpoint: "https://api.x.ai/v1", Key: "sk-shared", Model: "grok-4.5",
	})
	writeLegacyProfile(t, legacy, "pi", "grok", Spec{
		Endpoint: "https://api.x.ai/v1", Key: "sk-shared", Model: "grok-4.5",
	})
	writeLegacyProfile(t, legacy, "codex", "aliyun", Spec{
		Endpoint: "https://aliyun.example/v1", Key: "sk-aliyun", Model: "flash",
	})
	writeLegacyConfig(t, legacy, map[string]string{
		"opencode": "grok-api",
		"pi":       "grok",
		"codex":    "aliyun",
	})

	// 1api already has empty defaults + false-positive providersMigrated.
	apiRoot := filepath.Join(xdg, "1api")
	if err := os.MkdirAll(filepath.Join(apiRoot, "profiles", "opencode", "default"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(filepath.Join(apiRoot, "profiles", "opencode", "default"), Manifest{
		Label: "default", CreatedAt: time.Now(), Present: map[string]bool{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiRoot, "config.json"), []byte(`{
  "active": {"opencode": "default"},
  "providersMigrated": true
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if !s.readConfig().CharonImported {
		t.Fatal("CharonImported not set")
	}
	if !s.Exists("opencode", "grok-api") {
		t.Fatal("expected grok-api profile imported")
	}
	if !s.Exists("pi", "grok") {
		t.Fatal("expected pi/grok imported")
	}
	if !s.Exists("codex", "aliyun") {
		t.Fatal("expected codex/aliyun imported")
	}
	// existing default must remain
	if !s.Exists("opencode", "default") {
		t.Fatal("default should not be removed")
	}
	if s.Active("opencode") != "grok-api" {
		t.Fatalf("active opencode = %q, want grok-api (merged from charon)", s.Active("opencode"))
	}

	if err := s.MigrateProvidersOnce(); err != nil {
		t.Fatal(err)
	}
	ps, err := s.ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	names := ps.List()
	if len(names) < 2 {
		t.Fatalf("providers = %v, want ≥2 after fingerprint dedupe", names)
	}
	// shared key/ep → one provider; aliyun → another
	if len(names) != 2 {
		t.Fatalf("providers = %v, want exactly 2 (shared grok + aliyun)", names)
	}
	if s.ActiveProvider("opencode") == "" {
		t.Fatal("opencode should bind to migrated provider")
	}
	if s.ActiveProvider("pi") == "" {
		t.Fatal("pi should bind to migrated provider")
	}
	if s.ActiveProvider("opencode") != s.ActiveProvider("pi") {
		t.Fatalf("shared credential should bind same provider: %q vs %q",
			s.ActiveProvider("opencode"), s.ActiveProvider("pi"))
	}
	if s.ActiveProvider("codex") == "" || s.ActiveProvider("codex") == s.ActiveProvider("opencode") {
		t.Fatalf("aliyun should be distinct provider; codex=%q opencode=%q",
			s.ActiveProvider("codex"), s.ActiveProvider("opencode"))
	}

	s2, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.MigrateProvidersOnce(); err != nil {
		t.Fatal(err)
	}
	if len(s2.List("opencode")) != len(s.List("opencode")) {
		t.Fatal("re-open changed profile count")
	}
}

func TestImportLegacyCharon_skipsWhenNoLegacy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if !s.readConfig().CharonImported {
		t.Fatal("should mark imported even when no legacy dir")
	}
}

func TestMigrateProviders_rerunsWhenEmptyButSpecsExist(t *testing.T) {
	s := newStore(t)
	// Write profile with Spec without network
	dir := s.profDir("codex", "proxy")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(dir, Manifest{
		Label: "proxy", CreatedAt: time.Now(), Present: map[string]bool{},
		Spec: &Spec{Endpoint: "https://e/v1", Key: "sk-x", Model: "m"},
	}); err != nil {
		t.Fatal(err)
	}
	_ = s.setActive("codex", "proxy")
	// Pretend already migrated with empty providers
	c := s.readConfig()
	c.ProvidersMigrated = true
	if err := s.writeConfig(c); err != nil {
		t.Fatal(err)
	}
	if !s.needsProviderMigration() {
		t.Fatal("should need re-migration")
	}
	if err := s.MigrateProvidersOnce(); err != nil {
		t.Fatal(err)
	}
	ps, _ := s.ProviderStore()
	if len(ps.List()) != 1 {
		t.Fatalf("providers %v", ps.List())
	}
}

func writeLegacyProfile(t *testing.T, legacyRoot, tool, name string, sp Spec) {
	t.Helper()
	dir := filepath.Join(legacyRoot, "profiles", tool, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(dir, Manifest{
		Label: name, CreatedAt: time.Now(), Present: map[string]bool{"cfg": true},
		Spec: &sp,
	}); err != nil {
		t.Fatal(err)
	}
	// dummy artifact so copyDir has a file
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte(name), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeLegacyConfig(t *testing.T, legacyRoot string, active map[string]string) {
	t.Helper()
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(map[string]any{"active": active}, "", "  ")
	if err := os.WriteFile(filepath.Join(legacyRoot, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
