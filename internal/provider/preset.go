package provider

import (
	"fmt"
	"strings"
)

// Preset is a built-in known gateway whose endpoint and wire format are
// pre-filled; the user only supplies --name and --key.
type Preset struct {
	Name      string
	Endpoint  string
	Wire      string
	Note      string
	KeySource string
}

// presets lists curated gateways. Keys come from each vendor's console.
var presets = []Preset{
	{
		Name:      "ark",
		Endpoint:  "https://ark.cn-beijing.volces.com/api/coding/v3",
		Wire:      WireOpenAI,
		Note:      "Volcengine Ark Coding Plan (OpenAI-compatible V3 endpoint)",
		KeySource: "https://console.volcengine.com/ark",
	},
	{
		Name:      "ark-anthropic",
		Endpoint:  "https://ark.cn-beijing.volces.com/api/coding",
		Wire:      WireAnthropic,
		Note:      "Volcengine Ark Coding Plan (Anthropic-compatible endpoint for Claude Code)",
		KeySource: "https://console.volcengine.com/ark",
	},
	{
		Name:      "ocgo",
		Endpoint:  "https://opencode.ai/zen/go/v1",
		Wire:      WireOpenAI,
		Note:      "OpenCode Go subscription (open coding models, monthly plan)",
		KeySource: "https://opencode.ai/auth",
	},
	{
		Name:      "zen",
		Endpoint:  "https://opencode.ai/zen/v1",
		Wire:      WireOpenAI,
		Note:      "OpenCode Zen pay-as-you-go gateway",
		KeySource: "https://opencode.ai/auth",
	},
}

// LookupPreset resolves a preset name (case-insensitive) to its definition.
func LookupPreset(name string) (Preset, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return Preset{}, fmt.Errorf("provider: empty preset name")
	}
	for _, p := range presets {
		if p.Name == n {
			return p, nil
		}
	}
	return Preset{}, fmt.Errorf("unknown preset %q (available: %s)", name, PresetNames())
}

// PresetNames returns the comma-separated preset ids for help text.
func PresetNames() string {
	ids := make([]string, 0, len(presets))
	for _, p := range presets {
		ids = append(ids, p.Name)
	}
	return strings.Join(ids, ", ")
}
