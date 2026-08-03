package tools

import (
	"path/filepath"
	"testing"
)

func TestSessionSupported(t *testing.T) {
	if !SessionSupported("codex") || !SessionSupported("opencode") {
		t.Fatal("codex and opencode should be session-supported")
	}
	if SessionSupported("claude") || SessionSupported("pi") {
		t.Fatal("claude/pi should not be session-supported in MVP")
	}
}

func TestSessionArtifactPath(t *testing.T) {
	root := "/tmp/sbx"
	p, err := SessionArtifactPath("codex", "config.toml", root)
	if err != nil || p != filepath.Join(root, ".codex", "config.toml") {
		t.Fatalf("codex config path = %q err=%v", p, err)
	}
	p, err = SessionArtifactPath("opencode", "auth.json", root)
	if err != nil || p != filepath.Join(root, ".local", "share", "opencode", "auth.json") {
		t.Fatalf("opencode auth path = %q err=%v", p, err)
	}
	if _, err := SessionArtifactPath("codex", "nope", root); err == nil {
		t.Fatal("expected error for unknown artifact")
	}
}

func TestSessionBinary(t *testing.T) {
	b, err := SessionBinary("codex")
	if err != nil || b != "codex" {
		t.Fatalf("got %q %v", b, err)
	}
	if _, err := SessionBinary("claude"); err == nil {
		t.Fatal("expected error for claude")
	}
}
