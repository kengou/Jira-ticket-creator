// SPDX-License-Identifier: Apache-2.0

package jira

import "testing"

func TestMode_APIVersion(t *testing.T) {
	if got := ModeDataCenter.APIVersion(); got != "2" {
		t.Errorf("ModeDataCenter.APIVersion() = %q, want %q", got, "2")
	}
	if got := ModeCloud.APIVersion(); got != "3" {
		t.Errorf("ModeCloud.APIVersion() = %q, want %q", got, "3")
	}
}

func TestMode_String(t *testing.T) {
	if got := ModeDataCenter.String(); got != "Data Center" {
		t.Errorf("ModeDataCenter.String() = %q, want %q", got, "Data Center")
	}
	if got := ModeCloud.String(); got != "Cloud" {
		t.Errorf("ModeCloud.String() = %q, want %q", got, "Cloud")
	}
}

func TestResolveMode(t *testing.T) {
	trueVal := true
	falseVal := false
	cases := []struct {
		name     string
		url      string
		override *bool
		wantMode Mode
		wantAuto bool
	}{
		{"atlassian_net_auto_cloud", "https://acme.atlassian.net", nil, ModeCloud, true},
		{"atlassian_net_no_scheme", "acme.atlassian.net", nil, ModeCloud, true},
		{"atlassian_net_with_port", "https://acme.atlassian.net:443", nil, ModeCloud, true},
		{"atlassian_net_case_insensitive", "https://ACME.ATLASSIAN.NET", nil, ModeCloud, true},
		{"company_hosted_auto_dc", "https://jira.company.com", nil, ModeDataCenter, true},
		{"empty_url_auto_dc", "", nil, ModeDataCenter, true},
		{"override_true_forces_cloud", "https://jira.company.com", &trueVal, ModeCloud, false},
		{"override_false_forces_dc", "https://acme.atlassian.net", &falseVal, ModeDataCenter, false},
		{"override_true_on_atlassian", "https://acme.atlassian.net", &trueVal, ModeCloud, false},
		// Edge cases pinning current (safe) behavior:
		// Trailing-dot FQDN: "acme.atlassian.net." host has a trailing dot so
		// HasSuffix(".atlassian.net") is false → Data Center.
		{"trailing_dot_fqdn", "https://acme.atlassian.net.", nil, ModeDataCenter, true},
		// Unparseable URL (control character) → hostOf returns "" → Data Center.
		{"unparseable_url_control_char", "https://evil\x01acme.atlassian.net", nil, ModeDataCenter, true},
		// Suffix-spoofing host "evil-atlassian.net": ends in "-atlassian.net", not ".atlassian.net" → DC.
		{"suffix_spoof_dash", "https://evil-atlassian.net", nil, ModeDataCenter, true},
		// Subdomain-spoof "atlassian.net.evil.com": HasSuffix(".atlassian.net") is false → DC.
		{"suffix_spoof_dot_evil", "https://atlassian.net.evil.com", nil, ModeDataCenter, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, auto := ResolveMode(tc.url, tc.override)
			if mode != tc.wantMode {
				t.Errorf("ResolveMode(%q, %v) mode = %v, want %v", tc.url, tc.override, mode, tc.wantMode)
			}
			if auto != tc.wantAuto {
				t.Errorf("ResolveMode(%q, %v) autoDetected = %v, want %v", tc.url, tc.override, auto, tc.wantAuto)
			}
		})
	}
}
