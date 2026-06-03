package app

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestDecisionEvaluator_Source(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	if e.Source() != "decision" {
		t.Errorf("expected source 'decision', got %q", e.Source())
	}
}

func TestDecisionEvaluator_DecisionLogged_HighConfidence(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "decision.logged", "decision logged", map[string]string{
		"title":      "Use Redis for caching",
		"source":     "architect-agent",
		"confidence": "0.9",
		"projectId":  "proj-1",
	})

	result := e.Evaluate(context.Background(), event)
	if len(result) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result))
	}

	n := result[0]
	if n.Trigger != "decision_logged" {
		t.Errorf("expected trigger 'decision_logged', got %q", n.Trigger)
	}
	if n.Severity != domain.SeverityInfo {
		t.Errorf("expected severity info, got %q", n.Severity)
	}
	if n.UserID != "proj-1" {
		t.Errorf("expected userID 'proj-1', got %q", n.UserID)
	}
	if n.Source != "decision" {
		t.Errorf("expected source 'decision', got %q", n.Source)
	}
	if n.Link == nil || n.Link.Surface != "decisions" {
		t.Errorf("expected decisions link, got %v", n.Link)
	}
	if n.ThrottleKey != "decision:Use Redis for caching" {
		t.Errorf("expected throttle key 'decision:Use Redis for caching', got %q", n.ThrottleKey)
	}
}

func TestDecisionEvaluator_DecisionLogged_ExactThreshold(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "decision.logged", "decision logged", map[string]string{
		"title":      "Boundary test",
		"source":     "agent",
		"confidence": "0.8",
		"projectId":  "proj-1",
	})

	result := e.Evaluate(context.Background(), event)
	if len(result) != 1 {
		t.Fatalf("expected 1 notification at exact threshold 0.8, got %d", len(result))
	}
	if result[0].Trigger != "decision_logged" {
		t.Errorf("expected trigger 'decision_logged', got %q", result[0].Trigger)
	}
}

func TestDecisionEvaluator_DecisionLogged_LowConfidence(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "decision.logged", "decision logged", map[string]string{
		"title":      "Maybe use SQLite",
		"source":     "planner-agent",
		"confidence": "0.5",
		"projectId":  "proj-1",
	})

	result := e.Evaluate(context.Background(), event)
	if result != nil {
		t.Errorf("expected nil for low confidence decision, got %v", result)
	}
}

func TestDecisionEvaluator_DecisionLogged_InvalidConfidence(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "decision.logged", "decision logged", map[string]string{
		"title":      "Something",
		"source":     "agent",
		"confidence": "not-a-number",
		"projectId":  "proj-1",
	})

	result := e.Evaluate(context.Background(), event)
	if result != nil {
		t.Errorf("expected nil for invalid confidence, got %v", result)
	}
}

func TestDecisionEvaluator_OACreated_Critical(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "operator_action.created", "oa created", map[string]string{
		"title":     "Approve deployment to prod",
		"urgency":   "critical",
		"category":  "approval",
		"projectId": "proj-1",
		"oaId":      "oa-123",
	})

	result := e.Evaluate(context.Background(), event)
	if len(result) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result))
	}

	n := result[0]
	if n.Trigger != "oa_created" {
		t.Errorf("expected trigger 'oa_created', got %q", n.Trigger)
	}
	if n.Severity != domain.SeverityCritical {
		t.Errorf("expected severity critical, got %q", n.Severity)
	}
	if n.Link == nil || n.Link.Surface != "operator_actions" {
		t.Errorf("expected operator_actions link, got %v", n.Link)
	}
	if n.ThrottleKey != "oa:oa-123" {
		t.Errorf("expected throttle key 'oa:oa-123', got %q", n.ThrottleKey)
	}
}

func TestDecisionEvaluator_OACreated_High(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "operator_action.created", "oa created", map[string]string{
		"title":     "Review budget increase",
		"urgency":   "high",
		"category":  "review",
		"projectId": "proj-1",
		"oaId":      "oa-456",
	})

	result := e.Evaluate(context.Background(), event)
	if len(result) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result))
	}
	if result[0].Severity != domain.SeverityWarning {
		t.Errorf("expected severity warning for high urgency, got %q", result[0].Severity)
	}
}

func TestDecisionEvaluator_OACreated_Low(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "operator_action.created", "oa created", map[string]string{
		"title":     "Update docs",
		"urgency":   "low",
		"category":  "maintenance",
		"projectId": "proj-1",
		"oaId":      "oa-789",
	})

	result := e.Evaluate(context.Background(), event)
	if len(result) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result))
	}
	if result[0].Severity != domain.SeverityInfo {
		t.Errorf("expected severity info for low urgency, got %q", result[0].Severity)
	}
}

func TestDecisionEvaluator_OACreated_Medium(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "operator_action.created", "oa created", map[string]string{
		"title":     "Check logs",
		"urgency":   "medium",
		"category":  "investigation",
		"projectId": "proj-1",
		"oaId":      "oa-med",
	})

	result := e.Evaluate(context.Background(), event)
	if len(result) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result))
	}
	if result[0].Severity != domain.SeverityInfo {
		t.Errorf("expected severity info for medium urgency, got %q", result[0].Severity)
	}
}

func TestDecisionEvaluator_OAResolved(t *testing.T) {
	e := NewDecisionNotificationEvaluator()
	event := types.NewEventRecord("run-1", "operator_action.resolved", "oa resolved", map[string]string{
		"title":      "Approve deployment to prod",
		"resolution": "Approved with conditions",
		"resolvedBy": "operator-1",
		"projectId":  "proj-1",
		"oaId":       "oa-123",
	})

	result := e.Evaluate(context.Background(), event)
	if len(result) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result))
	}

	n := result[0]
	if n.Trigger != "oa_resolved" {
		t.Errorf("expected trigger 'oa_resolved', got %q", n.Trigger)
	}
	if n.Severity != domain.SeverityInfo {
		t.Errorf("expected severity info, got %q", n.Severity)
	}
	if n.Link == nil || n.Link.Surface != "operator_actions" {
		t.Errorf("expected operator_actions link, got %v", n.Link)
	}
}

func TestDecisionEvaluator_UnrelatedEvent(t *testing.T) {
	e := NewDecisionNotificationEvaluator()

	unrelatedTypes := []string{
		"task.created",
		"task.heartbeat.done",
		"run.started",
		"something.random",
	}

	for _, eventType := range unrelatedTypes {
		event := types.NewEventRecord("run-1", eventType, "some event", nil)
		result := e.Evaluate(context.Background(), event)
		if result != nil {
			t.Errorf("expected nil for unrelated event type %q, got %v", eventType, result)
		}
	}
}
