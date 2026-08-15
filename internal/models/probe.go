package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProbeResult is the outcome of a connectivity check against an endpoint + key.
type ProbeResult struct {
	// Models is the live model id list from GET .../models (sorted by Fetch).
	Models []string
	// ChatOK is true when a minimal chat/messages round-trip succeeded for ChatModel.
	ChatOK bool
	// ChatModel is the model id used for the chat probe (empty if skipped).
	ChatModel string
}

// ProbeOptions controls Probe.
type ProbeOptions struct {
	// Timeout bounds the whole probe (list + optional chat). Zero → 20s.
	Timeout time.Duration
	// ChatModel, when non-empty, runs a minimal completion against that id.
	// When empty, only the models list is checked.
	ChatModel string
	// SkipChat disables the chat round-trip even if ChatModel is set.
	SkipChat bool
	// HTTPClient overrides the client used for requests (tests inject httptest).
	HTTPClient *http.Client
}

// Probe verifies the endpoint accepts the key (models list) and optionally that
// ChatModel can run a one-token completion. It never logs the key.
func Probe(provider Provider, endpoint, key string, opt ProbeOptions) (ProbeResult, error) {
	if strings.TrimSpace(key) == "" {
		return ProbeResult{}, fmt.Errorf("probe: empty API key")
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	client := opt.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	infos, err := fetchInfoWithClient(client, provider, endpoint, key, timeout)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("probe list: %w", err)
	}
	ids := ModelIDs(infos)
	out := ProbeResult{Models: ids}

	chatModel := strings.TrimSpace(opt.ChatModel)
	if opt.SkipChat || chatModel == "" {
		return out, nil
	}
	// Chat model should appear in the live list when the provider enumerates it.
	if !containsID(ids, chatModel) {
		return out, fmt.Errorf("probe chat: model %q not in live list (%d models)", chatModel, len(ids))
	}
	if err := probeChat(client, provider, endpoint, key, chatModel, timeout); err != nil {
		return out, fmt.Errorf("probe chat %s: %w", chatModel, err)
	}
	out.ChatOK = true
	out.ChatModel = chatModel
	return out, nil
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// fetchInfoWithClient is FetchInfo with an injectable client and overall timeout context.
func fetchInfoWithClient(client *http.Client, provider Provider, endpoint, key string, timeout time.Duration) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var last error
	for _, listURL := range listURLCandidates(provider, endpoint) {
		infos, err := fetchModelsOnce(ctx, client, provider, listURL, key)
		if err == nil {
			return infos, nil
		}
		last = err
		if ctx.Err() != nil {
			return nil, err
		}
	}
	if last == nil {
		return nil, fmt.Errorf("no models returned by %s", endpoint)
	}
	return nil, last
}

// fetchWithClient returns only model ids (legacy helper for call sites).
func fetchWithClient(client *http.Client, provider Provider, endpoint, key string, timeout time.Duration) ([]string, error) {
	infos, err := fetchInfoWithClient(client, provider, endpoint, key, timeout)
	if err != nil {
		return nil, err
	}
	return ModelIDs(infos), nil
}

func fetchModelsOnce(ctx context.Context, client *http.Client, provider Provider, listURL, key string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req, provider, key)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API returned %s (check endpoint and key)", resp.Status)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read model list: %w", err)
	}
	infos, err := parseModelCatalog(raw)
	if err != nil {
		if err == errEmptyCatalog {
			return nil, fmt.Errorf("no models returned by %s", listURL)
		}
		return nil, fmt.Errorf("could not parse model list: %w", err)
	}
	return infos, nil
}

func setAuthHeaders(req *http.Request, provider Provider, key string) {
	switch provider {
	case Anthropic:
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

// chatURL builds OpenAI-compatible chat completions or Anthropic messages URL.
func chatURL(provider Provider, endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if provider == Anthropic {
		if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1/") {
			return base + "/messages"
		}
		return base + "/v1/messages"
	}
	if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1/") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

// chatURLCandidates is the chat URLs to try, primary first.
// DeepSeek-style hosts expose Messages only under /anthropic when the stored
// endpoint is the OpenAI origin.
func chatURLCandidates(provider Provider, endpoint string) []string {
	primary := chatURL(provider, endpoint)
	out := []string{primary}
	if provider != Anthropic {
		return out
	}
	u, ok := parseEndpoint(endpoint)
	if !ok {
		return out
	}
	p := strings.TrimRight(u.Path, "/")
	p = strings.TrimSuffix(p, "/v1")
	p = strings.TrimRight(p, "/")
	if strings.HasSuffix(p, "/anthropic") {
		return out
	}
	u.Path = p + "/anthropic"
	u.RawQuery = ""
	u.Fragment = ""
	alt := strings.TrimRight(u.String(), "/") + "/v1/messages"
	if alt != primary {
		out = append(out, alt)
	}
	return out
}

func probeChat(client *http.Client, provider Provider, endpoint, key, model string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var payload []byte
	var err error
	switch provider {
	case Anthropic:
		payload, err = json.Marshal(map[string]any{
			"model":      model,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		})
	default:
		payload, err = json.Marshal(map[string]any{
			"model":      model,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		})
	}
	if err != nil {
		return err
	}

	var last error
	for _, dest := range chatURLCandidates(provider, endpoint) {
		err := postChatOnce(ctx, client, provider, dest, key, payload)
		if err == nil {
			return nil
		}
		last = err
		if ctx.Err() != nil {
			return err
		}
	}
	if last == nil {
		return fmt.Errorf("API returned no chat URL")
	}
	return last
}

func postChatOnce(ctx context.Context, client *http.Client, provider Provider, dest, key string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	setAuthHeaders(req, provider, key)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		if msg == "" {
			return fmt.Errorf("API returned %s", resp.Status)
		}
		return fmt.Errorf("API returned %s: %s", resp.Status, msg)
	}
	return nil
}
