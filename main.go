// SPDX-License-Identifier: Apache-2.0
//
// jira-ai-creator creates Jira issues from YAML configuration files.
package main

import (
	"fmt"
	"os"

	"github.com/kengou/jira-ticket-creator/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
