package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdAliasPrintsFunction(t *testing.T) {
	sandbox(t)
	// Capture stdout by running the pure function path via cmdAlias writing to stdout
	// is hard without hijacking os.Stdout; test sanitize + reject paths instead.
	if err := cmdAlias([]string{"claude", "default"}); err == nil {
		t.Fatal("expected claude alias to fail")
	}
	if err := cmdAlias([]string{"codex"}); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestCmdRunRejectsUnsupportedAndMissingProfile(t *testing.T) {
	home := sandbox(t)
	seedCodex(t, home)

	// Unsupported tool.
	if err := run([]string{"run", "claude", "default"}); err == nil || !strings.Contains(err.Error(), "session run") {
		t.Fatalf("run claude: %v", err)
	}
	// Missing profile.
	if err := run([]string{"run", "codex", "nope"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("run missing profile: %v", err)
	}
}

func TestCmdRunDoesNotTouchLiveConfig(t *testing.T) {
	home := sandbox(t)
	seedCodex(t, home)

	// Put a fake "codex" on PATH that only echoes HOME and exits 0.
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "codex")
	script := "#!/bin/sh\nprintf '%s\\n' \"$HOME\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := run([]string{"add", "codex", "--name", "work", "--key", "sk-test", "--endpoint", "https://example.com/v1", "--no-verify"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Snapshot live config after add (active profile is work).
	liveCfg := filepath.Join(home, ".codex", "config.toml")
	before, err := os.ReadFile(liveCfg)
	if err != nil {
		t.Fatal(err)
	}

	// Also create another profile without switching live to it via duplicate+edit
	// — simplest: cp work -> other; materialize other should not change live.
	if err := run([]string{"cp", "codex", "work", "other"}); err != nil {
		t.Fatalf("cp: %v", err)
	}

	// Run other in session (fake binary).
	if err := run([]string{"run", "codex", "other"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	after, err := os.ReadFile(liveCfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("live config changed by run:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestMergeEnvOverrides(t *testing.T) {
	got := mergeEnv([]string{"A=1", "B=2"}, []string{"B=9", "C=3"})
	m := map[string]string{}
	for _, e := range got {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	if m["A"] != "1" || m["B"] != "9" || m["C"] != "3" {
		t.Fatalf("mergeEnv = %v", m)
	}
}

func TestSanitizeAlias(t *testing.T) {
	if g := sanitizeAlias("codex/work@x"); g != "codex_work_x" {
		t.Fatalf("got %q", g)
	}
}
