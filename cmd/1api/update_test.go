package main

import (
	"strings"
	"testing"
)

func TestUpdateInstallURLDefaultIsFork(t *testing.T) {
	t.Setenv("CHARON_UPDATE_URL", "")
	// Clear may not empty if unset; force empty then check default.
	// t.Setenv with empty still sets empty string which our code treats as override if TrimSpace != "".
	// So only unset via empty: our updateInstallURL uses TrimSpace empty → default.
	got := updateInstallURL()
	if !strings.Contains(got, "devwork2454/1api") {
		t.Errorf("default update URL = %q, want fork devwork2454", got)
	}
	if strings.Contains(got, "mingtheanlay") {
		t.Errorf("default must not use upstream mingtheanlay: %q", got)
	}
	if !strings.HasSuffix(got, "/install.sh") {
		t.Errorf("URL should end with install.sh: %q", got)
	}
}

func TestUpdateInstallURLOverride(t *testing.T) {
	want := "https://example.test/1api/install.sh"
	t.Setenv("CHARON_UPDATE_URL", want)
	if got := updateInstallURL(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("https://x/y"); got != "'https://x/y'" {
		t.Errorf("got %q", got)
	}
	if got := shellQuote("a'b"); got != `'a'"'"'b'` {
		t.Errorf("got %q", got)
	}
}
