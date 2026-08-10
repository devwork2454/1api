package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUpsertSkipVerifyAndGet(t *testing.T) {
	s := testStore(t)
	r, err := s.Upsert(Spec{
		Name:       "demo",
		Endpoint:   "https://example.com/v1",
		Key:        "sk-test",
		Model:      "gpt-test",
		SkipVerify: true,
	}, UpsertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Mid != "gpt-test" || r.Low != "gpt-test" || r.High != "gpt-test" {
		t.Fatalf("tiers %+v", r)
	}
	if !r.NeedsVerify {
		t.Fatal("expected NeedsVerify after skip")
	}

	// Pre-probed catalog: keep full usable and clear NeedsVerify.
	r2, err := s.Upsert(Spec{
		Name: "catalog", Endpoint: "https://c.example/v1", Key: "sk-c",
		Model: "mid-m", Usable: []string{"low-m", "mid-m", "high-m", "extra"},
		SkipVerify: true,
	}, UpsertOptions{SkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if r2.NeedsVerify {
		t.Fatal("catalog Upsert should not mark NeedsVerify")
	}
	if len(r2.Usable) != 4 {
		t.Fatalf("usable = %v", r2.Usable)
	}
	if r2.Mid != "mid-m" {
		t.Fatalf("mid = %s", r2.Mid)
	}
	got, err := s.Get("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "sk-test" || got.Endpoint != "https://example.com/v1" {
		t.Fatalf("got %+v", got)
	}
	// Permissions
	st, err := os.Stat(s.path("demo"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("perm %o want 0600", st.Mode().Perm())
	}
}

func TestUpsertRejectsUnsafeName(t *testing.T) {
	s := testStore(t)
	for _, name := range []string{"", "..", "a/b", "has space"} {
		if _, err := s.Upsert(Spec{Name: name, Key: "k", SkipVerify: true}, UpsertOptions{}); err == nil {
			t.Errorf("Upsert(%q) want error", name)
		}
	}
}

func TestSetTiersRejectsUnknownWithoutSkip(t *testing.T) {
	s := testStore(t)
	if _, err := s.Upsert(Spec{
		Name: "p", Key: "k", Endpoint: "https://x/v1", Model: "a",
		SkipVerify: true,
	}, UpsertOptions{}); err != nil {
		t.Fatal(err)
	}
	// Force usable known set without network: write usable=[a], NeedsVerify=false
	r, _ := s.Get("p")
	r.Usable = []string{"a"}
	r.NeedsVerify = false
	if err := s.write(r); err != nil {
		t.Fatal(err)
	}
	// Without HTTP, Probe will fail for unknown model
	if _, err := s.SetTiers("p", "not-there", "", "", UpsertOptions{}); err == nil {
		t.Fatal("expected reject unreachable model")
	}
}

func TestSetTiersAllowsWhenInUsable(t *testing.T) {
	s := testStore(t)
	if _, err := s.Upsert(Spec{
		Name: "p", Key: "k", Endpoint: "https://x/v1",
		Model: "mid-m", Low: "low-m", High: "high-m", SkipVerify: true,
	}, UpsertOptions{}); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Get("p")
	r.Usable = []string{"low-m", "mid-m", "high-m", "other"}
	r.NeedsVerify = false
	_ = s.write(r)

	got, err := s.SetTiers("p", "other", "low-m", "high-m", UpsertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mid != "other" || got.Low != "low-m" || got.High != "high-m" {
		t.Fatalf("got %+v", got)
	}
}

func TestUpsertWithFilterReachable(t *testing.T) {
	// httptest: list + chat OK for two models
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "m1"}, {"id": "m2-flash"}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	s := testStore(t)
	r, err := s.Upsert(Spec{
		Name:     "live",
		Endpoint: srv.URL + "/v1",
		Key:      "sk",
		Model:    "m1",
	}, UpsertOptions{HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if r.NeedsVerify {
		t.Fatal("should not need verify after live probe")
	}
	if len(r.Usable) != 2 {
		t.Fatalf("usable %v", r.Usable)
	}
	if r.Mid != "m1" {
		t.Fatalf("mid %q", r.Mid)
	}
	// flash → low
	if r.Low != "m2-flash" {
		t.Fatalf("low %q want m2-flash", r.Low)
	}
}

func TestUpsertRejectsUnreachablePrimary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "only"}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	s := testStore(t)
	_, err := s.Upsert(Spec{
		Name: "x", Endpoint: srv.URL + "/v1", Key: "k", Model: "missing",
	}, UpsertOptions{HTTPClient: srv.Client()})
	if err == nil || !strings.Contains(err.Error(), "not usable") {
		t.Fatalf("err = %v", err)
	}
}

func TestRemoveAndList(t *testing.T) {
	s := testStore(t)
	_, _ = s.Upsert(Spec{Name: "a", Key: "k", SkipVerify: true, Model: "m"}, UpsertOptions{})
	_, _ = s.Upsert(Spec{Name: "b", Key: "k", SkipVerify: true, Model: "m"}, UpsertOptions{})
	names := s.List()
	if len(names) != 2 {
		t.Fatalf("list %v", names)
	}
	if err := s.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if s.Exists("a") {
		t.Fatal("still exists")
	}
	// ensure dir layout
	if _, err := os.Stat(filepath.Join(s.Root, "providers")); err != nil {
		t.Fatal(err)
	}
}
