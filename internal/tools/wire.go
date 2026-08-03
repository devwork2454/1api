package tools

import (
	"fmt"
	"strings"
)

// Wire formats Charon can write into a tool that supports more than one
// protocol (today: OpenCode). Empty Wire means "auto" — pick from the URL.
const (
	WireOpenAI    = "openai"
	WireAnthropic = "anthropic"
)

// NormalizeWire returns WireOpenAI, WireAnthropic, or "" for auto/blank.
// Unknown values error so CLI typos fail before a live write.
func NormalizeWire(w string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case "", "auto":
		return "", nil
	case WireOpenAI, "openai-compatible", "oai":
		return WireOpenAI, nil
	case WireAnthropic, "claude":
		return WireAnthropic, nil
	default:
		return "", fmt.Errorf("unknown wire %q (want openai, anthropic, or auto)", w)
	}
}

// ResolveWire picks the wire for a tool write.
// explicit (after NormalizeWire) wins; else URL heuristics; else tool.Provider / openai.
func ResolveWire(t *Tool, endpoint, explicit string) string {
	if w, err := NormalizeWire(explicit); err == nil && w != "" {
		return w
	}
	ep := strings.ToLower(strings.TrimSpace(endpoint))
	if ep == "" && t != nil {
		ep = strings.ToLower(strings.TrimSpace(t.DefaultEndpoint))
	}
	if looksAnthropicEndpoint(ep) {
		return WireAnthropic
	}
	if t != nil && strings.EqualFold(t.Provider, WireAnthropic) {
		return WireAnthropic
	}
	return WireOpenAI
}

func looksAnthropicEndpoint(ep string) bool {
	ep = strings.ToLower(ep)
	return strings.Contains(ep, "api.anthropic.com") ||
		strings.Contains(ep, "anthropic.com/v1")
}

func looksOpenAIOfficial(ep string) bool {
	return strings.Contains(strings.ToLower(ep), "api.openai.com")
}

// NormalizeAnthropicBaseURL ensures OpenCode's @ai-sdk/anthropic baseURL ends
// with /v1 (the SDK appends /messages → …/v1/messages). Official Anthropic
// docs and OpenCode issues show bare hosts 404 without the suffix.
func NormalizeAnthropicBaseURL(ep string) string {
	ep = strings.TrimRight(strings.TrimSpace(ep), "/")
	if ep == "" {
		return "https://api.anthropic.com/v1"
	}
	if strings.HasSuffix(ep, "/v1") || strings.Contains(ep, "/v1/") {
		return ep
	}
	return ep + "/v1"
}

// WireHint returns a short, non-blocking warning when endpoint looks like the
// wrong vendor protocol for this tool's fixed or resolved wire. Empty means OK.
//
// For tools that only speak one wire (codex/pi = openai, claude = anthropic),
// a mismatched official host is a footgun. OpenCode can speak both when Wire
// is set or auto-detected — no hint in that case.
func WireHint(t *Tool, endpoint string) string {
	return WireHintFor(t, endpoint, "")
}

// WireHintFor is WireHint with an optional explicit wire (openai/anthropic/auto).
func WireHintFor(t *Tool, endpoint, explicitWire string) string {
	if t == nil {
		return ""
	}
	ep := strings.TrimSpace(endpoint)
	epLower := strings.ToLower(ep)
	if epLower == "" {
		epLower = strings.ToLower(strings.TrimSpace(t.DefaultEndpoint))
	}

	// OpenCode: dual wire — only warn when user forced openai against Anthropic host
	// or forced anthropic against OpenAI official host.
	if t.Name == "opencode" {
		w := ResolveWire(t, endpoint, explicitWire)
		if w == WireOpenAI && looksAnthropicEndpoint(epLower) {
			// explicit openai against anthropic host
			if ew, _ := NormalizeWire(explicitWire); ew == WireOpenAI {
				return fmt.Sprintf(
					"OpenCode wire=openai with %s; Anthropic Messages need wire=anthropic (or omit --wire to auto)",
					endpointOrDefault(endpoint, t.DefaultEndpoint),
				)
			}
		}
		if w == WireAnthropic && looksOpenAIOfficial(epLower) {
			if ew, _ := NormalizeWire(explicitWire); ew == WireAnthropic {
				return fmt.Sprintf(
					"OpenCode wire=anthropic with %s; use wire=openai for OpenAI's API",
					endpointOrDefault(endpoint, t.DefaultEndpoint),
				)
			}
		}
		return ""
	}

	switch strings.ToLower(t.Provider) {
	case WireOpenAI:
		if looksAnthropicEndpoint(epLower) {
			return fmt.Sprintf(
				"%s uses an OpenAI-compatible wire (Bearer + OpenAI-style paths); "+
					"%s looks like Anthropic's API — use tool \"claude\", OpenCode with wire=anthropic, or an OpenAI-compatible gateway",
				t.Title, endpointOrDefault(endpoint, t.DefaultEndpoint),
			)
		}
	case WireAnthropic:
		if looksOpenAIOfficial(epLower) {
			return fmt.Sprintf(
				"%s uses Anthropic-style credentials (x-api-key / ANTHROPIC_* env); "+
					"%s looks like OpenAI's API — use tool \"codex\"/\"opencode\"/\"pi\" or an Anthropic-compatible gateway",
				t.Title, endpointOrDefault(endpoint, t.DefaultEndpoint),
			)
		}
	}
	return ""
}

func endpointOrDefault(ep, def string) string {
	if strings.TrimSpace(ep) != "" {
		return strings.TrimSpace(ep)
	}
	return def
}
