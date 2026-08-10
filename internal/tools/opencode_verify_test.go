package tools

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"1api/internal/models"
)

func TestVerifyOpenCodeAuthLiveOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "gpt-flash"},
					{"id": "gpt-main"},
					{"id": "gpt-opus"},
					{"id": "dead-model"},
				},
			})
		case "/v1/chat/completions":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Model == "dead-model" {
				http.Error(w, "no", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	tiers, reach, err := VerifyOpenCodeAuth(srv.URL+"/v1", "sk-ok", "gpt-main",
		[]string{"gpt-flash", "gpt-main", "gpt-opus", "dead-model", "not-live"})
	if err != nil {
		t.Fatal(err)
	}
	if tiers.Mid != "gpt-main" {
		t.Errorf("mid = %q", tiers.Mid)
	}
	if tiers.Low != "gpt-flash" {
		t.Errorf("low = %q want gpt-flash", tiers.Low)
	}
	if tiers.High != "gpt-opus" {
		t.Errorf("high = %q want gpt-opus", tiers.High)
	}
	for _, id := range []string{"gpt-flash", "gpt-main", "gpt-opus"} {
		if !containsString(reach, id) {
			t.Errorf("reachable missing %s: %v", id, reach)
		}
	}
	if containsString(reach, "dead-model") || containsString(reach, "not-live") {
		t.Errorf("dead/not-live must be filtered: %v", reach)
	}
}

func TestVerifyOpenCodeAuthRejectsBadKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	_, _, err := VerifyOpenCodeAuth(srv.URL, "bad", "m", []string{"m"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyOpenCodeAuthAllChatFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "a"}, {"id": "b"}},
			})
			return
		}
		http.Error(w, "dead", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	_, _, err := VerifyOpenCodeAuth(srv.URL+"/v1", "sk", "a", nil)
	if !errors.Is(err, models.ErrNoUsableModels) {
		t.Fatalf("want ErrNoUsableModels, got %v", err)
	}
	if err.Error() != "暂无可用模型" {
		t.Errorf("msg = %q", err.Error())
	}
}

func TestVerifyOpenCodeAuthPrimaryMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "only"}},
			})
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	_, _, err := VerifyOpenCodeAuth(srv.URL+"/v1", "sk", "missing", []string{"missing"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyOpenCodeAuthEmptyKeyOffline(t *testing.T) {
	tiers, reach, err := VerifyOpenCodeAuth("https://x", "", "mid", []string{"low", "mid", "high"})
	if err != nil {
		t.Fatal(err)
	}
	if tiers.Mid != "mid" || tiers.Low != "low" || tiers.High != "high" {
		t.Fatalf("%+v", tiers)
	}
	if len(reach) != 3 {
		t.Fatalf("reach=%v", reach)
	}
}

func TestApplyAuthVerifyFailureDoesNotWrite(t *testing.T) {
	home := sandboxHome(t)
	jsoncPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, jsoncPath, `{"$schema":"https://opencode.ai/config.json","theme":"keep-me"}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := Find("opencode")
	err := c.ApplyAuth(AuthSpec{
		Endpoint: srv.URL + "/v1",
		Key:      "sk-bad",
		Model:    "mid",
	})
	if err == nil {
		t.Fatal("expected verify failure")
	}
	if !strings.Contains(err.Error(), "verify") {
		t.Errorf("error should mention verify: %v", err)
	}
	data, err := os.ReadFile(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"1api"`) {
		t.Errorf("must not write 1api provider on failed verify:\n%s", data)
	}
	if !strings.Contains(string(data), "keep-me") {
		t.Error("original config must remain")
	}
}

func TestApplyAuthVerifySuccessWritesTiers(t *testing.T) {
	home := sandboxHome(t)
	jsoncPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, jsoncPath, `{"$schema":"https://opencode.ai/config.json"}`)
	omoPath := filepath.Join(home, ".omo", "omo.jsonc")
	writeFile(t, omoPath, `{
  "[opencode]": {
    "agents": {
      "explore": {"model": "1api/old"},
      "sisyphus": {"model": "1api/old"}
    },
    "categories": {
      "quick": {"model": "1api/old"},
      "deep": {"model": "1api/old"}
    }
  }
}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "m-low-flash"},
					{"id": "m-mid"},
					{"id": "m-high-opus"},
				},
			})
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := Find("opencode")
	if err := c.ApplyAuth(AuthSpec{
		Endpoint:  srv.URL + "/v1",
		Key:       "sk-ok",
		Model:     "m-mid",
		AllModels: []string{"m-low-flash", "m-mid", "m-high-opus"},
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Model      string `json:"model"`
		SmallModel string `json:"small_model"`
		Provider   map[string]struct {
			Models map[string]any `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "1api/m-mid" {
		t.Errorf("model = %q", cfg.Model)
	}
	if cfg.SmallModel != "1api/m-low-flash" {
		t.Errorf("small_model = %q", cfg.SmallModel)
	}
	if _, ok := cfg.Provider["1api"].Models["m-mid"]; !ok {
		t.Errorf("models map = %#v", cfg.Provider["1api"].Models)
	}
	got := readOmoModels(t, omoPath)
	if got["agents.explore"] != "1api/m-low-flash" {
		t.Errorf("explore = %q", got["agents.explore"])
	}
	if got["agents.sisyphus"] != "1api/m-high-opus" {
		t.Errorf("sisyphus = %q", got["agents.sisyphus"])
	}
}

func TestApplyAuthAllUnusableNoWrite(t *testing.T) {
	home := sandboxHome(t)
	jsoncPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	writeFile(t, jsoncPath, `{"theme":"keep"}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "ghost"}},
			})
			return
		}
		http.Error(w, "nope", 503)
	}))
	t.Cleanup(srv.Close)

	err := Find("opencode").ApplyAuth(AuthSpec{
		Endpoint: srv.URL + "/v1",
		Key:      "sk",
		Model:    "ghost",
	})
	if err == nil || !strings.Contains(err.Error(), "暂无可用模型") {
		t.Fatalf("want 暂无可用模型, got %v", err)
	}
	data, _ := os.ReadFile(jsoncPath)
	if strings.Contains(string(data), "1api") {
		t.Fatalf("must not write: %s", data)
	}
}
