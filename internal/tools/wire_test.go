package tools

import "testing"

func TestWireHint(t *testing.T) {
	codex := Find("codex")
	claude := Find("claude")
	opencode := Find("opencode")

	cases := []struct {
		name     string
		tool     *Tool
		endpoint string
		wantHint bool
	}{
		{"codex+openrouter", codex, "https://openrouter.ai/api/v1", false},
		{"codex+openai", codex, "https://api.openai.com/v1", false},
		{"codex+anthropic", codex, "https://api.anthropic.com", true},
		{"codex+blank", codex, "", false},
		{"opencode+anthropic", opencode, "https://api.anthropic.com/v1", true},
		{"claude+anthropic", claude, "https://api.anthropic.com", false},
		{"claude+blank", claude, "", false},
		{"claude+openai", claude, "https://api.openai.com/v1", true},
		{"claude+gateway", claude, "https://my-gw.example.com", false},
		{"nil tool", nil, "https://api.anthropic.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WireHint(tc.tool, tc.endpoint)
			if tc.wantHint && got == "" {
				t.Fatalf("expected hint, got empty")
			}
			if !tc.wantHint && got != "" {
				t.Fatalf("expected no hint, got %q", got)
			}
		})
	}
}
