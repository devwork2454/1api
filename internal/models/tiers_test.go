package models

import "testing"

func TestResolveTiersExactIds(t *testing.T) {
	got := ResolveTiers("mid", []string{"low", "mid", "high", "other"})
	if got.Mid != "mid" || got.Low != "low" || got.High != "high" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveTiersFallbackToPrimary(t *testing.T) {
	got := ResolveTiers("only-model", []string{"only-model"})
	if got.Mid != "only-model" || got.Low != "only-model" || got.High != "only-model" {
		t.Fatalf("single model should fill all tiers: %+v", got)
	}
}

func TestResolveTiersContains(t *testing.T) {
	got := ResolveTiers("gpt-mid", []string{"gpt-low", "gpt-mid", "gpt-high-pro"})
	if got.Low != "gpt-low" || got.Mid != "gpt-mid" || got.High != "gpt-high-pro" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveTiersCapabilityHint(t *testing.T) {
	got := ResolveTiers("gpt-4o", []string{"gemini-2.0-flash", "gpt-4o", "claude-opus-4"})
	if got.Low != "gemini-2.0-flash" {
		t.Errorf("low = %q", got.Low)
	}
	if got.Mid != "gpt-4o" {
		t.Errorf("mid = %q", got.Mid)
	}
	if got.High != "claude-opus-4" {
		t.Errorf("high = %q", got.High)
	}
}
