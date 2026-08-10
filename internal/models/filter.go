package models

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNoUsableModels means the endpoint listed models but none passed a chat probe.
var ErrNoUsableModels = errors.New("暂无可用模型")

// FilterOptions controls FilterReachable.
type FilterOptions struct {
	// Timeout is the overall deadline (list + all chat probes). Zero → 90s.
	Timeout time.Duration
	// PerModelTimeout bounds one chat probe. Zero → 12s.
	PerModelTimeout time.Duration
	// Concurrency is max parallel chat probes. Zero → 6.
	Concurrency int
	// Candidates, when non-empty, limits probes to this set intersected with the live list.
	Candidates []string
	// HTTPClient overrides the client (tests inject httptest).
	HTTPClient *http.Client
}

// FilterReachable lists models then keeps only ids that complete a 1-token chat.
// Returns ErrNoUsableModels when the list is non-empty but every chat probe fails,
// or when the live catalog is empty after intersection.
func FilterReachable(provider Provider, endpoint, key string, opt FilterOptions) ([]string, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("filter: empty API key")
	}
	overall := opt.Timeout
	if overall <= 0 {
		overall = 90 * time.Second
	}
	per := opt.PerModelTimeout
	if per <= 0 {
		per = 12 * time.Second
	}
	conc := opt.Concurrency
	if conc <= 0 {
		conc = 6
	}
	client := opt.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: per}
	}

	listTO := overall
	if listTO > 25*time.Second {
		listTO = 25 * time.Second
	}
	ids, err := fetchWithClient(client, provider, endpoint, key, listTO)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	if len(opt.Candidates) > 0 {
		ids = intersectSorted(opt.Candidates, ids)
	}
	if len(ids) == 0 {
		return nil, ErrNoUsableModels
	}

	ctx, cancel := context.WithTimeout(context.Background(), overall)
	defer cancel()

	type hit struct {
		id string
		ok bool
	}
	outCh := make(chan hit, len(ids))
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				outCh <- hit{id: id, ok: false}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			err := probeChat(client, provider, endpoint, key, id, per)
			outCh <- hit{id: id, ok: err == nil}
		}()
	}
	go func() {
		wg.Wait()
		close(outCh)
	}()

	var okIDs []string
	for h := range outCh {
		if h.ok {
			okIDs = append(okIDs, h.id)
		}
	}
	sort.Strings(okIDs)
	if len(okIDs) == 0 {
		return nil, ErrNoUsableModels
	}
	return okIDs, nil
}

func intersectSorted(want, live []string) []string {
	set := map[string]struct{}{}
	for _, id := range live {
		set[id] = struct{}{}
	}
	var out []string
	seen := map[string]struct{}{}
	for _, id := range want {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := set[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
