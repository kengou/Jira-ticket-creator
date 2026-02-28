package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withTempDir runs fn inside a temporary directory (sets CWD and restores it).
func withTempDir(t *testing.T, fn func(dir string)) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	fn(dir)
}

// --- Load ---

func TestLoad_NewStateWhenFileDoesNotExist(t *testing.T) {
	withTempDir(t, func(_ string) {
		st, err := Load("PROJ", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if st.ProjectKey != "PROJ" {
			t.Errorf("ProjectKey = %q, want %q", st.ProjectKey, "PROJ")
		}
		if st.IssueMapping == nil {
			t.Fatal("IssueMapping should be initialized")
		}
		if len(st.IssueMapping) != 0 {
			t.Errorf("IssueMapping should be empty, got %d entries", len(st.IssueMapping))
		}
	})
}

func TestLoad_ReadsExistingState(t *testing.T) {
	withTempDir(t, func(dir string) {
		existing := &State{
			IssueMapping: map[string]IssueRecord{
				"task-1": {
					JiraKey:    "PROJ-1",
					InternalID: "task-1",
					IssueType:  "Task",
					Summary:    "A task",
					CreatedAt:  time.Now().Add(-time.Hour),
					ConfigFile: "config.yaml",
				},
			},
			UpdatedAt:  time.Now().Add(-time.Hour),
			ProjectKey: "PROJ",
		}
		writeStateFile(t, dir, existing)

		st, err := Load("PROJ", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if st.Count() != 1 {
			t.Fatalf("Count() = %d, want 1", st.Count())
		}
		key, ok := st.GetJiraKey("task-1")
		if !ok || key != "PROJ-1" {
			t.Errorf("GetJiraKey(task-1) = (%q, %v), want (PROJ-1, true)", key, ok)
		}
	})
}

func TestLoad_MismatchedProjectKey(t *testing.T) {
	withTempDir(t, func(dir string) {
		existing := &State{
			IssueMapping: make(map[string]IssueRecord),
			UpdatedAt:    time.Now(),
			ProjectKey:   "OTHER",
		}
		writeStateFile(t, dir, existing)

		_, err := Load("PROJ", "")
		if err == nil {
			t.Fatal("expected error for mismatched project key")
		}
	})
}

func TestLoad_CorruptedFile(t *testing.T) {
	withTempDir(t, func(dir string) {
		path := filepath.Join(dir, stateFileName)
		if err := os.WriteFile(path, []byte("{{{bad json"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := Load("PROJ", "")
		if err == nil {
			t.Fatal("expected error for corrupted state file")
		}
	})
}

func TestLoad_WithConfigFile_PlacesStateNextToYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "my-config.yaml")
	os.WriteFile(configPath, []byte(""), 0644)

	st, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Path() != filepath.Join(dir, stateFileName) {
		t.Errorf("Path() = %q, want state file next to config in %q", st.Path(), dir)
	}
}

func TestLoad_WithConfigFile_ReadsExistingState(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "my-config.yaml")
	os.WriteFile(configPath, []byte(""), 0644)

	// Write a state file next to the config
	existing := &State{
		IssueMapping: map[string]IssueRecord{
			"task-1": {
				JiraKey:    "PROJ-1",
				InternalID: "task-1",
				IssueType:  "Task",
				Summary:    "A task",
				CreatedAt:  time.Now().Add(-time.Hour),
				ConfigFile: "my-config.yaml",
			},
		},
		UpdatedAt:  time.Now().Add(-time.Hour),
		ProjectKey: "PROJ",
	}
	writeStateFile(t, dir, existing)

	st, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", st.Count())
	}
	key, ok := st.GetJiraKey("task-1")
	if !ok || key != "PROJ-1" {
		t.Errorf("GetJiraKey(task-1) = (%q, %v), want (PROJ-1, true)", key, ok)
	}
}

// --- Save ---

func TestSave_WritesFile(t *testing.T) {
	withTempDir(t, func(dir string) {
		st, err := Load("PROJ", "")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		st.AddIssue("task-1", "PROJ-42", "Task", "My task", "", "config.yaml")

		if err := st.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}

		// Verify file exists and is valid JSON
		data, err := os.ReadFile(filepath.Join(dir, stateFileName))
		if err != nil {
			t.Fatalf("read state file: %v", err)
		}
		var loaded State
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatalf("unmarshal saved state: %v", err)
		}
		if loaded.ProjectKey != "PROJ" {
			t.Errorf("saved ProjectKey = %q, want PROJ", loaded.ProjectKey)
		}
		if loaded.Count() != 1 {
			t.Errorf("saved Count = %d, want 1", loaded.Count())
		}
	})
}

func TestSave_UpdatesTimestamp(t *testing.T) {
	withTempDir(t, func(_ string) {
		st, err := Load("PROJ", "")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		st.UpdatedAt = time.Now().Add(-time.Hour)
		before := time.Now()
		if err := st.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if st.UpdatedAt.Before(before) {
			t.Error("Save should update UpdatedAt timestamp")
		}
	})
}

func TestSave_NextToConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "my-config.yaml")

	st, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	st.AddIssue("task-1", "PROJ-1", "Task", "Task", "", "my-config.yaml")

	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify state file is next to the config
	statePath := filepath.Join(dir, stateFileName)
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file should exist at %s: %v", statePath, err)
	}
}

// --- AddIssue / GetJiraKey / GetRecord / HasIssue ---

func TestAddIssue_And_GetJiraKey(t *testing.T) {
	st := &State{IssueMapping: make(map[string]IssueRecord)}
	st.AddIssue("epic-1", "PROJ-100", "Epic", "My Epic", "", "config.yaml")

	key, ok := st.GetJiraKey("epic-1")
	if !ok || key != "PROJ-100" {
		t.Errorf("GetJiraKey(epic-1) = (%q, %v), want (PROJ-100, true)", key, ok)
	}
}

func TestGetJiraKey_NotFound(t *testing.T) {
	st := &State{IssueMapping: make(map[string]IssueRecord)}
	_, ok := st.GetJiraKey("nonexistent")
	if ok {
		t.Error("expected false for nonexistent key")
	}
}

func TestGetRecord_Found(t *testing.T) {
	st := &State{IssueMapping: make(map[string]IssueRecord)}
	st.AddIssue("task-1", "PROJ-42", "Task", "My Task", "", "config.yaml")

	record, ok := st.GetRecord("task-1")
	if !ok {
		t.Fatal("expected GetRecord to return true")
	}
	if record.JiraKey != "PROJ-42" {
		t.Errorf("JiraKey = %q, want PROJ-42", record.JiraKey)
	}
	if record.Summary != "My Task" {
		t.Errorf("Summary = %q, want 'My Task'", record.Summary)
	}
}

func TestGetRecord_NotFound(t *testing.T) {
	st := &State{IssueMapping: make(map[string]IssueRecord)}
	_, ok := st.GetRecord("nonexistent")
	if ok {
		t.Error("expected false for nonexistent record")
	}
}

func TestHasIssue(t *testing.T) {
	st := &State{IssueMapping: make(map[string]IssueRecord)}
	st.AddIssue("task-1", "PROJ-1", "Task", "Task", "", "config.yaml")

	if !st.HasIssue("task-1") {
		t.Error("HasIssue should return true for existing issue")
	}
	if st.HasIssue("task-2") {
		t.Error("HasIssue should return false for missing issue")
	}
}

// --- ListIssues ---

func TestListIssues_SortedByCreationTime(t *testing.T) {
	st := &State{IssueMapping: make(map[string]IssueRecord)}

	now := time.Now()
	st.IssueMapping["b"] = IssueRecord{JiraKey: "PROJ-2", InternalID: "b", CreatedAt: now.Add(time.Minute)}
	st.IssueMapping["a"] = IssueRecord{JiraKey: "PROJ-1", InternalID: "a", CreatedAt: now}
	st.IssueMapping["c"] = IssueRecord{JiraKey: "PROJ-3", InternalID: "c", CreatedAt: now.Add(2 * time.Minute)}

	issues := st.ListIssues()
	if len(issues) != 3 {
		t.Fatalf("len = %d, want 3", len(issues))
	}
	if issues[0].InternalID != "a" || issues[1].InternalID != "b" || issues[2].InternalID != "c" {
		t.Errorf("issues not sorted by creation time: %v", issues)
	}
}

func TestListIssues_Empty(t *testing.T) {
	st := &State{IssueMapping: make(map[string]IssueRecord)}
	issues := st.ListIssues()
	if len(issues) != 0 {
		t.Errorf("expected empty list, got %d", len(issues))
	}
}

// --- Count ---

func TestCount(t *testing.T) {
	st := &State{IssueMapping: make(map[string]IssueRecord)}
	if st.Count() != 0 {
		t.Errorf("Count = %d, want 0", st.Count())
	}

	st.AddIssue("a", "K-1", "Task", "A", "", "c.yaml")
	st.AddIssue("b", "K-2", "Bug", "B", "", "c.yaml")
	if st.Count() != 2 {
		t.Errorf("Count = %d, want 2", st.Count())
	}
}

// --- Clear ---

func TestClear_RemovesFile(t *testing.T) {
	withTempDir(t, func(dir string) {
		st, err := Load("PROJ", "")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if err := st.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(filepath.Join(dir, stateFileName)); err != nil {
			t.Fatalf("state file should exist: %v", err)
		}

		if err := st.Clear(); err != nil {
			t.Fatalf("Clear: %v", err)
		}

		// Verify file is removed
		if _, err := os.Stat(filepath.Join(dir, stateFileName)); !os.IsNotExist(err) {
			t.Error("state file should be removed after Clear")
		}
	})
}

func TestClear_NoErrorWhenFileDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	st := &State{
		IssueMapping: make(map[string]IssueRecord),
		savePath:     filepath.Join(dir, stateFileName),
	}
	if err := st.Clear(); err != nil {
		t.Fatalf("Clear should not error when file doesn't exist: %v", err)
	}
}

func TestClearForConfig_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	st, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := ClearForConfig(configPath); err != nil {
		t.Fatalf("ClearForConfig: %v", err)
	}

	statePath := filepath.Join(dir, stateFileName)
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("state file should be removed after ClearForConfig")
	}
}

// --- Path ---

func TestPath_ReturnsResolvedPath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	st, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st.Path() != filepath.Join(dir, stateFileName) {
		t.Errorf("Path() = %q, want %q", st.Path(), filepath.Join(dir, stateFileName))
	}
}

// --- GetStatePath (deprecated compat) ---

func TestGetStatePath_ContainsFileName(t *testing.T) {
	path := GetStatePath()
	if filepath.Base(path) != stateFileName {
		t.Errorf("GetStatePath base = %q, want %q", filepath.Base(path), stateFileName)
	}
}

// --- Round-trip: Load -> modify -> Save -> Load ---

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	// Load fresh
	st, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Add issues
	st.AddIssue("epic-1", "PROJ-1", "Epic", "Epic 1", "", "config.yaml")
	st.AddIssue("story-1", "PROJ-2", "Story", "Story 1", "PROJ-1", "config.yaml")

	// Save
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload
	st2, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	if st2.Count() != 2 {
		t.Errorf("Count after reload = %d, want 2", st2.Count())
	}

	key, ok := st2.GetJiraKey("epic-1")
	if !ok || key != "PROJ-1" {
		t.Errorf("GetJiraKey(epic-1) = (%q, %v), want (PROJ-1, true)", key, ok)
	}

	// Verify epic link is persisted
	storyRecord := st2.IssueMapping["story-1"]
	if storyRecord.EpicLink != "PROJ-1" {
		t.Errorf("story-1 EpicLink = %q, want PROJ-1", storyRecord.EpicLink)
	}
	epicRecord := st2.IssueMapping["epic-1"]
	if epicRecord.EpicLink != "" {
		t.Errorf("epic-1 EpicLink = %q, want empty", epicRecord.EpicLink)
	}
}

func TestRoundTrip_Idempotency(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	// First run: create and save
	st, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	st.AddIssue("epic-1", "PROJ-1", "Epic", "My Epic", "", "config.yaml")
	st.AddIssue("story-1", "PROJ-2", "Story", "My Story", "", "config.yaml")
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Second run: load and check
	st2, err := Load("PROJ", configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Simulate what the applier does on re-run
	record, ok := st2.GetRecord("epic-1")
	if !ok {
		t.Fatal("epic-1 should exist in state")
	}
	if record.JiraKey != "PROJ-1" {
		t.Errorf("JiraKey = %q, want PROJ-1", record.JiraKey)
	}
	if record.Summary != "My Epic" {
		t.Errorf("Summary = %q, want 'My Epic'", record.Summary)
	}

	record2, ok := st2.GetRecord("story-1")
	if !ok {
		t.Fatal("story-1 should exist in state")
	}
	if record2.JiraKey != "PROJ-2" {
		t.Errorf("JiraKey = %q, want PROJ-2", record2.JiraKey)
	}
}

// --- helpers ---

func writeStateFile(t *testing.T, dir string, st *State) {
	t.Helper()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	path := filepath.Join(dir, stateFileName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write state file: %v", err)
	}
}
