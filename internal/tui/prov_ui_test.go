package tui

import (
	"testing"

	"1api/internal/profile"
	"1api/internal/provider"
)

func TestAppMenuHasThreeEntries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := profile.Open()
	if err != nil {
		t.Fatal(err)
	}
	m := newApp(s, "test")
	if m.view != aViewMenu {
		t.Fatalf("view = %v", m.view)
	}
	vals := map[string]bool{}
	for _, raw := range m.list.Items() {
		vals[raw.(item).value] = true
	}
	for _, want := range []string{aMenuProv, aMenuBind, aMenuSet} {
		if !vals[want] {
			t.Errorf("missing %q", want)
		}
	}
}

func TestAppProvidersListsSeeded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	s, err := profile.Open()
	if err != nil {
		t.Fatal(err)
	}
	ps, err := s.ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Upsert(provider.Spec{
		Name: "demo", Endpoint: "https://example.com/v1", Key: "sk-test",
		Model: "mid-model", SkipVerify: true,
	}, provider.UpsertOptions{SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	m := newApp(s, "test")
	m.view = aViewProviders
	m.loadProviders()
	found := false
	for _, raw := range m.list.Items() {
		if raw.(item).value == aProvPrefix+"demo" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected demo provider")
	}
	if !hasValueApp(m, aAddSentinel) {
		t.Fatal("missing add action")
	}
}

func TestTrDefaultZH(t *testing.T) {
	if tr(profile.LangZH, msgProviders) == "" {
		t.Fatal("empty zh")
	}
	if tr(profile.LangEN, msgProviders) != "Providers" {
		t.Fatalf("en = %q", tr(profile.LangEN, msgProviders))
	}
	if tr(profile.LangZH, msgProviders) != "供应商" {
		t.Fatalf("zh = %q", tr(profile.LangZH, msgProviders))
	}
}
