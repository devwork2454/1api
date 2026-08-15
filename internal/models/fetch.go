// Package models queries a provider's API for the models available to an endpoint + key.
package models

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider selects the wire format used to list models.
type Provider string

// OpenAI uses GET {base}/v1/models with an Authorization: Bearer header.
const OpenAI Provider = "openai"

// Anthropic uses GET {base}/v1/models with x-api-key + anthropic-version headers.
const Anthropic Provider = "anthropic"

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

// listURLCandidates is the models-list URLs to try, primary first.
// Anthropic-compatible gateways (DeepSeek) often serve GET /v1/models on the
// host origin while Messages live under a /anthropic prefix that has no catalog.
func listURLCandidates(provider Provider, endpoint string) []string {
	primary := modelsURL(endpoint)
	out := []string{primary}
	if provider != Anthropic {
		return out
	}
	u, ok := parseEndpoint(endpoint)
	if !ok {
		return out
	}
	u.Path = stripAnthropicAPIPath(u.Path)
	u.RawQuery = ""
	u.Fragment = ""
	alt := modelsURL(u.String())
	if alt != primary {
		out = append(out, alt)
	}
	return out
}

func parseEndpoint(endpoint string) (*url.URL, bool) {
	raw := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if raw == "" {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}
	return u, true
}

// stripAnthropicAPIPath removes a trailing /v1 and /anthropic path prefix only
// (never the host — api.anthropic.com must stay intact).
func stripAnthropicAPIPath(p string) string {
	p = strings.TrimRight(p, "/")
	p = strings.TrimSuffix(p, "/v1")
	p = strings.TrimRight(p, "/")
	p = strings.TrimSuffix(p, "/anthropic")
	return strings.TrimRight(p, "/")
}

// Fetch returns the sorted model IDs offered by endpoint for the given key.
func Fetch(provider Provider, endpoint, key string) ([]string, error) {
	infos, err := FetchInfo(provider, endpoint, key)
	if err != nil {
		return nil, err
	}
	return ModelIDs(infos), nil
}

// FetchInfo returns sorted model catalog entries (id + optional context window).
func FetchInfo(provider Provider, endpoint, key string) ([]ModelInfo, error) {
	return fetchInfoWithClient(http.DefaultClient, provider, endpoint, key, 20*time.Second)
}
