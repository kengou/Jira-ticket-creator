// SPDX-License-Identifier: Apache-2.0
package adf

import (
	"reflect"
	"testing"
)

func TestParseBlocks_Heading(t *testing.T) {
	content, degraded := parseBlocks("h2. My Heading")
	want := []any{
		map[string]any{
			"type":    "heading",
			"attrs":   map[string]any{"level": 2},
			"content": []any{map[string]any{"type": "text", "text": "My Heading"}},
		},
	}
	if !reflect.DeepEqual(content, want) {
		t.Errorf("content = %#v, want %#v", content, want)
	}
	if degraded {
		t.Error("heading should not degrade")
	}
}

func TestParseBlocks_ParagraphWithInline(t *testing.T) {
	content, _ := parseBlocks("This is *bold*.")
	want := []any{
		map[string]any{
			"type": "paragraph",
			"content": []any{
				map[string]any{"type": "text", "text": "This is "},
				map[string]any{"type": "text", "text": "bold", "marks": []any{map[string]any{"type": "strong"}}},
				map[string]any{"type": "text", "text": "."},
			},
		},
	}
	if !reflect.DeepEqual(content, want) {
		t.Errorf("content = %#v, want %#v", content, want)
	}
}

func TestParseBlocks_HorizontalRule(t *testing.T) {
	content, _ := parseBlocks("----")
	want := []any{map[string]any{"type": "rule"}}
	if !reflect.DeepEqual(content, want) {
		t.Errorf("content = %#v, want %#v", content, want)
	}
}

func TestParseBlocks_CodeBlockWithLanguage(t *testing.T) {
	content, _ := parseBlocks("{code:go}\nfmt.Println(\"x\")\n{code}")
	want := []any{
		map[string]any{
			"type":    "codeBlock",
			"attrs":   map[string]any{"language": "go"},
			"content": []any{map[string]any{"type": "text", "text": "fmt.Println(\"x\")"}},
		},
	}
	if !reflect.DeepEqual(content, want) {
		t.Errorf("content = %#v, want %#v", content, want)
	}
}

func TestParseBlocks_NoformatBlock(t *testing.T) {
	content, _ := parseBlocks("{noformat}\nraw text\n{noformat}")
	want := []any{
		map[string]any{
			"type":    "codeBlock",
			"content": []any{map[string]any{"type": "text", "text": "raw text"}},
		},
	}
	if !reflect.DeepEqual(content, want) {
		t.Errorf("content = %#v, want %#v", content, want)
	}
}

func TestParseBlocks_UnclosedCodeBlockDegrades(t *testing.T) {
	// An unclosed code block must not fail; the remaining lines are captured as
	// the code block body and degraded is reported true.
	content, degraded := parseBlocks("{code}\nline one\nline two")
	if !degraded {
		t.Error("unclosed code block should set degraded = true")
	}
	// Structure must still be a valid single codeBlock node containing the raw text.
	if len(content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(content))
	}
	node, ok := content[0].(map[string]any)
	if !ok || node["type"] != "codeBlock" {
		t.Fatalf("content[0] = %#v, want a codeBlock node", content[0])
	}
}

func TestParseBlocks_UnknownMacroDegradesToPlainText(t *testing.T) {
	// {panel} is unsupported. It must degrade to a plain paragraph and set degraded.
	content, degraded := parseBlocks("{panel}\ninside panel\n{panel}")
	if !degraded {
		t.Error("unknown macro should set degraded = true")
	}
	// Every emitted node must be structurally valid (paragraph or similar), never
	// a raw macro node. Assert at least one paragraph carries the literal text.
	found := false
	for _, n := range content {
		node, ok := n.(map[string]any)
		if !ok {
			t.Fatalf("node is not a map: %#v", n)
		}
		if node["type"] != "paragraph" && node["type"] != "heading" {
			t.Errorf("degraded node type = %v, want paragraph/heading", node["type"])
		}
		if inner, ok := node["content"].([]any); ok {
			for _, c := range inner {
				if tn, ok := c.(map[string]any); ok {
					if s, ok := tn["text"].(string); ok && (s == "{panel}" || s == "inside panel") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("degraded output should still contain the raw macro text as plain text")
	}
}

func TestWikiToADF_DelegatesToBlocks(t *testing.T) {
	got, _ := WikiToADF("h1. T")
	content, ok := got["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content = %#v, want one heading node", got["content"])
	}
	node, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] = %#v, want a map node", content[0])
	}
	if node["type"] != "heading" {
		t.Errorf("node type = %v, want heading (WikiToADF must delegate to parseBlocks)", node["type"])
	}
}

func TestParseBlocks_AIPromptTemplateConvertsCleanly(t *testing.T) {
	// The exact description template the AI prompt generates must convert to a
	// well-formed ADF document with no degradation.
	tmpl := "*As a* _platform engineer_\n\n" +
		"*I want to* _run the backend container as a non-root user_\n\n" +
		"*So that* _runtime permissions align_\n\n" +
		"h3. Acceptance Criteria\n\n" +
		"* Given the backend image is rebuilt\n" +
		"* Kubernetes securityContext passes\n\n" +
		"*References:*\n" +
		"* Docker Security: https://docs.docker.com/x/"
	content, degraded := parseBlocks(tmpl)
	if degraded {
		t.Errorf("AI prompt template should convert cleanly (degraded=false), got degraded=true")
	}
	if len(content) == 0 {
		t.Fatal("AI prompt template produced empty content")
	}
	// First block is a paragraph containing bold+italic inline marks.
	first, ok := content[0].(map[string]any)
	if !ok || first["type"] != "paragraph" {
		t.Fatalf("first block = %#v, want paragraph", content[0])
	}
}

func TestParseListRun_FlatBulletList(t *testing.T) {
	nodes, consumed, ok := parseListRun([]string{"* one", "* two", "next"})
	if !ok {
		t.Fatal("expected ok = true for bullet list")
	}
	if consumed != 2 {
		t.Errorf("consumed = %d, want 2", consumed)
	}
	want := []any{
		map[string]any{
			"type": "bulletList",
			"content": []any{
				map[string]any{
					"type": "listItem",
					"content": []any{
						map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "one"}}},
					},
				},
				map[string]any{
					"type": "listItem",
					"content": []any{
						map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "two"}}},
					},
				},
			},
		},
	}
	if !reflect.DeepEqual(nodes, want) {
		t.Errorf("nodes = %#v, want %#v", nodes, want)
	}
}

func TestParseListRun_FlatOrderedList(t *testing.T) {
	nodes, consumed, ok := parseListRun([]string{"# first", "# second"})
	if !ok || consumed != 2 {
		t.Fatalf("ok=%v consumed=%d, want ok=true consumed=2", ok, consumed)
	}
	list, ok := nodes[0].(map[string]any)
	if !ok {
		t.Fatalf("nodes[0] = %#v, want a map node", nodes[0])
	}
	if list["type"] != "orderedList" {
		t.Errorf("list type = %v, want orderedList", list["type"])
	}
}

func TestParseListRun_NestedBulletList(t *testing.T) {
	nodes, consumed, ok := parseListRun([]string{"* parent", "** child", "* sibling"})
	if !ok || consumed != 3 {
		t.Fatalf("ok=%v consumed=%d, want ok=true consumed=3", ok, consumed)
	}
	want := []any{
		map[string]any{
			"type": "bulletList",
			"content": []any{
				map[string]any{
					"type": "listItem",
					"content": []any{
						map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "parent"}}},
						map[string]any{
							"type": "bulletList",
							"content": []any{
								map[string]any{
									"type": "listItem",
									"content": []any{
										map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "child"}}},
									},
								},
							},
						},
					},
				},
				map[string]any{
					"type": "listItem",
					"content": []any{
						map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "sibling"}}},
					},
				},
			},
		},
	}
	if !reflect.DeepEqual(nodes, want) {
		t.Errorf("nodes = %#v, want %#v", nodes, want)
	}
}

func TestParseListRun_NotAList(t *testing.T) {
	_, _, ok := parseListRun([]string{"plain line"})
	if ok {
		t.Error("plain line should not be recognized as a list")
	}
}

func TestParseBlocks_ListInsideDocument(t *testing.T) {
	content, degraded := parseBlocks("intro\n\n* a\n* b\n\noutro")
	if degraded {
		t.Error("simple list document should not degrade")
	}
	// Expect: paragraph, bulletList, paragraph.
	if len(content) != 3 {
		t.Fatalf("len(content) = %d, want 3 (paragraph, bulletList, paragraph)", len(content))
	}
	mid, ok := content[1].(map[string]any)
	if !ok {
		t.Fatalf("content[1] = %#v, want a map node", content[1])
	}
	if mid["type"] != "bulletList" {
		t.Errorf("content[1] type = %v, want bulletList", mid["type"])
	}
}

// TestParseListRun_ThreeLevelNesting verifies the behaviour when wiki markup
// contains a depth-3 list marker ("***"). This package supports only depth 1
// and depth 2 nesting. The documented behaviour is:
//
//   - The depth-1 item ("* a") and depth-2 item ("** b") are consumed and
//     nested correctly by parseListRun.
//   - The depth-3 item ("*** c") is NOT consumed by parseListRun (listMarker
//     does not recognise it), so parseBlocks emits it as a plain paragraph
//     and sets degraded=true to signal the structural loss.
//
// This prevents silent data-loss: callers can inspect the degraded flag and
// warn users that the deep nesting was flattened.
func TestParseListRun_ThreeLevelNesting(t *testing.T) {
	// parseListRun itself stops at "*** c" — it only sees "* a" and "** b".
	lines := []string{"* a", "** b", "*** c"}
	_, consumed, ok := parseListRun(lines)
	if !ok {
		t.Fatal("parseListRun: expected ok=true for '* a' starter")
	}
	if consumed != 2 {
		t.Errorf("parseListRun consumed %d lines, want 2 (*** c must be left for parseBlocks)", consumed)
	}

	// parseBlocks must set degraded=true when it encounters the depth-3 line.
	_, degraded := parseBlocks("* a\n** b\n*** c")
	if !degraded {
		t.Error("parseBlocks: depth-3 marker '*** c' should set degraded=true")
	}
}

// TestParseBlocks_ThreeLevelOrderedNesting verifies the same degradation
// behaviour for ordered-list depth-3 markers ("###").
func TestParseBlocks_ThreeLevelOrderedNesting(t *testing.T) {
	_, degraded := parseBlocks("# a\n## b\n### c")
	if !degraded {
		t.Error("parseBlocks: depth-3 ordered marker '### c' should set degraded=true")
	}
}
