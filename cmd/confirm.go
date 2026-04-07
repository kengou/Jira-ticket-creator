// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"fmt"
	"io"
	"os"
)

// confirmPrompt writes msg to w and reads a y/N answer from r.
// Returns true only if the user typed "y" or "Y".
// If r is nil, os.Stdin is used. If w is nil, os.Stderr is used.
func confirmPrompt(w io.Writer, r io.Reader, msg string) bool {
	if w == nil {
		w = os.Stderr
	}
	if r == nil {
		r = os.Stdin
	}
	fmt.Fprint(w, msg) //nolint:errcheck // best-effort stderr prompt
	var answer string
	if _, err := fmt.Fscanln(r, &answer); err != nil {
		return false
	}
	return answer == "y" || answer == "Y"
}
