package app

import (
	"context"
	"fmt"
	"strings"

	notifdomain "github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// notifUserID resolves the routing user for a notification. Hosted mode
// will stamp event.Data["userId"] at emit time; until then we fall back
// to projectId so single-user-local behaviour is preserved.
func notifUserID(event types.EventRecord) string {
	if uid := strings.TrimSpace(event.Data["userId"]); uid != "" {
		return uid
	}
	return strings.TrimSpace(event.Data["projectId"])
}

type OANotificationEvaluator struct{}

func NewOANotificationEvaluator() *OANotificationEvaluator {
	return &OANotificationEvaluator{}
}

func (e *OANotificationEvaluator) Source() string { return "opaction" }

func (e *OANotificationEvaluator) Evaluate(_ context.Context, event types.EventRecord) []notifdomain.Notification {
	switch event.Type {
	case "oa.created":
		return e.evaluateCreated(event)
	case "oa.verified":
		return e.evaluateVerificationRejected(event)
	default:
		return nil
	}
}

func (e *OANotificationEvaluator) evaluateCreated(event types.EventRecord) []notifdomain.Notification {
	actionID := event.Data["actionId"]
	projectID := event.Data["projectId"]
	title := event.Data["title"]
	if title == "" {
		title = actionID
	}
	urgency := event.Data["urgency"]
	category := event.Data["category"]
	body := "A new operator action needs attention."
	if category != "" && urgency != "" {
		body = fmt.Sprintf("A new %s operator action (%s urgency) needs attention.", category, urgency)
	} else if urgency != "" {
		body = fmt.Sprintf("A new operator action (%s urgency) needs attention.", urgency)
	}
	return []notifdomain.Notification{{
		UserID:      notifUserID(event),
		ProjectID:   projectID,
		Source:      "opaction",
		Trigger:     "oa_created",
		Severity:    oaUrgencyToSeverity(urgency),
		Subject:     notifdomain.Subject{Kind: "operator_action", ID: actionID},
		Title:       fmt.Sprintf("Operator action required: %s", title),
		Body:        body,
		Link:        oaNotificationLink(projectID, actionID),
		ThrottleKey: "oa:" + actionID,
		Metadata:    buildOANotificationMetadata(event),
	}}
}

func (e *OANotificationEvaluator) evaluateVerificationRejected(event types.EventRecord) []notifdomain.Notification {
	if event.Data["oldStatus"] != "pending_verification" || event.Data["newStatus"] != "in_progress" {
		return nil
	}

	actionID := event.Data["actionId"]
	projectID := event.Data["projectId"]
	title := event.Data["title"]
	if title == "" {
		title = actionID
	}
	return []notifdomain.Notification{{
		UserID:      notifUserID(event),
		ProjectID:   projectID,
		Source:      "opaction",
		Trigger:     "oa_verification_requested_changes",
		Severity:    notifdomain.SeverityWarning,
		Subject:     notifdomain.Subject{Kind: "operator_action", ID: actionID},
		Title:       fmt.Sprintf("Changes requested: %s", title),
		Body:        "An agent reviewed this operator action and sent it back for rework.",
		Link:        oaNotificationLink(projectID, actionID),
		ThrottleKey: "oa:" + actionID + ":verification",
		Metadata:    buildOANotificationMetadata(event),
	}}
}

func oaUrgencyToSeverity(urgency string) notifdomain.Severity {
	switch urgency {
	case "critical":
		return notifdomain.SeverityCritical
	case "high", "medium":
		return notifdomain.SeverityWarning
	default:
		return notifdomain.SeverityInfo
	}
}

func oaNotificationLink(projectID, actionID string) *notifdomain.NotificationLink {
	if projectID == "" || actionID == "" {
		return nil
	}
	return &notifdomain.NotificationLink{
		Surface: "operator_actions",
		URL:     fmt.Sprintf("/project/%s/operator-actions/%s", projectID, actionID),
	}
}

func buildOANotificationMetadata(event types.EventRecord) map[string]string {
	m := map[string]string{}
	for _, key := range []string{"actionId", "projectId", "spaceId", "title", "urgency", "category", "oldStatus", "newStatus"} {
		if v, ok := event.Data[key]; ok && v != "" {
			m[key] = v
		}
	}
	return m
}
