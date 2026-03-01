// SPDX-License-Identifier: Apache-2.0
//
// Package state provides persistent state tracking for idempotent issue creation.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const stateFileName = ".jira-ai-creator-state.json"

// State tracks the mapping between internal IDs and Jira issue keys.
type State struct {
	// IssueMapping maps internal ID to issue record
	IssueMapping map[string]IssueRecord `json:"issueMapping"`
	// UpdatedAt is the last modification timestamp
	UpdatedAt time.Time `json:"updatedAt"`
	// ProjectKey is the Jira project this state is for
	ProjectKey string `json:"projectKey"`

	// savePath is the resolved file path used for Save/Clear (not serialized)
	savePath string `json:"-"`
}

// IssueRecord represents a created issue.
type IssueRecord struct {
	JiraKey    string    `json:"jiraKey"`
	InternalID string    `json:"internalId"`
	IssueType  string    `json:"issueType"`
	Summary    string    `json:"summary"`
	EpicLink   string    `json:"epicLink,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	ConfigFile string    `json:"configFile"`
}

// Load loads state from disk. The state file is always placed in the current
// working directory, so running from the project root always uses the same
// state regardless of where the config file lives. Returns a new empty state
// if the file doesn't exist.
func Load(projectKey, configFile string) (*State, error) {
	statePath := statePathFor()

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return &State{
			IssueMapping: make(map[string]IssueRecord),
			UpdatedAt:    time.Now(),
			ProjectKey:   projectKey,
			savePath:     statePath,
		}, nil
	}

	data, err := os.ReadFile(statePath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state file: %w", err)
	}

	state.savePath = statePath

	// Verify project key matches
	if state.ProjectKey != projectKey {
		return nil, fmt.Errorf(
			"state file is for project %q but config uses %q; "+
				"clear state with 'jira-ai-creator state clear' or use a different directory",
			state.ProjectKey, projectKey,
		)
	}

	return &state, nil
}

// Save persists state to disk.
func (s *State) Save() error {
	s.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(s.savePath, data, 0600); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// GetJiraKey returns the Jira key for an internal ID, if it exists.
func (s *State) GetJiraKey(internalID string) (string, bool) {
	record, ok := s.IssueMapping[internalID]
	if !ok {
		return "", false
	}
	return record.JiraKey, true
}

// GetRecord returns the full IssueRecord for an internal ID, if it exists.
func (s *State) GetRecord(internalID string) (IssueRecord, bool) {
	record, ok := s.IssueMapping[internalID]
	return record, ok
}

// AddIssue records a newly created issue.
func (s *State) AddIssue(internalID, jiraKey, issueType, summary, epicLink, configFile string) {
	s.IssueMapping[internalID] = IssueRecord{
		JiraKey:    jiraKey,
		InternalID: internalID,
		IssueType:  issueType,
		Summary:    summary,
		EpicLink:   epicLink,
		CreatedAt:  time.Now(),
		ConfigFile: configFile,
	}
}

// HasIssue checks if an internal ID exists in state.
func (s *State) HasIssue(internalID string) bool {
	_, ok := s.IssueMapping[internalID]
	return ok
}

// ListIssues returns all issues sorted by creation time.
func (s *State) ListIssues() []IssueRecord {
	issues := make([]IssueRecord, 0, len(s.IssueMapping))
	for _, record := range s.IssueMapping {
		issues = append(issues, record)
	}

	sort.Slice(issues, func(i, j int) bool {
		return issues[i].CreatedAt.Before(issues[j].CreatedAt)
	})

	return issues
}

// Count returns the number of issues in state.
func (s *State) Count() int {
	return len(s.IssueMapping)
}

// Clear removes the state file.
func (s *State) Clear() error {
	if _, err := os.Stat(s.savePath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(s.savePath)
}

// ClearForConfig removes the state file in the current working directory.
// The configFile argument is ignored; it is kept for API compatibility.
func ClearForConfig(configFile string) error {
	statePath := statePathFor()
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(statePath)
}

// Path returns the resolved state file path.
func (s *State) Path() string {
	return s.savePath
}

// statePathFor returns the state file path in the current working directory.
// Keeping state in cwd (typically the project root) ensures a single unified
// state file regardless of where config YAML files are located within the repo.
func statePathFor() string {
	cwd, err := os.Getwd()
	if err != nil {
		return stateFileName
	}
	return filepath.Join(cwd, stateFileName)
}

// GetStatePath returns the state file path for display purposes.
//
// Deprecated: use State.Path() on a loaded state instead.
func GetStatePath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return stateFileName
	}
	return filepath.Join(cwd, stateFileName)
}
