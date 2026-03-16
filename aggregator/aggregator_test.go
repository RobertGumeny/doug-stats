package aggregator

import (
	"fmt"
	"testing"

	"github.com/robertgumeny/doug-stats/provider"
)

// helpers

func session(id, model, project, taskID string, input, output int64) *provider.SessionMeta {
	return &provider.SessionMeta{
		ID:                 id,
		Model:              model,
		ProjectPath:        project,
		CanonicalProjectID: project,
		TaskID:             taskID,
		Class:              provider.ClassDoug,
		Tokens:             provider.TokenCounts{Input: input, Output: output},
	}
}

func sessionWithClass(id, model, project, canonicalProjectID, taskID string, class provider.SessionClass, input, output int64) *provider.SessionMeta {
	s := session(id, model, project, taskID, input, output)
	s.CanonicalProjectID = canonicalProjectID
	s.Class = class
	return s
}

// --- Session-level aggregation ---

func TestAggregate_SessionCount(t *testing.T) {
	sessions := []*provider.SessionMeta{
		session("s1", "claude-sonnet-4-6", "/proj/a", "TASK-1", 0, 0),
		session("s2", "claude-sonnet-4-6", "/proj/a", "TASK-1", 0, 0),
	}
	summary := Aggregate(sessions)
	if len(summary.Sessions) != 2 {
		t.Errorf("got %d sessions, want 2", len(summary.Sessions))
	}
}

func TestAggregate_SessionCost_KnownModel(t *testing.T) {
	// 1M output tokens, claude-sonnet-4-6 = $15/M output → $15.0000
	sessions := []*provider.SessionMeta{
		session("s1", "claude-sonnet-4-6", "/proj/a", "", 0, 1_000_000),
	}
	summary := Aggregate(sessions)
	got := summary.Sessions[0].TotalCost
	if got.Unknown {
		t.Fatal("expected known cost")
	}
	if got.USD != 15.0 {
		t.Errorf("got %v, want 15.0", got.USD)
	}
}

func TestAggregate_SessionCost_UnknownModel(t *testing.T) {
	sessions := []*provider.SessionMeta{
		session("s1", "unknown-model", "/proj/a", "", 1000, 1000),
	}
	summary := Aggregate(sessions)
	if !summary.Sessions[0].TotalCost.Unknown {
		t.Fatal("expected Unknown cost for unrecognized model")
	}
}

// --- Task-level aggregation ---

func TestAggregate_TaskTotal_TwoSessions(t *testing.T) {
	// Two sessions in same task, each with 1M output at $15/M → total $30
	sessions := []*provider.SessionMeta{
		session("s1", "claude-sonnet-4-6", "/proj/a", "TASK-1", 0, 1_000_000),
		session("s2", "claude-sonnet-4-6", "/proj/a", "TASK-1", 0, 1_000_000),
	}
	summary := Aggregate(sessions)
	if len(summary.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(summary.Tasks))
	}
	task := summary.Tasks[0]
	if task.TaskID != "TASK-1" {
		t.Errorf("got task %q, want TASK-1", task.TaskID)
	}
	if task.CanonicalProjectID != "/proj/a" {
		t.Errorf("got canonical project %q, want /proj/a", task.CanonicalProjectID)
	}
	if task.SessionCount != 2 {
		t.Errorf("got session count %d, want 2", task.SessionCount)
	}
	if task.TotalCost.Unknown {
		t.Fatal("expected known task cost")
	}
	if task.TotalCost.USD != 30.0 {
		t.Errorf("got %v, want 30.0", task.TotalCost.USD)
	}
}

func TestAggregate_TaskTotal_MultipleTasks(t *testing.T) {
	sessions := []*provider.SessionMeta{
		session("s1", "claude-sonnet-4-6", "/proj/a", "TASK-1", 0, 1_000_000),
		session("s2", "claude-opus-4-6", "/proj/a", "TASK-2", 1_000_000, 0),
	}
	summary := Aggregate(sessions)
	if len(summary.Tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(summary.Tasks))
	}
}

func TestAggregate_SessionWithNoTaskID_ExcludedFromTasks(t *testing.T) {
	sessions := []*provider.SessionMeta{
		session("s1", "claude-sonnet-4-6", "/proj/a", "", 0, 1_000_000),
	}
	summary := Aggregate(sessions)
	if len(summary.Tasks) != 0 {
		t.Errorf("got %d tasks, want 0 (Doug session with no taskID)", len(summary.Tasks))
	}
}

func TestAggregate_TaskTotal_UnknownPropagates(t *testing.T) {
	// One known session + one unknown-model session in same task → task is Unknown
	sessions := []*provider.SessionMeta{
		session("s1", "claude-sonnet-4-6", "/proj/a", "TASK-1", 0, 1_000_000),
		session("s2", "unknown-model", "/proj/a", "TASK-1", 1000, 0),
	}
	summary := Aggregate(sessions)
	if len(summary.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(summary.Tasks))
	}
	if !summary.Tasks[0].TotalCost.Unknown {
		t.Error("expected Unknown task cost when a session has unknown model")
	}
}

func TestAggregate_TaskTotal_SameTaskIDDifferentProjectsStaySeparate(t *testing.T) {
	sessions := []*provider.SessionMeta{
		sessionWithClass("s1", "claude-sonnet-4-6", "/provider/a", "project-alpha", "TASK-1", provider.ClassDoug, 0, 1_000_000),
		sessionWithClass("s2", "claude-sonnet-4-6", "/provider/b", "project-beta", "TASK-1", provider.ClassDoug, 0, 1_000_000),
	}

	summary := Aggregate(sessions)
	if len(summary.Tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(summary.Tasks))
	}

	got := make(map[string]TaskAggregate, len(summary.Tasks))
	for _, task := range summary.Tasks {
		got[fmt.Sprintf("%s::%s", task.CanonicalProjectID, task.TaskID)] = task
	}

	if _, ok := got["project-alpha::TASK-1"]; !ok {
		t.Fatal("missing task aggregate for project-alpha::TASK-1")
	}
	if _, ok := got["project-beta::TASK-1"]; !ok {
		t.Fatal("missing task aggregate for project-beta::TASK-1")
	}
}

func TestAggregate_TaskTotal_ManualAndUntaggedBucketPerCanonicalProject(t *testing.T) {
	sessions := []*provider.SessionMeta{
		sessionWithClass("s1", "claude-sonnet-4-6", "/provider/a", "project-alpha", "", provider.ClassManual, 0, 1_000_000),
		sessionWithClass("s2", "claude-sonnet-4-6", "/provider/b", "project-alpha", "", provider.ClassUntagged, 0, 1_000_000),
		sessionWithClass("s3", "claude-sonnet-4-6", "/provider/c", "project-beta", "", provider.ClassManual, 0, 1_000_000),
	}

	summary := Aggregate(sessions)
	if len(summary.Tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(summary.Tasks))
	}

	got := make(map[string]TaskAggregate, len(summary.Tasks))
	for _, task := range summary.Tasks {
		got[fmt.Sprintf("%s::%s", task.CanonicalProjectID, task.TaskID)] = task
	}

	alpha := got["project-alpha::manual"]
	if alpha.TaskID != "manual" {
		t.Fatalf("project-alpha manual bucket missing: %+v", alpha)
	}
	if alpha.SessionCount != 2 {
		t.Fatalf("project-alpha manual session count = %d, want 2", alpha.SessionCount)
	}
	if alpha.TotalCost.USD != 30.0 {
		t.Fatalf("project-alpha manual cost = %v, want 30.0", alpha.TotalCost.USD)
	}

	beta := got["project-beta::manual"]
	if beta.TaskID != "manual" {
		t.Fatalf("project-beta manual bucket missing: %+v", beta)
	}
	if beta.SessionCount != 1 {
		t.Fatalf("project-beta manual session count = %d, want 1", beta.SessionCount)
	}
}

// --- Project-level aggregation ---

func TestAggregate_ProjectTotal(t *testing.T) {
	// Two sessions in same project: $15 + $15 = $30
	sessions := []*provider.SessionMeta{
		session("s1", "claude-sonnet-4-6", "/proj/a", "", 0, 1_000_000),
		session("s2", "claude-sonnet-4-6", "/proj/a", "", 0, 1_000_000),
	}
	summary := Aggregate(sessions)
	if len(summary.Projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(summary.Projects))
	}
	proj := summary.Projects[0]
	if proj.CanonicalProjectID != "/proj/a" {
		t.Errorf("got canonical project %q, want /proj/a", proj.CanonicalProjectID)
	}
	if proj.SessionCount != 2 {
		t.Errorf("got session count %d, want 2", proj.SessionCount)
	}
	if proj.TotalCost.USD != 30.0 {
		t.Errorf("got %v, want 30.0", proj.TotalCost.USD)
	}
}

func TestAggregate_MultipleProjects(t *testing.T) {
	sessions := []*provider.SessionMeta{
		session("s1", "claude-sonnet-4-6", "/proj/a", "", 0, 1_000_000),
		session("s2", "claude-sonnet-4-6", "/proj/b", "", 0, 1_000_000),
	}
	summary := Aggregate(sessions)
	if len(summary.Projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(summary.Projects))
	}
}

func TestAggregate_ProjectTotal_CrossProviderSessionsMergeByCanonicalProjectID(t *testing.T) {
	sessions := []*provider.SessionMeta{
		sessionWithClass("s1", "claude-sonnet-4-6", "/claude/repo", "project-alpha", "TASK-1", provider.ClassDoug, 0, 1_000_000),
		sessionWithClass("s2", "gpt-5-codex", "/codex/repo", "project-alpha", "TASK-2", provider.ClassDoug, 0, 1_000_000),
	}

	summary := Aggregate(sessions)
	if len(summary.Projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(summary.Projects))
	}

	project := summary.Projects[0]
	if project.CanonicalProjectID != "project-alpha" {
		t.Fatalf("canonical project = %q, want project-alpha", project.CanonicalProjectID)
	}
	if project.SessionCount != 2 {
		t.Fatalf("session count = %d, want 2", project.SessionCount)
	}
	if project.TotalCost.Unknown {
		t.Fatal("expected known project cost")
	}
	if project.TotalCost.USD != 25.0 {
		t.Fatalf("project cost = %v, want 25.0", project.TotalCost.USD)
	}
}

// --- CacheTierMinutes metadata ---

func TestAggregate_CacheTierMinutes(t *testing.T) {
	summary := Aggregate(nil)
	if summary.CacheTierMinutes != 5 {
		t.Errorf("CacheTierMinutes = %d, want 5", summary.CacheTierMinutes)
	}
}

// --- Empty input ---

func TestAggregate_Empty(t *testing.T) {
	summary := Aggregate(nil)
	if len(summary.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(summary.Sessions))
	}
	if len(summary.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(summary.Tasks))
	}
	if len(summary.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(summary.Projects))
	}
}

// --- Message-level via ComputeMessageCosts ---

func TestComputeMessageCosts_AssistantOnly(t *testing.T) {
	transcript := &provider.Transcript{
		SessionID: "s1",
		Messages: []provider.Message{
			{UUID: "m1", Role: "user"},
			{UUID: "m2", Role: "assistant", Model: "claude-sonnet-4-6", Tokens: &provider.TokenCounts{Output: 1_000_000}},
		},
	}
	costs := ComputeMessageCosts(transcript)
	if len(costs) != 1 {
		t.Fatalf("got %d message costs, want 1", len(costs))
	}
	if costs[0].UUID != "m2" {
		t.Errorf("got UUID %q, want m2", costs[0].UUID)
	}
	if costs[0].Cost.Unknown {
		t.Fatal("expected known cost")
	}
	if costs[0].Cost.USD != 15.0 {
		t.Errorf("got %v, want 15.0", costs[0].Cost.USD)
	}
}

func TestComputeMessageCosts_NoTokens_Skipped(t *testing.T) {
	transcript := &provider.Transcript{
		SessionID: "s1",
		Messages: []provider.Message{
			{UUID: "m1", Role: "assistant", Model: "claude-sonnet-4-6", Tokens: nil},
		},
	}
	costs := ComputeMessageCosts(transcript)
	if len(costs) != 0 {
		t.Errorf("got %d costs, want 0 (no token data)", len(costs))
	}
}

func TestComputeMessageCosts_UnknownModel(t *testing.T) {
	transcript := &provider.Transcript{
		SessionID: "s1",
		Messages: []provider.Message{
			{UUID: "m1", Role: "assistant", Model: "gpt-99", Tokens: &provider.TokenCounts{Output: 1000}},
		},
	}
	costs := ComputeMessageCosts(transcript)
	if len(costs) != 1 {
		t.Fatalf("got %d costs, want 1", len(costs))
	}
	if !costs[0].Cost.Unknown {
		t.Error("expected Unknown cost for unrecognized model")
	}
}
