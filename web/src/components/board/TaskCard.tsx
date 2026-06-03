import clsx from "clsx";
import {
  Clock,
  Hash,
  AlertTriangle,
  Target,
  Flag,
  MessageSquare,
  BellRing,
} from "lucide-react";
import { Badge } from "../ui/badge";
import { spaceDisplayName } from "../../lib/spaceDisplayName";
import { taskStatusColor, taskStatusLabel } from "../../lib/statusLabels";
import type {
  Task,
  ProjectSpaceSummary,
  OpActionView,
  MissionView,
  KeyResultView,
} from "../../lib/types";
import {
  lookupSpaceForTask,
  parseRetryTask,
  isHeartbeatTask,
  getHeartbeatOutcome,
  getAttemptCount,
  getLatestReview,
  getOperatorActionBlocker,
  taskDuration,
  relativeTime,
  taskIdShort,
  urgencyBadgeVariant,
  urgencyAccentColor,
} from "../../pages/boardHelpers";

export interface TaskCardProps {
  task: Task;
  spaceLookup: Map<string, ProjectSpaceSummary>;
  accentColor: string;
  operatorActionById: Map<string, OpActionView>;
  krById: Map<string, KeyResultView>;
  missionById: Map<string, MissionView>;
  decisionCountByTask: Map<string, number>;
  escalationCountByTask: Map<string, number>;
  onSelect: (task: Task) => void;
  onOpenOperatorAction: (actionId: string) => void;
  isSelected: boolean;
}

export function TaskCard({
  task,
  spaceLookup,
  accentColor,
  operatorActionById,
  krById,
  missionById,
  decisionCountByTask,
  escalationCountByTask,
  onSelect,
  onOpenOperatorAction,
  isSelected,
}: TaskCardProps) {
  const space = lookupSpaceForTask(task, spaceLookup);
  const blockReason = (task.metadata?.blockReason as string) ?? null;
  const operatorActionBlocker = getOperatorActionBlocker(task);
  const operatorAction = operatorActionBlocker
    ? (operatorActionById.get(operatorActionBlocker.id) ?? null)
    : null;
  const operatorUrgency = operatorAction?.urgency ?? null;
  const operatorAccentColor = urgencyAccentColor(operatorUrgency);
  const isFailed = task.status === "failed";
  const isCanceled = task.status === "canceled";
  const retry = parseRetryTask(task);

  // Build title + summary text
  let titleText: string;
  let summaryText: string | null;
  if (retry.isRetry) {
    titleText = retry.originalGoal;
    summaryText = retry.feedback
      ? retry.attemptNum
        ? `Attempt ${retry.attemptNum}: ${retry.feedback}`
        : `Retry feedback: ${retry.feedback}`
      : null;
  } else {
    titleText = task.title || task.description;
    summaryText = task.summary
      ? task.summary
      : task.title && task.description && task.title !== task.description
        ? task.description
        : null;
  }

  return (
    <div
      style={{
        borderTop: isSelected ? "1px solid var(--accent)" : "1px solid var(--border)",
        borderRight: isSelected ? "1px solid var(--accent)" : "1px solid var(--border)",
        borderBottom: isSelected ? "1px solid var(--accent)" : "1px solid var(--border)",
        borderLeft: `3px solid ${operatorAccentColor ?? accentColor}`,
      }}
      className="bg-[var(--bg-panel)] rounded-[var(--r-md)] px-3.5 py-3 flex flex-col gap-2 cursor-pointer board-task-card"
      onClick={() => onSelect(task)}
    >
      {/* Header: task ID + cycle time / timestamp */}
      <div className="flex items-center gap-1">
        <span className="text-[10px] text-[var(--text-3)] font-[var(--font-mono,monospace)] opacity-60">
          <Hash size={9} className="inline align-middle" />
          {taskIdShort(task.id)}
        </span>
        <div style={{ flex: 1 }} />
        {(() => {
          const dur = taskDuration(task);
          if (dur) {
            return (
              <span
                title={`Duration: ${dur} · Completed ${new Date(task.completedAt!).toLocaleString()}`}
                style={{
                  fontSize: 10,
                  color: "var(--text-3)",
                  fontVariantNumeric: "tabular-nums",
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 3,
                }}
              >
                <Clock size={9} className="inline align-middle" />
                {dur}
              </span>
            );
          }
          return (
            <span
              title={task.createdAt ? new Date(task.createdAt).toLocaleString() : "Created time unavailable"}
              style={{ fontSize: 10, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}
            >
              {relativeTime(task.createdAt)}
            </span>
          );
        })()}
      </div>

      {/* Goal */}
      <div
        style={{
          fontSize: 12,
          fontWeight: 500,
          color: isFailed ? "var(--red)" : isCanceled ? "var(--text-3)" : "var(--text-1)",
          lineHeight: 1.5,
          display: "-webkit-box",
          WebkitLineClamp: 3,
          WebkitBoxOrient: "vertical",
          overflow: "hidden",
        }}
      >
        {titleText}
      </div>

      {/* Summary */}
      {summaryText && (
        <div
          className={clsx(
            "text-[11px] leading-normal overflow-hidden",
            retry.isRetry ? "text-[var(--amber)]" : "text-[var(--text-3)]",
          )}
          style={{
            display: "-webkit-box",
            WebkitLineClamp: 2,
            WebkitBoxOrient: "vertical",
          }}
        >
          {summaryText}
        </div>
      )}

      {operatorAction?.title && task.status === "blocked" && (
        <div className="text-[10px] text-[var(--text-2)] bg-[var(--bg-elevated)] px-2 py-[3px] rounded-[var(--r-sm)] leading-snug">
          {operatorAction.title}
        </div>
      )}

      {/* Block reason */}
      {blockReason && task.status === "blocked" && (
        <div className="text-[10px] text-[var(--amber)] bg-[var(--amber-dim)] px-2 py-[3px] rounded-[var(--r-sm)] leading-snug">
          {blockReason}
        </div>
      )}

      {/* Footer: badges */}
      <div className="flex items-center gap-1.5 flex-wrap pt-0.5 border-t border-[var(--border)]">
        {/* KR linkage badge */}
        {task.keyResultRef && (() => {
          const kr = krById.get(task.keyResultRef);
          const mission = kr ? missionById.get(kr.missionId) : undefined;
          const label = kr
            ? kr.title.length > 22
              ? kr.title.slice(0, 21) + "…"
              : kr.title
            : task.keyResultRef.slice(-8);
          const tooltip = [
            `KR: ${kr?.title ?? task.keyResultRef}`,
            mission ? `Mission: ${mission.title}` : null,
          ]
            .filter(Boolean)
            .join(" · ");
          return (
            <Badge
              variant="accent"
              className="gap-1 text-[9px] px-1.5 py-0 max-w-[140px] truncate"
              title={tooltip}
            >
              <Target size={9} className="shrink-0" />
              {label}
            </Badge>
          );
        })()}
        {/* Mission badge */}
        {task.keyResultRef && (() => {
          const kr = krById.get(task.keyResultRef);
          if (!kr) return null;
          const mission = missionById.get(kr.missionId);
          if (!mission) return null;
          const label =
            mission.title.length > 20 ? mission.title.slice(0, 19) + "…" : mission.title;
          return (
            <Badge
              variant="outline"
              className="gap-1 text-[9px] px-1.5 py-0 max-w-[130px] truncate"
              title={`Mission: ${mission.title}`}
            >
              <Flag size={9} className="shrink-0" />
              {label}
            </Badge>
          );
        })()}
        {/* OA blocked badge */}
        {task.status === "blocked" && operatorActionBlocker && (
          <button
            type="button"
            className="bg-transparent border-0 p-0 cursor-pointer"
            onClick={(event) => {
              event.stopPropagation();
              onOpenOperatorAction(operatorActionBlocker.id);
            }}
            aria-label={`Awaiting Operator${operatorUrgency ? ` (${operatorUrgency})` : ""}`}
            title={
              operatorUrgency
                ? `Awaiting operator (${operatorUrgency} urgency)`
                : "Awaiting operator"
            }
          >
            <Badge
              variant={urgencyBadgeVariant(operatorUrgency)}
              className="gap-1 text-[9px] px-1.5 py-0 animate-pulse-subtle"
            >
              <AlertTriangle size={9} />
              Awaiting Operator
            </Badge>
          </button>
        )}
        {/* Heartbeat badge */}
        {isHeartbeatTask(task) && (
          <span
            className="board-badge"
            style={{ color: "var(--accent)", background: "var(--accent-dim)" }}
          >
            {"\u2661"} heartbeat
          </span>
        )}
        {/* Heartbeat outcome chip */}
        {(() => {
          if (!isHeartbeatTask(task)) return null;
          const outcome = getHeartbeatOutcome(task);
          if (!outcome) return null;
          const chipColors: Record<string, { bg: string; fg: string }> = {
            ok:       { bg: "var(--green-dim)",  fg: "var(--green)" },
            warning:  { bg: "var(--amber-dim)",  fg: "var(--amber)" },
            critical: { bg: "var(--red-dim)",    fg: "var(--red)"   },
            error:    { bg: "var(--red-dim)",    fg: "var(--red)"   },
          };
          const c = chipColors[outcome.status];
          if (!c) return null;
          return (
            <span
              className="text-[9px] font-bold tracking-[0.06em] uppercase px-1.5 py-0.5 rounded-full"
              style={{ background: c.bg, color: c.fg }}
            >
              {outcome.status}
            </span>
          );
        })()}
        {/* Decision count badge */}
        {(decisionCountByTask.get(task.id) ?? 0) > 0 && (
          <span
            className="board-badge"
            style={{ color: "var(--green)", background: "var(--green-dim)" }}
            title={`${decisionCountByTask.get(task.id)} decision${decisionCountByTask.get(task.id) === 1 ? "" : "s"} logged`}
          >
            <MessageSquare size={8} className="inline align-middle mr-0.5" />
            {decisionCountByTask.get(task.id)}
          </span>
        )}
        {/* Escalation count badge */}
        {(escalationCountByTask.get(task.id) ?? 0) > 0 && (
          <span
            className="board-badge"
            style={{ color: "var(--red)", background: "var(--red-dim)" }}
            title={`${escalationCountByTask.get(task.id)} pending escalation${escalationCountByTask.get(task.id) === 1 ? "" : "s"}`}
          >
            <BellRing size={8} className="inline align-middle mr-0.5" />
            {escalationCountByTask.get(task.id)}
          </span>
        )}
        {(task.assignedToLabel || task.assignedTo) && <span className="board-badge board-badge-role">{task.assignedToLabel || task.assignedTo}</span>}
        {space && (
          <span className="board-badge board-badge-space">
            {spaceDisplayName(space.spaceId, space.spaceName)}
          </span>
        )}
        {/* Attempt count badge */}
        {getAttemptCount(task) > 0 && (
          <span
            className="board-badge"
            style={{ color: "var(--amber)", background: "var(--amber-dim)" }}
          >
            ↻ {getAttemptCount(task)}
          </span>
        )}
        {/* Review status badge */}
        {(() => {
          const review = getLatestReview(task);
          if (!review) return null;
          const colors: Record<string, { bg: string; fg: string }> = {
            approved: { bg: "var(--green-dim)",                    fg: "var(--green)" },
            retry:    { bg: "var(--amber-dim)",                    fg: "var(--amber)" },
            escalated:{ bg: "var(--red-dim)",    fg: "var(--red)"   },
          };
          const c = colors[review.decision];
          if (!c) return null;
          return (
            <span
              className="text-[9px] font-bold tracking-[0.06em] uppercase px-1.5 py-0.5 rounded-full"
              style={{ background: c.bg, color: c.fg }}
            >
              {review.decision}
            </span>
          );
        })()}
        {/* Status indicator */}
        {taskStatusColor(task.status) && (
          <span
            style={{
              fontSize: 9,
              fontWeight: 700,
              letterSpacing: "0.04em",
              textTransform: "uppercase",
              color: taskStatusColor(task.status),
            }}
          >
            {taskStatusLabel(task.status)}
          </span>
        )}
      </div>

    </div>
  );
}
