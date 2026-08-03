package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"charon/internal/profile"
	"charon/internal/tools"
)

// exitError carries a process exit code without printing as a normal error message.
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

// cmdRun launches a tool CLI under a temporary HOME/XDG populated from a profile.
// Live config is never modified. MVP tools: codex, opencode.
//
//	charon run <tool> <profile> [--keep] [--] [args...]
func cmdRun(store *profile.Store, args []string) error {
	keep := false
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if a == "--keep" {
			keep = true
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fmt.Errorf("unknown flag %q\nusage: charon run <tool> <profile> [--keep] [--] [args...]", a)
		}
		positional = append(positional, a)
	}
	if len(positional) < 2 {
		return fmt.Errorf("usage: charon run <tool> <profile> [--keep] [--] [args...]")
	}
	toolName, profileName := positional[0], positional[1]
	childArgs := positional[2:]

	t, err := requireTool(toolName)
	if err != nil {
		return err
	}
	if !tools.SessionSupported(t.Name) {
		return fmt.Errorf("%s does not support session run (want codex or opencode)", t.Name)
	}
	binName, err := tools.SessionBinary(t.Name)
	if err != nil {
		return err
	}
	bin, err := exec.LookPath(binName)
	if err != nil {
		return fmt.Errorf("%s not found on PATH: %w", binName, err)
	}

	root, err := os.MkdirTemp("", "charon-run-*")
	if err != nil {
		return err
	}
	if !keep {
		defer func() { _ = os.RemoveAll(root) }()
	} else {
		fmt.Fprintf(os.Stderr, "charon: session dir kept at %s\n", root)
	}

	if err := store.MaterializeSession(t, profileName, root); err != nil {
		return err
	}

	sessionEnv, err := tools.SessionEnv(t.Name, root)
	if err != nil {
		return err
	}
	env := mergeEnv(os.Environ(), sessionEnv)

	cmd := exec.Command(bin, childArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	// Inherit cwd so relative paths in tool args still work.
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code := 1
			if status, ok := ee.Sys().(syscall.WaitStatus); ok {
				code = status.ExitStatus()
			}
			return &exitError{code: code}
		}
		return err
	}
	return nil
}

// mergeEnv returns base with override keys replacing same-name entries.
func mergeEnv(base, override []string) []string {
	keys := map[string]string{}
	order := make([]string, 0, len(base)+len(override))
	put := func(kv string) {
		k, _, _ := strings.Cut(kv, "=")
		if _, seen := keys[k]; !seen {
			order = append(order, k)
		}
		keys[k] = kv
	}
	for _, e := range base {
		put(e)
	}
	for _, e := range override {
		put(e)
	}
	out := make([]string, 0, len(order))
	for _, k := range order {
		out = append(out, keys[k])
	}
	return out
}

// cmdAlias prints a shell alias or function for `charon run`.
//
//	charon alias <tool> <profile> [--name NAME] [--shell bash|zsh]
func cmdAlias(args []string) error {
	name := ""
	shell := "bash"
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--name" && i+1 < len(args):
			i++
			name = args[i]
		case strings.HasPrefix(a, "--name="):
			name = strings.TrimPrefix(a, "--name=")
		case a == "--shell" && i+1 < len(args):
			i++
			shell = args[i]
		case strings.HasPrefix(a, "--shell="):
			shell = strings.TrimPrefix(a, "--shell=")
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %q\nusage: charon alias <tool> <profile> [--name NAME] [--shell bash|zsh]", a)
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) < 2 {
		return fmt.Errorf("usage: charon alias <tool> <profile> [--name NAME] [--shell bash|zsh]")
	}
	toolName, profileName := positional[0], positional[1]
	t, err := requireTool(toolName)
	if err != nil {
		return err
	}
	if !tools.SessionSupported(t.Name) {
		return fmt.Errorf("%s does not support session run (want codex or opencode)", t.Name)
	}
	if name == "" {
		// Default: <tool>-<profile> with unsafe chars replaced.
		name = sanitizeAlias(toolName + "-" + profileName)
	}
	if name == "" {
		return fmt.Errorf("alias name is empty")
	}

	// Resolve charon path so aliases work even if charon is not on PATH later.
	self, err := os.Executable()
	if err != nil {
		self = "charon"
	} else if resolved, rerr := filepath.EvalSymlinks(self); rerr == nil {
		self = resolved
	}

	switch shell {
	case "bash", "zsh":
		// Function form so extra args are forwarded: codex-work --help
		fmt.Printf("%s() { %q run %s %s -- \"$@\"; }\n", name, self, toolName, profileName)
		return nil
	case "fish":
		fmt.Printf("function %s; %q run %s %s -- $argv; end\n", name, self, toolName, profileName)
		return nil
	default:
		return fmt.Errorf("unsupported shell %q (want bash, zsh, or fish)", shell)
	}
}

func sanitizeAlias(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
