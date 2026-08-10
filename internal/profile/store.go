// Package profile stores and applies named snapshots of each tool's auth surface,
// with automatic backups and an always-available "default".
//
// The package is split across a few files:
//   - store.go     the Store, its on-disk layout, config, and name validation
//   - snapshot.go  capturing profiles: Save / AddProfile / EditProfile / EnsureDefault
//   - apply.go     restoring profiles: Apply / Undo / refresh / Drift
//   - backup.go    timestamped pre-switch backups and pruning
//   - manage.go    Remove / Rename / Duplicate
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"1api/internal/artifact"
	"1api/internal/tools"
)

// DefaultName is the reserved profile capturing config as first seen by 1api.
const DefaultName = "default"

// Spec is the endpoint/key/model a profile was created from, so the edit form can prefill.
type Spec struct {
	Endpoint string `json:"endpoint,omitempty"`
	Key      string `json:"key,omitempty"`
	Model    string `json:"model,omitempty"`
	// SkipVerify is request-only (not persisted): skip OpenCode live probe on apply.
	SkipVerify bool `json:"-"`
}

// Manifest records a stored profile's metadata and which artifacts it contained
// (an absent artifact is restored by removal).
type Manifest struct {
	Label     string          `json:"label"`
	Note      string          `json:"note,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	Present   map[string]bool `json:"present"`
	Spec      *Spec           `json:"spec,omitempty"`
	Account   string          `json:"account,omitempty"` // logged-in account this snapshot captured, if any
	Active    string          `json:"active,omitempty"`  // profile active when a backup was taken, for undo
}

// Store is rooted at ~/.config/1api.
type Store struct {
	Root string
}

// UI language and skin identifiers for the interactive menu.
const (
	LangZH   = "zh"
	LangEN   = "en"
	SkinTeal = "teal"
	SkinMono = "mono"
	SkinWarm = "warm"
)

// Legacy TUI mode values kept for config compatibility.
const (
	TUIModeNew = "new"
	TUIModeOld = "old"
)

type config struct {
	Active            map[string]string `json:"active"`
	OAuthFingerprint  map[string]string `json:"oauthFingerprint,omitempty"`
	ToolProvider      map[string]string `json:"toolProvider,omitempty"`
	ProvidersMigrated bool              `json:"providersMigrated,omitempty"`
	CharonImported    bool              `json:"charonImported,omitempty"`
	TUIMode           string            `json:"tuiMode,omitempty"`
	UILang            string            `json:"uiLang,omitempty"`
	UISkin            string            `json:"uiSkin,omitempty"`
}

// Open returns the store rooted at $XDG_CONFIG_HOME/1api (default ~/.config/1api).
// On first open it merges a sibling legacy ~/.config/charon tree when present.
func Open() (*Store, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(h, ".config")
	}
	root := filepath.Join(base, "1api")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	s := &Store{Root: root}
	if err := s.ImportLegacyCharonOnce(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) toolDir(tool string) string    { return filepath.Join(s.Root, "profiles", tool) }
func (s *Store) profDir(tool, n string) string { return filepath.Join(s.toolDir(tool), n) }
func (s *Store) configPath() string            { return filepath.Join(s.Root, "config.json") }

// warn appends a timestamped line to 1api.log for a failure in a best-effort
// operation (e.g. refreshing a profile snapshot before a switch) that is deliberately
// not surfaced as a hard error — so it's still diagnosable instead of vanishing
// silently. Never returns an error: logging itself is best-effort.
func (s *Store) warn(context string, err error) {
	f, oerr := os.OpenFile(filepath.Join(s.Root, "1api.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if oerr != nil {
		return
	}
	defer func() { _ = f.Close() }()
	fmt.Fprintf(f, "%s\t%s: %v\n", time.Now().Format(time.RFC3339), context, err)
}

func (s *Store) readConfig() config {
	var c config
	if data, err := os.ReadFile(s.configPath()); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	if c.Active == nil {
		c.Active = map[string]string{}
	}
	if c.ToolProvider == nil {
		c.ToolProvider = map[string]string{}
	}
	return c
}

func (s *Store) writeConfig(c config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return artifact.AtomicWrite(s.configPath(), data, 0o600)
}

// Active returns the profile name currently marked active for a tool, or "".
func (s *Store) Active(tool string) string { return s.readConfig().Active[tool] }

// ActiveProvider returns the central provider bound to a tool, or "".
func (s *Store) ActiveProvider(tool string) string {
	return s.readConfig().ToolProvider[tool]
}

// SetToolProvider records which central provider a tool uses (does not apply live config).
func (s *Store) SetToolProvider(tool, providerName string) error {
	c := s.readConfig()
	if providerName == "" {
		delete(c.ToolProvider, tool)
	} else {
		c.ToolProvider[tool] = providerName
	}
	return s.writeConfig(c)
}

// TUIMode returns the legacy mode preference (new or old).
func (s *Store) TUIMode() string {
	switch s.readConfig().TUIMode {
	case TUIModeOld:
		return TUIModeOld
	default:
		return TUIModeNew
	}
}

// TUIModeSet reports whether a legacy tuiMode was ever saved.
func (s *Store) TUIModeSet() bool {
	switch s.readConfig().TUIMode {
	case TUIModeNew, TUIModeOld:
		return true
	default:
		return false
	}
}

// SetTUIMode persists the legacy mode preference.
func (s *Store) SetTUIMode(mode string) error {
	switch mode {
	case TUIModeNew, TUIModeOld:
	default:
		return fmt.Errorf("invalid tui mode %q (want %s or %s)", mode, TUIModeNew, TUIModeOld)
	}
	c := s.readConfig()
	c.TUIMode = mode
	return s.writeConfig(c)
}

// UILang returns the TUI language (zh or en; default zh).
func (s *Store) UILang() string {
	switch s.readConfig().UILang {
	case LangEN:
		return LangEN
	default:
		return LangZH
	}
}

// SetUILang persists the TUI language (zh or en).
func (s *Store) SetUILang(lang string) error {
	switch lang {
	case LangZH, LangEN:
	default:
		return fmt.Errorf("invalid ui lang %q (want %s or %s)", lang, LangZH, LangEN)
	}
	c := s.readConfig()
	c.UILang = lang
	return s.writeConfig(c)
}

// UISkin returns the TUI skin id (teal, mono, or warm; default teal).
func (s *Store) UISkin() string {
	skin := s.readConfig().UISkin
	switch skin {
	case SkinMono, SkinWarm:
		return skin
	default:
		return SkinTeal
	}
}

// SetUISkin persists the TUI skin id.
func (s *Store) SetUISkin(skin string) error {
	switch skin {
	case SkinTeal, SkinMono, SkinWarm:
	default:
		return fmt.Errorf("invalid ui skin %q", skin)
	}
	c := s.readConfig()
	c.UISkin = skin
	return s.writeConfig(c)
}

func (s *Store) setActive(tool, name string) error {
	c := s.readConfig()
	c.Active[tool] = name
	return s.writeConfig(c)
}

// SetActiveName marks a profile active without applying files (used right after Save).
func (s *Store) SetActiveName(tool, name string) error { return s.setActive(tool, name) }

// lastOAuthFingerprint returns the OAuth credential fingerprint last recorded for a
// tool, or "" if none has been seen yet.
func (s *Store) lastOAuthFingerprint(tool string) string {
	return s.readConfig().OAuthFingerprint[tool]
}

// setOAuthFingerprint records the OAuth credential fingerprint currently seen for a tool.
func (s *Store) setOAuthFingerprint(tool, fp string) error {
	c := s.readConfig()
	if c.OAuthFingerprint == nil {
		c.OAuthFingerprint = map[string]string{}
	}
	c.OAuthFingerprint[tool] = fp
	return s.writeConfig(c)
}

// List returns stored profile names for a tool, "default" first.
func (s *Store) List(tool string) []string {
	entries, err := os.ReadDir(s.toolDir(tool))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == DefaultName {
			return true
		}
		if names[j] == DefaultName {
			return false
		}
		return names[i] < names[j]
	})
	return names
}

// Exists reports whether a named profile is stored for a tool.
func (s *Store) Exists(tool, name string) bool {
	_, err := os.Stat(filepath.Join(s.profDir(tool, name), "manifest.json"))
	return err == nil
}

// LoadManifest reads a stored profile's metadata.
func (s *Store) LoadManifest(tool, name string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(s.profDir(tool, name), "manifest.json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

// writeManifest writes a profile/backup dir's manifest.json atomically.
func writeManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return artifact.AtomicWrite(filepath.Join(dir, "manifest.json"), data, 0o600)
}

// ProfileModelEffort reads a stored profile's own config snapshot and returns its
// captured model/effort — e.g. so the profile list can show each account's own
// model and reasoning-effort level, not just whatever is live right now. Both are ""
// if the tool's config artifact doesn't track them or the profile has no record.
func (s *Store) ProfileModelEffort(t *tools.Tool, name string) (model, effort string) {
	if !s.Exists(t.Name, name) {
		return "", ""
	}
	dir := s.profDir(t.Name, name)

	for _, a := range t.Artifacts {
		peeker, ok := a.(artifact.Peeker)
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, a.ID()))
		if err != nil {
			continue
		}
		return peeker.Peek(data)
	}
	return "", ""
}

// validateName rejects a profile name the store cannot safely use as a directory
// name under its root: empty, "." / ".." (which would escape or alias the profiles
// dir), or anything sanitizeProfileName would alter (path separators, spaces, ...).
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if name == "." || name == ".." || name != sanitizeProfileName(name) {
		return fmt.Errorf("invalid profile name %q (use letters, digits, and . _ @ -)", name)
	}
	return nil
}

// validateNewName is validateName for a profile being created or renamed, where
// the reserved default name is additionally off limits.
func validateNewName(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if name == DefaultName {
		return fmt.Errorf("%q is a reserved name", DefaultName)
	}
	return nil
}

// sanitizeProfileName maps an account identity to a filesystem-safe profile name,
// keeping [A-Za-z0-9._@-] and replacing every other run with a single "-".
func sanitizeProfileName(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '@', r == '-':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
