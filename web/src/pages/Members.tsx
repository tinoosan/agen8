import { useMemo, useState, type ReactNode } from 'react'
import { Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { useNavigation } from '../lib/routing'
import { useProjectMembers, useRemoveMember } from '../hooks/useProjectMembers'
import { memberDisplayName } from '../lib/memberDisplay'
import { cn } from '@/lib/utils'
import { formatRelative } from '@/lib/format'
import { isUuid } from '@/lib/displaySanitizers'
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
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Skeleton } from '@/components/ui/skeleton'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogFooter,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogTrigger,
  AlertDialogCancel,
} from '@/components/ui/alert-dialog'

/* ── Helpers ─────────────────────────────────────────── */

// A machine-issued native ref is a session UUID or a bridge-<hex> stdio ref.
// Anything else ("claude-frontend-engineer", "dogfood-A") was typed by a human
// at register time — a frequent source of accidental duplicate members, so the
// roster hints it.
function isHandTypedRef(ref?: string): boolean {
  const r = (ref ?? '').trim()
  if (!r) return false
  if (isUuid(r)) return false
  if (/^bridge-[0-9a-f]{6,}$/i.test(r)) return false
  return true
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
  const members = useMemo(() => data ?? [], [data])

  const active = useMemo(
    () => members.filter((m) => m.lifecycleState === 'active'),
    [members],
  )
  const removed = useMemo(
    () => members.filter((m) => m.lifecycleState === 'removed'),
    [members],
  )
  const dupeIds = useMemo(() => findDuplicateIds(active), [active])
  const handTypedCount = useMemo(
    () => active.filter((m) => isHandTypedRef(m.nativeSessionRef)).length,
    [active],
  )

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-6">
        <div className="flex flex-col gap-1">
          <h1 className="m-0 hidden text-2xl font-bold text-[var(--text-1)] md:block">
            Members
          </h1>
          <p className="m-0 max-w-prose text-[0.8125rem] leading-relaxed text-[var(--text-3)]">
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
                <div className="flex flex-col gap-4">
                  <div className="grid grid-cols-3 gap-2 sm:max-w-lg">
                    <StatTile label="Active" value={active.length} />
                    <StatTile
                      label="Possible dupes"
                      value={dupeIds.size}
                      tone="danger"
                    />
                    <StatTile
                      label="Hand-typed"
                      value={handTypedCount}
                      tone="warning"
                    />
                  </div>
                  <MemberRoster members={active} dupeIds={dupeIds} />
                </div>
              )}
            </TabsContent>

            <TabsContent value="removed">
              {removed.length === 0 ? (
                <EmptyState text="No removed members." />
              ) : (
                <MemberRoster members={removed} dupeIds={EMPTY_IDS} removed />
              )}
            </TabsContent>
          </Tabs>
        )}
      </div>
    </div>
  )
}

const EMPTY_IDS: Set<string> = new Set()

/* ── Roster (responsive) ─────────────────────────────────
 * A data table can't reflow its seven columns, so narrow widths get the same
 * roster as stacked cards and only wide widths get the table. The switch is a
 * CONTAINER query, not a viewport one: the inline sidebar eats ~272px, so an
 * iPad-width *viewport* still leaves the roster far less room than its pixel
 * count implies. Querying the roster's own width (≥720px ⇒ the 7-col table fits
 * with breathing room) keeps that decision honest — and makes the table return
 * on its own if the sidebar is collapsed to its icon rail. */

function MemberRoster(props: {
  members: ProjectMember[]
  dupeIds: Set<string>
  removed?: boolean
}) {
  return (
    <div className="@container">
      <div className="flex flex-col gap-3 @min-[720px]:hidden">
        {props.members.map((m) => (
          <MemberCard
            key={m.id}
            m={m}
            isDupe={props.dupeIds.has(m.id)}
            removed={props.removed}
          />
        ))}
      </div>
      <MemberTable {...props} />
    </div>
  )
}

/* ── Card (mobile/narrow) ────────────────────────────── */

function MemberCard({
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
    <div
      className={cn(
        'flex flex-col gap-3 rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-surface)] p-4',
        removed && 'opacity-60',
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="font-medium text-[var(--text-1)]">{name}</span>
          {isDupe && <DupeBadge />}
        </div>
        {!removed && <RemoveMemberButton member={m} name={name} />}
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3">
        <CardField label="Harness" value={m.harnessKind || '—'} />
        <CardField label="Model" value={m.model || '—'} />
        <CardField label="Effort" value={m.effort || '—'} />
        <CardField
          label="Registered"
          value={formatRelative(m.registeredAt, { seconds: true, fallback: '—' })}
          title={absTime(m.registeredAt)}
        />
      </dl>
      <div className="flex flex-col gap-1">
        <CardLabel>Session ref</CardLabel>
        <div className="flex flex-wrap items-center gap-1.5">
          <code className="break-all text-[0.75rem] text-[var(--text-3)]">
            {m.nativeSessionRef || '—'}
          </code>
          {handTyped && <HandTypedBadge />}
        </div>
      </div>
    </div>
  )
}

function CardLabel({ children }: { children: ReactNode }) {
  return (
    <dt className="text-[0.625rem] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)]">
      {children}
    </dt>
  )
}

function CardField({
  label,
  value,
  title,
}: {
  label: string
  value: string
  title?: string
}) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <CardLabel>{label}</CardLabel>
      <dd className="m-0 truncate text-[0.8125rem] text-[var(--text-2)]" title={title}>
        {value}
      </dd>
    </div>
  )
}

/* ── Table (wide containers) ─────────────────────────── */

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
    <div className="hidden overflow-hidden rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-surface)] @min-[720px]:block">
      <Table>
        <TableHeader>
          <TableRow className="border-[var(--border)] hover:bg-transparent">
            <Th>Member</Th>
            <Th>Harness</Th>
            <Th>Model</Th>
            <Th>Effort</Th>
            <Th>Session ref</Th>
            <Th>Registered</Th>
            {!removed && (
              <TableHead className="w-[1%] px-4">
                <span className="sr-only">Actions</span>
              </TableHead>
            )}
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

function Th({ children }: { children: ReactNode }) {
  return (
    <TableHead className="h-auto px-4 py-2.5 text-[0.625rem] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)]">
      {children}
    </TableHead>
  )
}

function Td({ children }: { children: ReactNode }) {
  return <TableCell className="px-4 py-3 text-[var(--text-2)]">{children}</TableCell>
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
    <TableRow
      className={cn(
        'group border-[var(--border)] hover:bg-[var(--bg-hover)]',
        removed && 'opacity-60',
      )}
    >
      <TableCell className="px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="font-medium text-[var(--text-1)]">{name}</span>
          {isDupe && <DupeBadge />}
        </div>
      </TableCell>
      <Td>{m.harnessKind || '—'}</Td>
      <Td>{m.model || '—'}</Td>
      <Td>{m.effort || '—'}</Td>
      <TableCell className="px-4 py-3">
        <div className="flex items-center gap-1.5">
          <code className="break-all text-[0.75rem] text-[var(--text-3)]">
            {m.nativeSessionRef || '—'}
          </code>
          {handTyped && <HandTypedBadge />}
        </div>
      </TableCell>
      <TableCell className="px-4 py-3">
        <span
          className="whitespace-nowrap text-[0.8125rem] text-[var(--text-3)]"
          title={absTime(m.registeredAt)}
        >
          {formatRelative(m.registeredAt, { seconds: true, fallback: '—' })}
        </span>
      </TableCell>
      {!removed && (
        <TableCell className="px-4 py-3 text-right">
          <RemoveMemberButton member={m} name={name} revealOnHover />
        </TableCell>
      )}
    </TableRow>
  )
}

/* ── Remove action ───────────────────────────────────── */

function RemoveMemberButton({
  member,
  name,
  revealOnHover,
}: {
  member: ProjectMember
  name: string
  revealOnHover?: boolean
}) {
  const [open, setOpen] = useState(false)
  const removeMember = useRemoveMember()

  const onConfirm = async () => {
    try {
      await removeMember.mutateAsync({ memberId: member.id })
      toast.success(`Removed ${name}`)
      setOpen(false)
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to remove member',
      )
    }
  }

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        // Don't let an outside-click / Esc dismiss mid-removal.
        if (!removeMember.isPending) setOpen(next)
      }}
    >
      <AlertDialogTrigger asChild>
        <Button
          variant="ghost-danger"
          size="icon"
          aria-label={`Remove ${name}`}
          className={cn(
            'transition-opacity focus-visible:opacity-100',
            // Hide-until-hover only where a pointer can actually hover. On touch
            // (iPad, phones) there's no hover, so the button stays visible and
            // the remove action remains reachable.
            revealOnHover &&
              '[@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100',
          )}
        >
          <Trash2 size={14} />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove {name}?</AlertDialogTitle>
          <AlertDialogDescription>
            This marks the member as removed and drops it from the active
            roster. Anything it created — decisions, tasks, graph history — is
            preserved. This can't be undone from here.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={removeMember.isPending}>
            Cancel
          </AlertDialogCancel>
          <Button
            variant="destructive"
            onClick={onConfirm}
            disabled={removeMember.isPending}
          >
            {removeMember.isPending ? 'Removing…' : 'Remove member'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

/* ── Badges (shared by table + card) ─────────────────── */

function DupeBadge() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">
          <Badge variant="warning" className="cursor-default">
            possible duplicate
          </Badge>
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-[260px]">
        Another active member shares this name and harness. Compare the session
        refs and registration times to find the stale one.
      </TooltipContent>
    </Tooltip>
  )
}

function HandTypedBadge() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">
          <Badge variant="secondary" className="shrink-0 cursor-default">
            hand-typed
          </Badge>
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-[260px]">
        This session ref was typed by a human at register time, not issued by
        the harness. Hand-typed refs often create accidental duplicate members.
      </TooltipContent>
    </Tooltip>
  )
}

/* ── Small bits ──────────────────────────────────────── */

function StatTile({
  label,
  value,
  tone,
}: {
  label: string
  value: number
  tone?: 'danger' | 'warning'
}) {
  const valueColor =
    value > 0 && tone === 'danger'
      ? 'text-[var(--red)]'
      : value > 0 && tone === 'warning'
        ? 'text-[var(--amber)]'
        : 'text-[var(--text-1)]'
  return (
    <div className="rounded-[var(--r-md)] bg-[var(--bg-elevated)] px-3 py-2.5">
      <div className="text-[0.625rem] font-medium uppercase tracking-[0.04em] text-[var(--text-3)]">
        {label}
      </div>
      <div className={cn('mt-1 text-xl font-semibold tabular-nums', valueColor)}>
        {value}
      </div>
    </div>
  )
}

function CountPill({ n }: { n: number }) {
  return (
    <span className="ml-1.5 rounded-full bg-[var(--bg-active)] px-1.5 text-[0.6875rem] text-[var(--text-2)]">
      {n}
    </span>
  )
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
      {text}
    </div>
  )
}

function MembersSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <Skeleton key={i} className="h-12 w-full rounded-[var(--r-md)]" />
      ))}
    </div>
  )
}
