// SPDX-License-Identifier: Apache-2.0
package adf

import (
	"strings"
)

// allowedLinkHref reports whether href is safe to emit as an ADF link mark.
// Only http/https URLs, relative paths (starting with "/") and anchor
// fragments (starting with "#") are permitted. Any other scheme — including
// javascript: and data: — is rejected so that untrusted config text cannot
// inject script-execution or data-URI content through Jira comments/descriptions.
func allowedLinkHref(href string) bool {
	lower := strings.ToLower(href)
	// Explicit safe absolute schemes.
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}
	// Relative paths and anchors are safe (no scheme at all).
	if strings.HasPrefix(href, "/") || strings.HasPrefix(href, "#") {
		return true
	}
	// Reject any other construct that contains a ":" before the first "/" or
	// end of string — those are scheme-prefixed URIs we do not recognise.
	colonIdx := strings.IndexByte(href, ':')
	slashIdx := strings.IndexByte(href, '/')
	if colonIdx >= 0 && (slashIdx < 0 || colonIdx < slashIdx) {
		return false
	}
	// No scheme detected; treat as a relative reference and allow it.
	return true
}

// inlineRule describes one inline mark: a delimiter pair and the mark it applies.
type inlineRule struct {
	open  string
	close string
	mark  func(inner string) []any // returns the ADF marks slice for this run
}

// mark builds a single-mark slice with the given type.
func mark(markType string) []any {
	return []any{map[string]any{keyType: markType}}
}

// inlineRules is the ordered set of inline constructs. Order matters: the longer
// "{{" delimiter is checked before single-char delimiters so it is not mistaken
// for two separate tokens.
var inlineRules = []inlineRule{
	{open: "{{", close: "}}", mark: func(string) []any { return mark("code") }},
	{open: "*", close: "*", mark: func(string) []any { return mark("strong") }},
	{open: "_", close: "_", mark: func(string) []any { return mark("em") }},
}

// parseInline converts a single line of wiki markup into a slice of ADF inline
// nodes. Recognized runs (*bold*, _italic_, {{code}}, [text|url]) become marked
// text nodes; everything else becomes plain text nodes. It never fails — an
// unmatched delimiter is treated as literal text.
//
// For [text|url] links, the URL is validated by allowedLinkHref; disallowed
// schemes (e.g. javascript:, data:) fall back to plain text so untrusted
// config input cannot inject script-execution content.
func parseInline(line string) []any {
	var nodes []any
	var plain strings.Builder

	flush := func() {
		if plain.Len() > 0 {
			nodes = append(nodes, textNode(plain.String(), nil))
			plain.Reset()
		}
	}

	i := 0
	for i < len(line) {
		// Link: [text|url]
		if line[i] == '[' {
			if end := strings.IndexByte(line[i:], ']'); end >= 0 {
				inner := line[i+1 : i+end]
				if bar := strings.IndexByte(inner, '|'); bar >= 0 {
					text := inner[:bar]
					url := inner[bar+1:]
					flush()
					if allowedLinkHref(url) {
						nodes = append(nodes, textNode(text, []any{
							map[string]any{keyType: "link", "attrs": map[string]any{"href": url}},
						}))
					} else {
						// Disallowed scheme: render the whole construct as plain text.
						nodes = append(nodes, textNode("["+inner+"]", nil))
					}
					i += end + 1
					continue
				}
			}
		}

		matched := false
		for _, r := range inlineRules {
			if !strings.HasPrefix(line[i:], r.open) {
				continue
			}
			rest := line[i+len(r.open):]
			if end := strings.Index(rest, r.close); end >= 0 {
				inner := rest[:end]
				if inner == "" {
					continue // e.g. "**" — not a valid run, treat as literal
				}
				flush()
				nodes = append(nodes, textNode(inner, r.mark(inner)))
				i += len(r.open) + end + len(r.close)
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		plain.WriteByte(line[i])
		i++
	}
	flush()

	if len(nodes) == 0 {
		return []any{textNode(line, nil)}
	}
	return nodes
}
