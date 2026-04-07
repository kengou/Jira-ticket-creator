// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// interactiveEnabled reports whether stdin is an interactive terminal,
// meaning we can prompt the user for missing inputs.
func interactiveEnabled() bool {
	return isTTY(os.Stdin)
}

// promptLine prints msg to stderr and reads a full line from r.
// Returns the trimmed input. Returns "" on read error or empty input.
func promptLine(w io.Writer, r *bufio.Reader, msg string) string {
	fmt.Fprint(w, msg) //nolint:errcheck // best-effort stderr prompt
	line, err := r.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

// promptSelect prints a numbered list of choices and asks the user to pick one.
// Returns the 0-based index of the selected choice, or -1 on invalid input.
func promptSelect(w io.Writer, r *bufio.Reader, msg string, choices []string) int {
	fmt.Fprintln(w, msg) //nolint:errcheck // best-effort stderr prompt
	for i, c := range choices {
		fmt.Fprintf(w, "  %d) %s\n", i+1, c) //nolint:errcheck // best-effort stderr prompt
	}
	answer := promptLine(w, r, "Choose [1-"+strconv.Itoa(len(choices))+"]: ")
	n, err := strconv.Atoi(answer)
	if err != nil || n < 1 || n > len(choices) {
		return -1
	}
	return n - 1
}
