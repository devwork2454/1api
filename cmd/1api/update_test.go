package main

import (
	"strings"
	"testing"
)

func TestUpdateInstallURLOverride(t *testing.T) {
	want := "https://example.test/1api/install.sh"
	t.Setenv("CHARON_UPDATE_URL", want)
	if got := updateInstallURL(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUpdateInstallURLRespectsSource(t *testing.T) {
	t.Setenv("CHARON_UPDATE_URL", "")
	t.Setenv("CHARON_UPDATE_SOURCE", "gitee")
	got := updateInstallURL()
	if !strings.Contains(got, "gitee.com") || !strings.Contains(got, "wbff/1api") {
		t.Errorf("gitee source URL = %q", got)
	}

	t.Setenv("CHARON_UPDATE_SOURCE", "github")
	got = updateInstallURL()
	if !strings.Contains(got, "github.com/devwork2454/1api") {
		t.Errorf("github source URL = %q", got)
	}
	if strings.Contains(got, "mingtheanlay") {
		t.Errorf("must not use upstream: %q", got)
	}
}

func TestPreferGiteeUpdateEnv(t *testing.T) {
	t.Setenv("CHARON_UPDATE_SOURCE", "")
	if !preferGiteeUpdate() {
		t.Fatal("default want gitee")
	}
	t.Setenv("CHARON_UPDATE_SOURCE", "gitee")
	if !preferGiteeUpdate() {
		t.Fatal("want gitee")
	}
	t.Setenv("CHARON_UPDATE_SOURCE", "github")
	if preferGiteeUpdate() {
		t.Fatal("want github")
	}
	t.Setenv("CHARON_UPDATE_SOURCE", "gh")
	if preferGiteeUpdate() {
		t.Fatal("gh want github")
	}
}

func TestUpdateInstallURLDefaultsToGitee(t *testing.T) {
	t.Setenv("CHARON_UPDATE_URL", "")
	t.Setenv("CHARON_UPDATE_SOURCE", "")
	got := updateInstallURL()
	if !strings.Contains(got, "gitee.com") || !strings.Contains(got, "wbff/1api") {
		t.Errorf("default URL = %q, want Gitee", got)
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
