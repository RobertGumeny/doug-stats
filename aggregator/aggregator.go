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
	SessionID          string
	ProjectPath        string
	CanonicalProjectID string
	TaskID             string
	Model              string
	TotalCost          pricing.Cost
}

// TaskAggregate holds the computed cost across all sessions for a task.
type TaskAggregate struct {
	CanonicalProjectID string
	TaskID             string
	SessionCount       int
	TotalCost          pricing.Cost
}

// ProjectAggregate holds the computed cost across all sessions for a project.
type ProjectAggregate struct {
	CanonicalProjectID string
	SessionCount       int
	TotalCost          pricing.Cost
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
	type taskKey struct {
		canonicalProjectID string
		taskID             string
	}
	type taskAcc struct {
		sessionCount int
		totalCost    pricing.Cost
	}
	type projectAcc struct {
		sessionCount int
		totalCost    pricing.Cost
	}

	taskTotals := make(map[taskKey]*taskAcc)
	projectTotals := make(map[string]*projectAcc)

	summary := &Summary{
		CacheTierMinutes: pricing.CacheTierMinutes,
		Sessions:         make([]SessionAggregate, 0, len(sessions)),
	}

	for _, s := range sessions {
		cost := pricing.Compute(s.Model, s.Tokens)
		canonicalProjectID := s.CanonicalProjectID
		if canonicalProjectID == "" {
			canonicalProjectID = s.ProjectPath
		}
		summary.Sessions = append(summary.Sessions, SessionAggregate{
			SessionID:          s.ID,
			ProjectPath:        s.ProjectPath,
			CanonicalProjectID: canonicalProjectID,
			TaskID:             s.TaskID,
			Model:              s.Model,
			TotalCost:          cost,
		})

		taskID := aggregatedTaskID(s)
		if taskID != "" {
			key := taskKey{canonicalProjectID: canonicalProjectID, taskID: taskID}
			if taskTotals[key] == nil {
				taskTotals[key] = &taskAcc{}
			}
			taskTotals[key].sessionCount++
			taskTotals[key].totalCost = taskTotals[key].totalCost.Add(cost)
		}
		if projectTotals[canonicalProjectID] == nil {
			projectTotals[canonicalProjectID] = &projectAcc{}
		}
		projectTotals[canonicalProjectID].sessionCount++
		projectTotals[canonicalProjectID].totalCost = projectTotals[canonicalProjectID].totalCost.Add(cost)
	}

	for key, acc := range taskTotals {
		summary.Tasks = append(summary.Tasks, TaskAggregate{
			CanonicalProjectID: key.canonicalProjectID,
			TaskID:             key.taskID,
			SessionCount:       acc.sessionCount,
			TotalCost:          acc.totalCost,
		})
	}

	for canonicalProjectID, acc := range projectTotals {
		summary.Projects = append(summary.Projects, ProjectAggregate{
			CanonicalProjectID: canonicalProjectID,
			SessionCount:       acc.sessionCount,
			TotalCost:          acc.totalCost,
		})
	}

	return summary
}

func aggregatedTaskID(s *provider.SessionMeta) string {
	if s.TaskID != "" {
		return s.TaskID
	}
	if s.Class == provider.ClassManual || s.Class == provider.ClassUntagged {
		return "manual"
	}
	return ""
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
