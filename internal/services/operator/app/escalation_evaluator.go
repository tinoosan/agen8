// Package app — evaluator.go implements the EscalationNotificationEvaluator
// which plugs into the notification system via the TriggerEvaluator interface
// (OCP). It raises notifications for escalation.created and
// escalation.escalated events, mapping urgency → notification severity.
package app

import (
	"context"
	"fmt"

	notifdomain "github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// EscalationNotificationEvaluator evaluates escalation domain events and
// raises notifications for conditions that warrant operator attention.
//
// It reacts to these event types:
//   - escalation.created   — new escalation requires operator decision
//   - escalation.escalated — deadline auto-escalation bumped urgency
type EscalationNotificationEvaluator struct{}

// NewEscalationNotificationEvaluator creates a new evaluator.
func NewEscalationNotificationEvaluator() *EscalationNotificationEvaluator {
	return &EscalationNotificationEvaluator{}
}

// Source returns the domain identifier for escalation notifications.
func (e *EscalationNotificationEvaluator) Source() string { return "escalation" }

// Evaluate inspects a domain event and returns zero or more notifications.
// Only escalation events are evaluated; all others return nil.
func (e *EscalationNotificationEvaluator) Evaluate(_ context.Context, event types.EventRecord) []notifdomain.Notification {
	switch event.Type {
	case "escalation.created":
		return e.evaluateCreated(event)
	case "escalation.escalated":
		return e.evaluateEscalated(event)
	default:
		return nil
	}
}

// evaluateCreated handles escalation.created events.
// All new escalations generate a notification. Severity is derived from urgency.
func (e *EscalationNotificationEvaluator) evaluateCreated(event types.EventRecord) []notifdomain.Notification {
	title := event.Data["title"]
	urgency := event.Data["urgency"]
	category := event.Data["category"]
	projectID := event.Data["projectId"]
	escID := event.Data["escalationId"]

	severity := escalationUrgencyToSeverity(urgency)

	return []notifdomain.Notification{{
		UserID:      notifUserID(event),
		ProjectID:   projectID,
		Source:      "escalation",
		Trigger:     "escalation_created",
		Severity:    severity,
		Subject:     notifdomain.Subject{Kind: "escalation", ID: escID},
		Title:       fmt.Sprintf("Escalation: %s", title),
		Body:        fmt.Sprintf("New %s escalation (%s urgency) requires operator decision.", category, urgency),
		Link:        escalationLink(projectID, escID),
		ThrottleKey: "esc:" + escID,
		Metadata:    buildEscalationMetadata(event),
	}}
}

// evaluateEscalated handles escalation.escalated events (deadline auto-escalation).
// Uses a distinct ThrottleKey suffix so the escalated notification bypasses
// any cooldown from the original creation notification.
func (e *EscalationNotificationEvaluator) evaluateEscalated(event types.EventRecord) []notifdomain.Notification {
	escID := event.Data["escalationId"]
	newUrgency := event.Data["newUrgency"]
	previousUrgency := event.Data["previousUrgency"]
	projectID := event.Data["projectId"]
	title := event.Data["title"]

	severity := escalationUrgencyToSeverity(newUrgency)

	return []notifdomain.Notification{{
		UserID:      notifUserID(event),
		ProjectID:   projectID,
		Source:      "escalation",
		Trigger:     "escalation_escalated",
		Severity:    severity,
		Subject:     notifdomain.Subject{Kind: "escalation", ID: escID},
		Title:       fmt.Sprintf("Escalation urgency raised: %s", title),
		Body:        fmt.Sprintf("Deadline auto-escalation: %s → %s.", previousUrgency, newUrgency),
		Link:        escalationLink(projectID, escID),
		ThrottleKey: "esc:" + escID + ":escalated",
		Metadata:    buildEscalationMetadata(event),
	}}
}

// -- Helpers ------------------------------------------------------------------

// escalationUrgencyToSeverity maps escalation urgency strings to notification
// severity using the domain's UrgencyToSeverity mapping.
func escalationUrgencyToSeverity(urgency string) notifdomain.Severity {
	sev, err := domain.UrgencyToSeverity(domain.Urgency(urgency))
	if err != nil {
		return notifdomain.SeverityInfo
	}
	return notifdomain.Severity(sev)
}

// escalationLink builds a deep-link to an escalation.
func escalationLink(projectID, escID string) *notifdomain.NotificationLink {
	if projectID == "" || escID == "" {
		return nil
	}
	return &notifdomain.NotificationLink{
		Surface: "escalations",
		URL:     fmt.Sprintf("/project/%s/escalations/%s", projectID, escID),
	}
}

// buildEscalationMetadata extracts standard fields from the event.
func buildEscalationMetadata(event types.EventRecord) map[string]string {
	m := map[string]string{}
	for _, key := range []string{"escalationId", "title", "urgency", "category", "source", "projectId", "previousUrgency", "newUrgency", "taskRef"} {
		if v, ok := event.Data[key]; ok && v != "" {
			m[key] = v
		}
	}
	return m
}
