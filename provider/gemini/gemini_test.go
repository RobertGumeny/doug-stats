package gemini

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/robertgumeny/doug-stats/provider"
)

func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

func TestLoadSessions_UsesLogsJSONAsPrimaryIndex(t *testing.T) {
	p := New(testdataDir())
	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	if len(metas) != 3 {
		t.Fatalf("got %d sessions, want 3", len(metas))
	}
	for _, m := range metas {
		if m.ID == "zzzzzzzz-1111-2222-3333-444444444444" {
			t.Fatal("orphan chat discovered without logs.json index")
		}
	}
}

func TestLoadSessions_ProjectRootResolution(t *testing.T) {
	p := New(testdataDir())
	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	for _, m := range metas {
		switch m.ID {
		case "aaaaaaaa-1111-2222-3333-444444444444", "bbbbbbbb-1111-2222-3333-444444444444":
			if m.ProjectPath != "/home/test/project-alpha" {
				t.Fatalf("alpha project path = %q, want /home/test/project-alpha", m.ProjectPath)
			}
		case "cccccccc-1111-2222-3333-444444444444":
			if m.ProjectPath != "/home/test/project-beta" {
				t.Fatalf("beta project path = %q, want /home/test/project-beta", m.ProjectPath)
			}
		}
	}
}

func TestSessionClassificationAndTaskID(t *testing.T) {
	p := New(testdataDir())
	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	byID := map[string]*provider.SessionMeta{}
	for _, m := range metas {
		byID[m.ID] = m
	}

	if got := byID["aaaaaaaa-1111-2222-3333-444444444444"]; got.Class != provider.ClassDoug || got.TaskID != "EPIC-3-001" {
		t.Fatalf("doug session mismatch: class=%v task=%q", got.Class, got.TaskID)
	}
	if got := byID["bbbbbbbb-1111-2222-3333-444444444444"]; got.Class != provider.ClassManual || got.TaskID != "" {
		t.Fatalf("manual session mismatch: class=%v task=%q", got.Class, got.TaskID)
	}
	if got := byID["cccccccc-1111-2222-3333-444444444444"]; got.Class != provider.ClassUntagged || got.TaskID != "" {
		t.Fatalf("untagged session mismatch: class=%v task=%q", got.Class, got.TaskID)
	}
}

func TestTokenParsing_AllGeminiFields(t *testing.T) {
	p := New(testdataDir())
	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	var got *provider.SessionMeta
	for _, m := range metas {
		if m.ID == "aaaaaaaa-1111-2222-3333-444444444444" {
			got = m
			break
		}
	}
	if got == nil {
		t.Fatal("target session not found")
	}

	if got.Tokens.Input != 100 || got.Tokens.Output != 20 || got.Tokens.CacheRead != 30 || got.Tokens.Thoughts != 7 || got.Tokens.Tool != 5 {
		t.Fatalf("unexpected tokens: %+v", got.Tokens)
	}
	if got.Model != "gemini-3-flash-preview" {
		t.Fatalf("model = %q, want gemini-3-flash-preview", got.Model)
	}
}

func TestLoadTranscript_ParsesAssistantTurnsAndToolParts(t *testing.T) {
	p := New(testdataDir())
	if _, err := p.LoadSessions(); err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	tr, err := p.LoadTranscript("aaaaaaaa-1111-2222-3333-444444444444")
	if err != nil {
		t.Fatalf("LoadTranscript failed: %v", err)
	}
	if len(tr.Messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(tr.Messages))
	}
	asst := tr.Messages[1]
	if asst.Role != "assistant" {
		t.Fatalf("message role = %q, want assistant", asst.Role)
	}
	if asst.Tokens == nil || asst.Tokens.Thoughts != 7 || asst.Tokens.Tool != 5 {
		t.Fatalf("unexpected assistant tokens: %+v", asst.Tokens)
	}

	hasToolUse := false
	hasToolResult := false
	for _, cp := range asst.Content {
		if cp.Type == "tool_use" {
			hasToolUse = true
		}
		if cp.Type == "tool_result" {
			hasToolResult = true
		}
	}
	if !hasToolUse || !hasToolResult {
		t.Fatalf("expected tool_use and tool_result parts, got: %+v", asst.Content)
	}
}

func TestLoadTranscript_NotInIndex(t *testing.T) {
	p := New(testdataDir())
	if _, err := p.LoadTranscript("missing"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

// --- canonical identity fields ---

func TestLoadSessions_CanonicalIdentityFields(t *testing.T) {
	p := New(testdataDir())
	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	for _, m := range metas {
		if m.RawProjectPath == "" {
			t.Errorf("session %s: RawProjectPath is empty", m.ID)
		}
		if m.CanonicalProjectID == "" {
			t.Errorf("session %s: CanonicalProjectID is empty", m.ID)
		}
		if m.CanonicalProjectSource == "" {
			t.Errorf("session %s: CanonicalProjectSource is empty", m.ID)
		}
		// Raw path must not be overwritten.
		if m.RawProjectPath != m.ProjectPath {
			t.Errorf("session %s: RawProjectPath %q != ProjectPath %q", m.ID, m.RawProjectPath, m.ProjectPath)
		}
	}
}

func TestLoadSessions_CrossProviderSameRepo(t *testing.T) {
	// Two Gemini sessions pointing at the same absolute project path must
	// produce identical CanonicalProjectIDs. This mirrors the cross-provider
	// guarantee: any provider using resolver.Resolve with the same RawPath
	// produces the same canonical ID.
	p := New(testdataDir())
	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	byID := map[string]*provider.SessionMeta{}
	for _, m := range metas {
		byID[m.ID] = m
	}

	alpha1 := byID["aaaaaaaa-1111-2222-3333-444444444444"]
	alpha2 := byID["bbbbbbbb-1111-2222-3333-444444444444"]
	if alpha1 == nil || alpha2 == nil {
		t.Fatal("expected both alpha sessions to be present")
	}
	// Both sessions have projectPath=/home/test/project-alpha; they must
	// resolve to the same canonical ID regardless of any other metadata.
	if alpha1.CanonicalProjectID != alpha2.CanonicalProjectID {
		t.Errorf("same-repo sessions differ: %q vs %q",
			alpha1.CanonicalProjectID, alpha2.CanonicalProjectID)
	}
}
