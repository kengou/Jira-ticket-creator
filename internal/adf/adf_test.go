// SPDX-License-Identifier: Apache-2.0
package adf

import (
	"reflect"
	"strings"
	"testing"
)

func TestWikiToADF_EmptyInput(t *testing.T) {
	got, degraded := WikiToADF("")
	want := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WikiToADF(\"\") = %#v, want %#v", got, want)
	}
	if degraded {
		t.Error("empty input should not be degraded")
	}
}

func TestWikiToADF_SinglePlainParagraph(t *testing.T) {
	got, degraded := WikiToADF("hello world")
	want := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "hello world"},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WikiToADF = %#v, want %#v", got, want)
	}
	if degraded {
		t.Error("plain paragraph should not be degraded")
	}
}

func TestDocHelper_WrapsContent(t *testing.T) {
	got := doc([]any{textNode("x", nil)})
	want := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{"type": "text", "text": "x"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("doc() = %#v, want %#v", got, want)
	}
}

func TestTextNode_WithMarks(t *testing.T) {
	got := textNode("bold", []any{map[string]any{"type": "strong"}})
	want := map[string]any{
		"type":  "text",
		"text":  "bold",
		"marks": []any{map[string]any{"type": "strong"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("textNode = %#v, want %#v", got, want)
	}
}

func TestParagraph_WrapsInlineContent(t *testing.T) {
	got := paragraph([]any{textNode("hi", nil)})
	want := map[string]any{
		"type": "paragraph",
		"content": []any{
			map[string]any{"type": "text", "text": "hi"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paragraph = %#v, want %#v", got, want)
	}
}

func TestADFToText_Nil(t *testing.T) {
	if got := ADFToText(nil); got != "" {
		t.Errorf("ADFToText(nil) = %q, want \"\"", got)
	}
}

func TestADFToText_ParagraphWithMarks(t *testing.T) {
	// Round-trip: build an ADF doc then flatten it. Marks must not appear in text.
	adfDoc, _ := WikiToADF("This is *bold* text.")
	got := ADFToText(adfDoc)
	want := "This is bold text."
	if got != want {
		t.Errorf("ADFToText = %q, want %q", got, want)
	}
}

func TestADFToText_HeadingAndParagraph(t *testing.T) {
	adfDoc, _ := WikiToADF("h1. Title\n\nBody line.")
	got := ADFToText(adfDoc)
	want := "Title\n\nBody line."
	if got != want {
		t.Errorf("ADFToText = %q, want %q", got, want)
	}
}

func TestADFToText_BulletList(t *testing.T) {
	adfDoc, _ := WikiToADF("* one\n* two")
	got := ADFToText(adfDoc)
	want := "- one\n- two"
	if got != want {
		t.Errorf("ADFToText = %q, want %q", got, want)
	}
}

func TestADFToText_DecodedJSON(t *testing.T) {
	// Simulate what json.Unmarshal into any produces (numbers become float64).
	adfDoc := map[string]any{
		"version": float64(1),
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "hello"},
				},
			},
		},
	}
	got := ADFToText(adfDoc)
	if got != "hello" {
		t.Errorf("ADFToText(decoded) = %q, want %q", got, "hello")
	}
}

func TestADFToText_NoJSONArtifacts(t *testing.T) {
	adfDoc, _ := WikiToADF("h2. Header\n\n* item *bold*\n\nplain")
	got := ADFToText(adfDoc)
	for _, artifact := range []string{"{", "}", "map[", "type:", "\"text\""} {
		if strings.Contains(got, artifact) {
			t.Errorf("output contains JSON artifact %q: %q", artifact, got)
		}
	}
}

// TestADFToText_NestedBulletList exercises the flattening of a nested list
// document: bulletList → listItem → paragraph + nested bulletList. All items
// (parent and child) must appear in the output text and no JSON artifacts
// such as "map[" or "{" may be present.
//
// Bug found during review: the previous flattenBlock implementation appended
// nested-list text directly to the parent item text (producing e.g.
// "- parentchild") because it called flattenInline on the whole listItem
// content including the nested bulletList node, without inserting a newline.
// Fixed by introducing flattenList/flattenListItem that handle nested lists
// explicitly.
func TestADFToText_NestedBulletList(t *testing.T) {
	adfDoc, _ := WikiToADF("* parent\n** child\n* sibling")
	got := ADFToText(adfDoc)

	// All three item texts must appear independently.
	for _, want := range []string{"parent", "child", "sibling"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing item %q: %q", want, got)
		}
	}

	// "parent" and "child" must NOT be on the same line (no "parentchild").
	if strings.Contains(got, "parentchild") {
		t.Errorf("nested item was concatenated to parent item (got %q)", got)
	}

	// No JSON artifacts.
	for _, artifact := range []string{"map[", "{", "}"} {
		if strings.Contains(got, artifact) {
			t.Errorf("output contains JSON artifact %q: %q", artifact, got)
		}
	}
}
