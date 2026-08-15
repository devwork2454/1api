package models

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// ModelInfo is one entry from a provider's models catalog.
type ModelInfo struct {
	ID            string
	ContextWindow int // tokens; 0 when the catalog did not report a window
}

// ModelIDs extracts sorted-stable ids from infos (caller already sorts typically).
func ModelIDs(infos []ModelInfo) []string {
	ids := make([]string, 0, len(infos))
	for _, m := range infos {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// ContextWindowMap keeps only positive windows keyed by model id.
func ContextWindowMap(infos []ModelInfo) map[string]int {
	out := map[string]int{}
	for _, m := range infos {
		if m.ID != "" && m.ContextWindow > 0 {
			out[m.ID] = m.ContextWindow
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// WindowFor returns known[id] when positive, else 0.
func WindowFor(id string, known map[string]int) int {
	if known == nil {
		return 0
	}
	if w := known[id]; w > 0 {
		return w
	}
	return 0
}

// parseModelCatalog decodes an OpenAI-style {"data":[...]} body into ModelInfo.
func parseModelCatalog(raw []byte) ([]ModelInfo, error) {
	var body struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(body.Data))
	for _, item := range body.Data {
		id, window := parseModelEntry(item)
		if id == "" {
			continue
		}
		out = append(out, ModelInfo{ID: id, ContextWindow: window})
	}
	if len(out) == 0 {
		return nil, errEmptyCatalog
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// errEmptyCatalog is compared only inside the package (wrapped with list URL).
var errEmptyCatalog = errString("no models returned")

type errString string

func (e errString) Error() string { return string(e) }

func parseModelEntry(raw json.RawMessage) (id string, window int) {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		// Fallback: {"id":"..." } only.
		var simple struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &simple) == nil {
			return simple.ID, 0
		}
		return "", 0
	}
	id = stringField(m, "id")
	if id == "" {
		return "", 0
	}
	return id, extractContextWindow(m)
}

// extractContextWindow reads common gateway / OpenRouter / vLLM field names.
// max_tokens alone is ignored — it is often max *output*, not context.
func extractContextWindow(m map[string]any) int {
	if m == nil {
		return 0
	}
	keys := []string{
		"context_length",
		"context_window",
		"max_model_len",
		"max_input_tokens",
		"max_context_length",
		"max_context_window",
		"contextLength",
		"contextWindow",
	}
	for _, k := range keys {
		if v := intField(m, k); v > 0 {
			return v
		}
	}
	for _, nest := range []string{"top_provider", "limits", "meta", "architecture", "capabilities"} {
		child, ok := m[nest].(map[string]any)
		if !ok {
			continue
		}
		for _, k := range keys {
			if v := intField(child, k); v > 0 {
				return v
			}
		}
	}
	return 0
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return ""
	}
}

func intField(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		if t > 0 {
			return int(t)
		}
	case json.Number:
		n, err := t.Int64()
		if err == nil && n > 0 {
			return int(n)
		}
	case int:
		if t > 0 {
			return t
		}
	case int64:
		if t > 0 {
			return int(t)
		}
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}
