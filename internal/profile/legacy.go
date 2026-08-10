package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const legacyConfigDirName = "charon"

// ImportLegacyCharonOnce merges ~/.config/charon into this store (usually
// ~/.config/1api) when upgrading from the charon binary. Existing 1api profile
// names are never overwritten. The legacy tree is left intact.
func (s *Store) ImportLegacyCharonOnce() error {
	c := s.readConfig()
	if c.CharonImported {
		return nil
	}
	legacyRoot, err := legacyCharonRoot()
	if err != nil || legacyRoot == "" || samePath(legacyRoot, s.Root) {
		return s.markCharonImported()
	}
	if st, err := os.Stat(legacyRoot); err != nil || !st.IsDir() {
		return s.markCharonImported()
	}

	imported, err := s.copyMissingProfiles(filepath.Join(legacyRoot, "profiles"))
	if err != nil {
		return fmt.Errorf("import charon profiles: %w", err)
	}
	if err := s.mergeLegacyConfig(filepath.Join(legacyRoot, "config.json")); err != nil {
		return fmt.Errorf("import charon config: %w", err)
	}

	c = s.readConfig()
	c.CharonImported = true
	if imported > 0 || c.ProvidersMigrated {
		c.ProvidersMigrated = false
	}
	return s.writeConfig(c)
}

func (s *Store) markCharonImported() error {
	c := s.readConfig()
	c.CharonImported = true
	return s.writeConfig(c)
}

func legacyCharonRoot() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(h, ".config")
	}
	return filepath.Join(base, legacyConfigDirName), nil
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return aa == bb
}

func (s *Store) copyMissingProfiles(legacyProfiles string) (int, error) {
	entries, err := os.ReadDir(legacyProfiles)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, toolEnt := range entries {
		if !toolEnt.IsDir() {
			continue
		}
		tool := toolEnt.Name()
		toolSrc := filepath.Join(legacyProfiles, tool)
		profs, err := os.ReadDir(toolSrc)
		if err != nil {
			return n, err
		}
		for _, pEnt := range profs {
			if !pEnt.IsDir() {
				continue
			}
			name := pEnt.Name()
			if s.Exists(tool, name) {
				continue
			}
			src := filepath.Join(toolSrc, name)
			dst := s.profDir(tool, name)
			if err := copyDir(src, dst); err != nil {
				return n, fmt.Errorf("%s/%s: %w", tool, name, err)
			}
			n++
		}
	}
	return n, nil
}

func (s *Store) mergeLegacyConfig(legacyConfigPath string) error {
	data, err := os.ReadFile(legacyConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var leg config
	if err := json.Unmarshal(data, &leg); err != nil {
		return err
	}
	c := s.readConfig()
	if c.Active == nil {
		c.Active = map[string]string{}
	}
	for tool, name := range leg.Active {
		if name == "" || !s.Exists(tool, name) {
			continue
		}
		cur := c.Active[tool]
		if cur == "" || cur == DefaultName {
			c.Active[tool] = name
		}
	}
	if leg.OAuthFingerprint != nil {
		if c.OAuthFingerprint == nil {
			c.OAuthFingerprint = map[string]string{}
		}
		for tool, fp := range leg.OAuthFingerprint {
			if c.OAuthFingerprint[tool] == "" && fp != "" {
				c.OAuthFingerprint[tool] = fp
			}
		}
	}
	if leg.ToolProvider != nil {
		if c.ToolProvider == nil {
			c.ToolProvider = map[string]string{}
		}
		for tool, p := range leg.ToolProvider {
			if c.ToolProvider[tool] == "" && p != "" {
				c.ToolProvider[tool] = p
			}
		}
	}
	if c.TUIMode == "" && leg.TUIMode != "" {
		c.TUIMode = leg.TUIMode
	}
	return s.writeConfig(c)
}

func (s *Store) needsProviderMigration() bool {
	c := s.readConfig()
	if !c.ProvidersMigrated {
		return true
	}
	ps, err := s.ProviderStore()
	if err != nil {
		return false
	}
	if len(ps.List()) > 0 {
		return false
	}
	return s.hasAPIProxySpecs()
}

func (s *Store) hasAPIProxySpecs() bool {
	for _, t := range allToolNamesOnDisk(s) {
		for _, name := range s.List(t) {
			sp, ok := s.GetSpec(t, name)
			if ok && strings.TrimSpace(sp.Key) != "" {
				return true
			}
		}
	}
	return false
}

func allToolNamesOnDisk(s *Store) []string {
	seen := map[string]bool{}
	var names []string
	entries, err := os.ReadDir(filepath.Join(s.Root, "profiles"))
	if err != nil {
		return names
	}
	for _, e := range entries {
		if e.IsDir() && !seen[e.Name()] {
			seen[e.Name()] = true
			names = append(names, e.Name())
		}
	}
	return names
}
