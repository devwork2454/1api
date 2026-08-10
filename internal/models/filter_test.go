package models

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFilterReachableKeepsOnlyChatOK(t *testing.T) {
	var chats atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "good-a"},
					{"id": "bad-b"},
					{"id": "good-c"},
				},
			})
		case "/v1/chat/completions":
			chats.Add(1)
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.HasPrefix(body.Model, "bad") {
				http.Error(w, `{"error":"no"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	got, err := FilterReachable(OpenAI, srv.URL+"/v1", "sk", FilterOptions{
		Timeout:         30 * time.Second,
		PerModelTimeout: 5 * time.Second,
		Concurrency:     3,
		HTTPClient:      srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"good-a", "good-c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v (chats=%d)", got, want, chats.Load())
	}
}

func TestFilterReachableAllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "x"}, {"id": "y"}},
			})
			return
		}
		http.Error(w, "dead", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	_, err := FilterReachable(OpenAI, srv.URL, "sk", FilterOptions{
		HTTPClient:      srv.Client(),
		PerModelTimeout: 2 * time.Second,
	})
	if !errors.Is(err, ErrNoUsableModels) {
		t.Fatalf("want ErrNoUsableModels, got %v", err)
	}
	if err.Error() != "暂无可用模型" {
		t.Errorf("message = %q", err.Error())
	}
}

func TestFilterReachableCandidatesIntersect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "keep"},
					{"id": "drop-live"},
					{"id": "not-candidate"},
				},
			})
		case "/v1/chat/completions":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Model == "drop-live" {
				http.Error(w, "no", 404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	got, err := FilterReachable(OpenAI, srv.URL+"/v1", "sk", FilterOptions{
		Candidates: []string{"keep", "drop-live", "never-listed"},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"keep"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFilterReachableListUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	_, err := FilterReachable(OpenAI, srv.URL, "bad", FilterOptions{HTTPClient: srv.Client()})
	if err == nil || errors.Is(err, ErrNoUsableModels) {
		t.Fatalf("want list error, got %v", err)
	}
}
