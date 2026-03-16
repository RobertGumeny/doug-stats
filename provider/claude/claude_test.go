package claude

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/robertgumeny/doug-stats/provider"
)

// testdataDir returns the absolute path to this package's testdata directory.
func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

// --- extractText ---

func TestExtractText_String(t *testing.T) {
	raw := json.RawMessage(`"hello world"`)
	got := extractText(raw)
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestExtractText_ArrayOfParts(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"foo"},{"type":"tool_result","text":"ignored"},{"type":"text","text":"bar"}]`)
	got := extractText(raw)
	if got != "foobar" {
		t.Errorf("got %q, want %q", got, "foobar")
	}
}

func TestExtractText_Empty(t *testing.T) {
	if got := extractText(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// --- taskIDPattern ---

func TestTaskIDPattern_Match(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"[DOUG_TASK_ID: EPIC-2-001] Please activate", "EPIC-2-001"},
		{"[DOUG_TASK_ID:EPIC-1-003] no space", "EPIC-1-003"},
		{"prefix [DOUG_TASK_ID: EPIC-3-002] suffix", "EPIC-3-002"},
	}
	for _, c := range cases {
		m := taskIDPattern.FindStringSubmatch(c.input)
		if m == nil {
			t.Errorf("input %q: expected match, got none", c.input)
			continue
		}
		if got := strings.TrimSpace(m[1]); got != c.want {
			t.Errorf("input %q: got %q, want %q", c.input, got, c.want)
		}
	}
}

func TestTaskIDPattern_NoMatch(t *testing.T) {
	inputs := []string{
		"Hello Claude, how are you?",
		"[TASK_ID: EPIC-2-001]",
		"",
	}
	for _, input := range inputs {
		if m := taskIDPattern.FindStringSubmatch(input); m != nil {
			t.Errorf("input %q: expected no match, got %v", input, m)
		}
	}
}

// --- encodeProjectPath ---

func TestEncodeProjectPath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/home/user/myproject", "-home-user-myproject"},
		{"/test/project", "-test-project"},
		{"/", "-"},
	}
	for _, c := range cases {
		got := encodeProjectPath(c.input)
		if got != c.want {
			t.Errorf("encodeProjectPath(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// --- session classification via scanSessionPhase1 ---

func TestSessionClassification_Doug(t *testing.T) {
	p := New(testdataDir())
	filePath := filepath.Join(testdataDir(), "projects", "-test-project", "session-doug.jsonl")
	meta, err := p.scanSessionPhase1("session-doug", "/test/project", time.Time{}, filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Class != provider.ClassDoug {
		t.Errorf("got class %v, want ClassDoug", meta.Class)
	}
	if meta.TaskID != "EPIC-2-001" {
		t.Errorf("got taskID %q, want %q", meta.TaskID, "EPIC-2-001")
	}
}

func TestSessionClassification_Manual(t *testing.T) {
	p := New(testdataDir())
	filePath := filepath.Join(testdataDir(), "projects", "-test-project", "session-manual.jsonl")
	meta, err := p.scanSessionPhase1("session-manual", "/test/project", time.Time{}, filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Class != provider.ClassManual {
		t.Errorf("got class %v, want ClassManual", meta.Class)
	}
	if meta.TaskID != "" {
		t.Errorf("got taskID %q, want empty", meta.TaskID)
	}
}

func TestSessionClassification_Untagged(t *testing.T) {
	p := New(testdataDir())
	filePath := filepath.Join(testdataDir(), "projects", "-test-project", "session-untagged.jsonl")
	meta, err := p.scanSessionPhase1("session-untagged", "/test/project", time.Time{}, filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Class != provider.ClassUntagged {
		t.Errorf("got class %v, want ClassUntagged", meta.Class)
	}
}

// --- token field parsing ---

func TestTokenParsing_AllFourTypes(t *testing.T) {
	p := New(testdataDir())
	filePath := filepath.Join(testdataDir(), "projects", "-test-project", "session-doug.jsonl")
	meta, err := p.scanSessionPhase1("session-doug", "/test/project", time.Time{}, filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Tokens.Input != 10 {
		t.Errorf("Input: got %d, want 10", meta.Tokens.Input)
	}
	if meta.Tokens.CacheCreation != 20 {
		t.Errorf("CacheCreation: got %d, want 20", meta.Tokens.CacheCreation)
	}
	if meta.Tokens.CacheRead != 30 {
		t.Errorf("CacheRead: got %d, want 30", meta.Tokens.CacheRead)
	}
	if meta.Tokens.Output != 40 {
		t.Errorf("Output: got %d, want 40", meta.Tokens.Output)
	}
}

func TestTokenParsing_StreamingDeduplication(t *testing.T) {
	// session-stream has two assistant records with the same message ID:
	// - intermediate: stop_reason=null, output=1  (should be ignored)
	// - final:        stop_reason="end_turn", input=100, cache_creation=200, cache_read=300, output=50
	// Total should reflect only the final record.
	p := New(testdataDir())
	filePath := filepath.Join(testdataDir(), "projects", "-test-project", "session-stream.jsonl")
	meta, err := p.scanSessionPhase1("session-stream", "/test/project", time.Time{}, filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Tokens.Input != 100 {
		t.Errorf("Input: got %d, want 100", meta.Tokens.Input)
	}
	if meta.Tokens.CacheCreation != 200 {
		t.Errorf("CacheCreation: got %d, want 200", meta.Tokens.CacheCreation)
	}
	if meta.Tokens.CacheRead != 300 {
		t.Errorf("CacheRead: got %d, want 300", meta.Tokens.CacheRead)
	}
	if meta.Tokens.Output != 50 {
		t.Errorf("Output: got %d, want 50", meta.Tokens.Output)
	}
}

func TestMalformedLinesSkipped(t *testing.T) {
	// session-stream contains a line with an unknown type — must not crash.
	p := New(testdataDir())
	filePath := filepath.Join(testdataDir(), "projects", "-test-project", "session-stream.jsonl")
	meta, err := p.scanSessionPhase1("session-stream", "/test/project", time.Time{}, filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
}

// --- LoadSessions integration ---

func TestLoadSessions_DiscoversByHistoryJSONL(t *testing.T) {
	p := New(testdataDir())
	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	// history.jsonl references 4 unique sessions; session-doug appears twice
	// but must only be counted once.
	if len(metas) != 4 {
		t.Errorf("got %d sessions, want 4", len(metas))
	}
}

func TestLoadSessions_ProjectPath(t *testing.T) {
	p := New(testdataDir())
	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	for _, m := range metas {
		if m.ProjectPath != "/test/project" {
			t.Errorf("session %s: got project %q, want /test/project", m.ID, m.ProjectPath)
		}
		if m.Provider != providerName {
			t.Errorf("session %s: got provider %q, want %q", m.ID, m.Provider, providerName)
		}
	}
}

// --- LoadTranscript ---

func TestLoadTranscript_ReturnsMessages(t *testing.T) {
	p := New(testdataDir())
	if _, err := p.LoadSessions(); err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	transcript, err := p.LoadTranscript("session-doug")
	if err != nil {
		t.Fatalf("LoadTranscript failed: %v", err)
	}
	if len(transcript.Messages) != 2 {
		t.Errorf("got %d messages, want 2", len(transcript.Messages))
	}
	if transcript.Messages[0].Role != "user" {
		t.Errorf("first message role: got %q, want user", transcript.Messages[0].Role)
	}
	if transcript.Messages[1].Role != "assistant" {
		t.Errorf("second message role: got %q, want assistant", transcript.Messages[1].Role)
	}
	if transcript.Messages[1].Model != "claude-sonnet-4-6" {
		t.Errorf("assistant model: got %q, want claude-sonnet-4-6", transcript.Messages[1].Model)
	}
}

func TestLoadTranscript_TokensOnAssistantMessage(t *testing.T) {
	p := New(testdataDir())
	if _, err := p.LoadSessions(); err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	transcript, err := p.LoadTranscript("session-doug")
	if err != nil {
		t.Fatalf("LoadTranscript failed: %v", err)
	}
	asst := transcript.Messages[1]
	if asst.Tokens == nil {
		t.Fatal("expected non-nil Tokens on assistant message")
	}
	if asst.Tokens.Input != 10 || asst.Tokens.CacheCreation != 20 ||
		asst.Tokens.CacheRead != 30 || asst.Tokens.Output != 40 {
		t.Errorf("unexpected token counts: %+v", asst.Tokens)
	}
}

func TestLoadTranscript_NotInIndex(t *testing.T) {
	p := New(testdataDir())
	// Do NOT call LoadSessions — transcript must fail gracefully.
	_, err := p.LoadTranscript("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
}
