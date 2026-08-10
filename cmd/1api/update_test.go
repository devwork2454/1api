package main

import (
	"strings"
	"testing"
)

func TestUpdateInstallURLDefaultIsGitee(t *testing.T) {
	t.Setenv("CHARON_UPDATE_URL", "")
	// t.Setenv with empty → TrimSpace empty → default Gitee path.
	got := updateInstallURL()
	if !strings.Contains(got, "gitee.com") {
		t.Errorf("default update URL = %q, want gitee.com", got)
	}
	if !strings.Contains(got, "wbff/1api") {
		t.Errorf("default update URL = %q, want Gitee wbff/1api", got)
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

func TestGiteeInstallFetchScript(t *testing.T) {
	s := giteeInstallFetchScript()
	if !strings.Contains(s, "gitee.com/api/v5/repos/wbff/1api/releases/latest") {
		t.Errorf("script missing Gitee releases API: %q", s)
	}
	if !strings.Contains(s, "gitee.com/wbff/1api/releases/download") {
		t.Errorf("script missing Gitee download base: %q", s)
	}
	if !strings.Contains(s, "install.sh") {
		t.Errorf("script missing install.sh: %q", s)
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
