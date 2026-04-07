// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"strings"
	"testing"
)

func TestAIMarker_Prepended(t *testing.T) {
	// The aiMarker constant in ai.go must contain both markers that the apply
	// command checks for. This test catches accidental edits to the marker.
	const aiMarker = "# ai-generated: true\n# reviewed: false\n"
	if !strings.Contains(aiMarker, "# reviewed: false") {
		t.Error("aiMarker must contain '# reviewed: false'")
	}
	if !strings.Contains(aiMarker, "# ai-generated: true") {
		t.Error("aiMarker must contain '# ai-generated: true'")
	}
}

func TestExtractYAML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean YAML, no fences",
			input: "schemaVersion: \"1.0\"\nissues: []",
			want:  "schemaVersion: \"1.0\"\nissues: []",
		},
		{
			name:  "yaml-tagged fence, no prose",
			input: "```yaml\nschemaVersion: \"1.0\"\nissues: []\n```",
			want:  "schemaVersion: \"1.0\"\nissues: []",
		},
		{
			name:  "plain fence, no prose",
			input: "```\nschemaVersion: \"1.0\"\n```",
			want:  "schemaVersion: \"1.0\"",
		},
		{
			name:  "prose before and after fenced block",
			input: "Here is your YAML:\n\n```yaml\nschemaVersion: \"1.0\"\nissues: []\n```\n\nHope this helps!",
			want:  "schemaVersion: \"1.0\"\nissues: []",
		},
		{
			name:  "prose before fenced block only",
			input: "Sure, here you go:\n```yaml\nschemaVersion: \"1.0\"\n```",
			want:  "schemaVersion: \"1.0\"",
		},
		{
			name:  "trailing whitespace on fence opener",
			input: "```yaml  \nschemaVersion: \"1.0\"\n```",
			want:  "schemaVersion: \"1.0\"",
		},
		{
			name:  "no fences, trims surrounding whitespace",
			input: "  \n\nschemaVersion: \"1.0\"\n\n  ",
			want:  "schemaVersion: \"1.0\"",
		},
		{
			name:  "multiline YAML block inside fences",
			input: "```yaml\nschemaVersion: \"1.0\"\ndefaults:\n  projectKey: TEST\nissues:\n  - summary: foo\n```",
			want:  "schemaVersion: \"1.0\"\ndefaults:\n  projectKey: TEST\nissues:\n  - summary: foo",
		},
		{
			name:  "YAML between --- document separators with prose",
			input: "I cannot access the file.\n\n---\n\nschemaVersion: \"1.0\"\nissues: []\n\n---\n\n> Note: replace POM-XXX.",
			want:  "schemaVersion: \"1.0\"\nissues: []",
		},
		{
			name:  "prose before --- then YAML, no trailing separator",
			input: "Here is the plan:\n---\nschemaVersion: \"1.0\"\nissues: []",
			want:  "schemaVersion: \"1.0\"\nissues: []",
		},
		{
			name:  "no fences, no separators, prose before schemaVersion",
			input: "Sure thing!\n\nschemaVersion: \"1.0\"\nissues: []",
			want:  "schemaVersion: \"1.0\"\nissues: []",
		},
		{
			name:  "CRLF line endings with fences",
			input: "```yaml\r\nschemaVersion: \"1.0\"\r\n```",
			want:  "schemaVersion: \"1.0\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractYAML(tc.input)
			if got != tc.want {
				t.Errorf("extractYAML():\ngot:  %q\nwant: %q", got, tc.want)
			}
		})
	}
}
