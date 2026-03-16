// Package claude implements the Provider interface for Claude Code log files.
//
// Claude Code stores session data under ~/.claude/:
//   - history.jsonl        — primary session index (one entry per user message)
//   - projects/<encoded>/<session-uuid>.jsonl — session transcript files
//
// The encoded project directory name is derived by replacing all '/' in the
// project path with '-' (e.g. "/home/user/proj" → "-home-user-proj").
//
// Subagent session files live under <session-uuid>/subagents/ and are never
// scanned because they do not appear in history.jsonl.
//
// Two-phase loading:
//
//	Phase 1 (LoadSessions)   — reads history.jsonl and scans each session file
//	                           for token usage and task ID. Aggregates are
//	                           available immediately after this call returns.
//	Phase 2 (LoadTranscript) — parses the full transcript on demand when a
//	                           session is opened in the UI.
package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/robertgumeny/doug-stats/provider"
	"github.com/robertgumeny/doug-stats/provider/resolver"
)

const providerName = "claude"

// taskIDPattern matches [DOUG_TASK_ID: <id>] in user message text.
var taskIDPattern = regexp.MustCompile(`\[DOUG_TASK_ID:\s*([^\]]+)\]`)

// Provider implements provider.Provider for Claude Code log files.
type Provider struct {
	rootDir  string
	sessions map[string]*sessionIndex // keyed by sessionID, populated by LoadSessions
}

type sessionIndex struct {
	projectPath string
	sessionFile string
}

// New creates a Claude Provider rooted at rootDir (e.g. ~/.claude).
func New(rootDir string) *Provider {
	return &Provider{
		rootDir:  rootDir,
		sessions: make(map[string]*sessionIndex),
	}
}

func (p *Provider) Name() string { return providerName }

// --- internal JSON shapes ---

// historyEntry is one line of history.jsonl.
type historyEntry struct {
	SessionID string `json:"sessionId"`
	Project   string `json:"project"`
	Timestamp int64  `json:"timestamp"` // Unix milliseconds
}

// sessionLine is one line of a session JSONL file.
type sessionLine struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  *string         `json:"parentUuid"`
	IsSidechain bool            `json:"isSidechain"`
	Timestamp   string          `json:"timestamp"` // ISO-8601
	Message     json.RawMessage `json:"message"`
}

type assistantMessage struct {
	Model      string          `json:"model"`
	ID         string          `json:"id"`
	StopReason *string         `json:"stop_reason"`
	Content    json.RawMessage `json:"content"`
	Usage      *tokenUsage     `json:"usage"`
}

type tokenUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
}

type userMessageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []content part
}

// --- path helpers ---

// encodeProjectPath converts a filesystem path to the Claude project
// directory name by replacing all '/' with '-'.
func encodeProjectPath(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}

// sessionFilePath returns the path to a session's JSONL file.
func (p *Provider) sessionFilePath(projectPath, sessionID string) string {
	return filepath.Join(p.rootDir, "projects", encodeProjectPath(projectPath), sessionID+".jsonl")
}

// --- Phase 1 ---

// LoadSessions reads history.jsonl and scans each unique session file to
// build an index with token aggregates. It is safe to call once at startup.
func (p *Provider) LoadSessions() ([]*provider.SessionMeta, error) {
	historyPath := filepath.Join(p.rootDir, "history.jsonl")
	f, err := os.Open(historyPath)
	if err != nil {
		return nil, fmt.Errorf("opening history.jsonl: %w", err)
	}
	defer f.Close()

	type sessionRef struct {
		projectPath string
		firstSeen   time.Time
	}
	refs := make(map[string]*sessionRef)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry historyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("warning: claude: malformed history.jsonl line: %v", err)
			continue
		}
		if entry.SessionID == "" || entry.Project == "" {
			log.Printf("warning: claude: history.jsonl line missing sessionId or project, skipping")
			continue
		}
		if _, seen := refs[entry.SessionID]; !seen {
			refs[entry.SessionID] = &sessionRef{
				projectPath: entry.Project,
				firstSeen:   time.UnixMilli(entry.Timestamp),
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading history.jsonl: %w", err)
	}

	var metas []*provider.SessionMeta
	for sessionID, ref := range refs {
		filePath := p.sessionFilePath(ref.projectPath, sessionID)
		meta, err := p.scanSessionPhase1(sessionID, ref.projectPath, ref.firstSeen, filePath)
		if err != nil {
			log.Printf("warning: claude: scanning session %s: %v", sessionID, err)
			continue
		}
		p.sessions[sessionID] = &sessionIndex{
			projectPath: ref.projectPath,
			sessionFile: filePath,
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

// scanSessionPhase1 opens a session file and extracts token totals and task ID.
// It deduplicates assistant records by message ID, counting only the final
// record (stop_reason != nil) for each message to avoid double-counting
// streaming intermediate updates.
func (p *Provider) scanSessionPhase1(sessionID, projectPath string, startTime time.Time, filePath string) (*provider.SessionMeta, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening session file: %w", err)
	}
	defer f.Close()

	var (
		totals     provider.TokenCounts
		taskID     string
		model      string
		class      = provider.ClassUntagged
		hasUserMsg bool
		seenMsgIDs = make(map[string]bool)
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var sl sessionLine
		if err := json.Unmarshal([]byte(line), &sl); err != nil {
			log.Printf("warning: claude: session %s: malformed line: %v", sessionID, err)
			continue
		}

		switch sl.Type {
		case "assistant":
			var am assistantMessage
			if err := json.Unmarshal(sl.Message, &am); err != nil {
				log.Printf("warning: claude: session %s: malformed assistant message: %v", sessionID, err)
				continue
			}
			// Skip streaming intermediate records — only count the final record
			// for each message ID (identified by stop_reason being set).
			if am.StopReason == nil || am.ID == "" {
				continue
			}
			if seenMsgIDs[am.ID] {
				continue
			}
			seenMsgIDs[am.ID] = true
			if model == "" && am.Model != "" {
				model = am.Model
			}
			if am.Usage != nil {
				totals.Input += am.Usage.InputTokens
				totals.CacheCreation += am.Usage.CacheCreationInputTokens
				totals.CacheRead += am.Usage.CacheReadInputTokens
				totals.Output += am.Usage.OutputTokens
			}

		case "user":
			if sl.Message == nil {
				continue
			}
			var um userMessageEnvelope
			if err := json.Unmarshal(sl.Message, &um); err != nil {
				log.Printf("warning: claude: session %s: malformed user message: %v", sessionID, err)
				continue
			}
			hasUserMsg = true
			if taskID == "" {
				text := extractText(um.Content)
				if m := taskIDPattern.FindStringSubmatch(text); m != nil {
					taskID = strings.TrimSpace(m[1])
					class = provider.ClassDoug
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	if class != provider.ClassDoug {
		if hasUserMsg {
			class = provider.ClassManual
		} else {
			class = provider.ClassUntagged
		}
	}

	dougMeta := resolver.ParseDougMeta(filepath.Join(projectPath, "AGENTS.md"))
	res := resolver.Resolve(resolver.Input{
		DougProjectID:   dougMeta.ProjectID,
		DougProjectName: dougMeta.ProjectName,
		RawPath:         projectPath,
	})
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

// --- Phase 2 ---

// LoadTranscript parses the full session transcript on demand.
// LoadSessions must have been called before this method.
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

	transcript := &provider.Transcript{SessionID: sessionID}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var sl sessionLine
		if err := json.Unmarshal([]byte(line), &sl); err != nil {
			log.Printf("warning: claude: transcript %s: malformed line: %v", sessionID, err)
			continue
		}

		var msg *provider.Message
		switch sl.Type {
		case "user":
			msg = parseUserLine(sessionID, &sl)
		case "assistant":
			msg = parseAssistantLine(sessionID, &sl)
		default:
			continue // skip queue-operation and other internal record types
		}
		if msg != nil {
			transcript.Messages = append(transcript.Messages, *msg)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading session transcript: %w", err)
	}

	return transcript, nil
}

func parseUserLine(sessionID string, sl *sessionLine) *provider.Message {
	var um userMessageEnvelope
	if err := json.Unmarshal(sl.Message, &um); err != nil {
		log.Printf("warning: claude: session %s: malformed user message: %v", sessionID, err)
		return nil
	}
	msg := &provider.Message{
		UUID:        sl.UUID,
		Role:        "user",
		IsSidechain: sl.IsSidechain,
		Content:     parseContentParts(um.Content),
	}
	if sl.ParentUUID != nil {
		msg.ParentUUID = *sl.ParentUUID
	}
	if sl.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, sl.Timestamp); err == nil {
			msg.Timestamp = t
		}
	}
	return msg
}

func parseAssistantLine(sessionID string, sl *sessionLine) *provider.Message {
	var am assistantMessage
	if err := json.Unmarshal(sl.Message, &am); err != nil {
		log.Printf("warning: claude: session %s: malformed assistant message: %v", sessionID, err)
		return nil
	}
	msg := &provider.Message{
		UUID:        sl.UUID,
		Role:        "assistant",
		Model:       am.Model,
		IsSidechain: sl.IsSidechain,
		Content:     parseContentParts(am.Content),
	}
	if sl.ParentUUID != nil {
		msg.ParentUUID = *sl.ParentUUID
	}
	if sl.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, sl.Timestamp); err == nil {
			msg.Timestamp = t
		}
	}
	if am.Usage != nil {
		msg.Tokens = &provider.TokenCounts{
			Input:         am.Usage.InputTokens,
			CacheCreation: am.Usage.CacheCreationInputTokens,
			CacheRead:     am.Usage.CacheReadInputTokens,
			Output:        am.Usage.OutputTokens,
		}
	}
	return msg
}

// parseContentParts converts a raw JSON content field (string or array) into
// a slice of ContentPart values.
func parseContentParts(raw json.RawMessage) []provider.ContentPart {
	if len(raw) == 0 {
		return nil
	}
	// Try plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []provider.ContentPart{{Type: "text", Raw: raw}}
	}
	// Try array of typed parts.
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	result := make([]provider.ContentPart, 0, len(parts))
	for _, part := range parts {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(part, &typed); err != nil {
			log.Printf("warning: claude: malformed content part: %v", err)
			continue
		}
		result = append(result, provider.ContentPart{Type: typed.Type, Raw: part})
	}
	return result
}

// extractText returns the plain text from a content field that may be a JSON
// string or a JSON array of content parts. Only "text" parts are included.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}
