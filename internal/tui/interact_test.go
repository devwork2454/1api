package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"1api/internal/artifact"
	"1api/internal/profile"
	"1api/internal/provider"
	"1api/internal/tools"

	tea "github.com/charmbracelet/bubbletea"
)

func keyEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyEsc() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyEsc} }
func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
func keyStr(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
func keyCtrlC() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlC} }

func send(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		m = flush(m, msg)
	}
	return m
}

func flush(m tea.Model, msg tea.Msg) tea.Model {
	var cmd tea.Cmd
	m, cmd = m.Update(msg)
	return drain(m, cmd, 8)
}

func drain(m tea.Model, cmd tea.Cmd, depth int) tea.Model {
	for cmd != nil && depth > 0 {
		msg := cmd()
		if msg == nil {
			break
		}
		m, cmd = m.Update(msg)
		depth--
	}
	return m
}

func typeText(m tea.Model, s string) tea.Model {
	for _, r := range s {
		m = send(m, keyRune(r))
	}
	return m
}

func resize(m tea.Model) tea.Model {
	return send(m, tea.WindowSizeMsg{Width: 100, Height: 40})
}

func sandboxStore(t *testing.T) *profile.Store {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	s, err := profile.Open()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func seedProvider(t *testing.T, s *profile.Store, name, ep, key, model string) {
	t.Helper()
	seedProviderModels(t, s, name, ep, key, model, nil)
}

func seedProviderModels(t *testing.T, s *profile.Store, name, ep, key, model string, usable []string) {
	t.Helper()
	ps, err := s.ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Upsert(provider.Spec{
		Name: name, Endpoint: ep, Key: key, Model: model, Usable: usable, SkipVerify: true,
	}, provider.UpsertOptions{SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
}

func mockOpenAIServer(t *testing.T, ids ...string) *httptest.Server {
	t.Helper()
	if len(ids) == 0 {
		ids = []string{"m1"}
	}
	data := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]string{"id": id})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
		case strings.Contains(r.URL.Path, "/chat/completions"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"content": "ok"}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mustApp(t *testing.T, m tea.Model) app {
	t.Helper()
	a, ok := m.(app)
	if !ok {
		t.Fatalf("got %T, want app", m)
	}
	return a
}

func selectValueApp(m *app, value string) {
	for i, raw := range m.list.Items() {
		if it, ok := raw.(item); ok && it.value == value {
			m.list.Select(i)
			return
		}
	}
}

func hasValueApp(m app, value string) bool {
	for _, raw := range m.list.Items() {
		if it, ok := raw.(item); ok && it.value == value {
			return true
		}
	}
	return false
}

func bindableTool(t *testing.T, home string) *tools.Tool {
	t.Helper()
	cfg := filepath.Join(home, "bind-tool.cfg")
	if err := os.WriteFile(cfg, []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &tools.Tool{
		Name:  "bindtool",
		Title: "BindTool",
		Detected: func() bool {
			_, err := os.Stat(cfg)
			return err == nil
		},
		Artifacts: []artifact.Artifact{artifact.NewFile("config", cfg, 0o600)},
		ApplyAuth: func(a tools.AuthSpec) error {
			return os.WriteFile(cfg, []byte(a.Endpoint+"|"+a.Key+"|"+a.Model+"|"+a.Low+"|"+a.High), 0o600)
		},
	}
}

func TestApp_OpensMainMenuZH(t *testing.T) {
	s := sandboxStore(t)
	a := mustApp(t, resize(newApp(s, "vtest")))
	if a.view != aViewMenu {
		t.Fatalf("view = %v, want menu", a.view)
	}
	if a.lang != profile.LangZH {
		t.Fatalf("lang = %q, want zh", a.lang)
	}
	for _, want := range []string{aMenuProv, aMenuBind, aMenuSet} {
		if !hasValueApp(a, want) {
			t.Errorf("menu missing %q", want)
		}
	}
}

func TestApp_MenuEsc_quits(t *testing.T) {
	s := sandboxStore(t)
	a := mustApp(t, resize(newApp(s, "vtest")))
	_, cmd := a.Update(keyEsc())
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestApp_ProvidersListAndAddOffline(t *testing.T) {
	s := sandboxStore(t)
	a := mustApp(t, resize(newApp(s, "vtest")))
	selectValueApp(&a, aMenuProv)
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewProviders {
		t.Fatalf("view = %v", a.view)
	}
	selectValueApp(&a, aAddSentinel)
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewAddEndpoint {
		t.Fatalf("view = %v, want endpoint", a.view)
	}
	a = mustApp(t, send(a, keyEnter())) // blank endpoint
	if a.view != aViewAddKey {
		t.Fatalf("view = %v, want key", a.view)
	}
	a = mustApp(t, typeText(a, "sk-test-key"))
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewAddWire {
		t.Fatalf("view = %v, want wire", a.view)
	}
	a = mustApp(t, send(a, keyEnter())) // openai
	if a.view != aViewAddName {
		t.Fatalf("view = %v, want name", a.view)
	}
	a = mustApp(t, typeText(a, "demo-prov"))
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewFetching {
		t.Fatalf("view = %v, want fetching", a.view)
	}
	a = mustApp(t, send(a, fetchedMsg{err: errString("offline")}))
	if a.pending != nil {
		a = mustApp(t, send(a, minLoadElapsedMsg{}))
	}
	if a.view != aViewProviders {
		t.Fatalf("view = %v after add; status=%q", a.view, a.status)
	}
	ps, err := s.ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	if !ps.Exists("demo-prov") {
		t.Fatal("provider not saved")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestApp_DeleteBlockedWhenBound(t *testing.T) {
	s := sandboxStore(t)
	home := os.Getenv("HOME")
	seedProvider(t, s, "used", "https://u.example/v1", "sk-u", "m1")
	tool := bindableTool(t, home)
	if err := s.ApplyProvider(tool, "used", true); err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, resize(newApp(s, "vtest")))
	a.allTools = []*tools.Tool{tool}
	selectValueApp(&a, aMenuProv)
	a = mustApp(t, send(a, keyEnter()))
	a.loadProviders()
	selectValueApp(&a, aProvPrefix+"used")
	a = mustApp(t, send(a, keyStr("d")))
	if a.view == aViewConfirmDel {
		t.Fatal("delete confirm should not open when bound")
	}
	if a.statusLvl != statusErr {
		t.Fatalf("statusLvl = %v status=%q", a.statusLvl, a.status)
	}
}

func TestApp_DeleteUnusedProvider(t *testing.T) {
	s := sandboxStore(t)
	seedProvider(t, s, "free", "https://f.example/v1", "sk-f", "m1")
	a := mustApp(t, resize(newApp(s, "vtest")))
	selectValueApp(&a, aMenuProv)
	a = mustApp(t, send(a, keyEnter()))
	selectValueApp(&a, aProvPrefix+"free")
	a = mustApp(t, send(a, keyStr("d")))
	if a.view != aViewConfirmDel {
		t.Fatalf("view = %v, want confirm", a.view)
	}
	a = mustApp(t, send(a, keyStr("y")))
	ps, _ := s.ProviderStore()
	if ps.Exists("free") {
		t.Fatal("provider still exists")
	}
	if a.view != aViewProviders {
		t.Fatalf("view = %v", a.view)
	}
}

func TestApp_BindTool_setsTiers(t *testing.T) {
	s := sandboxStore(t)
	home := os.Getenv("HOME")
	seedProviderModels(t, s, "p1", "https://p1.example/v1", "sk-p1", "mid-m",
		[]string{"mid-m", "low-m", "high-m"})
	ps, _ := s.ProviderStore()
	if _, err := ps.SetTiers("p1", "mid-m", "low-m", "high-m", provider.UpsertOptions{SkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	tool := bindableTool(t, home)

	a := mustApp(t, resize(newApp(s, "vtest")))
	a.allTools = []*tools.Tool{tool}
	selectValueApp(&a, aMenuBind)
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewBindings {
		t.Fatalf("view = %v", a.view)
	}
	selectValueApp(&a, aToolPrefix+tool.Name)
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewPickProv {
		t.Fatalf("view = %v, want pickProv", a.view)
	}
	selectValueApp(&a, aProvPrefix+"p1")
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewPickMid {
		t.Fatalf("view = %v, want pickMid status=%q", a.view, a.status)
	}
	// pick mid
	selectValueApp(&a, "mid-m")
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewPickLow {
		t.Fatalf("view = %v, want pickLow", a.view)
	}
	selectValueApp(&a, "low-m")
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewPickHigh {
		t.Fatalf("view = %v, want pickHigh", a.view)
	}
	selectValueApp(&a, "high-m")
	a = mustApp(t, send(a, keyEnter()))
	if s.ActiveProvider(tool.Name) != "p1" {
		t.Fatalf("bind = %q status=%q", s.ActiveProvider(tool.Name), a.status)
	}
	rec, err := ps.Get("p1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Mid != "mid-m" || rec.Low != "low-m" || rec.High != "high-m" {
		t.Fatalf("tiers = mid=%s low=%s high=%s", rec.Mid, rec.Low, rec.High)
	}
	got, err := os.ReadFile(filepath.Join(home, "bind-tool.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "mid-m") || !strings.Contains(string(got), "low-m") {
		t.Fatalf("auth file %q", got)
	}
	if a.view != aViewBindings {
		t.Fatalf("view after bind = %v", a.view)
	}
}

func TestApp_Settings_langAndSkin(t *testing.T) {
	s := sandboxStore(t)
	a := mustApp(t, resize(newApp(s, "vtest")))
	selectValueApp(&a, aMenuSet)
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewSettings {
		t.Fatalf("view = %v", a.view)
	}
	selectValueApp(&a, aLangSentinel)
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewSetLang {
		t.Fatalf("view = %v", a.view)
	}
	selectValueApp(&a, profile.LangEN)
	a = mustApp(t, send(a, keyEnter()))
	if s.UILang() != profile.LangEN {
		t.Fatalf("UILang = %q", s.UILang())
	}
	if a.lang != profile.LangEN {
		t.Fatalf("app.lang = %q", a.lang)
	}
	selectValueApp(&a, aSkinSentinel)
	a = mustApp(t, send(a, keyEnter()))
	selectValueApp(&a, profile.SkinWarm)
	a = mustApp(t, send(a, keyEnter()))
	if s.UISkin() != profile.SkinWarm {
		t.Fatalf("UISkin = %q", s.UISkin())
	}
}

func TestApp_ModelPick_marksCurrent(t *testing.T) {
	s := sandboxStore(t)
	a := mustApp(t, resize(newApp(s, "vtest")))
	a.allModels = []string{"alpha", "beta", "gamma"}
	a.mid, a.low, a.high = "beta", "alpha", "gamma"
	a.view = aViewPickMid
	a.showModelPick(msgPickMid, false)
	it, ok := a.list.SelectedItem().(item)
	if !ok || it.value != "beta" || !it.active {
		t.Fatalf("mid select = %+v ok=%v", it, ok)
	}
	if !strings.HasPrefix(it.title, "✓ ") {
		t.Fatalf("title = %q, want ✓ prefix", it.title)
	}
	a.view = aViewPickLow
	a.showModelPick(msgPickLow, true)
	it, ok = a.list.SelectedItem().(item)
	if !ok || it.value != "alpha" || !it.active {
		t.Fatalf("low select = %+v", it)
	}
}

func TestApp_EditProvider_keepsKeyAndUpdatesEndpoint(t *testing.T) {
	s := sandboxStore(t)
	home := os.Getenv("HOME")
	srv := mockOpenAIServer(t, "m1", "m2")
	newEP := srv.URL + "/v1"
	seedProvider(t, s, "editme", "https://old.example/v1", "sk-old-key-123456", "m1")
	tool := bindableTool(t, home)
	if err := s.ApplyProvider(tool, "editme", true); err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, resize(newApp(s, "vtest")))
	a.allTools = []*tools.Tool{tool}
	selectValueApp(&a, aMenuProv)
	a = mustApp(t, send(a, keyEnter()))
	selectValueApp(&a, aProvPrefix+"editme")
	a = mustApp(t, send(a, keyStr("e")))
	if a.view != aViewEditEndpoint {
		t.Fatalf("view = %v, want edit endpoint", a.view)
	}
	if a.input.Value() != "https://old.example/v1" {
		t.Fatalf("endpoint prefill = %q", a.input.Value())
	}
	a.input.SetValue("")
	a = mustApp(t, typeText(a, newEP))
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewEditKey {
		t.Fatalf("view = %v, want edit key", a.view)
	}
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewEditWire {
		t.Fatalf("view = %v, want edit wire", a.view)
	}
	a = mustApp(t, send(a, keyEnter()))
	if a.view != aViewProviders {
		t.Fatalf("view = %v status=%q", a.view, a.status)
	}
	if a.statusLvl == statusErr {
		t.Fatalf("edit failed: %q", a.status)
	}
	ps, err := s.ProviderStore()
	if err != nil {
		t.Fatal(err)
	}
	r, err := ps.Get("editme")
	if err != nil {
		t.Fatal(err)
	}
	if r.Endpoint != newEP {
		t.Fatalf("endpoint = %q want %q", r.Endpoint, newEP)
	}
	if r.Key != "sk-old-key-123456" {
		t.Fatalf("key changed unexpectedly")
	}
	got, err := os.ReadFile(filepath.Join(home, "bind-tool.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), newEP) {
		t.Fatalf("bound tool not re-applied: %q", got)
	}
}

func TestApp_CtrlC_quits(t *testing.T) {
	s := sandboxStore(t)
	a := mustApp(t, resize(newApp(s, "vtest")))
	_, cmd := a.Update(keyCtrlC())
	if cmd == nil {
		t.Fatal("expected quit")
	}
}

func TestApp_ProvidersEsc_backToMenu(t *testing.T) {
	s := sandboxStore(t)
	a := mustApp(t, resize(newApp(s, "vtest")))
	selectValueApp(&a, aMenuProv)
	a = mustApp(t, send(a, keyEnter(), keyEsc()))
	if a.view != aViewMenu {
		t.Fatalf("view = %v", a.view)
	}
}
