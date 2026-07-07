// SPDX-License-Identifier: Apache-2.0

// Package ui holds small terminal-output helpers shared by the cmd layer and
// internal packages that print user-facing status lines. Keeping the emoji
// fallback logic in one place ensures every status marker degrades to a
// plain-text label (e.g. "[WARN]") in non-emoji terminals.
package ui

import "os"

// UseEmoji reports whether emoji output is appropriate (NO_COLOR is not set
// and TERM is not dumb).
func UseEmoji() bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

// Emoji returns e when emoji output is appropriate, otherwise fallback.
func Emoji(e, fallback string) string {
	if UseEmoji() {
		return e
	}
	return fallback
}
