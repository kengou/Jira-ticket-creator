// SPDX-License-Identifier: Apache-2.0
//
// This file holds the platform field-encoder seam. The Applier orchestrates
// (user resolution, createdIssues lookups, degraded/dropped accumulation) and
// delegates all VALUE SHAPING — the byte-for-byte payload differences between
// Jira Data Center (API v2) and Jira Cloud (API v3) — to a fieldEncoder chosen
// once from the resolved jira.Mode.
package apply

import (
	"github.com/kengou/jira-ticket-creator/internal/adf"
	"github.com/kengou/jira-ticket-creator/internal/jira"
)

// fieldEncoder shapes platform-specific field values. Implementations are pure:
// they never perform network calls or mutate applier state. The applier passes
// already-resolved inputs (e.g. the resolved epic key, the resolved user
// reference) so each encoder only decides the payload SHAPE.
type fieldEncoder interface {
	// description returns the value for the "description" field. degraded is
	// true when converting to the target representation lost markup (Cloud ADF).
	description(text string) (value any, degraded bool)

	// userRef shapes an already-resolved user reference (name on Data Center,
	// accountId on Cloud) into the assignee/reporter field value.
	userRef(resolved string) map[string]any

	// epicParent writes the fields entry for a non-sub-task whose parent is an
	// Epic. epicKey is the resolved parent key; epicLinkFieldID is the
	// configured epic-link custom field ID (used only on Data Center).
	epicParent(fields map[string]any, epicKey, epicLinkFieldID string)

	// epicName writes the epic-name field (Data Center) or reports that it was
	// dropped (Cloud, where epics use the summary as their name). dropped is
	// true when the name was not written.
	epicName(fields map[string]any, name, epicNameFieldID string) (dropped bool)

	// epicLink writes the epic-link entry. key is the resolved epic key (the
	// applier performs any internal-ID→created-key lookup before calling);
	// epicLinkFieldID is the configured custom field ID (Data Center only).
	epicLink(fields map[string]any, key, epicLinkFieldID string)

	// linkComment shapes an issue-link comment body. degraded is true when the
	// conversion to the target representation lost markup (Cloud ADF).
	linkComment(text string) (body any, degraded bool)
}

// dcEncoder shapes payloads for Jira Data Center (REST API v2): descriptions and
// comments are raw wiki-markup strings, users are {"name": ...}, and epic
// relationships use the configured epic-link/epic-name custom fields.
type dcEncoder struct{}

func (dcEncoder) description(text string) (any, bool) { return text, false }

func (dcEncoder) userRef(resolved string) map[string]any { return jira.FormatUser(resolved) }

func (dcEncoder) epicParent(fields map[string]any, epicKey, epicLinkFieldID string) {
	fields[epicLinkFieldID] = epicKey
}

func (dcEncoder) epicName(fields map[string]any, name, epicNameFieldID string) bool {
	fields[epicNameFieldID] = name
	return false
}

func (dcEncoder) epicLink(fields map[string]any, key, epicLinkFieldID string) {
	fields[epicLinkFieldID] = key
}

func (dcEncoder) linkComment(text string) (any, bool) { return text, false }

// cloudEncoder shapes payloads for Jira Cloud (REST API v3): descriptions and
// comments are ADF documents, users are {"id": accountId}, and all epic
// relationships collapse onto the unified parent field (epic-name is dropped).
type cloudEncoder struct{}

func (cloudEncoder) description(text string) (any, bool) {
	// Conversion never fails; degraded markup is reported for verbose output.
	return adf.WikiToADF(text)
}

func (cloudEncoder) userRef(resolved string) map[string]any {
	return map[string]any{"id": resolved}
}

func (cloudEncoder) epicParent(fields map[string]any, epicKey, _ string) {
	fields["parent"] = map[string]any{"key": epicKey}
}

func (cloudEncoder) epicName(_ map[string]any, _, _ string) bool {
	// Cloud epics use the summary as their name; the epicName is dropped.
	return true
}

func (cloudEncoder) epicLink(fields map[string]any, key, _ string) {
	fields["parent"] = map[string]any{"key": key}
}

func (cloudEncoder) linkComment(text string) (any, bool) {
	return adf.WikiToADF(text)
}

// newFieldEncoder selects the encoder for a resolved platform mode.
func newFieldEncoder(mode jira.Mode) fieldEncoder {
	if mode == jira.ModeCloud {
		return cloudEncoder{}
	}
	return dcEncoder{}
}

// encoder returns the applier's selected field encoder. NewApplier stores it, so
// production always hits the stored value; the fallback derives it from the mode
// for zero-value Applier structs assembled directly in tests.
func (a *Applier) encoder() fieldEncoder {
	if a.enc == nil {
		return newFieldEncoder(a.mode)
	}
	return a.enc
}
