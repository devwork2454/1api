// Package models queries a provider's API for the models available to an endpoint + key.
package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Provider selects the wire format used to list models.
type Provider string

// OpenAI uses GET {base}/v1/models with an Authorization: Bearer header.
const OpenAI Provider = "openai"

// Anthropic uses GET {base}/v1/models with x-api-key + anthropic-version headers.
const Anthropic Provider = "anthropic"

// errBodyLimit caps how much of an error response body we surface (gateways
// often return HTML or long JSON; keep the CLI message readable).
const errBodyLimit = 240

// modelsURL builds the models-list URL from an endpoint, with or without a trailing "/v1".
func modelsURL(endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1/") {
		return base + "/models"
	}
	return base + "/v1/models"
}

// snippet trims and collapses whitespace so multi-line HTML/JSON fits one line.
func snippet(b []byte, limit int) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if limit > 0 && len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}

// Fetch returns the sorted model IDs offered by endpoint for the given key.
func Fetch(provider Provider, endpoint, key string) ([]string, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("API key is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL(endpoint), nil)
	if err != nil {
		return nil, err
	}
	switch provider {
	case Anthropic:
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := snippet(body, errBodyLimit)
		if detail == "" {
			return nil, fmt.Errorf("API returned %s (check endpoint and key)", resp.Status)
		}
		return nil, fmt.Errorf("API returned %s: %s", resp.Status, detail)
	}

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("could not parse model list: %w", err)
	}

	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no models returned by %s", endpoint)
	}
	sort.Strings(ids)
	return ids, nil
}
