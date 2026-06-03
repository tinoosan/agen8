// Package app contains the decision application service components.
// This file implements the DecisionNotificationEvaluator which plugs into the
// notification system via the TriggerEvaluator interface (OCP).
package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// DecisionNotificationEvaluator evaluates decision domain events and raises
// notifications for conditions that warrant operator attention.
//
// It reacts to these event types:
//   - decision.logged          -- a decision was recorded (notify if high confidence)
//   - operator_action.created  -- an OA was created (needs operator attention)
//   - operator_action.resolved -- an OA was resolved (auto-dismiss the create notif)
type DecisionNotificationEvaluator struct{}

// NewDecisionNotificationEvaluator creates a new evaluator for decision events.
func NewDecisionNotificationEvaluator() *DecisionNotificationEvaluator {
	return &DecisionNotificationEvaluator{}
}

// Source returns the domain identifier for decision notifications.
func (e *DecisionNotificationEvaluator) Source() string { return "decision" }

// Evaluate inspects a domain event and returns zero or more notifications.
// Only decision and operator_action events are evaluated; all others return nil.
func (e *DecisionNotificationEvaluator) Evaluate(_ context.Context, event types.EventRecord) []domain.Notification {
	switch event.Type {
	case "decision.logged":
		return e.evaluateDecisionLogged(event)
	case "operator_action.created":
		return e.evaluateOACreated(event)
	case "operator_action.resolved":
		return e.evaluateOAResolved(event)
	default:
		return nil
	}
}

// evaluateDecisionLogged handles decision.logged events.
// Only high-confidence decisions (>= 0.8) warrant a notification.
func (e *DecisionNotificationEvaluator) evaluateDecisionLogged(event types.EventRecord) []domain.Notification {
	confidenceStr := event.Data["confidence"]
	confidence, err := strconv.ParseFloat(confidenceStr, 64)
	if err != nil {
		return nil
	}

	if confidence < 0.8 {
		return nil
	}

	title := event.Data["title"]
	source := event.Data["source"]
	projectID := event.Data["projectId"]
	decisionID := strings.TrimSpace(event.Data["decisionId"])

	return []domain.Notification{{
		UserID:      eventUserID(event),
		ProjectID:   projectID,
		Source:      "decision",
		Trigger:     "decision_logged",
		Severity:    domain.SeverityInfo,
		Subject:     domain.Subject{Kind: "decision", ID: decisionID},
		Title:       fmt.Sprintf("Decision logged: %s", title),
		Body:        fmt.Sprintf("High-confidence decision from %s (%.0f%% confidence).", source, confidence*100),
		Link:        decisionLink(projectID),
		ThrottleKey: "decision:" + title,
		Metadata:    buildDecisionMetadata(event),
	}}
}

// evaluateOACreated handles operator_action.created events.
// All created OAs generate notifications. Severity is derived from urgency.
func (e *DecisionNotificationEvaluator) evaluateOACreated(event types.EventRecord) []domain.Notification {
	title := event.Data["title"]
	urgency := event.Data["urgency"]
	category := event.Data["category"]
	projectID := event.Data["projectId"]
	oaID := event.Data["oaId"]

	severity := urgencyToSeverity(urgency)

	return []domain.Notification{{
		UserID:      eventUserID(event),
		ProjectID:   projectID,
		Source:      "decision",
		Trigger:     "oa_created",
		Severity:    severity,
		Subject:     domain.Subject{Kind: "operator_action", ID: oaID},
		Title:       fmt.Sprintf("Operator action required: %s", title),
		Body:        fmt.Sprintf("New %s operator action (%s urgency): %s", category, urgency, title),
		Link:        oaLink(projectID, oaID),
		ThrottleKey: "oa:" + oaID,
		Metadata:    buildDecisionMetadata(event),
	}}
}

// evaluateOAResolved handles operator_action.resolved events.
// Resolved OAs are always informational. The Subject ref enables the
// service's auto-dismiss path to clean up the matching "OA created" row.
func (e *DecisionNotificationEvaluator) evaluateOAResolved(event types.EventRecord) []domain.Notification {
	title := event.Data["title"]
	resolution := event.Data["resolution"]
	resolvedBy := event.Data["resolvedBy"]
	projectID := event.Data["projectId"]
	oaID := event.Data["oaId"]

	return []domain.Notification{{
		UserID:      eventUserID(event),
		ProjectID:   projectID,
		Source:      "decision",
		Trigger:     "oa_resolved",
		Severity:    domain.SeverityInfo,
		Subject:     domain.Subject{Kind: "operator_action", ID: oaID},
		Title:       fmt.Sprintf("Operator action resolved: %s", title),
		Body:        fmt.Sprintf("Resolved by %s: %s", resolvedBy, resolution),
		Link:        oaLink(projectID, oaID),
		ThrottleKey: "oa:" + oaID,
		Metadata:    buildDecisionMetadata(event),
	}}
}

// -- Helpers ------------------------------------------------------------------

// eventUserID returns the user identity to route the notification to.
// In hosted multi-user mode this'll come from event.Data["userId"]
// stamped at emit time. Until that wiring lands the fallback is
// projectId — preserving the current single-user-local behavior while
// the field name no longer lies about what it represents.
func eventUserID(event types.EventRecord) string {
	if uid := strings.TrimSpace(event.Data["userId"]); uid != "" {
		return uid
	}
	// Follow-up: drop this fallback once events stamp userId end-to-end.
	return strings.TrimSpace(event.Data["projectId"])
}

// urgencyToSeverity maps OA urgency strings to notification severity.
func urgencyToSeverity(urgency string) domain.Severity {
	switch urgency {
	case "critical":
		return domain.SeverityCritical
	case "high":
		return domain.SeverityWarning
	default:
		return domain.SeverityInfo
	}
}

// decisionLink builds a deep-link to the decisions view for a project.
func decisionLink(projectID string) *domain.NotificationLink {
	if projectID == "" {
		return nil
	}
	return &domain.NotificationLink{
		Surface: "decisions",
		URL:     fmt.Sprintf("/project/%s/decisions", projectID),
	}
}

// oaLink builds a deep-link to a specific operator action.
func oaLink(projectID, oaID string) *domain.NotificationLink {
	if projectID == "" || oaID == "" {
		return nil
	}
	return &domain.NotificationLink{
		Surface: "operator_actions",
		URL:     fmt.Sprintf("/project/%s/operator-actions/%s", projectID, oaID),
	}
}

// buildDecisionMetadata extracts standard fields from the event for notification metadata.
func buildDecisionMetadata(event types.EventRecord) map[string]string {
	m := map[string]string{}
	for _, key := range []string{"title", "source", "confidence", "projectId", "oaId", "urgency", "category", "resolution", "resolvedBy"} {
		if v, ok := event.Data[key]; ok && v != "" {
			m[key] = v
		}
	}
	return m
}
