import { useMemo } from 'react'
import { useNavigation } from '../lib/routing'
import { useProjectMembers } from '../hooks/useProjectMembers'
import { memberDisplayName } from '../lib/memberDisplay'
import type { ProjectMember } from '../lib/types'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Skeleton } from '@/components/ui/skeleton'

/* ── Helpers ─────────────────────────────────────────── */

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

// A machine-issued native ref is a session UUID or a bridge-<hex> stdio ref.
// Anything else ("claude-frontend-engineer", "dogfood-A") was typed by a human
// at register time — a frequent source of accidental duplicate members, so the
// roster hints it.
function isHandTypedRef(ref?: string): boolean {
  const r = (ref ?? '').trim()
  if (!r) return false
  if (UUID_RE.test(r)) return false
  if (/^bridge-[0-9a-f]{6,}$/i.test(r)) return false
  return true
}

function timeAgo(iso?: string): string {
  if (!iso) return '—'
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return '—'
  const diffMs = Date.now() - t
  if (diffMs < 0) return 'just now'
  const s = Math.floor(diffMs / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d ago`
  return new Date(iso).toLocaleDateString()
}

function absTime(iso?: string): string {
  if (!iso) return 'unknown'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? 'unknown' : d.toLocaleString()
}

// Members sharing a (displayName, harnessKind) pair within the active set are
// likely the same operator registered more than once (dedup keys on the native
// session ref, not the name). Returns the ids that should carry the badge.
function findDuplicateIds(members: ProjectMember[]): Set<string> {
  const groups = new Map<string, ProjectMember[]>()
  for (const m of members) {
    const name = (m.displayName ?? '').trim().toLowerCase()
    if (!name) continue
    const key = `${name}|${(m.harnessKind ?? '').trim().toLowerCase()}`
    const arr = groups.get(key) ?? []
    arr.push(m)
    groups.set(key, arr)
  }
  const dupes = new Set<string>()
  for (const arr of groups.values()) {
    if (arr.length > 1) for (const m of arr) dupes.add(m.id)
  }
  return dupes
}

/* ── Page ────────────────────────────────────────────── */

export default function Members() {
  const { projectId } = useNavigation()
  const { data, isLoading, isError } = useProjectMembers(projectId)
  const members = data ?? []

  const active = useMemo(
    () => members.filter((m) => m.lifecycleState === 'active'),
    [members],
  )
  const removed = useMemo(
    () => members.filter((m) => m.lifecycleState === 'removed'),
    [members],
  )
  const dupeIds = useMemo(() => findDuplicateIds(active), [active])

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-8">
        <div className="flex flex-col gap-1">
          <h1 className="m-0 hidden text-2xl font-bold text-[var(--text-1)] md:block">
            Members
          </h1>
          <p className="m-0 text-[0.8125rem] text-[var(--text-3)]">
            Agent sessions registered to this project. A member is one harness
            session — the same name registered from a different session is a
            separate member.
          </p>
        </div>

        {!projectId ? (
          <EmptyState text="Select a project to view its members." />
        ) : isLoading ? (
          <MembersSkeleton />
        ) : isError ? (
          <EmptyState text="Couldn't load members." />
        ) : (
          <Tabs defaultValue="active" className="w-full">
            <TabsList>
              <TabsTrigger value="active">
                Active <CountPill n={active.length} />
              </TabsTrigger>
              <TabsTrigger value="removed">
                Removed <CountPill n={removed.length} />
              </TabsTrigger>
            </TabsList>
            <TabsContent value="active">
              {active.length === 0 ? (
                <EmptyState text="No active members." />
              ) : (
                <MemberTable members={active} dupeIds={dupeIds} />
              )}
            </TabsContent>
            <TabsContent value="removed">
              {removed.length === 0 ? (
                <EmptyState text="No removed members." />
              ) : (
                <MemberTable members={removed} dupeIds={EMPTY_IDS} removed />
              )}
            </TabsContent>
          </Tabs>
        )}
      </div>
    </div>
  )
}

const EMPTY_IDS: Set<string> = new Set()

/* ── Table ───────────────────────────────────────────── */

function MemberTable({
  members,
  dupeIds,
  removed,
}: {
  members: ProjectMember[]
  dupeIds: Set<string>
  removed?: boolean
}) {
  return (
    <div className="overflow-hidden rounded-[10px] border border-[var(--border)]">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Member</TableHead>
            <TableHead>Harness</TableHead>
            <TableHead>Model</TableHead>
            <TableHead>Effort</TableHead>
            <TableHead>Permission</TableHead>
            <TableHead>Session ref</TableHead>
            <TableHead>Registered</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {members.map((m) => (
            <MemberRow
              key={m.id}
              m={m}
              isDupe={dupeIds.has(m.id)}
              removed={removed}
            />
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function MemberRow({
  m,
  isDupe,
  removed,
}: {
  m: ProjectMember
  isDupe: boolean
  removed?: boolean
}) {
  const name = memberDisplayName(m.displayName, m.id) ?? m.id
  const handTyped = isHandTypedRef(m.nativeSessionRef)

  return (
    <TableRow className={removed ? 'opacity-60' : undefined}>
      <TableCell>
        <div className="flex items-center gap-2">
          <span className="font-medium text-[var(--text-1)]">{name}</span>
          {isDupe && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  <Badge variant="warning" className="cursor-default">
                    possible duplicate
                  </Badge>
                </span>
              </TooltipTrigger>
              <TooltipContent className="max-w-[260px]">
                Another active member shares this name and harness. Compare the
                session refs and registration times to find the stale one.
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      </TableCell>
      <TableCell className="text-[var(--text-2)]">
        {m.harnessKind || '—'}
      </TableCell>
      <TableCell className="text-[var(--text-2)]">{m.model || '—'}</TableCell>
      <TableCell className="text-[var(--text-2)]">{m.effort || '—'}</TableCell>
      <TableCell className="text-[var(--text-2)]">
        {m.harnessPermissionMode || '—'}
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-1.5">
          <code className="break-all text-[0.75rem] text-[var(--text-3)]">
            {m.nativeSessionRef || '—'}
          </code>
          {handTyped && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  <Badge
                    variant="secondary"
                    className="shrink-0 cursor-default"
                  >
                    hand-typed
                  </Badge>
                </span>
              </TooltipTrigger>
              <TooltipContent className="max-w-[260px]">
                This session ref was typed by a human at register time, not
                issued by the harness. Hand-typed refs often create accidental
                duplicate members.
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      </TableCell>
      <TableCell>
        <span
          className="whitespace-nowrap text-[0.8125rem] text-[var(--text-3)]"
          title={absTime(m.registeredAt)}
        >
          {timeAgo(m.registeredAt)}
        </span>
      </TableCell>
    </TableRow>
  )
}

/* ── Small bits ──────────────────────────────────────── */

function CountPill({ n }: { n: number }) {
  return (
    <span className="ml-1.5 rounded-full bg-[var(--bg-active)] px-1.5 text-[0.6875rem] text-[var(--text-2)]">
      {n}
    </span>
  )
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="rounded-[10px] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
      {text}
    </div>
  )
}

function MembersSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <Skeleton key={i} className="h-12 w-full rounded-[8px]" />
      ))}
    </div>
  )
}
