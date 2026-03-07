// Package aggregator computes cost totals at message, session, task, and
// project levels from provider session and transcript data.
package aggregator

import (
	"github.com/robertgumeny/doug-stats/pricing"
	"github.com/robertgumeny/doug-stats/provider"
)

// MessageCost holds the computed cost for a single assistant message.
type MessageCost struct {
	UUID  string
	Model string
	Cost  pricing.Cost
}

// SessionAggregate holds the computed cost for a single session.
type SessionAggregate struct {
	SessionID   string
	ProjectPath string
	TaskID      string
	Model       string
	TotalCost   pricing.Cost
}

// TaskAggregate holds the computed cost across all sessions for a task.
type TaskAggregate struct {
	TaskID    string
	TotalCost pricing.Cost
}

// ProjectAggregate holds the computed cost across all sessions for a project.
type ProjectAggregate struct {
	ProjectPath string
	TotalCost   pricing.Cost
}

// Summary is the result of Phase 1 cost aggregation. It is safe to read
// from multiple goroutines once Aggregate returns.
type Summary struct {
	Sessions []SessionAggregate
	Tasks    []TaskAggregate
	Projects []ProjectAggregate
	// CacheTierMinutes is always 5: Claude's prompt cache uses a 5-minute
	// sliding window. Returned as metadata for UI display.
	CacheTierMinutes int
}

// Aggregate computes session, task, and project cost totals from Phase 1
// session metadata. The HTTP server must not start until this call returns.
func Aggregate(sessions []*provider.SessionMeta) *Summary {
	taskTotals := make(map[string]pricing.Cost)
	projectTotals := make(map[string]pricing.Cost)

	summary := &Summary{
		CacheTierMinutes: pricing.CacheTierMinutes,
		Sessions:         make([]SessionAggregate, 0, len(sessions)),
	}

	for _, s := range sessions {
		cost := pricing.Compute(s.Model, s.Tokens)
		summary.Sessions = append(summary.Sessions, SessionAggregate{
			SessionID:   s.ID,
			ProjectPath: s.ProjectPath,
			TaskID:      s.TaskID,
			Model:       s.Model,
			TotalCost:   cost,
		})

		if s.TaskID != "" {
			taskTotals[s.TaskID] = taskTotals[s.TaskID].Add(cost)
		}
		projectTotals[s.ProjectPath] = projectTotals[s.ProjectPath].Add(cost)
	}

	for taskID, cost := range taskTotals {
		summary.Tasks = append(summary.Tasks, TaskAggregate{
			TaskID:    taskID,
			TotalCost: cost,
		})
	}

	for path, cost := range projectTotals {
		summary.Projects = append(summary.Projects, ProjectAggregate{
			ProjectPath: path,
			TotalCost:   cost,
		})
	}

	return summary
}

// ComputeMessageCosts returns per-message costs for a transcript.
// Only assistant messages with token data contribute; user messages are skipped.
func ComputeMessageCosts(transcript *provider.Transcript) []MessageCost {
	var costs []MessageCost
	for _, msg := range transcript.Messages {
		if msg.Role != "assistant" || msg.Tokens == nil {
			continue
		}
		costs = append(costs, MessageCost{
			UUID:  msg.UUID,
			Model: msg.Model,
			Cost:  pricing.Compute(msg.Model, *msg.Tokens),
		})
	}
	return costs
}
