// SPDX-License-Identifier: Apache-2.0
package jira

import "regexp"

var jiraKeyRE = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

// IsJiraKey reports whether s is a valid Jira issue key (e.g. "DEMO-2679").
func IsJiraKey(s string) bool { return jiraKeyRE.MatchString(s) }
