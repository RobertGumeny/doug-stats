// Package gemini implements the Provider interface for Gemini CLI logs.
//
// Gemini CLI stores project-scoped logs under ~/.gemini/tmp/<project-name>/:
//   - logs.json      — primary session index (user message log with sessionId)
//   - .project_root  — optional absolute project path for display
//   - chats/*.json   — full session transcript files
//
// Two-phase loading:
//
//	Phase 1 (LoadSessions)   — discovers sessions via logs.json and scans each
//	                           session file for token totals and task ID.
//	Phase 2 (LoadTranscript) — parses the full transcript on demand.
package gemini

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/robertgumeny/doug-stats/provider"
	"github.com/robertgumeny/doug-stats/provider/resolver"
)

const providerName = "gemini"

var taskIDPattern = regexp.MustCompile(`\[DOUG_TASK_ID:\s*([^\]]+)\]`)

type Provider struct {
	rootDir  string
	sessions map[string]*sessionIndex
}

type sessionIndex struct {
	sessionFile string
}

func New(rootDir string) *Provider {
	return &Provider{rootDir: rootDir, sessions: make(map[string]*sessionIndex)}
}

func (p *Provider) Name() string { return providerName }

type logEntry struct {
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
}

type chatFile struct {
	SessionID string        `json:"sessionId"`
	StartTime string        `json:"startTime"`
	Messages  []chatMessage `json:"messages"`
}

type chatMessage struct {
	ID        string            `json:"id"`
	Timestamp string            `json:"timestamp"`
	Type      string            `json:"type"`
	Content   json.RawMessage   `json:"content"`
	Thoughts  []json.RawMessage `json:"thoughts"`
	ToolCalls []toolCall        `json:"toolCalls"`
	Tokens    *geminiTokens     `json:"tokens"`
	Model     string            `json:"model"`
}

type geminiTokens struct {
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Cached   int64 `json:"cached"`
	Thoughts int64 `json:"thoughts"`
	Tool     int64 `json:"tool"`
}

type toolCall struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Args   json.RawMessage   `json:"args"`
	Result []json.RawMessage `json:"result"`
}

func (p *Provider) LoadSessions() ([]*provider.SessionMeta, error) {
	tmpRoot := filepath.Join(p.rootDir, "tmp")
	entries, err := os.ReadDir(tmpRoot)
	if err != nil {
		return nil, fmt.Errorf("reading tmp directory: %w", err)
	}

	var metas []*provider.SessionMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectDir := filepath.Join(tmpRoot, entry.Name())
		projectPath := resolveProjectPath(projectDir)

		refs, hasLogs := readLogsIndex(projectDir)
		if !hasLogs {
			for _, sid := range discoverFromChats(projectDir) {
				if _, seen := refs[sid]; !seen {
					refs[sid] = time.Time{}
				}
			}
		}

		for sessionID, startTime := range refs {
			sessionFile := findSessionFile(projectDir, sessionID)
			if sessionFile == "" {
				log.Printf("warning: gemini: session %s: chat file not found", sessionID)
				continue
			}
			meta, err := p.scanSessionPhase1(sessionID, projectPath, startTime, sessionFile)
			if err != nil {
				log.Printf("warning: gemini: scanning session %s: %v", sessionID, err)
				continue
			}
			p.sessions[sessionID] = &sessionIndex{sessionFile: sessionFile}
			metas = append(metas, meta)
		}
	}

	return metas, nil
}

func readLogsIndex(projectDir string) (map[string]time.Time, bool) {
	refs := make(map[string]time.Time)
	path := filepath.Join(projectDir, "logs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return refs, false
	}

	var entries []logEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("warning: gemini: malformed logs.json at %s: %v", path, err)
		return refs, true
	}

	for _, e := range entries {
		if e.SessionID == "" {
			log.Printf("warning: gemini: logs.json entry missing sessionId in %s", path)
			continue
		}
		ts := parseTime(e.Timestamp)
		if cur, seen := refs[e.SessionID]; !seen || (ts.Before(cur) && !ts.IsZero()) || cur.IsZero() {
			refs[e.SessionID] = ts
		}
	}
	return refs, true
}

func discoverFromChats(projectDir string) []string {
	chatsDir := filepath.Join(projectDir, "chats")
	files, err := filepath.Glob(filepath.Join(chatsDir, "session-*.json"))
	if err != nil {
		return nil
	}
	var ids []string
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		var cf chatFile
		if err := json.NewDecoder(f).Decode(&cf); err == nil && cf.SessionID != "" {
			ids = append(ids, cf.SessionID)
		}
		f.Close()
	}
	return ids
}

func findSessionFile(projectDir, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	chatsDir := filepath.Join(projectDir, "chats")
	prefix := sessionID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	patterns := []string{
		filepath.Join(chatsDir, "session-*-"+prefix+".json"),
		filepath.Join(chatsDir, "session-*"+sessionID+"*.json"),
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		if len(matches) == 0 {
			continue
		}
		sort.Strings(matches)
		return matches[0]
	}
	return ""
}

func resolveProjectPath(projectDir string) string {
	rootFile := filepath.Join(projectDir, ".project_root")
	data, err := os.ReadFile(rootFile)
	if err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			return s
		}
	}
	return filepath.Base(projectDir)
}

func (p *Provider) scanSessionPhase1(sessionID, projectPath string, startTime time.Time, sessionFile string) (*provider.SessionMeta, error) {
	f, err := os.Open(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("opening session file: %w", err)
	}
	defer f.Close()

	var cf chatFile
	if err := json.NewDecoder(f).Decode(&cf); err != nil {
		return nil, fmt.Errorf("decoding session JSON: %w", err)
	}
	if cf.SessionID == "" {
		return nil, fmt.Errorf("missing sessionId")
	}
	if cf.SessionID != sessionID {
		log.Printf("warning: gemini: session id mismatch index=%s file=%s", sessionID, cf.SessionID)
	}
	if startTime.IsZero() {
		startTime = parseTime(cf.StartTime)
	}

	class := provider.ClassUntagged
	hasUser := false
	taskID := ""
	model := ""
	totals := provider.TokenCounts{}

	for i, msg := range cf.Messages {
		switch msg.Type {
		case "user":
			hasUser = true
			if taskID == "" {
				if m := taskIDPattern.FindStringSubmatch(extractText(msg.Content)); m != nil {
					taskID = strings.TrimSpace(m[1])
					class = provider.ClassDoug
				}
			}
		case "gemini", "assistant", "model":
			if msg.Model != "" {
				model = msg.Model
			}
			if msg.Tokens == nil {
				log.Printf("warning: gemini: session %s message %d missing tokens, skipping token aggregation", sessionID, i)
				continue
			}
			totals.Input += msg.Tokens.Input
			totals.Output += msg.Tokens.Output
			totals.CacheRead += msg.Tokens.Cached
			totals.Thoughts += msg.Tokens.Thoughts
			totals.Tool += msg.Tokens.Tool
		}
	}

	if class != provider.ClassDoug {
		if hasUser {
			class = provider.ClassManual
		} else {
			class = provider.ClassUntagged
		}
	}

	res := resolver.Resolve(resolver.Input{RawPath: projectPath})
	return &provider.SessionMeta{
		ID:                     sessionID,
		Provider:               providerName,
		ProjectPath:            projectPath,
		TaskID:                 taskID,
		Model:                  model,
		Class:                  class,
		StartTime:              startTime,
		Tokens:                 totals,
		RawProjectPath:         projectPath,
		CanonicalProjectID:     res.CanonicalProjectID,
		CanonicalProjectSource: res.CanonicalProjectSource,
		DisplayProjectName:     res.DisplayProjectName,
	}, nil
}

func (p *Provider) LoadTranscript(sessionID string) (*provider.Transcript, error) {
	idx, ok := p.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %s not in index; call LoadSessions first", sessionID)
	}

	f, err := os.Open(idx.sessionFile)
	if err != nil {
		return nil, fmt.Errorf("opening session file: %w", err)
	}
	defer f.Close()

	var cf chatFile
	if err := json.NewDecoder(f).Decode(&cf); err != nil {
		return nil, fmt.Errorf("decoding session JSON: %w", err)
	}

	transcript := &provider.Transcript{SessionID: sessionID}
	for _, m := range cf.Messages {
		msg := toProviderMessage(sessionID, m)
		if msg != nil {
			transcript.Messages = append(transcript.Messages, *msg)
		}
	}
	return transcript, nil
}

func toProviderMessage(sessionID string, m chatMessage) *provider.Message {
	msg := &provider.Message{
		UUID:      m.ID,
		Timestamp: parseTime(m.Timestamp),
	}

	switch m.Type {
	case "user":
		msg.Role = "user"
		msg.Content = parseUserContentParts(m.Content)
		return msg
	case "gemini", "assistant", "model":
		msg.Role = "assistant"
		msg.Model = m.Model
		msg.Content = parseAssistantContentParts(m)
		if m.Tokens != nil {
			msg.Tokens = &provider.TokenCounts{
				Input:     m.Tokens.Input,
				Output:    m.Tokens.Output,
				CacheRead: m.Tokens.Cached,
				Thoughts:  m.Tokens.Thoughts,
				Tool:      m.Tokens.Tool,
			}
		}
		return msg
	default:
		log.Printf("warning: gemini: session %s: unknown message type %q, skipping", sessionID, m.Type)
		return nil
	}
}

func parseUserContentParts(raw json.RawMessage) []provider.ContentPart {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []provider.ContentPart{{Type: "text", Raw: raw}}
	}

	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		cp := typedPartFromRaw(raw)
		if cp.Type == "" {
			cp.Type = "text"
		}
		return []provider.ContentPart{cp}
	}

	result := make([]provider.ContentPart, 0, len(parts))
	for _, part := range parts {
		cp := typedPartFromRaw(part)
		if cp.Type == "" {
			cp.Type = "text"
		}
		result = append(result, cp)
	}
	return result
}

func parseAssistantContentParts(m chatMessage) []provider.ContentPart {
	var out []provider.ContentPart

	if len(m.Content) > 0 {
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			raw, _ := json.Marshal(map[string]string{"text": s})
			out = append(out, provider.ContentPart{Type: "text", Raw: raw})
		} else {
			for _, cp := range parseUserContentParts(m.Content) {
				out = append(out, cp)
			}
		}
	}

	for _, thought := range m.Thoughts {
		out = append(out, provider.ContentPart{Type: "thinking", Raw: thought})
	}

	for _, tc := range m.ToolCalls {
		useRaw, _ := json.Marshal(map[string]any{
			"id":    tc.ID,
			"name":  tc.Name,
			"input": decodeAny(tc.Args),
		})
		out = append(out, provider.ContentPart{Type: "tool_use", Raw: useRaw})
		for _, r := range tc.Result {
			out = append(out, provider.ContentPart{Type: "tool_result", Raw: r})
		}
	}

	return out
}

func decodeAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

func typedPartFromRaw(raw json.RawMessage) provider.ContentPart {
	var typ struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &typ); err != nil {
		return provider.ContentPart{Raw: raw}
	}
	pt := typ.Type
	if pt == "" && typ.Text != "" {
		pt = "text"
	}
	return provider.ContentPart{Type: pt, Raw: raw}
}

func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	return sb.String()
}

func parseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return time.Time{}
}
