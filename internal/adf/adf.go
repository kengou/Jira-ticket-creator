// SPDX-License-Identifier: Apache-2.0
//
// Package adf converts Jira wiki markup to Atlassian Document Format (ADF)
// and flattens ADF documents back to plain text. It is a pure package: no
// HTTP, no config, no knowledge of the Jira client. Conversion never returns
// an error — unrecognized markup degrades to plain text inside a structurally
// valid ADF document, and the degraded flag reports when that happened.
package adf

import "strings"

// Document is an ADF document represented as a plain Go map tree so it
// marshals directly to the ADF JSON shape that Jira Cloud's v3 API accepts.
// The top-level map always contains:
//
//	"version": 1          — ADF schema version
//	"type":    "doc"      — root node type
//	"content": []any{…}  — slice of block-level ADF nodes
//
// Using a type alias (not a defined type) keeps Document assignment-compatible
// with map[string]any everywhere callers embed it into request payloads.
type Document = map[string]any

// ADF node object keys shared by every node kind in this package.
const (
	keyType    = "type"
	keyContent = "content"
)

// doc wraps top-level content nodes in an ADF document root.
func doc(content []any) Document {
	if content == nil {
		content = []any{}
	}
	return Document{
		"version":  1,
		keyType:    "doc",
		keyContent: content,
	}
}

// paragraph wraps inline content nodes in an ADF paragraph node.
func paragraph(content []any) map[string]any {
	return map[string]any{
		keyType:    "paragraph",
		keyContent: content,
	}
}

// textNode builds an ADF text node with optional marks. Pass nil marks for
// unmarked text.
func textNode(text string, marks []any) map[string]any {
	n := map[string]any{
		keyType: "text",
		"text":  text,
	}
	if len(marks) > 0 {
		n["marks"] = marks
	}
	return n
}

// WikiToADF converts a Jira wiki-markup string into an ADF document represented
// as map[string]any / []any structures that marshal to the ADF JSON shape.
//
// It never returns an error. The returned degraded flag is true when any input
// construct could not be represented structurally and was emitted as plain text
// instead.
//
// This initial implementation treats every non-empty line as a plain paragraph.
// Heading, inline-mark, list, code-block, rule, and link handling are layered in
// by later converter internals (see blocks.go and inline.go).
func WikiToADF(markup string) (Document, bool) {
	content, degraded := parseBlocks(markup)
	return doc(content), degraded
}

// ADFToText flattens an ADF document (as decoded JSON: map[string]any, []any,
// string) into readable plain text with no JSON artifacts. It never panics on
// malformed or partial input; unknown structures contribute the flattened text
// of any nested content. Nil or non-document input yields an empty string.
func ADFToText(node any) string {
	content := childContent(node)
	blocks := make([]string, 0, len(content))
	for _, child := range content {
		if s := flattenBlock(child); s != "" {
			blocks = append(blocks, s)
		}
	}
	return strings.Join(blocks, "\n\n")
}

// flattenBlock renders a single top-level block node to text.
func flattenBlock(node any) string {
	m, ok := node.(map[string]any)
	if !ok {
		return ""
	}
	switch m[keyType] {
	case "bulletList", "orderedList":
		return flattenList(m, "- ")
	case "rule":
		return "----"
	default:
		return flattenInline(childContent(node))
	}
}

// flattenList renders a list node (bulletList or orderedList) to text. Each
// listItem contributes one "- text" line; nested lists within a listItem are
// rendered recursively with the same prefix so child items appear on their own
// lines without JSON artifacts.
func flattenList(listNode map[string]any, prefix string) string {
	items := childContent(listNode)
	lines := make([]string, 0, len(items))
	for _, it := range items {
		lines = append(lines, flattenListItem(it, prefix))
	}
	return strings.Join(lines, "\n")
}

// flattenListItem renders a single listItem node. The first paragraph child
// supplies the item text; any subsequent nested list nodes are rendered
// recursively and appended as additional lines.
func flattenListItem(node any, prefix string) string {
	var parts []string
	for _, child := range childContent(node) {
		cm, ok := child.(map[string]any)
		if !ok {
			continue
		}
		switch cm[keyType] {
		case "bulletList", "orderedList":
			parts = append(parts, flattenList(cm, prefix))
		default:
			if s := flattenInline(childContent(child)); s != "" {
				parts = append(parts, prefix+s)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// childContent returns the "content" slice of a node, or nil.
func childContent(node any) []any {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	c, ok := m[keyContent].([]any)
	if !ok {
		return nil
	}
	return c
}

// flattenInline concatenates the text of inline/nested nodes, recursing into any
// nested content (e.g. a paragraph inside a listItem, or a nested list).
func flattenInline(content []any) string {
	var b strings.Builder
	for _, c := range content {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := m["text"].(string); ok {
			b.WriteString(t)
			continue
		}
		if nested, ok := m[keyContent].([]any); ok {
			b.WriteString(flattenInline(nested))
		}
	}
	return b.String()
}
