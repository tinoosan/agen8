import type { Task, TaskActivity } from "../lib/types";
import { getAttempts } from "./boardHelpers";

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function asTaskActivity(value: unknown): TaskActivity | null {
  if (!isRecord(value)) return null;
  const timestamp =
    typeof value.timestamp === "string" ? value.timestamp.trim() : "";
  const kind = typeof value.kind === "string" ? value.kind.trim() : "";
  const summary = typeof value.summary === "string" ? value.summary.trim() : "";
  if (!timestamp || !kind || !summary) return null;
  return {
    eventId: typeof value.eventId === "string" ? value.eventId : undefined,
    timestamp,
    kind,
    actor:
      typeof value.actor === "string" && value.actor.trim()
        ? value.actor
        : "system",
    agent_id: typeof value.agent_id === "string" ? value.agent_id : undefined,
    summary,
    details: isRecord(value.details) ? value.details : undefined,
  };
}

function legacyAttemptActivities(task: Task): TaskActivity[] {
  const out: TaskActivity[] = [];
  for (const attempt of getAttempts(task)) {
    if (attempt.summary && attempt.completedAt) {
      out.push({
        timestamp: attempt.completedAt,
        kind: "state_change",
        actor: attempt.workerRole || task.assignedToLabel || task.assignedTo || "agent",
        summary: attempt.summary,
        details: {
          attempt: attempt.attempt,
          outcome: attempt.outcome || "completed",
        },
      });
    }
    if (attempt.review?.reviewedAt) {
      out.push({
        timestamp: attempt.review.reviewedAt,
        kind: "state_change",
        actor:
          attempt.review.reviewerRole ||
          attempt.review.reviewedBy ||
          "coordinator",
        summary: `Review ${attempt.review.decision}`,
        details: {
          attempt: attempt.attempt,
          decision: attempt.review.decision,
          feedback: attempt.review.feedback || "",
        },
      });
    }
  }
  if (
    task.summary &&
    task.completedAt &&
    !out.some(
      (entry) =>
        entry.summary === task.summary && entry.timestamp === task.completedAt,
    )
  ) {
    out.push({
      timestamp: task.completedAt,
      kind: "state_change",
      actor: task.assignedToLabel || task.assignedTo || "agent",
      summary: task.summary,
    });
  }
  return out;
}

export function getTaskActivities(task: Task): TaskActivity[] {
  const raw = Array.isArray(task.metadata?.activity)
    ? task.metadata?.activity
    : [];
  const activities = raw
    .map(asTaskActivity)
    .filter((entry): entry is TaskActivity => entry !== null);
  const merged =
    activities.length > 0 ? activities : legacyAttemptActivities(task);
  return merged
    .slice()
    .sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp));
}
