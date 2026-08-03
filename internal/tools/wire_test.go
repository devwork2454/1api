package tools

import "testing"

func TestNormalizeWire(t *testing.T) {
	cases := []struct {
		in, want string
		err      bool
	}{
		{"", "", false},
		{"auto", "", false},
		{"openai", WireOpenAI, false},
		{"OpenAI", WireOpenAI, false},
		{"anthropic", WireAnthropic, false},
		{"claude", WireAnthropic, false},
		{"nope", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeWire(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("NormalizeWire(%q) want error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("NormalizeWire(%q) = %q, %v want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestResolveWire(t *testing.T) {
	oc := Find("opencode")
	if ResolveWire(oc, "https://openrouter.ai/api/v1", "") != WireOpenAI {
		t.Error("openrouter → openai")
	}
	if ResolveWire(oc, "https://api.anthropic.com", "") != WireAnthropic {
		t.Error("anthropic host → anthropic")
	}
	if ResolveWire(oc, "https://gw.example.com", WireAnthropic) != WireAnthropic {
		t.Error("explicit anthropic wins")
	}
	if ResolveWire(oc, "https://api.anthropic.com", WireOpenAI) != WireOpenAI {
		t.Error("explicit openai wins over host")
	}
}

func TestNormalizeAnthropicBaseURL(t *testing.T) {
	if got := NormalizeAnthropicBaseURL("https://proxy.example.com"); got != "https://proxy.example.com/v1" {
		t.Errorf("got %q", got)
	}
	if got := NormalizeAnthropicBaseURL("https://proxy.example.com/v1"); got != "https://proxy.example.com/v1" {
		t.Errorf("got %q", got)
	}
	if got := NormalizeAnthropicBaseURL(""); got != "https://api.anthropic.com/v1" {
		t.Errorf("empty → %q", got)
	}
}

func TestWireHint(t *testing.T) {
	codex := Find("codex")
	claude := Find("claude")
	opencode := Find("opencode")

	cases := []struct {
		name     string
		tool     *Tool
		endpoint string
		wire     string
		wantHint bool
	}{
		{"codex+openrouter", codex, "https://openrouter.ai/api/v1", "", false},
		{"codex+anthropic", codex, "https://api.anthropic.com", "", true},
		{"opencode+anthropic-auto", opencode, "https://api.anthropic.com/v1", "", false},
		{"opencode+anthropic-forced-openai", opencode, "https://api.anthropic.com", WireOpenAI, true},
		{"opencode+openai-forced-anthropic", opencode, "https://api.openai.com/v1", WireAnthropic, true},
		{"opencode+gateway-anthropic", opencode, "https://my-gw.example.com", WireAnthropic, false},
		{"claude+anthropic", claude, "https://api.anthropic.com", "", false},
		{"claude+openai", claude, "https://api.openai.com/v1", "", true},
		{"nil", nil, "https://api.anthropic.com", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WireHintFor(tc.tool, tc.endpoint, tc.wire)
			if tc.wantHint && got == "" {
				t.Fatal("expected hint")
			}
			if !tc.wantHint && got != "" {
				t.Fatalf("unexpected hint: %q", got)
			}
		})
	}
}
