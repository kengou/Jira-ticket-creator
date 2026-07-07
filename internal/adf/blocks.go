// SPDX-License-Identifier: Apache-2.0
package adf

import "strings"

// headingPrefix matches "hN. " where N is 1-6. Returns level and the remaining
// text, plus ok=false when the line is not a heading.
func headingPrefix(line string) (level int, text string, ok bool) {
	if len(line) < 4 || line[0] != 'h' || line[2] != '.' {
		return 0, "", false
	}
	n := line[1]
	if n < '1' || n > '6' {
		return 0, "", false
	}
	rest := line[3:]
	rest = strings.TrimPrefix(rest, " ")
	return int(n - '0'), rest, true
}

// isRule reports whether a line is a horizontal rule (4+ dashes, nothing else).
func isRule(line string) bool {
	t := strings.TrimSpace(line)
	if len(t) < 4 {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] != '-' {
			return false
		}
	}
	return true
}

// codeFence detects an opening {code}, {code:lang}, or {noformat} line. Returns
// the language (may be empty), a fence kind, and ok.
func codeFence(line string) (lang, closer string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "{noformat}" {
		return "", "{noformat}", true
	}
	if t == "{code}" {
		return "", "{code}", true
	}
	if strings.HasPrefix(t, "{code:") && strings.HasSuffix(t, "}") {
		return t[len("{code:") : len(t)-1], "{code}", true
	}
	return "", "", false
}

// unknownMacro reports whether a line looks like a standalone {macro} we do not
// support (e.g. {panel}, {info}). Used to trigger graceful degradation.
func unknownMacro(line string) bool {
	t := strings.TrimSpace(line)
	if len(t) < 3 || t[0] != '{' || t[len(t)-1] != '}' {
		return false
	}
	// Exclude the inline monospace token {{...}} and known fences.
	if strings.HasPrefix(t, "{{") {
		return false
	}
	if _, _, ok := codeFence(t); ok {
		return false
	}
	return true
}

// codeBlockNode builds a codeBlock node from body lines and an optional language.
func codeBlockNode(body []string, lang string) map[string]any {
	node := map[string]any{
		keyType:    "codeBlock",
		keyContent: []any{textNode(strings.Join(body, "\n"), nil)},
	}
	if lang != "" {
		node["attrs"] = map[string]any{"language": lang}
	}
	return node
}

// parseBlocks converts wiki markup into top-level ADF content nodes. It handles
// headings, the horizontal rule, {code}/{noformat} blocks, and paragraphs. List
// handling is layered in by parseListRun (see lists in blocks-list code). Unknown
// constructs degrade to plain paragraphs and set degraded=true. It never fails.
func parseBlocks(markup string) ([]any, bool) {
	if strings.TrimSpace(markup) == "" {
		return []any{}, false
	}

	lines := strings.Split(markup, "\n")
	var content []any
	degraded := false

	i := 0
	for i < len(lines) {
		line := lines[i]

		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		if _, closer, ok := codeFence(line); ok {
			lang, _, _ := codeFence(line)
			var body []string
			i++
			closed := false
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == closer {
					closed = true
					i++
					break
				}
				body = append(body, lines[i])
				i++
			}
			if !closed {
				degraded = true // unclosed fence
			}
			content = append(content, codeBlockNode(body, lang))
			continue
		}

		if isRule(line) {
			content = append(content, map[string]any{keyType: "rule"})
			i++
			continue
		}

		if level, text, ok := headingPrefix(line); ok {
			content = append(content, map[string]any{
				keyType:    "heading",
				"attrs":    map[string]any{"level": level},
				keyContent: parseInline(text),
			})
			i++
			continue
		}

		if listNodes, consumed, ok := parseListRun(lines[i:]); ok {
			content = append(content, listNodes...)
			i += consumed
			continue
		}

		if listOverflowMarker(line) {
			degraded = true // depth > 2 list item: nesting is not supported, degrades to paragraph
		} else if unknownMacro(line) {
			degraded = true // unknown macro degrades to plain text, but keep going
		}

		content = append(content, paragraph(parseInline(line)))
		i++
	}

	return content, degraded
}

// listOverflowMarker reports whether line looks like a depth-3+ list marker
// (e.g. "*** text" or "### text") that the wiki subset does not support beyond
// depth 2. Such lines would otherwise silently fall through to a plain paragraph
// without signalling degradation.
func listOverflowMarker(line string) bool {
	for _, prefix := range []string{"*** ", "### "} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// listMarker classifies a list line. ordered reports bullet (false) vs numbered
// (true); depth is 1 for "*"/"#" and 2 for "**"/"##"; text is the item text.
// ok is false when the line is not a list item.
func listMarker(line string) (ordered bool, depth int, text string, ok bool) {
	switch {
	case strings.HasPrefix(line, "** "):
		return false, 2, line[3:], true
	case strings.HasPrefix(line, "* "):
		return false, 1, line[2:], true
	case strings.HasPrefix(line, "## "):
		return true, 2, line[3:], true
	case strings.HasPrefix(line, "# "):
		return true, 1, line[2:], true
	}
	return false, 0, "", false
}

// listTypeName maps ordered/bullet to the ADF list node type.
func listTypeName(ordered bool) string {
	if ordered {
		return "orderedList"
	}
	return "bulletList"
}

// listItemNode builds a listItem whose first child is a paragraph of the item text.
func listItemNode(text string) map[string]any {
	return map[string]any{
		keyType: "listItem",
		keyContent: []any{
			paragraph(parseInline(text)),
		},
	}
}

// parseListRun consumes a contiguous run of list lines starting at lines[0] and
// returns the ADF list node(s). Depth-2 markers (** / ##) nest inside the most
// recent depth-1 listItem. ok is false when lines[0] is not a list line.
func parseListRun(lines []string) ([]any, int, bool) {
	firstOrdered, _, _, ok := listMarker(lines[0])
	if !ok {
		return nil, 0, false
	}

	items := []any{} // depth-1 listItems of the top-level list
	consumed := 0

	for consumed < len(lines) {
		ordered, depth, text, isItem := listMarker(lines[consumed])
		if !isItem || ordered != firstOrdered {
			break
		}
		if depth == 1 {
			items = append(items, listItemNode(text))
			consumed++
			continue
		}
		// depth == 2: nest inside the last depth-1 item. If there is no parent
		// item yet, promote this to a depth-1 item to stay structurally valid.
		if len(items) == 0 {
			items = append(items, listItemNode(text))
			consumed++
			continue
		}
		parent, okItem := items[len(items)-1].(map[string]any)
		parentContent, okChildren := parent[keyContent].([]any)
		if !okItem || !okChildren {
			// Unreachable in practice: items only ever holds listItemNode results.
			// Keep the item at depth 1 rather than dropping it.
			items = append(items, listItemNode(text))
			consumed++
			continue
		}
		// Reuse an existing nested list if the parent already has one.
		var nested map[string]any
		var nestedItems []any
		if len(parentContent) > 1 {
			last, okCast := parentContent[len(parentContent)-1].(map[string]any)
			if okCast && last[keyType] == listTypeName(ordered) {
				if c, okC := last[keyContent].([]any); okC {
					nested = last
					nestedItems = c
				}
			}
		}
		if nested == nil {
			nested = map[string]any{keyType: listTypeName(ordered)}
			parent[keyContent] = append(parentContent, nested)
		}
		nested[keyContent] = append(nestedItems, listItemNode(text))
		consumed++
	}

	list := map[string]any{
		keyType:    listTypeName(firstOrdered),
		keyContent: items,
	}
	return []any{list}, consumed, true
}
