package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"1api/internal/profile"
	"1api/internal/provider"
)

func newTestStore(t *testing.T) *profile.Store {
	t.Helper()
	store, err := profile.Open()
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestProviderAddPreset(t *testing.T) {
	sandbox(t)
	err := run([]string{"provider", "add", "--name", "ocgo", "--preset", "ocgo", "--key", "sk-test-123456", "--no-verify"})
	if err != nil {
		t.Fatalf("provider add --preset ocgo: %v", err)
	}
	ps, err := newTestStore(t).ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	r, err := ps.Get("ocgo")
	if err != nil {
		t.Fatal(err)
	}
	if r.Endpoint != "https://opencode.ai/zen/go/v1" {
		t.Errorf("endpoint = %q, want opencode go gateway", r.Endpoint)
	}
	if r.Wire != provider.WireOpenAI {
		t.Errorf("wire = %q, want openai", r.Wire)
	}
}

func TestProviderAddPresetZen(t *testing.T) {
	sandbox(t)
	if err := run([]string{"provider", "add", "--name", "zen", "--preset", "zen", "--key", "sk-test-123456", "--no-verify"}); err != nil {
		t.Fatalf("provider add --preset zen: %v", err)
	}
	ps, err := newTestStore(t).ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	r, err := ps.Get("zen")
	if err != nil {
		t.Fatal(err)
	}
	if r.Endpoint != "https://opencode.ai/zen/v1" {
		t.Errorf("endpoint = %q, want zen gateway", r.Endpoint)
	}
}

func TestProviderAddPresetConflicts(t *testing.T) {
	sandbox(t)
	cases := [][]string{
		{"--name", "x", "--preset", "ocgo", "--endpoint", "https://example.com/v1", "--key", "sk-test-123456"},
		{"--name", "x", "--preset", "ocgo", "--wire", "openai", "--key", "sk-test-123456"},
		{"--name", "x", "--preset", "bogus", "--key", "sk-test-123456"},
	}
	for _, args := range cases {
		err := run(append([]string{"provider", "add"}, args...))
		if err == nil {
			t.Errorf("provider add %v = nil, want error", args)
			continue
		}
		want := "preset"
		if strings.Contains(strings.Join(args, " "), "bogus") {
			want = "unknown preset"
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("provider add %v = %v, want %q", args, err, want)
		}
	}
	cfg := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "1api", "providers")
	entries, err := os.ReadDir(cfg)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				t.Errorf("provider %q persisted despite error", e.Name())
			}
		}
	}
}
