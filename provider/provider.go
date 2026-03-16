package provider

import (
	"encoding/json"
	"time"
)

// SessionClass categorizes how a session was initiated.
type SessionClass int

const (
	ClassDoug     SessionClass = iota // session has a DOUG_TASK_ID tag
	ClassManual                       // user-initiated, no task ID
	ClassUntagged                     // no identifiable user messages
)

// TokenCounts holds normalized token categories across providers.
type TokenCounts struct {
	Input         int64
	CacheCreation int64
	CacheRead     int64
	Output        int64
	Thoughts      int64
	Tool          int64
}

// CanonicalProjectSource identifies which resolution step produced the canonical project ID.
type CanonicalProjectSource string

const (
	SourceDoug           CanonicalProjectSource = "doug"
	SourceGitRemote      CanonicalProjectSource = "git-remote"
	SourceNormalizedPath CanonicalProjectSource = "normalized-path"
	SourceBasenameFallback CanonicalProjectSource = "basename-fallback"
)

// SessionMeta contains the Phase 1 index data for a session.
type SessionMeta struct {
	ID          string
	Provider    string
	ProjectPath string // raw project path from the provider (e.g. "/home/user/myproject")
	TaskID      string // empty unless Class == ClassDoug
	Model       string // last model identifier seen in assistant messages; empty if none
	Class       SessionClass
	StartTime   time.Time
	Tokens      TokenCounts

	// Canonical identity fields (populated by the project resolver).
	RawProjectPath         string                 // copy of the raw provider path before any normalization
	CanonicalProjectID     string                 // stable cross-provider grouping key
	CanonicalProjectSource CanonicalProjectSource // which resolution step produced CanonicalProjectID
	DisplayProjectName     string                 // human-readable project name
	ProjectAliases         []string               // other known paths/names for this project
}

// ContentPart is a single content element in a message.
type ContentPart struct {
	Type string          // "text", "tool_use", "tool_result", "thinking", etc.
	Raw  json.RawMessage // full JSON of this part
}

// Message represents a single turn in a session transcript.
type Message struct {
	UUID        string
	ParentUUID  string
	Role        string // "user" or "assistant"
	Model       string // populated for assistant turns
	Timestamp   time.Time
	IsSidechain bool
	Tokens      *TokenCounts // populated for assistant turns (final record only)
	Content     []ContentPart
}

// Transcript holds the Phase 2 (full transcript) data for a session.
type Transcript struct {
	SessionID string
	Messages  []Message
}

// Provider is the interface all AI log providers must implement.
// Implementations separate session-index/correlation logic (Phase 1)
// from full transcript parsing (Phase 2).
type Provider interface {
	// Name returns the provider identifier (e.g., "claude").
	Name() string

	// LoadSessions performs Phase 1: discovers sessions and builds the index
	// with token aggregates. Returns metadata for all discovered sessions.
	LoadSessions() ([]*SessionMeta, error)

	// LoadTranscript performs Phase 2: parses the full transcript for a
	// single session on demand.
	LoadTranscript(sessionID string) (*Transcript, error)
}
