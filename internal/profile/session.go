package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"1api/internal/artifact"
	"1api/internal/tools"
)

// MaterializeSession writes profile name's artifacts into sandbox root (a fake
// HOME) without touching the live tool config. Used by `1api run`.
func (s *Store) MaterializeSession(t *tools.Tool, name, root string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if !tools.SessionSupported(t.Name) {
		return fmt.Errorf("%s does not support session run (want codex or opencode)", t.Name)
	}
	if !s.Exists(t.Name, name) {
		return fmt.Errorf("profile %q not found for %s", name, t.Name)
	}
	if root == "" {
		return fmt.Errorf("session root is empty")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}

	dir := s.profDir(t.Name, name)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	// Snapshot ids are authoritative; tool Artifacts list may include keychain
	// entries we do not session-map — skip unmapped ids with a clear error only
	// when the snapshot actually has that artifact present.
	for id, present := range m.Present {
		if !present {
			continue
		}
		dest, err := tools.SessionArtifactPath(t.Name, id, root)
		if err != nil {
			return fmt.Errorf("session materialize %s: %w", id, err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, id))
		if err != nil {
			return fmt.Errorf("read snapshot %s: %w", id, err)
		}
		if err := writeSessionFile(dest, raw); err != nil {
			return err
		}
	}
	return nil
}

func writeSessionFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Same atomic + 0600 contract as live credential writes.
	return artifact.AtomicWrite(path, data, 0o600)
}
