package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestModelsURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "https://api.openai.com/v1/models"},
		{"http://x", "http://x/v1/models"},
		{"http://x/", "http://x/v1/models"},
		{"http://x/v1", "http://x/v1/models"},
		{"http://x/v1/", "http://x/v1/models"},
		{"  http://x/v1  ", "http://x/v1/models"},
		{"http://x/openai/v1", "http://x/openai/v1/models"},
	}
	for _, tt := range tests {
		if got := modelsURL(tt.in); got != tt.want {
			t.Errorf("modelsURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// modelsHandler serves a fixed model list and records the auth headers seen.
func modelsHandler(t *testing.T, wantHeader, wantValue string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get(wantHeader); got != wantValue {
			t.Errorf("header %s = %q, want %q", wantHeader, got, wantValue)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "b-model"}, {"id": "a-model"}, {"id": ""}},
		})
	})
}

func TestFetchOpenAI(t *testing.T) {
	srv := httptest.NewServer(modelsHandler(t, "Authorization", "Bearer sk-test"))
	defer srv.Close()

	got, err := Fetch(OpenAI, srv.URL, "sk-test")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := []string{"a-model", "b-model"} // sorted, empty id dropped
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFetchAnthropic(t *testing.T) {
	srv := httptest.NewServer(modelsHandler(t, "x-api-key", "sk-ant"))
	defer srv.Close()

	got, err := Fetch(Anthropic, srv.URL+"/v1", "sk-ant")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 models, got %v", got)
	}
}

func TestFetchHTTPErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid_api_key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := Fetch(OpenAI, srv.URL, "bad")
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "401") {
		t.Errorf("error should mention status, got %q", msg)
	}
	if !strings.Contains(msg, "invalid_api_key") {
		t.Errorf("error should include body snippet, got %q", msg)
	}
}

func TestFetchEmptyKey(t *testing.T) {
	if _, err := Fetch(OpenAI, "https://example.com/v1", "  "); err == nil {
		t.Fatal("expected error on empty key")
	}
}

func TestFetchEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	if _, err := Fetch(OpenAI, srv.URL, "k"); err == nil {
		t.Fatal("expected error on empty model list, got nil")
	}
}

func TestSnippet(t *testing.T) {
	got := snippet([]byte("  hello\n  world  "), 100)
	if got != "hello world" {
		t.Errorf("snippet collapsed = %q", got)
	}
	long := strings.Repeat("a", 300)
	got = snippet([]byte(long), 10)
	if !strings.HasSuffix(got, "…") || !strings.HasPrefix(got, "aaaaaaaaaa") {
		t.Errorf("snippet truncate = %q", got)
	}
}
