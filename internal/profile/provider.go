package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"1api/internal/provider"
	"1api/internal/tools"
)

func (s *Store) ProviderStore() (*provider.Store, error) {
	return provider.OpenAt(s.Root)
}

// MigrateProvidersOnce copies API-proxy profile Specs into central providers once.
func (s *Store) MigrateProvidersOnce() error {
	c := s.readConfig()
	if c.ProvidersMigrated {
		return nil
	}
	ps, err := s.ProviderStore()
	if err != nil {
		return err
	}

	byFP := map[string]string{}

	for _, t := range tools.All() {
		for _, name := range s.List(t.Name) {
			sp, ok := s.GetSpec(t.Name, name)
			if !ok || strings.TrimSpace(sp.Key) == "" {
				continue
			}
			wire := t.Provider
			if wire == "" {
				wire = provider.WireOpenAI
			}
			ep := strings.TrimSpace(sp.Endpoint)
			if ep == "" {
				ep = t.DefaultEndpoint
			}
			fp := fingerprint(wire, ep, sp.Key)
			pName, ok := byFP[fp]
			if !ok {
				pName = uniqueProviderName(ps, name, fp)
				_, err := ps.Upsert(provider.Spec{
					Name:       pName,
					Endpoint:   ep,
					Key:        sp.Key,
					Wire:       wire,
					Model:      sp.Model,
					SkipVerify: true,
				}, provider.UpsertOptions{SkipVerify: true})
				if err != nil {
					return fmt.Errorf("migrate profile %s/%s: %w", t.Name, name, err)
				}
				byFP[fp] = pName
			}
			if s.Active(t.Name) == name {
				if err := s.SetToolProvider(t.Name, pName); err != nil {
					return err
				}
			}
		}
	}

	c = s.readConfig()
	c.ProvidersMigrated = true
	return s.writeConfig(c)
}

func fingerprint(wire, endpoint, key string) string {
	sum := sha256.Sum256([]byte(wire + "\n" + endpoint + "\n" + key))
	return hex.EncodeToString(sum[:8])
}

func uniqueProviderName(ps *provider.Store, preferred, fp string) string {
	base := sanitizeProfileName(preferred)
	if base == "" || base == DefaultName {
		base = "migrated-" + fp
	}
	if err := validateName(base); err != nil || ps.Exists(base) {
		cand := base + "-" + fp[:6]
		if ps.Exists(cand) || validateName(cand) != nil {
			return "migrated-" + fp
		}
		return cand
	}
	return base
}

// ApplyProvider binds tool → provider and writes live auth (mid + usable models).
func (s *Store) ApplyProvider(t *tools.Tool, providerName string, skipVerify bool) error {
	if t.ApplyAuth == nil {
		return fmt.Errorf("%s does not support provider apply", t.Title)
	}
	ps, err := s.ProviderStore()
	if err != nil {
		return err
	}
	opt := provider.UpsertOptions{SkipVerify: skipVerify}
	rec, err := ps.EnsureReady(providerName, opt)
	if err != nil {
		return err
	}
	if t.Detected != nil && t.Detected() {
		s.refreshKeychainArtifacts(t)
		s.refreshMergerArtifacts(t)
		if _, err := s.backup(t, "auto-backup before provider "+providerName); err != nil {
			return fmt.Errorf("backup failed, aborting: %w", err)
		}
	}
	if err := t.ApplyAuth(tools.AuthSpec{
		Endpoint:   rec.Endpoint,
		Key:        rec.Key,
		Model:      rec.PrimaryModel(),
		AllModels:  rec.Usable,
		SkipVerify: skipVerify,
	}); err != nil {
		return err
	}
	if err := s.SetToolProvider(t.Name, providerName); err != nil {
		return err
	}
	if s.Active(t.Name) == "" {
		_ = s.setActive(t.Name, "provider:"+providerName)
	}
	_ = s.pruneBackups(t.Name, backupKeep)
	return nil
}

// UpsertProviderAndBind creates/updates a central provider and optionally applies it to a tool.
func (s *Store) UpsertProviderAndBind(t *tools.Tool, name string, spec provider.Spec, skipVerify bool) error {
	if err := validateName(name); err != nil {
		return err
	}
	if name == DefaultName {
		return fmt.Errorf("%q is a reserved name", DefaultName)
	}
	ps, err := s.ProviderStore()
	if err != nil {
		return err
	}
	spec.Name = name
	if spec.Wire == "" && t != nil {
		spec.Wire = t.Provider
	}
	spec.SkipVerify = skipVerify
	if _, err := ps.Upsert(spec, provider.UpsertOptions{SkipVerify: skipVerify}); err != nil {
		return err
	}
	if t != nil {
		return s.ApplyProvider(t, name, skipVerify)
	}
	return nil
}
