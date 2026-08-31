package provider

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"1api/internal/models"
)

// UpsertOptions controls network probes during Upsert / SetTiers / Refresh.
type UpsertOptions struct {
	HTTPClient *http.Client
	// SkipVerify overrides Spec.SkipVerify when set via Refresh paths.
	SkipVerify bool
}

// Upsert creates or replaces a provider after probing reachable models (unless skipped).
// Explicit Low/High/Model must be in the usable set when verify runs.
func (s *Store) Upsert(spec Spec, opt UpsertOptions) (Record, error) {
	if err := validateName(spec.Name); err != nil {
		return Record{}, err
	}
	wire, err := normalizeWire(spec.Wire)
	if err != nil {
		return Record{}, err
	}
	ep := strings.TrimSpace(spec.Endpoint)
	key := strings.TrimSpace(spec.Key)
	if key == "" {
		return Record{}, fmt.Errorf("provider: API key is required")
	}
	if ep == "" {
		if wire == WireAnthropic {
			ep = "https://api.anthropic.com"
		} else {
			ep = "https://api.openai.com/v1"
		}
	}

	skip := spec.SkipVerify || opt.SkipVerify
	var usable []string
	var windows map[string]int
	needsVerify := false
	primary := strings.TrimSpace(spec.Model)

	if skip {
		// Offline / test, or caller already ran FilterReachable and passed Usable.
		if len(spec.Usable) > 0 {
			usable = uniqueNonEmpty(append([]string{}, spec.Usable...))
			windows = filterWindows(spec.ContextWindows, usable)
			// Catalog supplied by caller (typically just probed) — not stale.
			needsVerify = false
		} else {
			usable = uniqueNonEmpty(append([]string{}, primary, spec.Low, spec.High))
			if len(usable) == 0 {
				usable = []string{"default"}
				if primary == "" {
					primary = "default"
				}
			}
			windows = filterWindows(spec.ContextWindows, usable)
			// Sparse offline set — re-probe on next bind/use.
			needsVerify = true
		}
	} else {
		if spec.Low == "default" {
			spec.Low = ""
		}
		if spec.High == "default" {
			spec.High = ""
		}
		if primary == "default" {
			primary = ""
		}
		fallbacks := uniqueNonEmpty([]string{primary, spec.Low, spec.High})
		detail, ferr := models.FilterReachableDetail(models.Provider(wire), ep, key, models.FilterOptions{
			FallbackCandidates: fallbacks,
			HTTPClient:         opt.HTTPClient,
		})
		if ferr != nil {
			return Record{}, ferr
		}
		usable = detail.Usable
		windows = detail.ContextWindows
		if primary != "" {
			if matched, ok := findUsableModel(primary, usable); ok {
				primary = matched
			} else {
				return Record{}, fmt.Errorf("model %q is not usable (not in reachable set)", primary)
			}
		}
		if lo := strings.TrimSpace(spec.Low); lo != "" {
			if matched, ok := findUsableModel(lo, usable); ok {
				spec.Low = matched
			} else {
				return Record{}, fmt.Errorf("model %q is not usable (not in reachable set)", lo)
			}
		}
		if hi := strings.TrimSpace(spec.High); hi != "" {
			if matched, ok := findUsableModel(hi, usable); ok {
				spec.High = matched
			} else {
				return Record{}, fmt.Errorf("model %q is not usable (not in reachable set)", hi)
			}
		}
	}

	tiers := models.ResolveTiers(primary, usable)
	if lo := strings.TrimSpace(spec.Low); lo != "" {
		tiers.Low = lo
	}
	if hi := strings.TrimSpace(spec.High); hi != "" {
		tiers.High = hi
	}
	if primary != "" {
		tiers.Mid = primary
	}
	if tiers.Low == "" {
		tiers.Low = tiers.Mid
	}
	if tiers.High == "" {
		tiers.High = tiers.Mid
	}

	r := Record{
		Name:           spec.Name,
		Endpoint:       ep,
		Key:            key,
		Wire:           wire,
		Mid:            tiers.Mid,
		Low:            tiers.Low,
		High:           tiers.High,
		Usable:         usable,
		ContextWindows: windows,
		NeedsVerify:    needsVerify,
	}
	if err := s.write(r); err != nil {
		return Record{}, err
	}
	return r, nil
}

// Refresh re-runs FilterReachable and updates usable + tiers (keeps mid if still ok).
func (s *Store) Refresh(name string, opt UpsertOptions) (Record, error) {
	cur, err := s.Get(name)
	if err != nil {
		return Record{}, err
	}
	if opt.SkipVerify {
		cur.NeedsVerify = true
		if err := s.write(cur); err != nil {
			return Record{}, err
		}
		return cur, nil
	}
	fallbacks := uniqueNonEmpty([]string{cur.Mid, cur.Low, cur.High})
	if len(fallbacks) == 0 {
		fallbacks = cur.Usable
	}
	detail, err := models.FilterReachableDetail(models.Provider(cur.Wire), cur.Endpoint, cur.Key, models.FilterOptions{
		FallbackCandidates: fallbacks,
		HTTPClient:         opt.HTTPClient,
	})
	if err != nil {
		return Record{}, err
	}
	usable := detail.Usable
	primary := cur.Mid
	if primary != "" && !slices.Contains(usable, primary) {
		// Mid gone — fall back to resolver default.
		primary = ""
	}
	// Keep explicit low/high only if still reachable.
	low, high := cur.Low, cur.High
	if low != "" && !slices.Contains(usable, low) {
		low = ""
	}
	if high != "" && !slices.Contains(usable, high) {
		high = ""
	}
	tiers := models.ResolveTiers(primary, usable)
	if low != "" {
		tiers.Low = low
	}
	if high != "" {
		tiers.High = high
	}
	if primary != "" {
		tiers.Mid = primary
	}
	cur.Usable = usable
	cur.ContextWindows = detail.ContextWindows
	cur.Mid = tiers.Mid
	cur.Low = tiers.Low
	cur.High = tiers.High
	cur.NeedsVerify = false
	if err := s.write(cur); err != nil {
		return Record{}, err
	}
	return cur, nil
}

// SetTiers updates mid/low/high. Each non-empty id must be in Usable, or a live
// probe must confirm it when NeedsVerify or forceProbe.
func (s *Store) SetTiers(name, mid, low, high string, opt UpsertOptions) (Record, error) {
	cur, err := s.Get(name)
	if err != nil {
		return Record{}, err
	}
	// Re-probe if catalog stale or empty.
	if !opt.SkipVerify && (cur.NeedsVerify || len(cur.Usable) == 0) {
		cur, err = s.Refresh(name, opt)
		if err != nil {
			return Record{}, err
		}
	}

	check := func(id string) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil
		}
		if slices.Contains(cur.Usable, id) {
			return nil
		}
		if opt.SkipVerify {
			// Allow offline assignment; extend usable.
			cur.Usable = append(cur.Usable, id)
			cur.NeedsVerify = true
			return nil
		}
		// Live single-model chat probe.
		_, err := models.Probe(models.Provider(cur.Wire), cur.Endpoint, cur.Key, models.ProbeOptions{
			ChatModel:  id,
			HTTPClient: opt.HTTPClient,
		})
		if err != nil {
			return fmt.Errorf("model %q is not usable: %w", id, err)
		}
		cur.Usable = append(cur.Usable, id)
		slices.Sort(cur.Usable)
		cur.Usable = slices.Compact(cur.Usable)
		return nil
	}
	if err := check(mid); err != nil {
		return Record{}, err
	}
	if err := check(low); err != nil {
		return Record{}, err
	}
	if err := check(high); err != nil {
		return Record{}, err
	}

	if m := strings.TrimSpace(mid); m != "" {
		cur.Mid = m
	}
	if l := strings.TrimSpace(low); l != "" {
		cur.Low = l
	}
	if h := strings.TrimSpace(high); h != "" {
		cur.High = h
	}
	if cur.Low == "" {
		cur.Low = cur.Mid
	}
	if cur.High == "" {
		cur.High = cur.Mid
	}
	if err := s.write(cur); err != nil {
		return Record{}, err
	}
	return cur, nil
}

// EnsureReady returns a provider with a fresh usable set when NeedsVerify is set.
func (s *Store) EnsureReady(name string, opt UpsertOptions) (Record, error) {
	cur, err := s.Get(name)
	if err != nil {
		return Record{}, err
	}
	if opt.SkipVerify {
		return cur, nil
	}
	if cur.NeedsVerify || len(cur.Usable) == 0 || cur.Mid == "" {
		return s.Refresh(name, opt)
	}
	return cur, nil
}

// ReplaceUsable writes a freshly probed model catalog without changing tiers
// (except filling empty low/high from mid). Used after bind-time refresh.
// windows is optional; when nil, existing ContextWindows are pruned to usable.
func (s *Store) ReplaceUsable(name string, usable []string, windows ...map[string]int) (Record, error) {
	cur, err := s.Get(name)
	if err != nil {
		return Record{}, err
	}
	usable = uniqueNonEmpty(usable)
	if len(usable) == 0 {
		return Record{}, fmt.Errorf("provider %q: empty usable catalog", name)
	}
	cur.Usable = usable
	cur.NeedsVerify = false
	if len(windows) > 0 && windows[0] != nil {
		cur.ContextWindows = filterWindows(windows[0], usable)
	} else {
		cur.ContextWindows = filterWindows(cur.ContextWindows, usable)
	}
	if cur.Mid == "" {
		tiers := models.ResolveTiers("", usable)
		cur.Mid, cur.Low, cur.High = tiers.Mid, tiers.Low, tiers.High
	} else {
		if cur.Low == "" {
			cur.Low = cur.Mid
		}
		if cur.High == "" {
			cur.High = cur.Mid
		}
	}
	if err := s.write(cur); err != nil {
		return Record{}, err
	}
	return cur, nil
}

func uniqueNonEmpty(ids []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func filterWindows(src map[string]int, usable []string) map[string]int {
	if len(src) == 0 {
		return nil
	}
	out := map[string]int{}
	for _, id := range usable {
		if w := src[id]; w > 0 {
			out[id] = w
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func findUsableModel(wanted string, usable []string) (string, bool) {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return "", false
	}
	if slices.Contains(usable, wanted) {
		return wanted, true
	}
	wantedLower := strings.ToLower(wanted)
	wantedNorm := strings.ReplaceAll(wantedLower, ".", "-")
	for _, id := range usable {
		idLower := strings.ToLower(id)
		idNorm := strings.ReplaceAll(idLower, ".", "-")
		if idLower == wantedLower || idNorm == wantedNorm {
			return id, true
		}
		if strings.HasPrefix(idLower, wantedLower) || strings.HasPrefix(idNorm, wantedNorm) {
			return id, true
		}
	}
	if wantedLower == "ark-code-latest" || wantedLower == "ark-code" {
		return wanted, true
	}
	return "", false
}
