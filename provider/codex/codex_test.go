package codex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertgumeny/doug-stats/provider"
)

func TestLoadSessions_UsesSQLiteThreadsOnly(t *testing.T) {
	root := setupCodexFixture(t)
	p := New(root)

	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}

	if len(metas) != 3 {
		t.Fatalf("got %d sessions, want 3", len(metas))
	}

	for _, m := range metas {
		if m.ID == "orphan-not-indexed" {
			t.Fatal("rollout discovered without sqlite thread row")
		}
	}
}

func TestLoadSessions_RolloutPathFromSQLite(t *testing.T) {
	root := setupCodexFixture(t)
	p := New(root)

	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}

	byID := map[string]*provider.SessionMeta{}
	for _, m := range metas {
		byID[m.ID] = m
	}

	m := byID["thread-manual"]
	if m == nil {
		t.Fatal("thread-manual missing")
	}
	if m.Tokens.Input != 11 || m.Tokens.Output != 7 {
		t.Fatalf("unexpected tokens for thread-manual: %+v", m.Tokens)
	}
}

func TestLoadSessions_ProjectAndClassificationAndTokens(t *testing.T) {
	root := setupCodexFixture(t)
	p := New(root)

	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}

	byID := map[string]*provider.SessionMeta{}
	for _, m := range metas {
		byID[m.ID] = m
	}

	doug := byID["thread-doug"]
	if doug == nil {
		t.Fatal("thread-doug missing")
	}
	if doug.ProjectPath != "project-alpha" {
		t.Fatalf("project path = %q, want project-alpha", doug.ProjectPath)
	}
	if doug.Class != provider.ClassDoug || doug.TaskID != "EPIC-3-002" {
		t.Fatalf("doug classification mismatch: class=%v task=%q", doug.Class, doug.TaskID)
	}
	if doug.Model != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", doug.Model)
	}
	// Uses last_token_usage only (10+20 input, 5 cache, 2+4 output, 1 thoughts).
	if doug.Tokens.Input != 30 || doug.Tokens.CacheRead != 5 || doug.Tokens.Output != 6 || doug.Tokens.Thoughts != 1 {
		t.Fatalf("unexpected doug tokens: %+v", doug.Tokens)
	}

	manual := byID["thread-manual"]
	if manual == nil {
		t.Fatal("thread-manual missing")
	}
	if manual.ProjectPath != "/home/test/manual-project" {
		t.Fatalf("manual project path = %q, want /home/test/manual-project", manual.ProjectPath)
	}
	if manual.Class != provider.ClassManual || manual.TaskID != "" {
		t.Fatalf("manual classification mismatch: class=%v task=%q", manual.Class, manual.TaskID)
	}

	untagged := byID["thread-untagged"]
	if untagged == nil {
		t.Fatal("thread-untagged missing")
	}
	if untagged.Class != provider.ClassUntagged {
		t.Fatalf("untagged class = %v, want ClassUntagged", untagged.Class)
	}
}

func TestLoadTranscript_TurnContextAndTokenCorrelation(t *testing.T) {
	root := setupCodexFixture(t)
	p := New(root)
	if _, err := p.LoadSessions(); err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}

	tr, err := p.LoadTranscript("thread-doug")
	if err != nil {
		t.Fatalf("LoadTranscript failed: %v", err)
	}
	if len(tr.Messages) != 3 {
		t.Fatalf("got %d transcript messages, want 3", len(tr.Messages))
	}

	if tr.Messages[0].Role != "user" {
		t.Fatalf("first role = %q, want user", tr.Messages[0].Role)
	}
	firstAsst := tr.Messages[1]
	secondAsst := tr.Messages[2]
	if firstAsst.Role != "assistant" || secondAsst.Role != "assistant" {
		t.Fatalf("assistant roles mismatch: %q %q", firstAsst.Role, secondAsst.Role)
	}
	if firstAsst.Model != "gpt-5-codex" || secondAsst.Model != "gpt-5-codex" {
		t.Fatalf("assistant models = %q/%q, want gpt-5-codex", firstAsst.Model, secondAsst.Model)
	}
	if firstAsst.Tokens == nil || secondAsst.Tokens == nil {
		t.Fatalf("expected assistant tokens, got %v and %v", firstAsst.Tokens, secondAsst.Tokens)
	}
	if firstAsst.Tokens.Input != 10 || firstAsst.Tokens.Output != 2 || firstAsst.Tokens.Thoughts != 1 {
		t.Fatalf("first assistant tokens mismatch: %+v", *firstAsst.Tokens)
	}
	if secondAsst.Tokens.Input != 20 || secondAsst.Tokens.Output != 4 || secondAsst.Tokens.Thoughts != 0 {
		t.Fatalf("second assistant tokens mismatch: %+v", *secondAsst.Tokens)
	}
}

func TestLoadSessions_MalformedLinesSkipped(t *testing.T) {
	root := setupCodexFixture(t)
	p := New(root)

	metas, err := p.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions failed: %v", err)
	}
	if len(metas) != 3 {
		t.Fatalf("got %d sessions, want 3", len(metas))
	}
}

func TestExtractSessionMetaCWD_DoubleNested(t *testing.T) {
	raw := []byte(`{"payload":{"cwd":"/tmp/project"}}`)
	if got := extractSessionMetaCWD(raw); got != "/tmp/project" {
		t.Fatalf("cwd = %q, want /tmp/project", got)
	}
}

func setupCodexFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	sessionsDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	dougRollout := filepath.Join(sessionsDir, "rollout-doug.jsonl")
	manualRollout := filepath.Join(root, "manual-special-rollout.jsonl")
	untaggedRollout := filepath.Join(sessionsDir, "rollout-untagged.jsonl")
	orphanRollout := filepath.Join(sessionsDir, "orphan.jsonl")

	writeFile(t, dougRollout, stringsJoinLines(
		`{"type":"session_meta","payload":{"payload":{"cwd":"/home/test/project-alpha"}},"timestamp":"2026-03-06T10:00:00Z"}`,
		`{"type":"turn_context","payload":{"model":"gpt-5-codex"},"timestamp":"2026-03-06T10:00:01Z"}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","id":"u1","content":"[DOUG_TASK_ID: EPIC-3-002] do it"},"timestamp":"2026-03-06T10:00:02Z"}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","id":"a1","content":"ok"},"timestamp":"2026-03-06T10:00:03Z"}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10,"cached_input_tokens":5,"output_tokens":2,"reasoning_output_tokens":1},"total_token_usage":{"input_tokens":1000,"cached_input_tokens":500,"output_tokens":200,"reasoning_output_tokens":100}}},"timestamp":"2026-03-06T10:00:04Z"}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","id":"a2","content":"done"},"timestamp":"2026-03-06T10:00:05Z"}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":20,"cached_input_tokens":0,"output_tokens":4,"reasoning_output_tokens":0},"total_token_usage":{"input_tokens":9999,"cached_input_tokens":9999,"output_tokens":9999,"reasoning_output_tokens":9999}}},"timestamp":"2026-03-06T10:00:06Z"}`,
		`{not-json}`,
	))

	writeFile(t, manualRollout, stringsJoinLines(
		`{"type":"session_meta","payload":{"payload":{"cwd":"/home/test/manual-project"}},"timestamp":"2026-03-06T11:00:00Z"}`,
		`{"type":"turn_context","payload":{"model":"codex-mini-latest"},"timestamp":"2026-03-06T11:00:01Z"}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","id":"u2","content":"manual prompt"},"timestamp":"2026-03-06T11:00:02Z"}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","id":"a3","content":"manual response"},"timestamp":"2026-03-06T11:00:03Z"}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":11,"cached_input_tokens":0,"output_tokens":7,"reasoning_output_tokens":0},"total_token_usage":{"input_tokens":11,"cached_input_tokens":0,"output_tokens":7,"reasoning_output_tokens":0}}},"timestamp":"2026-03-06T11:00:04Z"}`,
	))

	writeFile(t, untaggedRollout, stringsJoinLines(
		`{"type":"session_meta","payload":{"payload":{"cwd":"/home/test/untagged"}},"timestamp":"2026-03-06T12:00:00Z"}`,
		`{"type":"turn_context","payload":{"model":"gpt-5.1-codex"},"timestamp":"2026-03-06T12:00:01Z"}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","id":"a4","content":"agent-only"},"timestamp":"2026-03-06T12:00:02Z"}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":3,"cached_input_tokens":1,"output_tokens":2,"reasoning_output_tokens":0},"total_token_usage":{"input_tokens":3,"cached_input_tokens":1,"output_tokens":2,"reasoning_output_tokens":0}}},"timestamp":"2026-03-06T12:00:03Z"}`,
	))

	writeFile(t, orphanRollout, stringsJoinLines(
		`{"type":"response_item","payload":{"type":"message","role":"user","id":"u-x","content":"this should never be discovered"}}`,
	))

	createSQLiteFixture(t, root, manualRollout)

	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func stringsJoinLines(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func createSQLiteFixture(t *testing.T, root, manualRollout string) {
	t.Helper()
	dbPath := filepath.Join(root, "state_5.sqlite")
	script := `
import sqlite3
import sys

db_path = sys.argv[1]
manual_rollout = sys.argv[2]

conn = sqlite3.connect(db_path)
conn.execute("""
CREATE TABLE threads (
	id TEXT PRIMARY KEY,
	rollout_path TEXT NOT NULL,
	git_origin_url TEXT,
	cwd TEXT
)
""")
conn.executemany(
    "INSERT INTO threads (id, rollout_path, git_origin_url, cwd) VALUES (?, ?, ?, ?)",
    [
        ("thread-doug", "sessions/rollout-doug.jsonl", "git@github.com:acme/project-alpha.git", "/ignored/by/git-url"),
        ("thread-manual", manual_rollout, "", "/home/test/manual-project"),
        ("thread-untagged", "sessions/rollout-untagged.jsonl", "", "/home/test/untagged"),
    ],
)
conn.commit()
conn.close()
`
	cmd := exec.Command("python3", "-c", script, dbPath, manualRollout)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("create sqlite fixture failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
}
