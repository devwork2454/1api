// Package jsonc decodes JSON-with-comments (JSONC) used by tools like OpenCode.
// Comments are stripped before encoding/json parses the payload; string contents
// are left intact so URLs and text containing // or /* are not corrupted.
package jsonc

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Unmarshal strips // and /* */ comments, then json.Unmarshals into v.
func Unmarshal(data []byte, v any) error {
	clean, err := Strip(data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(clean, v); err != nil {
		return fmt.Errorf("jsonc: %w", err)
	}
	return nil
}

// Strip removes // line comments and /* block comments outside of JSON strings.
// Returns an error only for unterminated strings or block comments.
func Strip(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	var out bytes.Buffer
	out.Grow(len(data))

	for i := 0; i < len(data); {
		c := data[i]

		if c == '"' {
			start := i
			i++
			closed := false
			for i < len(data) {
				if data[i] == '\\' {
					if i+1 >= len(data) {
						return nil, fmt.Errorf("jsonc: unterminated string escape")
					}
					i += 2
					continue
				}
				if data[i] == '"' {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("jsonc: unterminated string")
			}
			out.Write(data[start:i])
			continue
		}

		if c == '/' && i+1 < len(data) && data[i+1] == '/' {
			i += 2
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
			continue
		}

		if c == '/' && i+1 < len(data) && data[i+1] == '*' {
			i += 2
			closed := false
			for i+1 < len(data) {
				if data[i] == '*' && data[i+1] == '/' {
					i += 2
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("jsonc: unterminated block comment")
			}
			continue
		}

		out.WriteByte(c)
		i++
	}
	return out.Bytes(), nil
}
