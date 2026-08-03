package jsonc

import (
	"encoding/json"
	"testing"
)

func TestStripLineAndBlockComments(t *testing.T) {
	in := []byte(`{
  // line comment
  "a": 1, /* block */
  "url": "https://example.com/path",
  "note": "has // inside",
  "blockish": "keep /* in string */"
}`)
	out, err := Strip(in)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("stripped payload not pure JSON: %v\n%s", err, out)
	}
	if m["a"] != float64(1) {
		t.Errorf("a = %v", m["a"])
	}
	if m["url"] != "https://example.com/path" {
		t.Errorf("url = %v", m["url"])
	}
	if m["note"] != "has // inside" {
		t.Errorf("note = %v", m["note"])
	}
	if m["blockish"] != "keep /* in string */" {
		t.Errorf("blockish = %v", m["blockish"])
	}
}

func TestUnmarshalJSONC(t *testing.T) {
	raw := []byte(`{
  "model": "charon/mid",
  "agent": {
    "compaction": {
      // OpenCode alias note
      "model": "charon/low"
    }
  }
}`)
	var m map[string]any
	if err := Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != "charon/mid" {
		t.Errorf("model = %v", m["model"])
	}
	agent, _ := m["agent"].(map[string]any)
	comp, _ := agent["compaction"].(map[string]any)
	if comp["model"] != "charon/low" {
		t.Errorf("agent.compaction.model = %v", comp["model"])
	}
}

func TestStripUnterminated(t *testing.T) {
	if _, err := Strip([]byte(`{"a": "unterminated`)); err == nil {
		t.Error("want error for unterminated string")
	}
	if _, err := Strip([]byte(`{"a": 1 /* open`)); err == nil {
		t.Error("want error for unterminated block comment")
	}
}

func TestStripEmptyAndPureJSON(t *testing.T) {
	if out, err := Strip(nil); err != nil || out != nil {
		t.Errorf("nil: %q %v", out, err)
	}
	pure := []byte(`{"x":1}`)
	out, err := Strip(pure)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(pure) {
		t.Errorf("pure JSON changed: %s", out)
	}
}
