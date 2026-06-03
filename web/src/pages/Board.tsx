import { useMemo, useState, useCallback, useRef, useEffect } from "react";
import { TaskCard } from "../components/board/TaskCard";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { useSearch } from "wouter";
import { useNavigation } from "../lib/routing";
import { useProjectSpaces } from "../hooks/useProjectSpaces";
import { usePendingOpActions } from "../hooks/useOpActions";
import { usePendingEscalations } from "../hooks/useEscalations";
import { useMissions, useProjectKRs } from "../hooks/useMissions";
import { useRecentDecisions } from "../hooks/useDecisions";
import { spaceDisplayName } from "../lib/spaceDisplayName";
import { CustomSelect } from "../components/fields";
import OpActionDetailPanel from "../components/dashboard/OpActionDetailPanel";
import { useProjectTasks, useProjectTasksSSE } from "../hooks/useProjectTasks";
import type {
  Task,
  ProjectSpaceSummary,
  OpActionView,
  MissionView,
  KeyResultView,
  DecisionView,
  EscalationView,
} from "../lib/types";
import {
  Columns3,
  X,
  Clock,
  Filter,
  Settings2,
  AlertTriangle,
} from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "../components/ui/popover";
import { RelatedSection, type RelatedItem } from "../components/strategy/RelatedSection";
import { CollapsibleSection } from "../components/strategy/CollapsibleSection";
import { SectionErrorBoundary } from "../components/ErrorBoundary";
import {
  isSystemTask as isSystemTaskFn,
  isHeartbeatTask,
  getHeartbeatOutcome,
  getLatestReview,
  getAcceptanceCriteria,
  taskMatchesSpaceFilter,
  resolveBoardSpaceQueryParam,
  lookupSpaceForTask,
  relativeTime,
  taskIdShort,
  parseRetryTask,
} from "./boardHelpers";
import { taskStatusColor, taskStatusLabel } from "../lib/statusLabels";
import { getTaskActivities } from "./taskActivity";
import { calendarLink } from "../lib/routing";
import type { TaskActivity } from "../lib/types";

/* ── Column definitions ────────────────────────────── */

interface Column {
  id: string;
  label: string;
  emptyLabel: string;
  statuses: string[];
  color: string;
  dimColor: string;
}

const COLUMNS: Column[] = [
  {
    id: "backlog",
    label: "Queued",
    emptyLabel: "No queued tasks",
    statuses: ["pending"],
    color: "var(--text-3)",
    dimColor: "var(--bg-elevated)",
  },
  {
    id: "blocked",
    label: "Blocked",
    emptyLabel: "Nothing blocked",
    statuses: ["blocked"],
    color: "var(--amber)",
    dimColor: "var(--amber-dim)",
  },
  {
    id: "paused",
    label: "Paused",
    emptyLabel: "No paused tasks",
    statuses: ["paused"],
    color: "var(--text-2)",
    dimColor: "var(--bg-elevated)",
  },
  {
    id: "in-progress",
    label: "Working",
    emptyLabel: "No active tasks",
    statuses: ["active"],
    color: "var(--blue)",
    dimColor: "var(--accent-dim)",
  },
  {
    id: "review",
    label: "In Review",
    emptyLabel: "No tasks in review",
    statuses: ["in_review"],
    color: "var(--accent)",
    dimColor: "var(--accent-dim)",
  },
  {
    id: "done",
    label: "Done",
    emptyLabel: "No completed tasks",
    statuses: ["succeeded"],
    color: "var(--green)",
    dimColor: "var(--green-dim)",
  },
  {
    id: "failed",
    label: "Failed",
    emptyLabel: "No failed tasks",
    statuses: ["failed", "canceled"],
    color: "var(--red)",
    dimColor: "var(--red-dim, rgba(239,68,68,0.08))",
  },
];

/* ── column capacity config ──────────────────────────────── */

type WipConfig = Record<string, number>; // columnId → limit

function wipStorageKey(projectId: string | null | undefined): string {
  return `agen8.board.wip.${projectId ?? ""}`;
}

function loadWipConfig(projectId: string | null | undefined): WipConfig {
  try {
    const raw = localStorage.getItem(wipStorageKey(projectId));
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed))
      return {};
    return parsed as WipConfig;
  } catch {
    return {};
  }
}

function saveWipConfig(
  projectId: string | null | undefined,
  config: WipConfig,
): void {
  localStorage.setItem(wipStorageKey(projectId), JSON.stringify(config));
}

/* ── Column Capacity Popover ─────────────────────────── */

function WipSettingsPopover({
  wipConfig,
  onUpdate,
}: {
  wipConfig: WipConfig;
  onUpdate: (columnId: string, limit: number | null) => void;
}) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          className="inline-flex items-center gap-1 text-[11px] font-medium text-[var(--text-3)] bg-transparent border border-[var(--border)] rounded-[var(--r-sm)] px-2 py-0.5 cursor-pointer hover:text-[var(--text-1)] hover:border-[var(--border-strong)] transition-colors font-[inherit]"
          title="Configure capacity limits per column"
          aria-label="Column capacity settings"
        >
          <Settings2 size={11} />
          Limits
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-3" align="end">
        <div className="text-[11px] font-semibold text-[var(--text-2)] uppercase tracking-[0.04em] mb-3">
          Column Limits
        </div>
        <div className="flex flex-col gap-2">
          {COLUMNS.map((col) => (
            <div key={col.id} className="flex items-center gap-2">
              <span
                className="text-[11px] font-medium flex-1 min-w-0 truncate"
                style={{ color: col.color }}
              >
                {col.label}
              </span>
              <input
                type="number"
                min={1}
                max={999}
                placeholder="—"
                value={wipConfig[col.id] ?? ""}
                onChange={(e) => {
                  const val = e.target.value.trim();
                  onUpdate(col.id, val ? parseInt(val, 10) : null);
                }}
                className="w-14 text-right text-[11px] px-1.5 py-0.5 rounded-[var(--r-sm)] border border-[var(--border)] bg-[var(--bg)] text-[var(--text-1)] focus:outline-none focus:border-[var(--accent)] tabular-nums"
                aria-label={`capacity limit for ${col.label}`}
              />
            </div>
          ))}
        </div>
        <div className="text-[10px] text-[var(--text-3)] mt-3 leading-snug">
          Leave blank to disable. Column turns red when over limit.
        </div>
      </PopoverContent>
    </Popover>
  );
}

/* ── Filter Bar ───────────────────────────────────── */

interface Filters {
  space: string | null;
  role: string | null;
}

function FilterBar({
  spaces,
  allTasks,
  filters,
  setFilters,
}: {
  spaces: ProjectSpaceSummary[];
  allTasks: Task[];
  filters: Filters;
  setFilters: (f: Filters) => void;
}) {
  const roles = useMemo(() => {
    const labels = new Map<string, string>();
    for (const t of allTasks) {
      if (t.assignedTo) labels.set(t.assignedTo, t.assignedToLabel || t.assignedTo);
    }
    return Array.from(labels.entries()).sort((a, b) => a[1].localeCompare(b[1]));
  }, [allTasks]);

  const hasFilters = filters.space !== null || filters.role !== null;

  return (
    <div className="flex items-center gap-2 pb-4 flex-wrap">
      <Filter size={13} className="text-[var(--text-3)] shrink-0" />

      <CustomSelect
        value={filters.space ?? ""}
        onChange={(v) => setFilters({ ...filters, space: v || null })}
        className="board-filter-select flex items-center gap-2 cursor-pointer"
        options={[
          { value: "", label: "All spaces" },
          ...spaces.map((t) => ({
            value: t.spaceId || t.spaceId,
            label: spaceDisplayName(t.spaceId, t.spaceName),
          })),
        ]}
      />

      <CustomSelect
        value={filters.role ?? ""}
        onChange={(v) => setFilters({ ...filters, role: v || null })}
        className="board-filter-select flex items-center gap-2 cursor-pointer"
        options={[
          { value: "", label: "All roles" },
          ...roles.map(([value, label]) => ({ value, label })),
        ]}
      />

      {hasFilters && (
        <button
          onClick={() => setFilters({ space: null, role: null })}
          className="inline-flex items-center gap-1 text-[11px] font-medium text-[var(--accent)] bg-none border-none cursor-pointer px-1.5 py-0.5 font-[inherit]"
        >
          <X size={11} />
          Clear
        </button>
      )}
    </div>
  );
}

/* ── CollapsibleContent ───────────────────────────── */

function CollapsibleContent({
  children,
  maxPx = 240,
}: {
  children: React.ReactNode;
  maxPx?: number;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [overflows, setOverflows] = useState(false);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const check = () => setOverflows(el.scrollHeight > maxPx + 4);
    check();
    const ro = new ResizeObserver(check);
    ro.observe(el);
    return () => ro.disconnect();
  }, [maxPx]);

  return (
    <div>
      <div
        ref={ref}
        style={{
          maxHeight: expanded ? undefined : maxPx,
          overflow: "hidden",
          position: "relative",
        }}
      >
        {children}
        {overflows && !expanded && (
          <div
            style={{
              position: "absolute",
              bottom: 0,
              left: 0,
              right: 0,
              height: 40,
              background:
                "linear-gradient(to bottom, transparent, var(--bg-elevated))",
              pointerEvents: "none",
            }}
          />
        )}
      </div>
      {overflows && (
        <button
          onClick={() => setExpanded((v) => !v)}
          className="text-[10px] font-semibold text-[var(--accent)] bg-transparent border-none cursor-pointer font-[inherit] mt-1.5 px-0 py-0 hover:underline"
        >
          {expanded ? "Show less" : "Show more"}
        </button>
      )}
    </div>
  );
}

/* ── Task Detail Drawer ───────────────────────────── */

function TaskDetailDrawer({
  task,
  spaceLookup,
  onClose,
  projectId,
  krById,
  missionById,
  taskDecisions,
  taskEscalations,
}: {
  task: Task;
  spaceLookup: Map<string, ProjectSpaceSummary>;
  onClose: () => void;
  projectId?: string | null;
  krById: Map<string, KeyResultView>;
  missionById: Map<string, MissionView>;
  taskDecisions: DecisionView[];
  taskEscalations: EscalationView[];
}) {
  const space = lookupSpaceForTask(task, spaceLookup);
  const color = taskStatusColor(task.status);

  const acceptanceCriteria = getAcceptanceCriteria(task);
  const drawerRetry = parseRetryTask(task);
  const drawerTitle = drawerRetry.isRetry
    ? drawerRetry.originalGoal
    : task.title || task.description;

  return (
    <div className="board-drawer-overlay" onClick={onClose}>
      <div
        className="board-drawer animate-modal-pop"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Drawer header */}
        <div className="flex items-start gap-3 px-5 pt-5 pb-4 border-b border-[var(--border)]">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-2">
              <span
                className="text-[10px] font-semibold tracking-[0.04em] uppercase px-2 py-0.5 rounded-full"
                style={{
                  background: color
                    ? `color-mix(in srgb, ${color} 15%, transparent)`
                    : "var(--bg-elevated)",
                  color: color || "var(--text-2)",
                  border: `1px solid ${color ? `color-mix(in srgb, ${color} 30%, transparent)` : "var(--border)"}`,
                }}
              >
                {taskStatusLabel(task.status)}
              </span>
              <span className="text-[10px] text-[var(--text-3)] font-[var(--font-mono,monospace)]">
                #{taskIdShort(task.id)}
              </span>
            </div>
            <h3
              style={{
                margin: 0,
                fontSize: 14,
                fontWeight: 600,
                lineHeight: 1.5,
                color: "var(--text-1)",
                letterSpacing: "-0.01em",
              }}
            >
              {drawerTitle}
            </h3>
          </div>
          <button onClick={onClose} className="board-drawer-close">
            <X size={16} />
          </button>
        </div>

        {/* Drawer body */}
        <div className="px-5 py-4 overflow-y-auto flex-1 min-h-0">
          {/* Meta grid */}
          <div className="board-drawer-meta">
            {(task.assignedToLabel || task.assignedTo) && (
              <div className="board-drawer-meta-item">
                <span className="board-drawer-meta-label">Assignee</span>
                <span className="text-[11px] font-semibold uppercase text-[var(--accent)] tracking-[0.02em]">
                  {task.assignedToLabel || task.assignedTo}
                </span>
              </div>
            )}
            {space && (
              <div className="board-drawer-meta-item">
                <span className="board-drawer-meta-label">Space</span>
                <span className="text-xs text-[var(--text-1)]">
                  {spaceDisplayName(space.spaceId, space.spaceName)}
                </span>
              </div>
            )}
            <div className="board-drawer-meta-item">
              <span className="board-drawer-meta-label">Created</span>
              <span className="text-xs text-[var(--text-2)] flex items-center gap-1">
                <Clock size={11} />
                {relativeTime(task.createdAt)}
              </span>
            </div>
            {task.completedAt && (
              <div className="board-drawer-meta-item">
                <span className="board-drawer-meta-label">Completed</span>
                <span className="text-xs text-[var(--text-2)] flex items-center gap-1">
                  <Clock size={11} />
                  {relativeTime(task.completedAt)}
                </span>
              </div>
            )}
            {task.taskKind && (
              <div className="board-drawer-meta-item">
                <span className="board-drawer-meta-label">Kind</span>
                <span className="text-xs text-[var(--text-2)]">
                  {task.taskKind}
                </span>
              </div>
            )}
          </div>

          {/* Full goal (only shown separately when a title is present) */}
          {task.title &&
            task.description &&
            task.title !== task.description &&
            !drawerRetry.isRetry && (
              <div style={{ marginTop: 16 }}>
                <div className="board-drawer-section-label">Description</div>
                <div className="text-xs text-[var(--text-2)] leading-relaxed min-w-0">
                  <CollapsibleContent maxPx={320}>
                    <div className="md-prose">
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>
                        {task.description}
                      </ReactMarkdown>
                    </div>
                  </CollapsibleContent>
                </div>
              </div>
            )}

          {/* Acceptance Criteria */}
          {acceptanceCriteria.length > 0 && (() => {
            const acDone = acceptanceCriteria.filter((criterion) => criterion.satisfied).length;
            const acTotal = acceptanceCriteria.length;
            const acColor = acDone === acTotal ? "var(--green)" : acDone > 0 ? "var(--amber)" : "var(--text-3)";
            return (
              <div className="mt-4">
                <CollapsibleSection
                  storageKey={`board-ac-${task.id}`}
                  defaultOpen={true}
                  label={<>Acceptance Criteria <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>{acDone}/{acTotal}</span></>}
                  accent={acColor}
                >
                  <div className="bg-[var(--bg-elevated)] border border-[var(--border)] rounded-[var(--r-md)] px-3.5 py-2.5">
                    <ul className="m-0 p-0 list-none">
                      {acceptanceCriteria.map((criterion, i) => {
                        const isChecked = criterion.satisfied;
                        return (
                          <li key={criterion.id || i} className="flex items-start gap-2 py-0.5">
                            <span
                              className={`flex-shrink-0 w-3.5 h-3.5 mt-0.5 rounded-sm border flex items-center justify-center ${
                                isChecked
                                  ? "bg-[var(--green)] border-[var(--green)]"
                                  : "border-[var(--border-strong)] bg-[var(--bg)]"
                              }`}
                            >
                              {isChecked && (
                                <svg width="8" height="8" viewBox="0 0 10 10" fill="none">
                                  <path
                                    d="M2 5.5L4 7.5L8 3"
                                    stroke="white"
                                    strokeWidth="1.5"
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                  />
                                </svg>
                              )}
                            </span>
                            <span
                              className={`text-xs leading-relaxed ${
                                isChecked ? "text-[var(--text-3)] line-through" : "text-[var(--text-2)]"
                              }`}
                            >
                              {criterion.text}
                            </span>
                          </li>
                        );
                      })}
                    </ul>
                  </div>
                </CollapsibleSection>
              </div>
            );
          })()}

          {/* Related — KR, Mission, Decisions, Escalations */}
          {(() => {
            const kr = task.keyResultRef ? krById.get(task.keyResultRef) : undefined;
            const mission = kr ? missionById.get(kr.missionId) : undefined;
            const items: RelatedItem[] = [
              ...(kr ? [{ nodeId: kr.id, type: 'Key Result', title: kr.title, badge: `${Math.round(kr.progressPercent ?? 0)}%` }] : []),
              ...(mission ? [{ nodeId: mission.id, type: 'Mission', title: mission.title }] : []),
              ...taskDecisions.map(d => ({
                nodeId: `decision:${d.id}`,
                type: 'Decision',
                title: d.title,
                ...(d.confidence > 0 ? {
                  badge: `${Math.round(d.confidence * 100)}%`,
                  badgeColor: d.confidence >= 0.8 ? 'var(--green)' : d.confidence >= 0.6 ? 'var(--amber)' : 'var(--red)',
                } : {}),
              })),
              ...taskEscalations.map(e => ({
                nodeId: `escalation:${e.id}`,
                type: 'Escalation',
                title: e.title,
                badge: e.urgency,
                badgeColor: e.urgency === 'critical' ? 'var(--red)' : e.urgency === 'high' ? 'var(--amber)' : 'var(--text-3)',
              })),
            ];
            if (items.length === 0) return null;
            return (
              <div className="mt-4">
                <RelatedSection items={items} grouped />
              </div>
            );
          })()}

          {/* Latest review */}
          {(() => {
            const review = getLatestReview(task);
            if (!review) return null;
            const decisionTone: Record<string, { fg: string; bg: string; label: string }> = {
              approved: {
                fg: "var(--green)",
                bg: "var(--green-dim)",
                label: "Approved",
              },
              retry: {
                fg: "var(--amber)",
                bg: "var(--amber-dim)",
                label: "Retry Requested",
              },
              escalated: {
                fg: "var(--red)",
                bg: "var(--red-dim,rgba(239,68,68,0.08))",
                label: "Escalated",
              },
              escalate: {
                fg: "var(--red)",
                bg: "var(--red-dim,rgba(239,68,68,0.08))",
                label: "Escalated",
              },
            };
            const tone = decisionTone[review.decision] ?? {
              fg: "var(--text-1)",
              bg: "var(--bg-elevated)",
              label: review.decision,
            };
            return (
              <div className="mt-4">
                <div className="board-drawer-section-label">Latest Review</div>
                <div className="bg-[var(--bg-elevated)] border border-[var(--border)] rounded-[var(--r-md)] px-3.5 py-2.5 mt-1.5">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span
                      className="text-[10px] font-bold tracking-[0.06em] uppercase px-2 py-0.5 rounded-full"
                      style={{ color: tone.fg, background: tone.bg }}
                    >
                      {tone.label}
                    </span>
                    {(review.reviewerRole || review.reviewedBy) && (
                      <span className="text-[11px] text-[var(--text-3)]">
                        by {review.reviewerRole || review.reviewedBy}
                      </span>
                    )}
                    {review.reviewedAt && (
                      <span className="text-[11px] text-[var(--text-3)]">
                        {relativeTime(review.reviewedAt)}
                      </span>
                    )}
                  </div>
                  {review.feedback && (
                    <div className="text-xs text-[var(--text-2)] leading-relaxed mt-2 whitespace-pre-wrap">
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>
                        {review.feedback}
                      </ReactMarkdown>
                    </div>
                  )}
                </div>
              </div>
            );
          })()}

          {/* Heartbeat section (F26) — shown for heartbeat tasks */}
          {isHeartbeatTask(task) && (
            <div className="mt-4">
              <div className="board-drawer-section-label">
                {"\u2661"} Heartbeat
              </div>
              <div className="flex flex-col gap-1.5 text-[12px]">
                {Boolean(task.metadata?.jobName) && (
                  <div className="flex gap-2">
                    <span className="text-[var(--text-3)] w-[75px] shrink-0">
                      Job
                    </span>
                    <span className="text-[var(--text-1)] font-[var(--font-mono,monospace)]">
                      {String(task.metadata!.jobName)}
                    </span>
                  </div>
                )}
                {Boolean(task.metadata?.occurrenceNumber) && (
                  <div className="flex gap-2">
                    <span className="text-[var(--text-3)] w-[75px] shrink-0">
                      Occurrence
                    </span>
                    <span className="text-[var(--text-1)]">
                      #{String(task.metadata!.occurrenceNumber)}
                      {Boolean(task.metadata!.maxOccurrences) &&
                        String(task.metadata!.maxOccurrences) !== "0" &&
                        ` of ${String(task.metadata!.maxOccurrences)}`}
                    </span>
                  </div>
                )}
                {Boolean(task.metadata?.scheduleExpression) && (
                  <div className="flex gap-2">
                    <span className="text-[var(--text-3)] w-[75px] shrink-0">
                      Schedule
                    </span>
                    <span className="text-[var(--text-1)]">
                      {String(task.metadata!.scheduleExpression)}
                    </span>
                  </div>
                )}
                {(() => {
                  const outcome = getHeartbeatOutcome(task);
                  if (!outcome) return null;
                  const outcomeColors: Record<string, string> = {
                    ok: "var(--green)",
                    warning: "var(--amber)",
                    critical: "var(--red)",
                    error: "#991b1b",
                  };
                  return (
                    <div className="flex gap-2">
                      <span className="text-[var(--text-3)] w-[75px] shrink-0">
                        Outcome
                      </span>
                      <span
                        style={{
                          color:
                            outcomeColors[outcome.status] ?? "var(--text-2)",
                        }}
                      >
                        {outcome.status} — &ldquo;{outcome.summary}&rdquo;
                      </span>
                    </div>
                  );
                })()}
                <a
                  href={calendarLink(
                    projectId ?? "",
                    task.id,
                    task.completedAt ?? task.createdAt,
                  )}
                  className="text-[11px] text-[var(--accent)] hover:underline self-start mt-1"
                >
                  View in calendar &rarr;
                </a>
              </div>
            </div>
          )}

          {/* Activity timeline — canonical task audit trail */}
          <TaskActivityTimeline task={task} />

          {/* Error */}
          {task.error && (
            <div className="mt-4">
              <div className="board-drawer-section-label text-[var(--red)]">
                <AlertTriangle size={11} className="inline align-middle mr-1" />
                Error
              </div>
              <div className="text-xs text-[var(--red)] leading-relaxed bg-[var(--red-dim,rgba(239,68,68,0.08))] rounded-[var(--r-md)] px-3.5 py-2.5 border border-[rgba(239,68,68,0.2)] whitespace-pre-wrap font-[var(--font-mono,monospace)]">
                {task.error}
              </div>
            </div>
          )}

          {/* Block reason */}
          {task.status === "blocked" && Boolean(task.metadata?.blockReason) && (
            <div className="mt-4">
              <div className="board-drawer-section-label text-[var(--amber)]">
                <AlertTriangle size={11} className="inline align-middle mr-1" />
                Blocked
              </div>
              <div className="text-xs text-[var(--amber)] leading-relaxed bg-[var(--amber-dim)] rounded-[var(--r-md)] px-3.5 py-2.5 whitespace-pre-wrap">
                {String(task.metadata?.blockReason ?? "")}
              </div>
            </div>
          )}

          {/* Artifacts */}
          {task.artifacts && task.artifacts.length > 0 && (
            <div className="mt-4">
              <div className="board-drawer-section-label">Artifacts</div>
              <div className="flex flex-col gap-1">
                {task.artifacts.map((a, i) => (
                  <div
                    key={i}
                    className="text-[11px] text-[var(--accent)] font-[var(--font-mono,monospace)] px-2 py-1 bg-[var(--bg-elevated)] rounded-[var(--r-sm)] border border-[var(--border)]"
                  >
                    {a}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function activityKindLabel(kind: string): string {
  switch (kind) {
    case "tool_call":
      return "Tool";
    case "oa_created":
      return "OA";
    case "oa_completed":
      return "OA Done";
    case "decision_logged":
      return "Decision";
    case "escalation_created":
      return "Escalation";
    case "escalation_resolved":
      return "Resolved";
    case "message_sent":
      return "Message";
    case "sub_task_created":
      return "Sub-task";
    default:
      return "State";
  }
}

function activityKindColor(kind: string): string {
  switch (kind) {
    case "decision_logged":
      return "var(--green)";
    case "escalation_created":
      return "var(--red)";
    case "escalation_resolved":
      return "var(--green)";
    case "oa_created":
      return "var(--amber)";
    case "oa_completed":
      return "var(--green)";
    case "tool_call":
      return "var(--accent)";
    case "message_sent":
      return "var(--text-2)";
    case "sub_task_created":
      return "var(--accent)";
    default:
      return "var(--text-3)";
  }
}

function ActivityEntry({ activity }: { activity: TaskActivity }) {
  const details = activity.details
    ? Object.entries(activity.details).filter(
        ([, value]) => value !== "" && value != null,
      )
    : [];
  return (
    <div className="flex gap-3">
      <div className="w-10 shrink-0 pt-0.5">
        <div className="text-[10px] text-[var(--text-3)]">
          {relativeTime(activity.timestamp)}
        </div>
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5 flex-wrap mb-1">
          <span
            className="text-[10px] font-semibold uppercase tracking-[0.04em]"
            style={{ color: activityKindColor(activity.kind) }}
          >
            {activityKindLabel(activity.kind)}
          </span>
          <span className="text-[10px] text-[var(--text-3)]">·</span>
          <span className="text-[10px] text-[var(--text-2)]">
            {activity.actor}
          </span>
        </div>
        <div className="text-xs text-[var(--text-2)] leading-relaxed">
          {activity.summary}
        </div>
        {details.length > 0 && (
          <details className="mt-1.5">
            <summary className="text-[10px] font-semibold text-[var(--accent)] cursor-pointer">
              Details
            </summary>
            <div className="mt-1.5 bg-[var(--bg-elevated)] border border-[var(--border)] rounded-[var(--r-md)] px-3 py-2">
              <div className="flex flex-col gap-1">
                {details.map(([key, value]) => (
                  <div
                    key={key}
                    className="text-[11px] text-[var(--text-2)] break-words"
                  >
                    <span className="font-semibold text-[var(--text-1)]">
                      {key}:
                    </span>{" "}
                    {String(value)}
                  </div>
                ))}
              </div>
            </div>
          </details>
        )}
      </div>
    </div>
  );
}

function TaskActivityTimeline({ task }: { task: Task }) {
  const activities = getTaskActivities(task);
  const [showOlder, setShowOlder] = useState(false);
  const visibleActivities = showOlder ? activities : activities.slice(-8);
  const hiddenCount = activities.length - visibleActivities.length;

  return (
    <div className="mt-4">
      <div className="board-drawer-section-label">Activity</div>
      <div className="flex flex-col gap-3 mt-2 pl-3 border-l-2 border-[var(--border)]">
        {hiddenCount > 0 && (
          <button
            onClick={() => setShowOlder((v) => !v)}
            className="text-[10px] font-semibold text-[var(--accent)] bg-transparent border-none cursor-pointer font-[inherit] px-0 py-0 hover:underline text-left"
          >
            {showOlder
              ? "Hide earlier activity"
              : `Show ${hiddenCount} earlier event${hiddenCount > 1 ? "s" : ""}`}
          </button>
        )}
        {visibleActivities.length === 0 ? (
          <div className="text-xs text-[var(--text-3)] italic">
            No activity recorded yet.
          </div>
        ) : (
          visibleActivities.map((activity) => (
            <ActivityEntry
              key={
                activity.eventId ?? `${activity.timestamp}:${activity.summary}`
              }
              activity={activity}
            />
          ))
        )}
      </div>
    </div>
  );
}

/* ── Board Column ──────────────────────────────────── */

function BoardColumn({
  column,
  tasks,
  spaceLookup,
  operatorActionById,
  krById,
  missionById,
  decisionCountByTask,
  escalationCountByTask,
  onSelectTask,
  onOpenOperatorAction,
  selectedTaskId,
  wipLimit,
}: {
  column: Column;
  tasks: Task[];
  spaceLookup: Map<string, ProjectSpaceSummary>;
  operatorActionById: Map<string, OpActionView>;
  krById: Map<string, KeyResultView>;
  missionById: Map<string, MissionView>;
  decisionCountByTask: Map<string, number>;
  escalationCountByTask: Map<string, number>;
  onSelectTask: (task: Task) => void;
  onOpenOperatorAction: (actionId: string) => void;
  selectedTaskId: string | null;
  wipLimit?: number;
}) {
  const overLimit = wipLimit != null && tasks.length > wipLimit;
  const countLabel =
    wipLimit != null ? `${tasks.length}/${wipLimit}` : String(tasks.length);
  const countColor = overLimit ? "var(--red)" : column.color;
  const countBg = overLimit ? "var(--red-dim, rgba(239,68,68,0.08))" : column.dimColor;

  return (
    <div className="board-col flex-1 min-w-[180px] flex flex-col">
      {/* Column header */}
      <div
        className="board-col-header flex items-center gap-2 px-1 pb-2.5 mb-2.5"
        style={{ borderBottom: `2px solid ${overLimit ? "var(--red)" : column.color}` }}
      >
        <span
          className="text-xs font-bold tracking-[-0.01em]"
          style={{ color: overLimit ? "var(--red)" : column.color }}
        >
          {column.label}
        </span>
        <span
          className="text-[10px] font-semibold opacity-70 px-1.5 py-px rounded-full tabular-nums"
          style={{ color: countColor, background: countBg }}
          title={wipLimit != null ? `${tasks.length} tasks · limit: ${wipLimit}` : undefined}
        >
          {countLabel}
        </span>
      </div>

      {/* Cards */}
      <div className="board-col-cards">
        {tasks.map((task) => (
          <TaskCard
            key={task.id}
            task={task}
            spaceLookup={spaceLookup}
            accentColor={column.color}
            operatorActionById={operatorActionById}
            krById={krById}
            missionById={missionById}
            decisionCountByTask={decisionCountByTask}
            escalationCountByTask={escalationCountByTask}
            onSelect={onSelectTask}
            onOpenOperatorAction={onOpenOperatorAction}
            isSelected={selectedTaskId === task.id}
          />
        ))}
        {tasks.length === 0 && (
          <div className="board-col-empty-state">{column.emptyLabel}</div>
        )}
      </div>
    </div>
  );
}

/* ── Main Board Page ───────────────────────────────── */

export default function Board() {
  const { projectId } = useNavigation();
  const searchString = useSearch();
  const spacesQuery = useProjectSpaces(projectId);
  const spaces = useMemo(() => spacesQuery.data ?? [], [spacesQuery.data]);
  const tasksQuery = useProjectTasks(spaces);
  const pendingOpActionsQuery = usePendingOpActions(projectId);
  const missionsQuery = useMissions(projectId);
  const krMapQuery = useProjectKRs(projectId);
  const decisionsQuery = useRecentDecisions(projectId);
  const escalationsQuery = usePendingEscalations(projectId);
  const allTasks = useMemo(() => tasksQuery.data ?? [], [tasksQuery.data]);

  // Real-time task updates via SSE — invalidates board query on task events.
  useProjectTasksSSE();

  // column capacity config — persisted in localStorage per project.
  const [wipConfig, setWipConfig] = useState<WipConfig>(() =>
    loadWipConfig(projectId),
  );
  const updateWipLimit = useCallback(
    (columnId: string, limit: number | null) => {
      setWipConfig((prev) => {
        const next = { ...prev };
        if (limit == null || limit <= 0) {
          delete next[columnId];
        } else {
          next[columnId] = limit;
        }
        saveWipConfig(projectId, next);
        return next;
      });
    },
    [projectId],
  );

  const [filters, setFilters] = useState<Filters>({ space: null, role: null });
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [selectedOperatorActionId, setSelectedOperatorActionId] = useState<
    string | null
  >(null);
  const spaceQueryAppliedRef = useRef(false);

  useEffect(() => {
    if (spaceQueryAppliedRef.current) return;
    const raw = new URLSearchParams(searchString).get("space")?.trim();
    if (!raw || spaces.length === 0) return;
    const resolved = resolveBoardSpaceQueryParam(raw, spaces);
    if (!resolved) return;
    spaceQueryAppliedRef.current = true;
    const id = requestAnimationFrame(() =>
      setFilters((f) => ({ ...f, space: resolved })),
    );
    return () => cancelAnimationFrame(id);
  }, [searchString, spaces]);

  // Auto-open task detail drawer from ?task= query param (F26)
  const taskQueryAppliedRef = useRef(false);
  useEffect(() => {
    if (taskQueryAppliedRef.current) return;
    const taskIdParam = new URLSearchParams(searchString).get("task")?.trim();
    if (!taskIdParam || allTasks.length === 0) return;
    const task = allTasks.find((t) => t.id === taskIdParam);
    if (!task) return;
    taskQueryAppliedRef.current = true;
    const id = requestAnimationFrame(() => setSelectedTask(task));
    return () => cancelAnimationFrame(id);
  }, [searchString, allTasks]);

  const spaceLookup = useMemo(() => {
    const map = new Map<string, ProjectSpaceSummary>();
    for (const space of spaces) {
      map.set(space.spaceId, space);
      if (space.spaceId) map.set(space.spaceId, space);
    }
    return map;
  }, [spaces]);

  const operatorActionById = useMemo(() => {
    const map = new Map<string, OpActionView>();
    for (const action of pendingOpActionsQuery.data ?? []) {
      map.set(action.id, action);
    }
    return map;
  }, [pendingOpActionsQuery.data]);

  const missionById = useMemo(() => {
    const map = new Map<string, MissionView>();
    for (const m of missionsQuery.data ?? []) map.set(m.id, m);
    return map;
  }, [missionsQuery.data]);

  const krById = useMemo(
    () => krMapQuery.data ?? new Map<string, KeyResultView>(),
    [krMapQuery.data],
  );

  const decisionCountByTask = useMemo(() => {
    const map = new Map<string, number>();
    for (const d of decisionsQuery.data ?? []) {
      if (d.taskRef) map.set(d.taskRef, (map.get(d.taskRef) ?? 0) + 1);
    }
    return map;
  }, [decisionsQuery.data]);

  const escalationCountByTask = useMemo(() => {
    const map = new Map<string, number>();
    for (const e of escalationsQuery.data ?? []) {
      if (e.taskRef) map.set(e.taskRef, (map.get(e.taskRef) ?? 0) + 1);
    }
    return map;
  }, [escalationsQuery.data]);

  // Pre-filter decisions and escalations for the selected task (used by drawer)
  const taskDecisions = useMemo(() => {
    if (!selectedTask) return [] as DecisionView[];
    return (decisionsQuery.data ?? []).filter((d) => d.taskRef === selectedTask.id);
  }, [decisionsQuery.data, selectedTask]);

  const taskEscalations = useMemo(() => {
    if (!selectedTask) return [] as EscalationView[];
    return (escalationsQuery.data ?? []).filter((e) => e.taskRef === selectedTask.id);
  }, [escalationsQuery.data, selectedTask]);

  const isSystemTask = useCallback((t: Task) => isSystemTaskFn(t), []);

  // Filter: real work tasks only, plus resolve review status
  const filteredTasks = useMemo(() => {
    return allTasks.filter((t) => {
      if (isSystemTask(t)) return false;
      if (filters.space && !taskMatchesSpaceFilter(t, filters.space)) return false;
      if (filters.role && t.assignedTo !== filters.role) return false;
      return true;
    });
  }, [allTasks, filters, isSystemTask]);

  const columnData = useMemo(() => {
    return COLUMNS.map((col) => ({
      column: col,
      tasks: filteredTasks
        .filter((t) => col.statuses.includes(t.status))
        .sort(
          (a, b) =>
            new Date(b.createdAt ?? 0).getTime() - new Date(a.createdAt ?? 0).getTime(),
        ),
    }));
  }, [filteredTasks]);

  const onSelectTask = useCallback((task: Task) => {
    setSelectedTask((prev) => (prev?.id === task.id ? null : task));
  }, []);

  const totalTasks = filteredTasks.length;
  const isFiltered = filters.space !== null || filters.role !== null;

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="board-header">
        <div className="flex items-center gap-3">
          <h1 className="m-0 text-[22px] font-bold tracking-[-0.04em] text-[var(--text-1)]">
            Board
          </h1>
          <span
            style={{
              fontSize: 11,
              color: "var(--text-3)",
              fontVariantNumeric: "tabular-nums",
              display: "flex",
              alignItems: "center",
              gap: 6,
            }}
          >
            {(() => {
              const doneCount = filteredTasks.filter((t) =>
                ["succeeded"].includes(t.status),
              ).length;
              return doneCount > 0
                ? `${doneCount} of ${totalTasks} done`
                : `${totalTasks} task${totalTasks !== 1 ? "s" : ""}`;
            })()}
            {isFiltered && (
              <span style={{ color: "var(--accent)" }}>(filtered)</span>
            )}
            <span style={{ color: "var(--border-strong)" }}>&middot;</span>
            {spaces.length} space{spaces.length !== 1 ? "s" : ""}
          </span>
        </div>

        {/* Quick stats + column limit settings */}
        <div className="flex items-center gap-2 ml-auto">
          {totalTasks > 0 && (
            <div className="flex gap-1.5">
              {columnData
                .filter((c) => c.tasks.length > 0)
                .map(({ column, tasks }) => (
                  <span
                    key={column.id}
                    className="text-[10px] font-semibold px-2 py-0.5 rounded-full tabular-nums"
                    style={{
                      background: column.dimColor,
                      color: column.color,
                    }}
                  >
                    {tasks.length} {column.label.toLowerCase()}
                  </span>
                ))}
            </div>
          )}
          <WipSettingsPopover wipConfig={wipConfig} onUpdate={updateWipLimit} />
        </div>
      </div>

      {/* Filter bar */}
      {spaces.length > 0 && allTasks.length > 0 && (
        <div className="px-7">
          <FilterBar
            spaces={spaces}
            allTasks={allTasks}
            filters={filters}
            setFilters={setFilters}
          />
        </div>
      )}

      {/* Board columns */}
      {tasksQuery.isLoading && allTasks.length === 0 ? (
        <div className="flex items-center justify-center flex-1">
          <span className="spinner spinner-md" />
        </div>
      ) : spaces.length === 0 ? (
        <div className="flex flex-col items-center justify-center flex-1 gap-3">
          <div className="w-12 h-12 rounded-[14px] bg-[var(--bg-elevated)] border border-[var(--border)] flex items-center justify-center">
            <Columns3 size={22} className="text-[var(--text-3)]" />
          </div>
          <div className="text-[13px] text-[var(--text-3)] text-center max-w-[320px]">
            No active spaces. Start a space from the sidebar to see tasks on the board.
          </div>
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-auto px-7 pb-4">
          {/* Filtered out everything hint */}
          {isFiltered && filteredTasks.length === 0 && allTasks.length > 0 && (
            <div className="flex flex-col items-center gap-2 py-8 text-center">
              <div className="text-[13px] text-[var(--text-3)]">
                No tasks match the current filters
              </div>
              <button
                onClick={() => setFilters({ space: null, role: null })}
                className="text-xs text-[var(--accent)] bg-transparent border-none cursor-pointer font-[inherit] hover:underline"
              >
                Clear filters
              </button>
            </div>
          )}
          <SectionErrorBoundary>
            <div className="flex gap-3.5 items-start">
              {columnData.map(({ column, tasks }) => (
                <BoardColumn
                  key={column.id}
                  column={column}
                  tasks={tasks}
                  spaceLookup={spaceLookup}
                  onSelectTask={onSelectTask}
                  selectedTaskId={selectedTask?.id ?? null}
                  operatorActionById={operatorActionById}
                  krById={krById}
                  missionById={missionById}
                  decisionCountByTask={decisionCountByTask}
                  escalationCountByTask={escalationCountByTask}
                  onOpenOperatorAction={setSelectedOperatorActionId}
                  wipLimit={wipConfig[column.id]}
                />
              ))}
            </div>
          </SectionErrorBoundary>
        </div>
      )}

      {/* Task detail drawer */}
      {selectedTask && (
        <TaskDetailDrawer
          task={selectedTask}
          spaceLookup={spaceLookup}
          onClose={() => setSelectedTask(null)}
          projectId={projectId}
          krById={krById}
          missionById={missionById}
          taskDecisions={taskDecisions}
          taskEscalations={taskEscalations}
        />
      )}

      <OpActionDetailPanel
        actionId={selectedOperatorActionId}
        projectId={projectId}
        onClose={() => setSelectedOperatorActionId(null)}
      />
    </div>
  );
}
