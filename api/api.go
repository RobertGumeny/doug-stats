// Package api implements the in-memory HTTP API layer for doug-stats.
//
// All data is loaded during Phase 1 startup; no endpoint except
// /api/sessions/:id/messages reads from disk at runtime.
//
// Endpoint contract:
//
//	GET /api/health
//	  → {"status":"ok"}
//
//	GET /api/projects[?provider=<name>&doug_only=true]
//	  → [{"path":"...","cost_usd":1.23,"unknown":false}, ...]
//
//	GET /api/tasks?project=<path>[&provider=<name>&doug_only=true]
//	  → [{"task_id":"...","cost_usd":1.23,"unknown":false}, ...]
//	  Includes a virtual task_id "manual" for ClassManual+ClassUntagged sessions
//	  unless doug_only=true.
//
//	GET /api/sessions?task=<id>[&project=<path>&provider=<name>&doug_only=true]
//	  task="manual" matches ClassManual and ClassUntagged sessions.
//	  → [{"id":"...","provider":"...","project_path":"...","task_id":"...",
//	      "model":"...","class":"...","start_time":"...","cost_usd":1.23,"unknown":false}, ...]
//
//	GET /api/sessions/:id/messages
//	  Performs a full JSONL parse on demand; no other endpoint reads from disk.
//	  → [{"uuid":"...","parent_uuid":"...","role":"...","model":"...",
//	      "timestamp":"...","is_sidechain":false,"cost_usd":0,"cost_unknown":false,
//	      "content":[{"type":"...","raw":{...}}]}, ...]
//
// Common query parameters:
//
//	provider=<name>    limit results to the named provider; may repeat for multiple
//	doug_only=true     exclude ClassManual and ClassUntagged sessions from results
//	                   and aggregates
//
// Error responses use a consistent JSON envelope: {"error":"<message>"}
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/robertgumeny/doug-stats/aggregator"
	"github.com/robertgumeny/doug-stats/pricing"
	"github.com/robertgumeny/doug-stats/provider"
)

// Handler serves all API endpoints from in-memory data built during Phase 1.
// It is safe for concurrent use once constructed.
type Handler struct {
	sessions  []*provider.SessionMeta
	costs     map[string]pricing.Cost      // session ID → cost
	providers map[string]provider.Provider // provider name → Provider
}

// ProjectItem is the response element for GET /api/projects.
type ProjectItem struct {
	Path    string  `json:"path"`
	CostUSD float64 `json:"cost_usd"`
	Unknown bool    `json:"unknown"`
}

// TaskItem is the response element for GET /api/tasks.
type TaskItem struct {
	TaskID  string  `json:"task_id"`
	CostUSD float64 `json:"cost_usd"`
	Unknown bool    `json:"unknown"`
}

// SessionItem is the response element for GET /api/sessions.
type SessionItem struct {
	ID          string    `json:"id"`
	Provider    string    `json:"provider"`
	ProjectPath string    `json:"project_path"`
	TaskID      string    `json:"task_id"`
	Model       string    `json:"model"`
	Class       string    `json:"class"`
	StartTime   time.Time `json:"start_time"`
	DurationMs  *int64    `json:"duration_ms,omitempty"`
	CostUSD     float64   `json:"cost_usd"`
	Unknown     bool      `json:"unknown"`
}

// ContentPartItem is a single element in a message's content array.
type ContentPartItem struct {
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"raw"`
}

// MessageItem is the response element for GET /api/sessions/:id/messages.
type MessageItem struct {
	UUID        string            `json:"uuid"`
	ParentUUID  string            `json:"parent_uuid,omitempty"`
	Role        string            `json:"role"`
	Model       string            `json:"model,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	IsSidechain bool              `json:"is_sidechain"`
	CostUSD     float64           `json:"cost_usd"`
	CostUnknown bool              `json:"cost_unknown"`
	Content     []ContentPartItem `json:"content"`
}

// New constructs a Handler from the Phase 1 data.
// sessions is the full session list from all providers.
// summary provides pre-computed per-session costs.
// providers maps provider name to its Provider implementation for on-demand Phase 2 loading.
func New(sessions []*provider.SessionMeta, summary *aggregator.Summary, providers map[string]provider.Provider) *Handler {
	costs := make(map[string]pricing.Cost, len(summary.Sessions))
	for _, sa := range summary.Sessions {
		costs[sa.SessionID] = sa.TotalCost
	}
	return &Handler{
		sessions:  sessions,
		costs:     costs,
		providers: providers,
	}
}

// Register mounts all API routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.handleHealth)
	mux.HandleFunc("/api/projects", h.handleProjects)
	mux.HandleFunc("/api/tasks", h.handleTasks)
	mux.HandleFunc("/api/sessions", h.handleSessions)
	// /api/sessions/ prefix catches /api/sessions/:id/messages
	mux.HandleFunc("/api/sessions/", h.handleSessionMessages)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func classString(c provider.SessionClass) string {
	switch c {
	case provider.ClassDoug:
		return "doug"
	case provider.ClassManual:
		return "manual"
	default:
		return "untagged"
	}
}

// parseFilters extracts the provider list and doug_only flag from query params.
// provider may appear multiple times; doug_only must equal "true" to be active.
func parseFilters(r *http.Request) (providers []string, dougOnly bool) {
	providers = r.URL.Query()["provider"]
	dougOnly = r.URL.Query().Get("doug_only") == "true"
	return
}

// filterSessions returns the subset of sessions that match the given filters.
// If providers is empty, no provider filter is applied.
// If dougOnly is true, ClassManual and ClassUntagged sessions are excluded.
func filterSessions(sessions []*provider.SessionMeta, providers []string, dougOnly bool) []*provider.SessionMeta {
	if len(providers) == 0 && !dougOnly {
		return sessions
	}
	providerSet := make(map[string]bool, len(providers))
	for _, p := range providers {
		providerSet[p] = true
	}
	result := make([]*provider.SessionMeta, 0, len(sessions))
	for _, s := range sessions {
		if len(providerSet) > 0 && !providerSet[s.Provider] {
			continue
		}
		if dougOnly && s.Class != provider.ClassDoug {
			continue
		}
		result = append(result, s)
	}
	return result
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleProjects(w http.ResponseWriter, r *http.Request) {
	provs, dougOnly := parseFilters(r)
	filtered := filterSessions(h.sessions, provs, dougOnly)

	type acc struct{ cost pricing.Cost }
	byProject := make(map[string]*acc)
	for _, s := range filtered {
		if _, ok := byProject[s.ProjectPath]; !ok {
			byProject[s.ProjectPath] = &acc{}
		}
		byProject[s.ProjectPath].cost = byProject[s.ProjectPath].cost.Add(h.costs[s.ID])
	}

	items := make([]ProjectItem, 0, len(byProject))
	for path, a := range byProject {
		items = append(items, ProjectItem{
			Path:    path,
			CostUSD: a.cost.USD,
			Unknown: a.cost.Unknown,
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleTasks(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project parameter required")
		return
	}
	provs, dougOnly := parseFilters(r)

	// Filter to project first, then apply provider/dougOnly filters.
	var projectSessions []*provider.SessionMeta
	for _, s := range h.sessions {
		if s.ProjectPath == project {
			projectSessions = append(projectSessions, s)
		}
	}
	filtered := filterSessions(projectSessions, provs, dougOnly)

	type acc struct{ cost pricing.Cost }
	byTask := make(map[string]*acc)
	manualCost := pricing.Cost{}
	hasManual := false

	for _, s := range filtered {
		switch s.Class {
		case provider.ClassDoug:
			if _, ok := byTask[s.TaskID]; !ok {
				byTask[s.TaskID] = &acc{}
			}
			byTask[s.TaskID].cost = byTask[s.TaskID].cost.Add(h.costs[s.ID])
		case provider.ClassManual, provider.ClassUntagged:
			hasManual = true
			manualCost = manualCost.Add(h.costs[s.ID])
		}
	}

	items := make([]TaskItem, 0, len(byTask)+1)
	for taskID, a := range byTask {
		items = append(items, TaskItem{
			TaskID:  taskID,
			CostUSD: a.cost.USD,
			Unknown: a.cost.Unknown,
		})
	}
	if hasManual {
		items = append(items, TaskItem{
			TaskID:  "manual",
			CostUSD: manualCost.USD,
			Unknown: manualCost.Unknown,
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleSessions(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task parameter required")
		return
	}
	project := r.URL.Query().Get("project") // optional scope
	provs, dougOnly := parseFilters(r)

	var candidates []*provider.SessionMeta
	if taskID == "manual" {
		// Virtual task: matches ClassManual and ClassUntagged sessions.
		for _, s := range h.sessions {
			if s.Class == provider.ClassManual || s.Class == provider.ClassUntagged {
				candidates = append(candidates, s)
			}
		}
	} else {
		for _, s := range h.sessions {
			if s.TaskID == taskID {
				candidates = append(candidates, s)
			}
		}
	}

	if project != "" {
		var inProject []*provider.SessionMeta
		for _, s := range candidates {
			if s.ProjectPath == project {
				inProject = append(inProject, s)
			}
		}
		candidates = inProject
	}

	filtered := filterSessions(candidates, provs, dougOnly)

	items := make([]SessionItem, 0, len(filtered))
	for _, s := range filtered {
		cost := h.costs[s.ID]
		items = append(items, SessionItem{
			ID:          s.ID,
			Provider:    s.Provider,
			ProjectPath: s.ProjectPath,
			TaskID:      s.TaskID,
			Model:       s.Model,
			Class:       classString(s.Class),
			StartTime:   s.StartTime,
			DurationMs:  s.DurationMs,
			CostUSD:     cost.USD,
			Unknown:     cost.Unknown,
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleSessionMessages(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from /api/sessions/:id/messages
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if !strings.HasSuffix(path, "/messages") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	sessionID := strings.TrimSuffix(path, "/messages")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID required")
		return
	}

	// Locate the session to find its provider.
	var sessionMeta *provider.SessionMeta
	for _, s := range h.sessions {
		if s.ID == sessionID {
			sessionMeta = s
			break
		}
	}
	if sessionMeta == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	p, ok := h.providers[sessionMeta.Provider]
	if !ok {
		writeError(w, http.StatusInternalServerError, "provider not available")
		return
	}

	// Phase 2: full JSONL parse on demand.
	transcript, err := p.LoadTranscript(sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load transcript: "+err.Error())
		return
	}

	msgCosts := aggregator.ComputeMessageCosts(transcript)
	costByUUID := make(map[string]pricing.Cost, len(msgCosts))
	for _, mc := range msgCosts {
		costByUUID[mc.UUID] = mc.Cost
	}

	items := make([]MessageItem, 0, len(transcript.Messages))
	for _, msg := range transcript.Messages {
		cost := costByUUID[msg.UUID]
		content := make([]ContentPartItem, 0, len(msg.Content))
		for _, cp := range msg.Content {
			content = append(content, ContentPartItem{Type: cp.Type, Raw: cp.Raw})
		}
		items = append(items, MessageItem{
			UUID:        msg.UUID,
			ParentUUID:  msg.ParentUUID,
			Role:        msg.Role,
			Model:       msg.Model,
			Timestamp:   msg.Timestamp,
			IsSidechain: msg.IsSidechain,
			CostUSD:     cost.USD,
			CostUnknown: cost.Unknown,
			Content:     content,
		})
	}
	writeJSON(w, http.StatusOK, items)
}
