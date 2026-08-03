package tools

import (
	"fmt"
	"path/filepath"
)

// Session-supported tools for `charon run`: isolated HOME/XDG sandbox only.
// Claude/Pi are out of MVP (keychain / extension edge cases).
var sessionTools = map[string]struct{}{
	"codex":    {},
	"opencode": {},
}

// SessionSupported reports whether tool can be launched via `charon run`.
func SessionSupported(name string) bool {
	_, ok := sessionTools[name]
	return ok
}

// SessionBinary is the PATH executable for a session-capable tool.
func SessionBinary(name string) (string, error) {
	switch name {
	case "codex":
		return "codex", nil
	case "opencode":
		return "opencode", nil
	default:
		return "", fmt.Errorf("%s does not support session run (want codex or opencode)", name)
	}
}

// SessionArtifactPath is where a stored artifact id should land under sandbox root.
// root is a temporary home directory for the session.
func SessionArtifactPath(tool, artifactID, root string) (string, error) {
	if !SessionSupported(tool) {
		return "", fmt.Errorf("%s does not support session run", tool)
	}
	switch tool {
	case "codex":
		switch artifactID {
		case "config.toml", "auth.json":
			return filepath.Join(root, ".codex", artifactID), nil
		}
	case "opencode":
		switch artifactID {
		case "opencode.jsonc", "opencode.json":
			return filepath.Join(root, ".config", "opencode", artifactID), nil
		case "auth.json":
			return filepath.Join(root, ".local", "share", "opencode", artifactID), nil
		}
	}
	return "", fmt.Errorf("unknown session artifact %q for %s", artifactID, tool)
}

// SessionEnv returns environment entries that redirect a tool's config into root.
// Callers should prepend these over os.Environ() so they win.
func SessionEnv(tool, root string) ([]string, error) {
	if !SessionSupported(tool) {
		return nil, fmt.Errorf("%s does not support session run", tool)
	}
	// Always isolate HOME so ~/.codex and similar resolve under root.
	env := []string{
		"HOME=" + root,
		"USERPROFILE=" + root, // Windows-ish CLIs; harmless elsewhere
	}
	switch tool {
	case "opencode":
		// OpenCode follows XDG; pin both so auth + config stay in the sandbox.
		env = append(env,
			"XDG_CONFIG_HOME="+filepath.Join(root, ".config"),
			"XDG_DATA_HOME="+filepath.Join(root, ".local", "share"),
			"XDG_STATE_HOME="+filepath.Join(root, ".local", "state"),
			"XDG_CACHE_HOME="+filepath.Join(root, ".cache"),
		)
	case "codex":
		// Codex is HOME-relative (~/.codex); no extra XDG required.
	}
	return env, nil
}
