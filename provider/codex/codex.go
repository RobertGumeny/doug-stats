// Package codex implements the Provider interface for Codex CLI logs.
//
// Codex stores a session index in state_5.sqlite (threads table), with each
// row pointing directly at a rollout JSONL path. Session discovery is driven
// exclusively by SQLite; rollout directories are never walked.
package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/robertgumeny/doug-stats/provider"
	"github.com/robertgumeny/doug-stats/provider/resolver"
)

const providerName = "codex"

var taskIDPattern = regexp.MustCompile(`\[DOUG_TASK_ID:\s*([^\]]+)\]`)

type Provider struct {
	rootDir  string
	sessions map[string]*sessionIndex
}

type sessionIndex struct {
	projectPath string
	rolloutPath string
}

type threadRow struct {
	ID           string
	RolloutPath  string
	GitOriginURL string
	CWD          string
}

type rolloutLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type parsedTurn struct {
	msg  provider.Message
	text string
}

func New(rootDir string) *Provider {
	return &Provider{rootDir: rootDir, sessions: make(map[string]*sessionIndex)}
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) LoadSessions() ([]*provider.SessionMeta, error) {
	dbPath := filepath.Join(p.rootDir, "state_5.sqlite")
	threads, err := loadThreadRows(dbPath)
	if err != nil {
		return nil, fmt.Errorf("querying threads table: %w", err)
	}

	metas := make([]*provider.SessionMeta, 0, len(threads))
	for _, th := range threads {
		rolloutPath := resolveRolloutPath(p.rootDir, th.RolloutPath)
		projectPath := deriveProjectPath(th.GitOriginURL, th.CWD)

		meta, parsedProjectPath, err := p.scanSessionPhase1(th.ID, projectPath, rolloutPath)
		if err != nil {
			log.Printf("warning: codex: scanning session %s: %v", th.ID, err)
			continue
		}
		if projectPath == "unknown" && parsedProjectPath != "" {
			projectPath = parsedProjectPath
			meta.ProjectPath = parsedProjectPath
		}

		dougMeta := resolver.ParseDougMeta(filepath.Join(th.CWD, "AGENTS.md"))
		res := resolver.Resolve(resolver.Input{
			DougProjectID:   dougMeta.ProjectID,
			DougProjectName: dougMeta.ProjectName,
			GitRemoteURL:    th.GitOriginURL,
			RawPath:         th.CWD,
		})
		meta.RawProjectPath = meta.ProjectPath
		meta.CanonicalProjectID = res.CanonicalProjectID
		meta.CanonicalProjectSource = res.CanonicalProjectSource
		meta.DisplayProjectName = res.DisplayProjectName

		p.sessions[th.ID] = &sessionIndex{projectPath: projectPath, rolloutPath: rolloutPath}
		metas = append(metas, meta)
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].ID < metas[j].ID
	})
	return metas, nil
}

func (p *Provider) scanSessionPhase1(sessionID, projectPath, rolloutPath string) (*provider.SessionMeta, string, error) {
	f, err := os.Open(rolloutPath)
	if err != nil {
		return nil, "", fmt.Errorf("opening rollout file: %w", err)
	}
	defer f.Close()

	var (
		totals             provider.TokenCounts
		taskID             string
		model              string
		hasUser            bool
		parsedProjectPath  string
		startTime          time.Time
		lastAssistantTurnN = -1
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 8<<20)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rl rolloutLine
		if err := json.Unmarshal([]byte(line), &rl); err != nil {
			log.Printf("warning: codex: session %s line %d malformed JSON: %v", sessionID, lineNo, err)
			continue
		}

		if startTime.IsZero() {
			startTime = parseTime(rl.Timestamp)
		}

		switch strings.ToLower(strings.TrimSpace(rl.Type)) {
		case "session_meta":
			if parsedProjectPath == "" {
				parsedProjectPath = extractSessionMetaCWD(rl.Payload)
			}
		case "turn_context":
			if m := extractTurnContextModel(rl.Payload); m != "" {
				model = m
			}
		case "response_item":
			turn := parseResponseItem(rl.Payload, model, lineNo)
			if turn == nil {
				continue
			}
			if turn.msg.Role == "user" {
				hasUser = true
				if taskID == "" {
					taskID = extractTaskID(turn.text)
				}
				continue
			}
			if turn.msg.Role == "assistant" {
				if turn.msg.Model != "" {
					model = turn.msg.Model
				}
				lastAssistantTurnN = lineNo
			}
		case "event_msg":
			text, isUserMessage := extractUserEventText(rl.Payload)
			if isUserMessage {
				hasUser = true
				if taskID == "" {
					taskID = extractTaskID(text)
				}
			}

			tok, ok := extractLastTokenUsage(rl.Payload)
			if ok && lastAssistantTurnN >= 0 {
				totals.Input += tok.Input
				totals.CacheRead += tok.CacheRead
				totals.Output += tok.Output
				totals.Thoughts += tok.Thoughts
				lastAssistantTurnN = -1
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("reading rollout file: %w", err)
	}

	class := provider.ClassUntagged
	if taskID != "" {
		class = provider.ClassDoug
	} else if hasUser {
		class = provider.ClassManual
	}

	meta := &provider.SessionMeta{
		ID:          sessionID,
		Provider:    providerName,
		ProjectPath: projectPath,
		TaskID:      taskID,
		Model:       model,
		Class:       class,
		StartTime:   startTime,
		Tokens:      totals,
	}
	return meta, parsedProjectPath, nil
}

func (p *Provider) LoadTranscript(sessionID string) (*provider.Transcript, error) {
	idx, ok := p.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %s not in index; call LoadSessions first", sessionID)
	}

	f, err := os.Open(idx.rolloutPath)
	if err != nil {
		return nil, fmt.Errorf("opening rollout file: %w", err)
	}
	defer f.Close()

	tr := &provider.Transcript{SessionID: sessionID}
	currentModel := ""
	lastAssistantIdx := -1

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 8<<20)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rl rolloutLine
		if err := json.Unmarshal([]byte(line), &rl); err != nil {
			log.Printf("warning: codex: transcript %s line %d malformed JSON: %v", sessionID, lineNo, err)
			continue
		}

		switch strings.ToLower(strings.TrimSpace(rl.Type)) {
		case "session_meta":
			if idx.projectPath == "" {
				if cwd := extractSessionMetaCWD(rl.Payload); cwd != "" {
					idx.projectPath = cwd
				}
			}
		case "turn_context":
			if m := extractTurnContextModel(rl.Payload); m != "" {
				currentModel = m
			}
		case "response_item":
			turn := parseResponseItem(rl.Payload, currentModel, lineNo)
			if turn == nil {
				continue
			}
			tr.Messages = append(tr.Messages, turn.msg)
			if turn.msg.Role == "assistant" {
				lastAssistantIdx = len(tr.Messages) - 1
			}
		case "event_msg":
			tok, ok := extractLastTokenUsage(rl.Payload)
			if !ok || lastAssistantIdx < 0 {
				continue
			}
			tr.Messages[lastAssistantIdx].Tokens = &provider.TokenCounts{
				Input:     tok.Input,
				CacheRead: tok.CacheRead,
				Output:    tok.Output,
				Thoughts:  tok.Thoughts,
			}
			if tr.Messages[lastAssistantIdx].Model == "" {
				tr.Messages[lastAssistantIdx].Model = currentModel
			}
			lastAssistantIdx = -1
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading rollout transcript: %w", err)
	}

	return tr, nil
}

func loadThreadRows(dbPath string) ([]threadRow, error) {
	script := `
import json
import sqlite3
import sys

conn = sqlite3.connect(sys.argv[1])
conn.row_factory = sqlite3.Row
try:
    cols = {row["name"] for row in conn.execute("PRAGMA table_info(threads)")}
except sqlite3.Error as e:
    print(f"ERROR:{e}")
    sys.exit(2)

id_col = "id" if "id" in cols else ("thread_id" if "thread_id" in cols else None)
rollout_col = "rollout_path" if "rollout_path" in cols else None
if id_col is None or rollout_col is None:
    print("ERROR:threads table missing required id/rollout_path columns")
    sys.exit(2)

git_col = "git_origin_url" if "git_origin_url" in cols else None
cwd_col = "cwd" if "cwd" in cols else ("working_directory" if "working_directory" in cols else None)
git_expr = f'COALESCE("{git_col}", \'\')' if git_col else "''"
cwd_expr = f'COALESCE("{cwd_col}", \'\')' if cwd_col else "''"

q = f"""
SELECT "{id_col}" AS id,
       "{rollout_col}" AS rollout_path,
       {git_expr} AS git_origin_url,
       {cwd_expr} AS cwd
FROM threads
"""

out = []
for row in conn.execute(q):
    rid = (row["id"] or "").strip()
    rpath = (row["rollout_path"] or "").strip()
    if not rid or not rpath:
        continue
    out.append({
        "ID": rid,
        "RolloutPath": rpath,
        "GitOriginURL": row["git_origin_url"] or "",
        "CWD": row["cwd"] or "",
    })
print(json.dumps(out))
`
	cmd := exec.Command("python3", "-c", script, dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sqlite query bridge failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	var rows []threadRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("decoding sqlite rows: %w", err)
	}
	return rows, nil
}

func resolveRolloutPath(rootDir, rolloutPath string) string {
	if filepath.IsAbs(rolloutPath) {
		return rolloutPath
	}
	return filepath.Join(rootDir, rolloutPath)
}

func deriveProjectPath(gitOriginURL, cwd string) string {
	if name := repoNameFromRemote(gitOriginURL); name != "" {
		return name
	}
	if strings.TrimSpace(cwd) != "" {
		return strings.TrimSpace(cwd)
	}
	return "unknown"
}

func repoNameFromRemote(raw string) string {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(raw, "/"), ".git"))
	if raw == "" {
		return ""
	}

	if strings.Contains(raw, ":") && strings.Contains(raw, "@") && !strings.Contains(raw, "://") {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) == 2 {
			base := path.Base(parts[1])
			if base != "." && base != "/" {
				return base
			}
		}
	}

	if u, err := url.Parse(raw); err == nil {
		if b := path.Base(strings.TrimSuffix(u.Path, "/")); b != "" && b != "." && b != "/" {
			return b
		}
	}

	segs := strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == ':' })
	if len(segs) == 0 {
		return ""
	}
	return segs[len(segs)-1]
}

func extractSessionMetaCWD(raw json.RawMessage) string {
	v, ok := decodeObject(raw)
	if !ok {
		return ""
	}
	return firstString(v,
		[]string{"payload", "cwd"},
		[]string{"cwd"},
	)
}

func extractTurnContextModel(raw json.RawMessage) string {
	v, ok := decodeObject(raw)
	if !ok {
		return ""
	}
	return firstString(v,
		[]string{"model"},
		[]string{"payload", "model"},
	)
}

func parseResponseItem(raw json.RawMessage, currentModel string, seq int) *parsedTurn {
	v, ok := decodeObject(raw)
	if !ok {
		return nil
	}

	role := strings.ToLower(firstString(v,
		[]string{"role"},
		[]string{"payload", "role"},
		[]string{"message", "role"},
		[]string{"payload", "message", "role"},
	))
	if role != "user" && role != "assistant" {
		typ := strings.ToLower(firstString(v,
			[]string{"type"},
			[]string{"payload", "type"},
			[]string{"message", "type"},
		))
		switch {
		case strings.Contains(typ, "user"):
			role = "user"
		case strings.Contains(typ, "assistant") || strings.Contains(typ, "agent"):
			role = "assistant"
		default:
			return nil
		}
	}

	uuid := firstString(v,
		[]string{"id"},
		[]string{"uuid"},
		[]string{"payload", "id"},
		[]string{"message", "id"},
		[]string{"payload", "message", "id"},
	)
	if uuid == "" {
		uuid = fmt.Sprintf("codex-%06d", seq)
	}

	model := firstString(v,
		[]string{"model"},
		[]string{"payload", "model"},
		[]string{"message", "model"},
		[]string{"payload", "message", "model"},
	)
	if model == "" {
		model = currentModel
	}

	parent := firstString(v,
		[]string{"parent_id"},
		[]string{"parent_uuid"},
		[]string{"payload", "parent_id"},
		[]string{"message", "parent_id"},
	)

	ts := parseTime(firstString(v,
		[]string{"timestamp"},
		[]string{"created_at"},
		[]string{"payload", "timestamp"},
		[]string{"message", "timestamp"},
	))

	content := firstAny(v,
		[]string{"content"},
		[]string{"payload", "content"},
		[]string{"message", "content"},
		[]string{"payload", "message", "content"},
	)
	parts, text := contentParts(content)

	msg := provider.Message{
		UUID:       uuid,
		ParentUUID: parent,
		Role:       role,
		Model:      model,
		Timestamp:  ts,
		Content:    parts,
	}
	return &parsedTurn{msg: msg, text: text}
}

func extractUserEventText(raw json.RawMessage) (string, bool) {
	v, ok := decodeObject(raw)
	if !ok {
		return "", false
	}
	eventType := strings.ToLower(firstString(v,
		[]string{"type"},
		[]string{"payload", "type"},
	))
	if eventType != "user_message" {
		return "", false
	}
	text := firstString(v,
		[]string{"text"},
		[]string{"payload", "text"},
		[]string{"message"},
		[]string{"payload", "message"},
	)
	return text, true
}

func extractLastTokenUsage(raw json.RawMessage) (provider.TokenCounts, bool) {
	v, ok := decodeObject(raw)
	if !ok {
		return provider.TokenCounts{}, false
	}
	eventType := strings.ToLower(firstString(v,
		[]string{"type"},
		[]string{"payload", "type"},
	))
	if eventType != "token_count" {
		return provider.TokenCounts{}, false
	}

	usage := firstAny(v,
		[]string{"info", "last_token_usage"},
		[]string{"payload", "info", "last_token_usage"},
		[]string{"last_token_usage"},
	)
	m, ok := usage.(map[string]any)
	if !ok {
		return provider.TokenCounts{}, false
	}
	return provider.TokenCounts{
		Input:     int64At(m, "input_tokens"),
		CacheRead: int64At(m, "cached_input_tokens"),
		Output:    int64At(m, "output_tokens"),
		Thoughts:  int64At(m, "reasoning_output_tokens"),
	}, true
}

func extractTaskID(text string) string {
	if text == "" {
		return ""
	}
	m := taskIDPattern.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func contentParts(v any) ([]provider.ContentPart, string) {
	if v == nil {
		return nil, ""
	}
	switch x := v.(type) {
	case string:
		raw, _ := json.Marshal(x)
		return []provider.ContentPart{{Type: "text", Raw: raw}}, x
	case []any:
		parts := make([]provider.ContentPart, 0, len(x))
		var sb strings.Builder
		for _, item := range x {
			raw, _ := json.Marshal(item)
			typ := partType(item)
			parts = append(parts, provider.ContentPart{Type: typ, Raw: raw})
			if t := partText(item); t != "" {
				sb.WriteString(t)
			}
		}
		return parts, sb.String()
	case map[string]any:
		raw, _ := json.Marshal(x)
		return []provider.ContentPart{{Type: partType(x), Raw: raw}}, partText(x)
	default:
		raw, _ := json.Marshal(x)
		return []provider.ContentPart{{Type: "text", Raw: raw}}, ""
	}
}

func partType(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return "text"
	}
	if t := stringAt(m["type"]); t != "" {
		return t
	}
	if _, ok := m["text"]; ok {
		return "text"
	}
	return "text"
}

func partText(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	return stringAt(m["text"])
}

func decodeObject(raw json.RawMessage) (map[string]any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false
	}
	return v, true
}

func firstAny(v map[string]any, paths ...[]string) any {
	for _, p := range paths {
		if got, ok := anyAtPath(v, p...); ok {
			return got
		}
	}
	return nil
}

func firstString(v map[string]any, paths ...[]string) string {
	for _, p := range paths {
		if got, ok := anyAtPath(v, p...); ok {
			if s := stringAt(got); s != "" {
				return s
			}
		}
	}
	return ""
}

func anyAtPath(v map[string]any, path ...string) (any, bool) {
	var cur any = v
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := obj[key]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func stringAt(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	default:
		return ""
	}
}

func int64At(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i
	default:
		return 0
	}
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if n > 1_000_000_000_000 {
			return time.UnixMilli(n)
		}
		return time.Unix(n, 0)
	}
	return time.Time{}
}
