// Package provider stores named external API endpoints (endpoint + key + tiers)
// once, so tools only bind to a provider instead of re-entering credentials.
package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"1api/internal/artifact"
	"1api/internal/models"
)

// Wire formats supported by the models package probes.
const (
	WireOpenAI    = "openai"
	WireAnthropic = "anthropic"
)

// Record is one named external API configuration.
type Record struct {
	Name        string    `json:"name"`
	Endpoint    string    `json:"endpoint"`
	Key         string    `json:"key"`
	Wire        string    `json:"wire"` // openai | anthropic
	Mid         string    `json:"mid,omitempty"`
	Low         string    `json:"low,omitempty"`
	High        string    `json:"high,omitempty"`
	Usable      []string  `json:"usable,omitempty"`
	NeedsVerify bool      `json:"needsVerify,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Spec is the input for creating or updating a provider.
type Spec struct {
	Name     string
	Endpoint string
	Key      string
	Wire     string // empty → openai
	Model    string // preferred mid / primary
	Low      string // optional explicit low
	High     string // optional explicit high
	// SkipVerify skips FilterReachable (tests / offline).
	SkipVerify bool
	// HTTPClient is optional; tests inject httptest via models options through Verify.
	// Not stored on disk.
}

// Store is rooted at the same ~/.config/1api root as profile.Store.
type Store struct {
	Root string
}

// OpenAt returns a provider store under root (usually profile.Store.Root).
func OpenAt(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("provider store: empty root")
	}
	if err := os.MkdirAll(filepath.Join(root, "providers"), 0o700); err != nil {
		return nil, err
	}
	return &Store{Root: root}, nil
}

func (s *Store) dir(name string) string {
	return filepath.Join(s.Root, "providers", name)
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir(name), "provider.json")
}

// List returns provider names sorted.
func (s *Store) List() []string {
	entries, err := os.ReadDir(filepath.Join(s.Root, "providers"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// Exists reports whether a named provider is stored.
func (s *Store) Exists(name string) bool {
	_, err := os.Stat(s.path(name))
	return err == nil
}

// Get loads a provider record.
func (s *Store) Get(name string) (Record, error) {
	if err := validateName(name); err != nil {
		return Record{}, err
	}
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		return Record{}, fmt.Errorf("provider %q: %w", name, err)
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, err
	}
	if r.Name == "" {
		r.Name = name
	}
	return r, nil
}

// Remove deletes a provider directory.
func (s *Store) Remove(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if !s.Exists(name) {
		return fmt.Errorf("provider %q not found", name)
	}
	return os.RemoveAll(s.dir(name))
}

// write saves a record atomically with 0600.
func (s *Store) write(r Record) error {
	if err := validateName(r.Name); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir(r.Name), 0o700); err != nil {
		return err
	}
	r.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return artifact.AtomicWrite(s.path(r.Name), data, 0o600)
}

// AuthSpec builds tools.AuthSpec fields from a provider (mid as primary model).
func (r Record) PrimaryModel() string {
	if r.Mid != "" {
		return r.Mid
	}
	if len(r.Usable) > 0 {
		return r.Usable[0]
	}
	return ""
}

// Tiers returns mid/low/high as models.Tiers.
func (r Record) Tiers() models.Tiers {
	return models.Tiers{Mid: r.Mid, Low: r.Low, High: r.High}
}

// normalizeWire returns a valid wire or error.
func normalizeWire(w string) (string, error) {
	w = strings.TrimSpace(strings.ToLower(w))
	if w == "" {
		return WireOpenAI, nil
	}
	switch w {
	case WireOpenAI, WireAnthropic:
		return w, nil
	default:
		return "", fmt.Errorf("unknown wire %q (want openai or anthropic)", w)
	}
}

// validateName rejects unsafe directory names (same charset as profile names).
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("provider name is required")
	}
	if name == "." || name == ".." || name != sanitizeName(name) {
		return fmt.Errorf("invalid provider name %q (use letters, digits, and . _ @ -)", name)
	}
	return nil
}

func sanitizeName(s string) string {
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
