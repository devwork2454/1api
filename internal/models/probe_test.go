package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProbeListOnlyOpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-ok" {
			t.Errorf("auth = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "mid"}, {"id": "low"}},
		})
	}))
	t.Cleanup(srv.Close)

	got, err := Probe(OpenAI, srv.URL, "sk-ok", ProbeOptions{SkipChat: true})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	want := []string{"low", "mid"}
	if !reflect.DeepEqual(got.Models, want) {
		t.Errorf("Models = %v, want %v", got.Models, want)
	}
	if got.ChatOK {
		t.Error("ChatOK should be false when SkipChat")
	}
}

func TestProbeChatOpenAI(t *testing.T) {
	var sawChat bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "gpt-test"}},
			})
		case "/v1/chat/completions":
			sawChat = true
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["model"] != "gpt-test" {
				t.Errorf("chat model = %v", body["model"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "ok"}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	got, err := Probe(OpenAI, srv.URL+"/v1", "sk-ok", ProbeOptions{ChatModel: "gpt-test"})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !sawChat || !got.ChatOK || got.ChatModel != "gpt-test" {
		t.Fatalf("chat not exercised: saw=%v got=%+v", sawChat, got)
	}
}

func TestProbeChatModelMissingFromList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "only-a"}},
		})
	}))
	t.Cleanup(srv.Close)

	_, err := Probe(OpenAI, srv.URL, "sk", ProbeOptions{ChatModel: "missing"})
	if err == nil || !strings.Contains(err.Error(), "not in live list") {
		t.Fatalf("want missing-model error, got %v", err)
	}
}

func TestProbeUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	_, err := Probe(OpenAI, srv.URL, "bad", ProbeOptions{SkipChat: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "Unauthorized") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestProbeChatAnthropicPath(t *testing.T) {
	var sawMessages bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			if r.Header.Get("x-api-key") != "sk-ant" {
				t.Errorf("missing x-api-key")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "claude-test"}},
			})
		case "/v1/messages":
			sawMessages = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg_1", "type": "message"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	got, err := Probe(Anthropic, srv.URL+"/v1", "sk-ant", ProbeOptions{
		ChatModel:  "claude-test",
		Timeout:    5 * time.Second,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawMessages || !got.ChatOK {
		t.Fatalf("anthropic messages not hit: %+v", got)
	}
}

func TestProbeChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "m"}},
			})
			return
		}
		http.Error(w, `{"error":"quota"}`, http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	_, err := Probe(OpenAI, srv.URL, "sk", ProbeOptions{ChatModel: "m"})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("want 429, got %v", err)
	}
}

func TestChatURL(t *testing.T) {
	if got := chatURL(OpenAI, "http://x"); got != "http://x/v1/chat/completions" {
		t.Errorf("openai bare = %s", got)
	}
	if got := chatURL(OpenAI, "http://x/v1"); got != "http://x/v1/chat/completions" {
		t.Errorf("openai v1 = %s", got)
	}
	if got := chatURL(Anthropic, "http://x/v1"); got != "http://x/v1/messages" {
		t.Errorf("anthropic = %s", got)
	}
}
