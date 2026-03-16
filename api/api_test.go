package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/robertgumeny/doug-stats/aggregator"
	"github.com/robertgumeny/doug-stats/provider"
)

// --- mock provider ---

type mockProvider struct {
	name       string
	transcript *provider.Transcript
	err        error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) LoadSessions() ([]*provider.SessionMeta, error) {
	return nil, nil
}
func (m *mockProvider) LoadTranscript(id string) (*provider.Transcript, error) {
	return m.transcript, m.err
}

// --- test helpers ---

func newSession(id, provName, project, taskID string, class provider.SessionClass) *provider.SessionMeta {
	return &provider.SessionMeta{
		ID:                 id,
		Provider:           provName,
		ProjectPath:        project,
		RawProjectPath:     project,
		CanonicalProjectID: project,
		DisplayProjectName: project,
		TaskID:             taskID,
		Model:              "claude-sonnet-4-6",
		Class:              class,
		StartTime:          time.Time{},
		Tokens:             provider.TokenCounts{Output: 1_000_000}, // $15 with sonnet-4-6
	}
}

// buildHandler creates a Handler with the given sessions and a summary computed
// from those sessions. All sessions use "claude-sonnet-4-6" (1M output = $15).
func buildHandler(sessions []*provider.SessionMeta, prov provider.Provider) *Handler {
	summary := aggregator.Aggregate(sessions)
	providers := map[string]provider.Provider{}
	if prov != nil {
		providers[prov.Name()] = prov
	}
	return New(sessions, summary, providers)
}

func buildHandlerWithProviders(sessions []*provider.SessionMeta, provs ...provider.Provider) *Handler {
	summary := aggregator.Aggregate(sessions)
	providers := map[string]provider.Provider{}
	for _, prov := range provs {
		if prov == nil {
			continue
		}
		providers[prov.Name()] = prov
	}
	return New(sessions, summary, providers)
}

func doGet(h *Handler, path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// --- filterSessions ---

func TestFilterSessions_NoFilters(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj", "T1", provider.ClassDoug),
		newSession("s2", "gemini", "/proj", "", provider.ClassManual),
	}
	got := filterSessions(sessions, nil, false)
	if len(got) != 2 {
		t.Errorf("want 2, got %d", len(got))
	}
}

func TestFilterSessions_ProviderFilter(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj", "T1", provider.ClassDoug),
		newSession("s2", "gemini", "/proj", "", provider.ClassManual),
	}
	got := filterSessions(sessions, []string{"claude"}, false)
	if len(got) != 1 || got[0].ID != "s1" {
		t.Errorf("want [s1], got %v", got)
	}
}

func TestFilterSessions_MultipleProviders(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj", "T1", provider.ClassDoug),
		newSession("s2", "gemini", "/proj", "", provider.ClassManual),
		newSession("s3", "codex", "/proj", "", provider.ClassUntagged),
	}
	got := filterSessions(sessions, []string{"claude", "gemini"}, false)
	if len(got) != 2 {
		t.Errorf("want 2, got %d", len(got))
	}
}

func TestFilterSessions_DougOnly(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj", "", provider.ClassManual),
		newSession("s3", "claude", "/proj", "", provider.ClassUntagged),
	}
	got := filterSessions(sessions, nil, true)
	if len(got) != 1 || got[0].ID != "s1" {
		t.Errorf("want [s1], got %v", got)
	}
}

func TestFilterSessions_ProviderAndDougOnly(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj", "", provider.ClassManual),
		newSession("s3", "gemini", "/proj", "T2", provider.ClassDoug),
	}
	got := filterSessions(sessions, []string{"claude"}, true)
	if len(got) != 1 || got[0].ID != "s1" {
		t.Errorf("want [s1], got %v", got)
	}
}

// --- /api/health ---

func TestHealth(t *testing.T) {
	h := buildHandler(nil, nil)
	w := doGet(h, "/api/health")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("want status=ok, got %q", resp["status"])
	}
}

// --- /api/projects ---

func TestProjects_AllSessions(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "", provider.ClassManual),
		newSession("s3", "claude", "/proj/b", "T2", provider.ClassDoug),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/projects")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var items []ProjectItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 projects, got %d", len(items))
	}
}

func TestProjects_AggregateIncludesAllTypes(t *testing.T) {
	// /proj/a has a Doug session and a Manual session (both $15 each)
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "", provider.ClassManual),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/projects")
	var items []ProjectItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 {
		t.Fatalf("want 1 project, got %d", len(items))
	}
	if items[0].TotalCost != 30.0 {
		t.Errorf("want 30.0 (both session types), got %v", items[0].TotalCost)
	}
}

func TestProjects_DougOnlyExcludesManual(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "", provider.ClassManual),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/projects?doug_only=true")
	var items []ProjectItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 {
		t.Fatalf("want 1 project, got %d", len(items))
	}
	if items[0].TotalCost != 15.0 {
		t.Errorf("want 15.0 (only doug session), got %v", items[0].TotalCost)
	}
}

func TestProjects_ProviderFilter(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "gemini", "/proj/b", "T2", provider.ClassDoug),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/projects?provider=claude")
	var items []ProjectItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 || items[0].CanonicalProjectID != "/proj/a" {
		t.Errorf("want only /proj/a, got %v", items)
	}
}

func TestProjects_ResponseShapeAndUnknownPricing(t *testing.T) {
	sessions := []*provider.SessionMeta{
		{
			ID:                 "s1",
			Provider:           "claude",
			ProjectPath:        "/claude/proj/a",
			RawProjectPath:     "/claude/proj/a",
			CanonicalProjectID: "project-alpha",
			DisplayProjectName: "Project Alpha",
			ProjectAliases:     []string{"alpha-repo"},
			TaskID:             "T1",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Output: 1_000_000},
		},
		{
			ID:                 "s2",
			Provider:           "gemini",
			ProjectPath:        "/gemini/proj/a",
			RawProjectPath:     "/gemini/proj/a",
			CanonicalProjectID: "project-alpha",
			DisplayProjectName: "Project Alpha",
			Class:              provider.ClassManual,
			Model:              "unknown-model",
			Tokens:             provider.TokenCounts{Input: 1},
		},
	}

	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/projects")
	var items []ProjectItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 project, got %d", len(items))
	}

	item := items[0]
	if item.CanonicalProjectID != "project-alpha" {
		t.Fatalf("canonicalProjectID = %q, want project-alpha", item.CanonicalProjectID)
	}
	if item.DisplayName != "Project Alpha" {
		t.Fatalf("displayName = %q, want Project Alpha", item.DisplayName)
	}
	if len(item.Aliases) != 3 {
		t.Fatalf("aliases = %v, want 3 aliases", item.Aliases)
	}
	if len(item.ProviderCoverage) != 2 {
		t.Fatalf("providerCoverage = %v, want 2 providers", item.ProviderCoverage)
	}
	if item.SessionCount != 2 {
		t.Fatalf("sessionCount = %d, want 2", item.SessionCount)
	}
	if item.TaskCount != 2 {
		t.Fatalf("taskCount = %d, want 2", item.TaskCount)
	}
	if !item.UnknownPricing {
		t.Fatal("unknownPricing = false, want true")
	}
}

// --- /api/tasks ---

func TestTasks_MissingProject(t *testing.T) {
	h := buildHandler(nil, nil)
	w := doGet(h, "/api/tasks")
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestTasks_TasksForProject(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "T2", provider.ClassDoug),
		newSession("s3", "claude", "/proj/b", "T3", provider.ClassDoug),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/tasks?project=/proj/a")
	var items []TaskItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(items))
	}
}

func TestTasks_VirtualManualEntry(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "", provider.ClassManual),
		newSession("s3", "claude", "/proj/a", "", provider.ClassUntagged),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/tasks?project=/proj/a")
	var items []TaskItem
	json.Unmarshal(w.Body.Bytes(), &items)
	// Expect T1 + virtual "manual"
	if len(items) != 2 {
		t.Fatalf("want 2 tasks (T1 + manual), got %d", len(items))
	}
	var hasManual bool
	for _, item := range items {
		if item.TaskID == "manual" {
			hasManual = true
			// Manual ($15) + Untagged ($15) = $30
			if item.TotalCost != 30.0 {
				t.Errorf("manual cost: want 30.0, got %v", item.TotalCost)
			}
		}
	}
	if !hasManual {
		t.Error("want virtual manual task in response")
	}
}

func TestTasks_DougOnlyNoManualEntry(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "", provider.ClassManual),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/tasks?project=/proj/a&doug_only=true")
	var items []TaskItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 {
		t.Fatalf("want 1 task (T1 only), got %d: %v", len(items), items)
	}
	if items[0].TaskID == "manual" {
		t.Error("manual task should not appear with doug_only=true")
	}
}

func TestTasks_AggregateCorrect(t *testing.T) {
	// Two sessions in T1, each $15 → T1 total $30
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "T1", provider.ClassDoug),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/tasks?project=/proj/a")
	var items []TaskItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 {
		t.Fatalf("want 1 task, got %d", len(items))
	}
	if items[0].TotalCost != 30.0 {
		t.Errorf("want 30.0, got %v", items[0].TotalCost)
	}
}

func TestTasks_SortedByCostDescending(t *testing.T) {
	sessions := []*provider.SessionMeta{
		{
			ID:                 "s1",
			Provider:           "claude",
			ProjectPath:        "/proj/a",
			RawProjectPath:     "/proj/a",
			CanonicalProjectID: "/proj/a",
			TaskID:             "T-expensive",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Output: 1_000_000}, // $15.0
		},
		{
			ID:                 "s2",
			Provider:           "gemini",
			ProjectPath:        "/proj/a",
			RawProjectPath:     "/proj/a",
			CanonicalProjectID: "/proj/a",
			TaskID:             "T-cheap",
			Model:              "gemini-2.5-flash",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Input: 1_000_000}, // $0.3
		},
	}
	h := buildHandler(sessions, nil)

	w := doGet(h, "/api/tasks?project=/proj/a&sort=cost")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	var items []TaskItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(items))
	}
	if items[0].TaskID != "T-expensive" || items[1].TaskID != "T-cheap" {
		t.Fatalf("got order %q, %q; want T-expensive, T-cheap", items[0].TaskID, items[1].TaskID)
	}
}

func TestTasks_InvalidSort(t *testing.T) {
	h := buildHandler(nil, nil)
	w := doGet(h, "/api/tasks?project=/proj/a&sort=recent")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTasks_CanonicalProjectScopeAndShape(t *testing.T) {
	sessions := []*provider.SessionMeta{
		{
			ID:                 "s1",
			Provider:           "claude",
			ProjectPath:        "/claude/proj/a",
			RawProjectPath:     "/claude/proj/a",
			CanonicalProjectID: "project-alpha",
			TaskID:             "TASK-1",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Output: 1_000_000},
		},
		{
			ID:                 "s2",
			Provider:           "gemini",
			ProjectPath:        "/gemini/proj/a",
			RawProjectPath:     "/gemini/proj/a",
			CanonicalProjectID: "project-alpha",
			TaskID:             "TASK-1",
			Model:              "gemini-2.5-flash",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Input: 1_000_000},
		},
		{
			ID:                 "s3",
			Provider:           "claude",
			ProjectPath:        "/claude/proj/b",
			RawProjectPath:     "/claude/proj/b",
			CanonicalProjectID: "project-beta",
			TaskID:             "TASK-1",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Output: 1_000_000},
		},
	}

	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/tasks?project=project-alpha")
	var items []TaskItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 task, got %d", len(items))
	}
	if items[0].TaskID != "TASK-1" {
		t.Fatalf("taskID = %q, want TASK-1", items[0].TaskID)
	}
	if len(items[0].ProviderCoverage) != 2 {
		t.Fatalf("providerCoverage = %v, want 2 providers", items[0].ProviderCoverage)
	}
	if items[0].SessionCount != 2 {
		t.Fatalf("sessionCount = %d, want 2", items[0].SessionCount)
	}
	if items[0].UnknownPricing {
		t.Fatal("unknownPricing = true, want false")
	}
}

// --- /api/sessions ---

func TestSessions_MissingTask(t *testing.T) {
	h := buildHandler(nil, nil)
	w := doGet(h, "/api/sessions")
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestSessions_ByTaskID(t *testing.T) {
	durationMs := int64(2000)
	sessions := []*provider.SessionMeta{
		{
			ID:                 "s1",
			Provider:           "claude",
			ProjectPath:        "/proj/a",
			RawProjectPath:     "/proj/a",
			CanonicalProjectID: "project-alpha",
			TaskID:             "T1",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			StartTime:          time.Time{},
			DurationMs:         &durationMs,
			Tokens:             provider.TokenCounts{Output: 1_000_000},
		},
		newSession("s2", "claude", "/proj/a", "T2", provider.ClassDoug),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/sessions?task=T1")
	var items []SessionItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 || items[0].ID != "s1" {
		t.Errorf("want [s1], got %v", items)
	}
	if items[0].Duration == nil || *items[0].Duration != 2000 {
		t.Fatalf("duration = %v, want 2000", items[0].Duration)
	}
	if items[0].CanonicalProjectID != "project-alpha" {
		t.Fatalf("canonicalProjectID = %q, want project-alpha", items[0].CanonicalProjectID)
	}
	if items[0].RawProjectPath != "/proj/a" {
		t.Fatalf("rawProjectPath = %q, want /proj/a", items[0].RawProjectPath)
	}
}

func TestSessions_ManualVirtualTask(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "", provider.ClassManual),
		newSession("s3", "claude", "/proj/a", "", provider.ClassUntagged),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/sessions?task=manual")
	var items []SessionItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 2 {
		t.Fatalf("want 2 manual sessions, got %d", len(items))
	}
}

func TestSessions_ManualWithProjectScope(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "", provider.ClassManual),
		newSession("s2", "claude", "/proj/b", "", provider.ClassManual),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/sessions?task=manual&project=/proj/a")
	var items []SessionItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 || items[0].ID != "s1" {
		t.Errorf("want [s1], got %v", items)
	}
}

func TestSessions_DougOnlyExcludesManual(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "", provider.ClassManual),
		newSession("s2", "claude", "/proj/a", "", provider.ClassUntagged),
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/sessions?task=manual&doug_only=true")
	var items []SessionItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 0 {
		t.Errorf("want 0 sessions with doug_only=true, got %d", len(items))
	}
}

func TestSessions_DefaultSortsByRecencyDescending(t *testing.T) {
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)
	sessions := []*provider.SessionMeta{
		{
			ID:                 "s-old",
			Provider:           "claude",
			ProjectPath:        "/proj/a",
			RawProjectPath:     "/proj/a",
			CanonicalProjectID: "/proj/a",
			TaskID:             "T1",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			StartTime:          now.Add(-2 * time.Hour),
			Tokens:             provider.TokenCounts{Output: 1_000_000},
		},
		{
			ID:                 "s-new",
			Provider:           "claude",
			ProjectPath:        "/proj/a",
			RawProjectPath:     "/proj/a",
			CanonicalProjectID: "/proj/a",
			TaskID:             "T1",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			StartTime:          now,
			Tokens:             provider.TokenCounts{Output: 1_000_000},
		},
	}
	h := buildHandler(sessions, nil)

	w := doGet(h, "/api/sessions?task=T1")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	var items []SessionItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(items))
	}
	if items[0].ID != "s-new" || items[1].ID != "s-old" {
		t.Fatalf("got order %q, %q; want s-new, s-old", items[0].ID, items[1].ID)
	}
}

func TestSessions_SortByCostDescending(t *testing.T) {
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)
	sessions := []*provider.SessionMeta{
		{
			ID:                 "s-cheap",
			Provider:           "gemini",
			ProjectPath:        "/proj/a",
			RawProjectPath:     "/proj/a",
			CanonicalProjectID: "/proj/a",
			TaskID:             "T1",
			Model:              "gemini-2.5-flash",
			Class:              provider.ClassDoug,
			StartTime:          now,
			Tokens:             provider.TokenCounts{Input: 1_000_000}, // $0.3
		},
		{
			ID:                 "s-expensive",
			Provider:           "claude",
			ProjectPath:        "/proj/a",
			RawProjectPath:     "/proj/a",
			CanonicalProjectID: "/proj/a",
			TaskID:             "T1",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			StartTime:          now.Add(-time.Hour),
			Tokens:             provider.TokenCounts{Output: 1_000_000}, // $15.0
		},
	}
	h := buildHandler(sessions, nil)

	w := doGet(h, "/api/sessions?task=T1&sort=cost")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	var items []SessionItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(items))
	}
	if items[0].ID != "s-expensive" || items[1].ID != "s-cheap" {
		t.Fatalf("got order %q, %q; want s-expensive, s-cheap", items[0].ID, items[1].ID)
	}
}

func TestSessions_InvalidSort(t *testing.T) {
	h := buildHandler(nil, nil)
	w := doGet(h, "/api/sessions?task=T1&sort=project")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestSessions_ClassField(t *testing.T) {
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
		newSession("s2", "claude", "/proj/a", "", provider.ClassManual),
		newSession("s3", "claude", "/proj/a", "", provider.ClassUntagged),
	}
	h := buildHandler(sessions, nil)

	check := func(taskParam, wantClass string) {
		t.Helper()
		// For task=manual we get both manual+untagged; use project to narrow.
		var items []SessionItem
		if taskParam == "T1" {
			w := doGet(h, "/api/sessions?task=T1")
			json.Unmarshal(w.Body.Bytes(), &items)
		} else {
			w := doGet(h, "/api/sessions?task=manual")
			json.Unmarshal(w.Body.Bytes(), &items)
		}
		for _, item := range items {
			if item.Class == wantClass {
				return
			}
		}
		t.Errorf("class %q not found in response for task=%s", wantClass, taskParam)
	}
	check("T1", "doug")
	check("manual", "manual")
	check("manual", "untagged")
}

func TestAPIShape_RequiredFieldsPresent(t *testing.T) {
	durationMs := int64(2000)
	sessions := []*provider.SessionMeta{
		{
			ID:                 "s1",
			Provider:           "claude",
			ProjectPath:        "/claude/proj/a",
			RawProjectPath:     "/claude/proj/a",
			CanonicalProjectID: "project-alpha",
			DisplayProjectName: "Project Alpha",
			ProjectAliases:     []string{"alpha-repo"},
			TaskID:             "TASK-1",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			DurationMs:         &durationMs,
			Tokens:             provider.TokenCounts{Output: 1_000_000},
		},
	}
	h := buildHandler(sessions, nil)

	checkKeys := func(path string, wantKeys ...string) {
		t.Helper()
		w := doGet(h, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", path, w.Code)
		}
		var items []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
			t.Fatalf("%s: unmarshal: %v", path, err)
		}
		if len(items) != 1 {
			t.Fatalf("%s: want 1 item, got %d", path, len(items))
		}
		for _, key := range wantKeys {
			if _, ok := items[0][key]; !ok {
				t.Fatalf("%s: missing key %q in %v", path, key, items[0])
			}
		}
	}

	checkKeys("/api/projects",
		"canonicalProjectID", "displayName", "aliases", "providerCoverage",
		"sessionCount", "taskCount", "totalCost", "unknownPricing",
	)
	checkKeys("/api/tasks?project=project-alpha",
		"taskID", "providerCoverage", "sessionCount", "totalCost", "unknownPricing",
	)
	checkKeys("/api/sessions?task=TASK-1&project=project-alpha",
		"id", "provider", "canonicalProjectID", "rawProjectPath", "taskID",
		"model", "class", "startTime", "duration", "totalCost", "unknownPricing",
	)
}

// --- /api/sessions/:id/messages ---

func TestMessages_NotFound(t *testing.T) {
	h := buildHandler(nil, nil)
	w := doGet(h, "/api/sessions/nosuchid/messages")
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestMessages_BadPath(t *testing.T) {
	h := buildHandler(nil, nil)
	// Path is under /api/sessions/ but doesn't end with /messages
	w := doGet(h, "/api/sessions/abc/notmessages")
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestMessages_LoadsTranscript(t *testing.T) {
	tok := provider.TokenCounts{Output: 1_000_000} // $15
	transcript := &provider.Transcript{
		SessionID: "s1",
		Messages: []provider.Message{
			{UUID: "m1", Role: "user", Content: []provider.ContentPart{{Type: "text", Raw: json.RawMessage(`"hello"`)}}},
			{UUID: "m2", Role: "assistant", Model: "claude-sonnet-4-6", Tokens: &tok,
				Content: []provider.ContentPart{{Type: "text", Raw: json.RawMessage(`"world"`)}}},
		},
	}
	mock := &mockProvider{name: "claude", transcript: transcript}

	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
	}
	h := buildHandler(sessions, mock)
	w := doGet(h, "/api/sessions/s1/messages")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var items []MessageItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 messages, got %d", len(items))
	}
	if items[0].UUID != "m1" || items[0].Role != "user" {
		t.Errorf("message 0: want m1/user, got %v/%v", items[0].UUID, items[0].Role)
	}
	if items[1].UUID != "m2" || items[1].Role != "assistant" {
		t.Errorf("message 1: want m2/assistant, got %v/%v", items[1].UUID, items[1].Role)
	}
	if items[1].CostUSD != 15.0 {
		t.Errorf("message 1 cost: want 15.0, got %v", items[1].CostUSD)
	}
	if items[0].CostUSD != 0 {
		t.Errorf("user message cost: want 0, got %v", items[0].CostUSD)
	}
}

func TestMessages_ContentPartsPreserved(t *testing.T) {
	transcript := &provider.Transcript{
		SessionID: "s1",
		Messages: []provider.Message{
			{UUID: "m1", Role: "user", Content: []provider.ContentPart{
				{Type: "text", Raw: json.RawMessage(`"hello"`)},
				{Type: "tool_result", Raw: json.RawMessage(`{"type":"tool_result","content":"ok"}`)},
			}},
		},
	}
	mock := &mockProvider{name: "claude", transcript: transcript}
	sessions := []*provider.SessionMeta{
		newSession("s1", "claude", "/proj/a", "T1", provider.ClassDoug),
	}
	h := buildHandler(sessions, mock)
	w := doGet(h, "/api/sessions/s1/messages")
	var items []MessageItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items[0].Content) != 2 {
		t.Errorf("want 2 content parts, got %d", len(items[0].Content))
	}
	if items[0].Content[0].Type != "text" {
		t.Errorf("want type=text, got %q", items[0].Content[0].Type)
	}
}

// --- error envelope ---

func TestErrorEnvelope(t *testing.T) {
	h := buildHandler(nil, nil)
	w := doGet(h, "/api/tasks") // missing project param
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Error("want error key in error response")
	}
}

func TestE2E_MultiProvider_ProjectAggregateIncludesAllProviders(t *testing.T) {
	sessions := []*provider.SessionMeta{
		{
			ID:                 "claude-s1",
			Provider:           "claude",
			ProjectPath:        "/claude/proj/all",
			RawProjectPath:     "/claude/proj/all",
			CanonicalProjectID: "project-all",
			DisplayProjectName: "Project All",
			TaskID:             "T-CLAUDE",
			Model:              "claude-sonnet-4-6",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Output: 1_000_000}, // $15.0
		},
		{
			ID:                 "gemini-s1",
			Provider:           "gemini",
			ProjectPath:        "/gemini/proj/all",
			RawProjectPath:     "/gemini/proj/all",
			CanonicalProjectID: "project-all",
			DisplayProjectName: "Project All",
			TaskID:             "T-GEMINI",
			Model:              "gemini-2.5-flash",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Input: 1_000_000}, // $0.3
		},
		{
			ID:                 "codex-s1",
			Provider:           "codex",
			ProjectPath:        "/codex/proj/all",
			RawProjectPath:     "/codex/proj/all",
			CanonicalProjectID: "project-all",
			DisplayProjectName: "Project All",
			TaskID:             "T-CODEX",
			Model:              "gpt-5-codex",
			Class:              provider.ClassDoug,
			Tokens:             provider.TokenCounts{Output: 1_000_000}, // $10.0
		},
	}
	h := buildHandler(sessions, nil)
	w := doGet(h, "/api/projects")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	var items []ProjectItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 project, got %d", len(items))
	}
	if items[0].CanonicalProjectID != "project-all" {
		t.Fatalf("canonicalProjectID = %q, want project-all", items[0].CanonicalProjectID)
	}
	if items[0].UnknownPricing {
		t.Fatal("expected known aggregate cost")
	}
	if items[0].TotalCost != 25.3 {
		t.Fatalf("aggregate cost = %v, want 25.3", items[0].TotalCost)
	}
}

func TestE2E_MultiProvider_ProviderFilterCombinations(t *testing.T) {
	sessions := []*provider.SessionMeta{
		{
			ID:          "claude-s1",
			Provider:    "claude",
			ProjectPath: "/proj/all",
			TaskID:      "T-CLAUDE",
			Model:       "claude-sonnet-4-6",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Output: 1_000_000}, // $15.0
		},
		{
			ID:          "gemini-s1",
			Provider:    "gemini",
			ProjectPath: "/proj/all",
			TaskID:      "T-GEMINI",
			Model:       "gemini-2.5-flash",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Input: 1_000_000}, // $0.3
		},
		{
			ID:          "codex-s1",
			Provider:    "codex",
			ProjectPath: "/proj/all",
			TaskID:      "T-CODEX",
			Model:       "gpt-5-codex",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Output: 1_000_000}, // $10.0
		},
	}
	h := buildHandler(sessions, nil)

	assertSingleProjectCost := func(path string, want float64) {
		t.Helper()
		w := doGet(h, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", path, w.Code)
		}
		var items []ProjectItem
		if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
			t.Fatalf("%s: unmarshal: %v", path, err)
		}
		if len(items) != 1 {
			t.Fatalf("%s: want 1 project, got %d", path, len(items))
		}
		if items[0].TotalCost != want {
			t.Fatalf("%s: cost = %v, want %v", path, items[0].TotalCost, want)
		}
	}

	assertSingleProjectCost("/api/projects?provider=claude", 15.0)
	assertSingleProjectCost("/api/projects?provider=gemini", 0.3)
	assertSingleProjectCost("/api/projects?provider=codex", 10.0)
	assertSingleProjectCost("/api/projects?provider=claude&provider=codex", 25.0)
}

func TestE2E_MultiProvider_PerSessionCostRepresentativePerProvider(t *testing.T) {
	sessions := []*provider.SessionMeta{
		{
			ID:          "claude-s1",
			Provider:    "claude",
			ProjectPath: "/proj/all",
			TaskID:      "T-CLAUDE",
			Model:       "claude-sonnet-4-6",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Output: 1_000_000}, // $15.0
		},
		{
			ID:          "gemini-s1",
			Provider:    "gemini",
			ProjectPath: "/proj/all",
			TaskID:      "T-GEMINI",
			Model:       "gemini-2.5-flash",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Input: 1_000_000}, // $0.3
		},
		{
			ID:          "codex-s1",
			Provider:    "codex",
			ProjectPath: "/proj/all",
			TaskID:      "T-CODEX",
			Model:       "gpt-5-codex",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Output: 1_000_000}, // $10.0
		},
	}
	h := buildHandler(sessions, nil)

	checkSessionCost := func(taskID string, want float64, wantProvider string) {
		t.Helper()
		w := doGet(h, "/api/sessions?task="+taskID)
		if w.Code != http.StatusOK {
			t.Fatalf("task=%s: want 200, got %d", taskID, w.Code)
		}
		var items []SessionItem
		if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
			t.Fatalf("task=%s: unmarshal: %v", taskID, err)
		}
		if len(items) != 1 {
			t.Fatalf("task=%s: want 1 session, got %d", taskID, len(items))
		}
		if items[0].Provider != wantProvider {
			t.Fatalf("task=%s: provider=%q, want %q", taskID, items[0].Provider, wantProvider)
		}
		if items[0].TotalCost != want {
			t.Fatalf("task=%s: cost=%v, want %v", taskID, items[0].TotalCost, want)
		}
	}

	checkSessionCost("T-CLAUDE", 15.0, "claude")
	checkSessionCost("T-GEMINI", 0.3, "gemini")
	checkSessionCost("T-CODEX", 10.0, "codex")
}

func TestE2E_MultiProvider_TranscriptViewWorksForEachProvider(t *testing.T) {
	claudeTranscript := &provider.Transcript{
		SessionID: "claude-s1",
		Messages: []provider.Message{
			{UUID: "claude-u1", Role: "user", Content: []provider.ContentPart{{Type: "text", Raw: json.RawMessage(`"hello"`)}}, Timestamp: time.Now()},
		},
	}
	geminiTranscript := &provider.Transcript{
		SessionID: "gemini-s1",
		Messages: []provider.Message{
			{UUID: "gemini-a1", Role: "assistant", Model: "gemini-2.5-flash", Tokens: &provider.TokenCounts{Input: 1, Output: 1}, Content: []provider.ContentPart{{Type: "text", Raw: json.RawMessage(`"ok"`)}}, Timestamp: time.Now()},
		},
	}
	codexTranscript := &provider.Transcript{
		SessionID: "codex-s1",
		Messages: []provider.Message{
			{UUID: "codex-a1", Role: "assistant", Model: "gpt-5-codex", Tokens: &provider.TokenCounts{Input: 1, Output: 1}, Content: []provider.ContentPart{{Type: "text", Raw: json.RawMessage(`"done"`)}}, Timestamp: time.Now()},
		},
	}

	claudeProv := &mockProvider{name: "claude", transcript: claudeTranscript}
	geminiProv := &mockProvider{name: "gemini", transcript: geminiTranscript}
	codexProv := &mockProvider{name: "codex", transcript: codexTranscript}

	sessions := []*provider.SessionMeta{
		{
			ID:          "claude-s1",
			Provider:    "claude",
			ProjectPath: "/proj/all",
			TaskID:      "T-CLAUDE",
			Model:       "claude-sonnet-4-6",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Output: 1_000_000},
		},
		{
			ID:          "gemini-s1",
			Provider:    "gemini",
			ProjectPath: "/proj/all",
			TaskID:      "T-GEMINI",
			Model:       "gemini-2.5-flash",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Input: 1_000_000},
		},
		{
			ID:          "codex-s1",
			Provider:    "codex",
			ProjectPath: "/proj/all",
			TaskID:      "T-CODEX",
			Model:       "gpt-5-codex",
			Class:       provider.ClassDoug,
			Tokens:      provider.TokenCounts{Output: 1_000_000},
		},
	}
	h := buildHandlerWithProviders(sessions, claudeProv, geminiProv, codexProv)

	checkTranscript := func(sessionID, wantUUID string) {
		t.Helper()
		w := doGet(h, "/api/sessions/"+sessionID+"/messages")
		if w.Code != http.StatusOK {
			t.Fatalf("session=%s: want 200, got %d", sessionID, w.Code)
		}
		var items []MessageItem
		if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
			t.Fatalf("session=%s: unmarshal: %v", sessionID, err)
		}
		if len(items) != 1 {
			t.Fatalf("session=%s: want 1 message, got %d", sessionID, len(items))
		}
		if items[0].UUID != wantUUID {
			t.Fatalf("session=%s: uuid=%q, want %q", sessionID, items[0].UUID, wantUUID)
		}
	}

	checkTranscript("claude-s1", "claude-u1")
	checkTranscript("gemini-s1", "gemini-a1")
	checkTranscript("codex-s1", "codex-a1")
}
