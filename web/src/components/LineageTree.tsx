import { useState, useMemo } from 'react'
import clsx from 'clsx'
import type { Task, ProjectSpaceSummary } from '../lib/types'
import { spaceDisplayName } from '../lib/spaceDisplayName'
import { ChevronRight, GitBranch } from 'lucide-react'

/* ── Tree node type ────────────────────────────────── */

interface TreeNode {
  task: Task
  children: TreeNode[]
}

/* ── Helpers ───────────────────────────────────────── */

function buildForest(tasks: Task[]): TreeNode[] {
  const roots: TreeNode[] = tasks.map((task) => ({ task, children: [] }))

  // Sort children by creation time
  function sortChildren(node: TreeNode) {
    node.children.sort((a, b) => new Date(a.task.createdAt ?? 0).getTime() - new Date(b.task.createdAt ?? 0).getTime())
    for (const child of node.children) sortChildren(child)
  }
  for (const root of roots) sortChildren(root)

  roots.sort((a, b) => new Date(b.task.createdAt ?? 0).getTime() - new Date(a.task.createdAt ?? 0).getTime())
  return roots
}

function statusBadge(status: string): { bg: string; fg: string } {
  switch (status) {
    case 'succeeded': return { bg: 'var(--green-dim)', fg: 'var(--green)' }
    case 'failed': return { bg: 'var(--red-dim)', fg: 'var(--red)' }
    case 'canceled': return { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' }
    case 'active':
    case 'delegated':
    case 'resumed': return { bg: 'var(--accent-dim)', fg: 'var(--accent)' }
    case 'blocked': return { bg: 'var(--amber-dim)', fg: 'var(--amber)' }
    case 'in_review': return { bg: 'var(--accent-dim)', fg: 'var(--accent)' }
    default: return { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' }
  }
}

/* ── Tree node component ───────────────────────────── */

function TreeNodeRow({ node, depth, spaceLookup }: {
  node: TreeNode
  depth: number
  spaceLookup: Map<string, ProjectSpaceSummary>
}) {
  const [collapsed, setCollapsed] = useState(depth > 1)
  const hasChildren = node.children.length > 0
  const task = node.task
  const badge = statusBadge(task.status)
  const space = task.spaceId ? spaceLookup.get(task.spaceId) : null
  return (
    <div>
      <button
        type="button"
        style={{
          paddingLeft: 8 + depth * 20,
        }}
        className={clsx(
          'row-hover w-full text-left bg-transparent border-none font-[inherit]',
          'flex items-center gap-1.5 py-[5px] px-2',
          'rounded-[var(--r-sm)] transition-[background] duration-[120ms]',
          hasChildren ? 'cursor-pointer' : 'cursor-default',
        )}
        onClick={() => hasChildren && setCollapsed(c => !c)}
        aria-expanded={hasChildren ? !collapsed : undefined}
      >
        {/* Expand/collapse */}
        {hasChildren ? (
          <ChevronRight
            size={10}
            className={clsx(
              'text-[var(--text-3)] shrink-0 transition-transform duration-150',
              collapsed ? 'rotate-0' : 'rotate-90',
            )}
          />
        ) : (
          <span className="w-[10px] shrink-0" />
        )}

        {/* Status badge */}
        <span
          className="text-[9px] font-semibold tracking-[0.04em] uppercase px-[5px] py-px rounded-full shrink-0"
          style={{ background: badge.bg, color: badge.fg }}
        >
          {task.status}
        </span>

        {/* Goal */}
        <span className="truncate text-[11px] text-[var(--text-1)] flex-1">
          {task.description}
        </span>

        {/* Space badge */}
        {space && (
          <span
            className="text-[9px] font-semibold tracking-[0.03em] uppercase px-[5px] py-px rounded-full border border-[var(--border)] shrink-0"
            style={{
              background: 'var(--bg-elevated)',
              color: 'var(--text-3)',
            }}
          >
            {spaceDisplayName(space.spaceId, space.spaceName)}
          </span>
        )}

        {/* Role */}
        {task.assignedTo && (
          <span className="text-[10px] text-[var(--text-3)] shrink-0">
            {task.assignedTo}
          </span>
        )}
      </button>

      {/* Children */}
      {!collapsed && hasChildren && (
        <div className="animate-fade-in">
          {node.children.map(child => (
            <TreeNodeRow key={child.task.id} node={child} depth={depth + 1} spaceLookup={spaceLookup} />
          ))}
        </div>
      )}
    </div>
  )
}

/* ── Main component ────────────────────────────────── */

export default function LineageTree({ tasks, spaceLookup }: {
  tasks: Task[]
  spaceLookup: Map<string, ProjectSpaceSummary>
}) {
  const forest = useMemo(() => buildForest(tasks), [tasks])

  return (
    <div className="bg-[var(--bg-panel)] border border-[var(--border)] rounded-[var(--r-lg)] flex flex-col overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-2 py-2.5 px-3.5 border-b border-[var(--border)] shrink-0">
        <GitBranch size={14} className="text-[var(--text-3)]" />
        <span className="text-[13px] font-semibold text-[var(--text-1)]">Task Lineage</span>
        <span className="text-[10px] text-[var(--text-3)]">{tasks.length} tasks</span>
      </div>

      {/* Tree */}
      <div className="flex-1 overflow-y-auto py-1">
        {forest.length === 0 ? (
          <div className="p-6 text-center text-[11px] text-[var(--text-3)]">
            No tasks with delegation chains
          </div>
        ) : (
          forest.map(root => (
            <TreeNodeRow key={root.task.id} node={root} depth={0} spaceLookup={spaceLookup} />
          ))
        )}
      </div>
    </div>
  )
}
