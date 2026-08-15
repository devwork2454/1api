package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseModelCatalogContextFields(t *testing.T) {
	raw := []byte(`{
		"data": [
			{"id": "a", "context_length": 131072},
			{"id": "b", "max_model_len": 32768},
			{"id": "c", "top_provider": {"context_length": 200000}},
			{"id": "d", "max_tokens": 8192},
			{"id": ""},
			{"id": "e"}
		]
	}`)
	got, err := parseModelCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	// sorted by id
	want := []ModelInfo{
		{ID: "a", ContextWindow: 131072},
		{ID: "b", ContextWindow: 32768},
		{ID: "c", ContextWindow: 200000},
		{ID: "d", ContextWindow: 0}, // max_tokens ignored
		{ID: "e", ContextWindow: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	wm := ContextWindowMap(got)
	if wm["a"] != 131072 || wm["c"] != 200000 || len(wm) != 3 {
		t.Fatalf("windows %v", wm)
	}
}

func TestFetchInfoParsesWindows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "z-model", "context_window": 64000},
				{"id": "a-model", "context_length": "128000"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	infos, err := FetchInfo(OpenAI, srv.URL, "sk")
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].ID != "a-model" || infos[0].ContextWindow != 128000 {
		t.Fatalf("infos = %+v", infos)
	}
	if infos[1].ID != "z-model" || infos[1].ContextWindow != 64000 {
		t.Fatalf("infos = %+v", infos)
	}
	ids, err := Fetch(OpenAI, srv.URL, "sk")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{"a-model", "z-model"}) {
		t.Fatalf("ids %v", ids)
	}
}

func TestFilterReachableDetailKeepsWindows(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "ok", "context_length": 100000},
				{"id": "bad", "context_length": 50000},
			},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model == "bad" {
			http.Error(w, "nope", http.StatusBadRequest)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := FilterReachableDetail(OpenAI, srv.URL+"/v1", "sk", FilterOptions{
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Usable, []string{"ok"}) {
		t.Fatalf("usable %v", res.Usable)
	}
	if res.ContextWindows["ok"] != 100000 || len(res.ContextWindows) != 1 {
		t.Fatalf("windows %v", res.ContextWindows)
	}
}
