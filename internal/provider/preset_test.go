package provider

import "testing"

func TestLookupPreset(t *testing.T) {
	for _, id := range []string{"ocgo", "zen", "OCGO", " Zen "} {
		if _, err := LookupPreset(id); err != nil {
			t.Errorf("LookupPreset(%q) = %v, want nil", id, err)
		}
	}
	for _, id := range []string{"", "bogus"} {
		if _, err := LookupPreset(id); err == nil {
			t.Errorf("LookupPreset(%q) = nil, want error", id)
		}
	}
}

func TestLookupPresetFields(t *testing.T) {
	p, err := LookupPreset("ocgo")
	if err != nil {
		t.Fatal(err)
	}
	if p.Endpoint != "https://opencode.ai/zen/go/v1" {
		t.Errorf("ocgo endpoint = %q", p.Endpoint)
	}
	if p.Wire != WireOpenAI {
		t.Errorf("ocgo wire = %q", p.Wire)
	}
	z, err := LookupPreset("zen")
	if err != nil {
		t.Fatal(err)
	}
	if z.Endpoint != "https://opencode.ai/zen/v1" {
		t.Errorf("zen endpoint = %q", z.Endpoint)
	}
	if PresetNames() == "" {
		t.Error("PresetNames() is empty")
	}
}
