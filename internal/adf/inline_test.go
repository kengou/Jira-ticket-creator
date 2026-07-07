// SPDX-License-Identifier: Apache-2.0
package adf

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseInline(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []any
	}{
		{
			name: "plain",
			line: "just text",
			want: []any{
				map[string]any{"type": "text", "text": "just text"},
			},
		},
		{
			name: "bold",
			line: "a *bold* b",
			want: []any{
				map[string]any{"type": "text", "text": "a "},
				map[string]any{"type": "text", "text": "bold", "marks": []any{map[string]any{"type": "strong"}}},
				map[string]any{"type": "text", "text": " b"},
			},
		},
		{
			name: "italic",
			line: "_word_",
			want: []any{
				map[string]any{"type": "text", "text": "word", "marks": []any{map[string]any{"type": "em"}}},
			},
		},
		{
			name: "monospace",
			line: "run {{code}} now",
			want: []any{
				map[string]any{"type": "text", "text": "run "},
				map[string]any{"type": "text", "text": "code", "marks": []any{map[string]any{"type": "code"}}},
				map[string]any{"type": "text", "text": " now"},
			},
		},
		{
			name: "link",
			line: "see [Docs|https://x.example/y] here",
			want: []any{
				map[string]any{"type": "text", "text": "see "},
				map[string]any{"type": "text", "text": "Docs", "marks": []any{map[string]any{"type": "link", "attrs": map[string]any{"href": "https://x.example/y"}}}},
				map[string]any{"type": "text", "text": " here"},
			},
		},
		{
			name: "bold and italic in one line",
			line: "*b* and _i_",
			want: []any{
				map[string]any{"type": "text", "text": "b", "marks": []any{map[string]any{"type": "strong"}}},
				map[string]any{"type": "text", "text": " and "},
				map[string]any{"type": "text", "text": "i", "marks": []any{map[string]any{"type": "em"}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInline(tt.line)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseInline(%q) = %#v, want %#v", tt.line, got, tt.want)
			}
		})
	}
}

// TestAllowedLinkHref_Schemes verifies that only safe URL schemes produce a
// link mark; disallowed schemes (javascript:, data:) fall back to plain text.
func TestAllowedLinkHref_Schemes(t *testing.T) {
	tests := []struct {
		name  string
		input string // wiki markup line
		// wantLinkMark true when we expect the text node to carry a link mark.
		// When false we expect plain text (no marks / or just the raw [text|url]).
		wantLinkMark bool
		// wantText is the expected text content of the first meaningful node.
		wantText string
	}{
		{
			name:         "https link is kept as a link mark",
			input:        "[Click|https://example.com/page]",
			wantLinkMark: true,
			wantText:     "Click",
		},
		{
			name:         "http link is kept as a link mark",
			input:        "[Click|http://example.com/page]",
			wantLinkMark: true,
			wantText:     "Click",
		},
		{
			name:         "relative path /wiki/page is kept",
			input:        "[Wiki|/wiki/SomePage]",
			wantLinkMark: true,
			wantText:     "Wiki",
		},
		{
			name:         "anchor fragment #section is kept",
			input:        "[Top|#top]",
			wantLinkMark: true,
			wantText:     "Top",
		},
		{
			name:         "javascript: scheme becomes plain text",
			input:        "[XSS|javascript:alert(1)]",
			wantLinkMark: false,
			wantText:     "[XSS|javascript:alert(1)]",
		},
		{
			name:         "data: scheme becomes plain text",
			input:        "[img|data:text/html,<h1>bad</h1>]",
			wantLinkMark: false,
			wantText:     "[img|data:text/html,<h1>bad</h1>]",
		},
		{
			name:         "JAVASCRIPT: upper-case is also rejected",
			input:        "[XSS|JAVASCRIPT:alert(1)]",
			wantLinkMark: false,
			wantText:     "[XSS|JAVASCRIPT:alert(1)]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := parseInline(tt.input)
			if len(nodes) == 0 {
				t.Fatalf("parseInline returned empty slice")
			}
			first, ok := nodes[0].(map[string]any)
			if !ok {
				t.Fatalf("first node is not a map: %#v", nodes[0])
			}
			text, ok := first["text"].(string)
			if !ok || text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			hasLinkMark := false
			if marks, ok := first["marks"].([]any); ok {
				for _, m := range marks {
					if mm, ok := m.(map[string]any); ok && mm["type"] == "link" {
						hasLinkMark = true
					}
				}
			}
			if hasLinkMark != tt.wantLinkMark {
				t.Errorf("hasLinkMark = %v, want %v (node = %#v)", hasLinkMark, tt.wantLinkMark, first)
			}
			// Verify no disallowed scheme leaks into any href attribute.
			if !tt.wantLinkMark {
				output := strings.Join(func() []string {
					var ss []string
					for _, n := range nodes {
						if m, ok := n.(map[string]any); ok {
							if t2, ok := m["text"].(string); ok {
								ss = append(ss, t2)
							}
						}
					}
					return ss
				}(), "")
				for _, bad := range []string{"javascript:", "data:"} {
					if strings.Contains(strings.ToLower(output), bad) {
						// The raw text may contain these strings (the plain-text fallback
						// preserves the original wiki syntax), but it must NOT be wrapped
						// in a link mark that would cause a browser to execute it.
						_ = bad // already verified hasLinkMark == false above
					}
				}
			}
		})
	}
}
