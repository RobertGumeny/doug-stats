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
//	  → [{"canonicalProjectID":"...","displayName":"...","aliases":["..."],
//	      "providerCoverage":["claude"],"sessionCount":1,"taskCount":1,
//	      "totalCost":1.23,"unknownPricing":false}, ...]
//
//	GET /api/tasks?project=<canonicalProjectID>[&provider=<name>&doug_only=true&sort=cost]
//	  → [{"taskID":"...","providerCoverage":["claude"],"sessionCount":1,
//	      "totalCost":1.23,"unknownPricing":false}, ...]
//	  Includes a virtual taskID "manual" for ClassManual+ClassUntagged sessions
//	  unless doug_only=true.
//
//	GET /api/sessions?task=<id>[&project=<canonicalProjectID>&provider=<name>&doug_only=true&sort=<recent|cost>]
//	  task="manual" matches ClassManual and ClassUntagged sessions.
//	  → [{"id":"...","provider":"...","canonicalProjectID":"...",
//	      "rawProjectPath":"...","taskID":"...","model":"...","class":"...",
//	      "startTime":"...","duration":1234,"totalCost":1.23,
//	      "unknownPricing":false}, ...]
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
//	sort=...           endpoint-specific sort key; invalid values return 400
//
// Error responses use a consistent JSON envelope: {"error":"<message>"}
package api

import (
	"encoding/json"
	"net/http"
	"sort"
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
	CanonicalProjectID string   `json:"canonicalProjectID"`
	DisplayName        string   `json:"displayName"`
	Aliases            []string `json:"aliases"`
	ProviderCoverage   []string `json:"providerCoverage"`
	SessionCount       int      `json:"sessionCount"`
	TaskCount          int      `json:"taskCount"`
	TotalCost          float64  `json:"totalCost"`
	UnknownPricing     bool     `json:"unknownPricing"`
}

// TaskItem is the response element for GET /api/tasks.
type TaskItem struct {
	TaskID           string   `json:"taskID"`
	ProviderCoverage []string `json:"providerCoverage"`
	SessionCount     int      `json:"sessionCount"`
	TotalCost        float64  `json:"totalCost"`
	UnknownPricing   bool     `json:"unknownPricing"`
}

// SessionItem is the response element for GET /api/sessions.
type SessionItem struct {
	ID                 string    `json:"id"`
	Provider           string    `json:"provider"`
	CanonicalProjectID string    `json:"canonicalProjectID"`
	RawProjectPath     string    `json:"rawProjectPath"`
	TaskID             string    `json:"taskID"`
	Model              string    `json:"model"`
	Class              string    `json:"class"`
	StartTime          time.Time `json:"startTime"`
	Duration           *int64    `json:"duration,omitempty"`
	TotalCost          float64   `json:"totalCost"`
	UnknownPricing     bool      `json:"unknownPricing"`
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

func canonicalProjectID(s *provider.SessionMeta) string {
	if s.CanonicalProjectID != "" {
		return s.CanonicalProjectID
	}
	if s.RawProjectPath != "" {
		return s.RawProjectPath
	}
	return s.ProjectPath
}

func rawProjectPath(s *provider.SessionMeta) string {
	if s.RawProjectPath != "" {
		return s.RawProjectPath
	}
	return s.ProjectPath
}

func displayProjectName(s *provider.SessionMeta) string {
	if s.DisplayProjectName != "" {
		return s.DisplayProjectName
	}
	if id := canonicalProjectID(s); id != "" {
		return id
	}
	return rawProjectPath(s)
}

func aggregateTaskID(s *provider.SessionMeta) string {
	if s.TaskID != "" {
		return s.TaskID
	}
	if s.Class == provider.ClassManual || s.Class == provider.ClassUntagged {
		return "manual"
	}
	return ""
}

func setToSortedSlice(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}

const (
	taskSortCost      = "cost"
	sessionSortRecent = "recent"
	sessionSortCost   = "cost"
)

// parseFilters extracts the provider list and doug_only flag from query params.
// provider may appear multiple times; doug_only must equal "true" to be active.
func parseFilters(r *http.Request) (providers []string, dougOnly bool) {
	providers = r.URL.Query()["provider"]
	dougOnly = r.URL.Query().Get("doug_only") == "true"
	return
}

func parseTaskSort(r *http.Request) (string, error) {
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		return taskSortCost, nil
	}
	if sortBy != taskSortCost {
		return "", http.ErrNotSupported
	}
	return sortBy, nil
}

func parseSessionSort(r *http.Request) (string, error) {
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		return sessionSortRecent, nil
	}
	switch sortBy {
	case sessionSortRecent, sessionSortCost:
		return sortBy, nil
	default:
		return "", http.ErrNotSupported
	}
}

func compareRecentDesc(a, b time.Time) int {
	aZero := a.IsZero()
	bZero := b.IsZero()
	switch {
	case aZero && bZero:
		return 0
	case aZero:
		return 1
	case bZero:
		return -1
	case a.After(b):
		return -1
	case a.Before(b):
		return 1
	default:
		return 0
	}
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

	type acc struct {
		displayName  string
		aliases      map[string]struct{}
		providers    map[string]struct{}
		tasks        map[string]struct{}
		sessionCount int
		cost         pricing.Cost
	}
	byProject := make(map[string]*acc)
	for _, s := range filtered {
		projectID := canonicalProjectID(s)
		if _, ok := byProject[projectID]; !ok {
			byProject[projectID] = &acc{
				displayName: displayProjectName(s),
				aliases:     make(map[string]struct{}),
				providers:   make(map[string]struct{}),
				tasks:       make(map[string]struct{}),
			}
		}
		project := byProject[projectID]
		if project.displayName == "" {
			project.displayName = displayProjectName(s)
		}
		project.sessionCount++
		project.providers[s.Provider] = struct{}{}
		if alias := rawProjectPath(s); alias != "" && alias != projectID {
			project.aliases[alias] = struct{}{}
		}
		for _, alias := range s.ProjectAliases {
			if alias != "" && alias != projectID {
				project.aliases[alias] = struct{}{}
			}
		}
		if taskID := aggregateTaskID(s); taskID != "" {
			project.tasks[taskID] = struct{}{}
		}
		project.cost = project.cost.Add(h.costs[s.ID])
	}

	items := make([]ProjectItem, 0, len(byProject))
	for projectID, a := range byProject {
		items = append(items, ProjectItem{
			CanonicalProjectID: projectID,
			DisplayName:        a.displayName,
			Aliases:            setToSortedSlice(a.aliases),
			ProviderCoverage:   setToSortedSlice(a.providers),
			SessionCount:       a.sessionCount,
			TaskCount:          len(a.tasks),
			TotalCost:          a.cost.USD,
			UnknownPricing:     a.cost.Unknown,
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
	if _, err := parseTaskSort(r); err != nil {
		writeError(w, http.StatusBadRequest, "invalid sort parameter")
		return
	}
	provs, dougOnly := parseFilters(r)

	// Filter to canonical project first, then apply provider/dougOnly filters.
	var projectSessions []*provider.SessionMeta
	for _, s := range h.sessions {
		if canonicalProjectID(s) == project {
			projectSessions = append(projectSessions, s)
		}
	}
	filtered := filterSessions(projectSessions, provs, dougOnly)

	type acc struct {
		providers    map[string]struct{}
		sessionCount int
		cost         pricing.Cost
	}
	byTask := make(map[string]*acc)

	for _, s := range filtered {
		taskID := aggregateTaskID(s)
		if taskID == "" {
			continue
		}
		if _, ok := byTask[taskID]; !ok {
			byTask[taskID] = &acc{providers: make(map[string]struct{})}
		}
		byTask[taskID].providers[s.Provider] = struct{}{}
		byTask[taskID].sessionCount++
		byTask[taskID].cost = byTask[taskID].cost.Add(h.costs[s.ID])
	}

	items := make([]TaskItem, 0, len(byTask))
	for taskID, a := range byTask {
		items = append(items, TaskItem{
			TaskID:           taskID,
			ProviderCoverage: setToSortedSlice(a.providers),
			SessionCount:     a.sessionCount,
			TotalCost:        a.cost.USD,
			UnknownPricing:   a.cost.Unknown,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TotalCost != items[j].TotalCost {
			return items[i].TotalCost > items[j].TotalCost
		}
		if items[i].SessionCount != items[j].SessionCount {
			return items[i].SessionCount > items[j].SessionCount
		}
		return items[i].TaskID < items[j].TaskID
	})
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleSessions(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task parameter required")
		return
	}
	sortBy, err := parseSessionSort(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sort parameter")
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
			if canonicalProjectID(s) == project {
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
			ID:                 s.ID,
			Provider:           s.Provider,
			CanonicalProjectID: canonicalProjectID(s),
			RawProjectPath:     rawProjectPath(s),
			TaskID:             s.TaskID,
			Model:              s.Model,
			Class:              classString(s.Class),
			StartTime:          s.StartTime,
			Duration:           s.DurationMs,
			TotalCost:          cost.USD,
			UnknownPricing:     cost.Unknown,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		switch sortBy {
		case sessionSortCost:
			if items[i].TotalCost != items[j].TotalCost {
				return items[i].TotalCost > items[j].TotalCost
			}
			if cmp := compareRecentDesc(items[i].StartTime, items[j].StartTime); cmp != 0 {
				return cmp < 0
			}
		default:
			if cmp := compareRecentDesc(items[i].StartTime, items[j].StartTime); cmp != 0 {
				return cmp < 0
			}
			if items[i].TotalCost != items[j].TotalCost {
				return items[i].TotalCost > items[j].TotalCost
			}
		}
		return items[i].ID < items[j].ID
	})
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
