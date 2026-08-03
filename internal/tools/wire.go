package tools

import (
	"fmt"
	"strings"
)

// WireHint returns a short, non-blocking warning when endpoint looks like the
// wrong vendor protocol for this tool's fixed wire format. Empty string means
// no mismatch detected (or not enough signal to warn).
//
// Charon does not auto-switch SDKs: each tool always writes its own wire
// (OpenAI-compatible for codex/opencode/pi; Anthropic env for claude). The hint
// only surfaces obvious footguns so users can pick another tool or a gateway
// that speaks the expected protocol.
func WireHint(t *Tool, endpoint string) string {
	if t == nil {
		return ""
	}
	ep := strings.ToLower(strings.TrimSpace(endpoint))
	if ep == "" {
		ep = strings.ToLower(strings.TrimSpace(t.DefaultEndpoint))
	}
	if ep == "" {
		return ""
	}

	looksAnthropic := strings.Contains(ep, "api.anthropic.com") ||
		strings.Contains(ep, "anthropic.com/v1")
	looksOpenAIOfficial := strings.Contains(ep, "api.openai.com")

	switch strings.ToLower(t.Provider) {
	case "openai":
		if looksAnthropic {
			return fmt.Sprintf(
				"%s uses an OpenAI-compatible wire (Bearer + OpenAI-style paths); "+
					"%s looks like Anthropic's API — use tool \"claude\" or an OpenAI-compatible gateway",
				t.Title, endpointOrDefault(endpoint, t.DefaultEndpoint),
			)
		}
	case "anthropic":
		if looksOpenAIOfficial {
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
