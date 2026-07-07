// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"net/url"
	"strings"
)

// Mode is the resolved Jira platform mode. It is decided exactly once when the
// client is constructed; downstream code branches on the client's stored mode
// instead of re-inspecting URLs or environment variables.
type Mode int

const (
	// ModeDataCenter is Jira Data Center / Server: Bearer PAT auth, REST API v2.
	ModeDataCenter Mode = iota
	// ModeCloud is Jira Cloud: Basic email+API-token auth, REST API v3.
	ModeCloud
)

// APIVersion returns the REST API version path segment for the mode
// ("2" for Data Center, "3" for Cloud).
func (m Mode) APIVersion() string {
	if m == ModeCloud {
		return "3"
	}
	return "2"
}

// String returns a human-readable name for the mode, used in verbose output.
func (m Mode) String() string {
	if m == ModeCloud {
		return "Cloud"
	}
	return "Data Center"
}

// Credentials carries the authentication input for a Jira client. Email is only
// used in Cloud mode (Basic auth); Token is used in both modes.
type Credentials struct {
	Email string
	Token string
}

// cloudHostSuffix is the DNS suffix that identifies a Jira Cloud site.
const cloudHostSuffix = ".atlassian.net"

// ResolveMode decides the platform mode from the Jira base URL and an optional
// explicit override. It returns the resolved Mode and autoDetected: autoDetected
// is true when the mode came from host inspection, and false when override
// forced it. Hosts ending in ".atlassian.net" auto-select Cloud; every other
// host (including an empty/unparseable URL) auto-selects Data Center.
func ResolveMode(rawURL string, override *bool) (Mode, bool) {
	if override != nil {
		if *override {
			return ModeCloud, false
		}
		return ModeDataCenter, false
	}

	host := hostOf(rawURL)
	if strings.HasSuffix(host, cloudHostSuffix) {
		return ModeCloud, true
	}
	return ModeDataCenter, true
}

// hostOf extracts the lowercase hostname from rawURL. If rawURL has no scheme,
// https:// is assumed so that url.Parse populates Host. Returns "" when the URL
// is empty or cannot be parsed.
func hostOf(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
