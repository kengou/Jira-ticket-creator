// SPDX-License-Identifier: Apache-2.0
package source_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kengou/Jira-ticket-creator/internal/source"
)

// makeDir creates a temp directory and writes the given files into it.
// files is a map of relative path → content.
func makeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}

// --- ListFiles security regression tests ---

func TestListFiles_ExcludesDocsByDefault(t *testing.T) {
	dir := makeDir(t, map[string]string{
		"main.go":   "package main",
		"README.md": "# README",
		"notes.txt": "some notes",
	})
	files, err := source.ListFiles(dir, source.BuildOptions{})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".md") || strings.HasSuffix(f, ".txt") {
			t.Errorf("doc file %q should be excluded by default", f)
		}
	}
	var hasGo bool
	for _, f := range files {
		if f == "main.go" {
			hasGo = true
		}
	}
	if !hasGo {
		t.Error("expected main.go to be included")
	}
}

func TestListFiles_IncludesDocsWhenOptIn(t *testing.T) {
	dir := makeDir(t, map[string]string{
		"main.go":   "package main",
		"README.md": "# README",
	})
	files, err := source.ListFiles(dir, source.BuildOptions{IncludeDocs: true})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	var hasMD bool
	for _, f := range files {
		if strings.HasSuffix(f, ".md") {
			hasMD = true
		}
	}
	if !hasMD {
		t.Error("expected .md file when IncludeDocs=true")
	}
}

func TestListFiles_ExcludesSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real.go"), filepath.Join(dir, "link.go")); err != nil {
		t.Skip("symlinks not supported on this platform")
	}
	files, err := source.ListFiles(dir, source.BuildOptions{})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if f == "link.go" {
			t.Error("symlink should be excluded from ListFiles")
		}
	}
}

// --- BuildContextFromFiles security regression tests ---

func TestBuildContextFromFiles_PathTraversalDotDot(t *testing.T) {
	dir := makeDir(t, map[string]string{
		"main.go": "package main",
	})
	// Attempt path traversal via relative path
	ctx, err := source.BuildContextFromFiles(dir, []string{"../../../etc/passwd"})
	if err != nil {
		t.Fatalf("BuildContextFromFiles: %v", err)
	}
	// Check that actual file content was not leaked (the relative path string
	// "../../../etc/passwd" naturally contains "/etc/passwd" as a substring and
	// will appear in the tree listing — that is acceptable).
	if strings.Contains(ctx, "root:") || strings.Contains(ctx, "nobody:") {
		t.Error("path traversal succeeded — /etc/passwd content found in context")
	}
}

func TestBuildContextFromFiles_PathTraversalAbsolute(t *testing.T) {
	dir := makeDir(t, map[string]string{
		"main.go": "package main",
	})
	ctx, err := source.BuildContextFromFiles(dir, []string{"/etc/passwd"})
	if err != nil {
		t.Fatalf("BuildContextFromFiles: %v", err)
	}
	if strings.Contains(ctx, "root:") {
		t.Error("absolute path traversal succeeded — /etc/passwd content found in context")
	}
}

func TestBuildContextFromFiles_DelimitersPresent(t *testing.T) {
	dir := makeDir(t, map[string]string{
		"main.go": "package main",
	})
	files, err := source.ListFiles(dir, source.BuildOptions{})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	ctx, err := source.BuildContextFromFiles(dir, files)
	if err != nil {
		t.Fatalf("BuildContextFromFiles: %v", err)
	}
	if !strings.Contains(ctx, "<source-file") {
		t.Error("expected <source-file> opening delimiter in context")
	}
	if !strings.Contains(ctx, "</source-file>") {
		t.Error("expected </source-file> closing delimiter in context")
	}
	if !strings.Contains(ctx, "PASSIVE CONTEXT ONLY") {
		t.Error("expected PASSIVE CONTEXT ONLY header in context")
	}
}
