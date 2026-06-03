/**
 * MemberList — the expandable member sublist under a focused space row.
 * Contains MemberRow (individual member with live-activity dot and
 * context menu) and MemberSubList (loading/error/empty states + sorted list).
 */
import { useLocation } from 'wouter'
import { Hash, MoreHorizontal, Trash2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useNavigation } from '../../lib/routing'
import { useSpaceMemberList } from '../../hooks/useSpace'
import { resolveMemberIdentity } from '../../lib/memberIdentity'
import type { SpaceMember } from '../../lib/types'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from '@/components/ui/sidebar'

const CHILD_INDENT = 'pl-7 pr-1'

/* ── Single member row ─────────────────────────────── */

/**
 * Single member row inside the member sublist. Extracted so the
 * per-row hooks (run-activity subscription, lastSeen tracking) can run
 * once per row without violating the rules of hooks.
 */
function MemberRow({
  member,
  spaceId,
  accentVar,
  isActive,
  projectId,
  navigate,
  onRequestRemoveMember,
}: {
  member: SpaceMember
  spaceId: string
  accentVar: string | undefined
  isActive: boolean
  projectId: string | null
  navigate: (to: string) => void
  onRequestRemoveMember: (memberId: string, displayName: string) => void
}) {
  const memberIdentity = resolveMemberIdentity(member.id)
  const KindIcon = memberIdentity ? memberIdentity.icon : Hash
  const iconStyle = memberIdentity ? { color: memberIdentity.colorVar } : undefined
  const displayName = member.displayName || member.memberType || 'Member'
  const removableMemberId = member.id.trim()
  const canRemove = removableMemberId !== ''

  const lastSeen = member.lastSeenAt?.trim()
  const dotColor = lastSeen ? (accentVar ?? 'var(--accent)') : null

  return (
    <SidebarMenuSubItem
      data-testid={`member-row-${member.id}`}
      className="group/channel relative flex items-center"
    >
      {/* Active accent bar */}
      {isActive && accentVar && (
        <span
          aria-hidden
          className="absolute left-0 top-1 bottom-1 w-[1.5px] rounded-full"
          style={{ backgroundColor: accentVar, opacity: 0.85 }}
        />
      )}
      <SidebarMenuSubButton
        size="sm"
        isActive={isActive}
        className={cn(
          CHILD_INDENT,
          'flex-1 min-w-0 gap-1.5 cursor-pointer h-[24px] rounded-[7px] bg-transparent hover:bg-transparent',
          isActive
            ? 'text-[var(--text-1)] bg-white/[0.03] hover:bg-white/[0.03]'
            : 'text-[var(--text-2)] hover:text-[var(--text-1)]',
        )}
        onClick={() => {
          if (projectId) {
            navigate(
              `/project/${encodeURIComponent(projectId)}/space/${encodeURIComponent(spaceId)}`
            )
          }
        }}
      >
        <KindIcon
          size={12}
          strokeWidth={1.75}
          aria-hidden
          className={cn('shrink-0', !iconStyle && 'text-[var(--text-3)]')}
          style={iconStyle}
        />
        <span
          className="flex-1 truncate text-[13px] font-normal capitalize"
          style={
            memberIdentity && !isActive
              ? { color: `color-mix(in srgb, var(--text-2) 78%, ${memberIdentity.colorVar} 22%)` }
              : undefined
          }
        >
          {displayName}
        </span>
        {dotColor && (
          <span
            aria-hidden
            className={cn(
              'shrink-0 h-1.5 w-1.5 rounded-full',
            )}
            style={{ backgroundColor: dotColor }}
            title={lastSeen ? `Last seen ${lastSeen}` : 'Seen'}
          />
        )}
      </SidebarMenuSubButton>
      {canRemove && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="shrink-0 mr-1 h-5 w-5 flex items-center justify-center rounded text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] opacity-0 group-hover/channel:opacity-100 focus:opacity-100 transition-opacity cursor-pointer border-0 bg-transparent"
              aria-label={`Member actions for ${displayName}`}
              data-testid={`member-actions-${member.id}`}
            >
              <MoreHorizontal size={11} aria-hidden />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="right" align="start" className="text-xs">
            <DropdownMenuItem
              onSelect={() => onRequestRemoveMember(removableMemberId, displayName)}
              className="text-[var(--red)] focus:text-[var(--red)]"
              data-testid={`remove-member-${member.id}`}
            >
              <Trash2 size={12} className="mr-2" aria-hidden />
              Remove member
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </SidebarMenuSubItem>
  )
}

/* ── Member sublist (loading, error, list) ─────────── */

export function MemberSubList({
  spaceId,
  accentVar,
  onRequestRemoveMember,
}: {
  spaceId: string
  accentVar: string | undefined
  onRequestRemoveMember: (memberId: string, displayName: string) => void
}) {
  const membersQuery = useSpaceMemberList({ spaceId, enabled: !!spaceId })
  const members = membersQuery.data ?? []
  const { projectId } = useNavigation()
  const [, navigate] = useLocation()

  if (membersQuery.isLoading) {
    return (
      <SidebarMenuSubItem>
        <div className={cn(CHILD_INDENT, 'py-1 text-[10px] text-[var(--text-3)] flex items-center gap-1')}>
          <span className="spinner w-2 h-2 border-[1.5px] shrink-0" aria-hidden />
          Loading…
        </div>
      </SidebarMenuSubItem>
    )
  }

  if (membersQuery.isError) {
    return (
      <SidebarMenuSubItem>
        <div className={cn(CHILD_INDENT, 'py-1 text-[10px] text-[var(--red)] flex items-center gap-1.5')}>
          <span>Members unavailable</span>
          <button
            type="button"
            className="underline cursor-pointer bg-transparent border-0 p-0 font-inherit text-inherit leading-none"
            onClick={() => void membersQuery.refetch()}
            data-testid="member-list-retry"
          >
            Retry
          </button>
        </div>
      </SidebarMenuSubItem>
    )
  }

  const sorted = [...members]
    .filter(member => member.lifecycleState !== 'removed')
    .sort((a, b) => {
      const left = a.displayName || a.memberType || a.id
      const right = b.displayName || b.memberType || b.id
      return left.localeCompare(right)
    })

  return (
    <>
      {sorted.map(member => {
        return (
          <MemberRow
            key={member.id}
            member={member}
            spaceId={spaceId}
            accentVar={accentVar}
            isActive={false}
            projectId={projectId}
            navigate={navigate}
            onRequestRemoveMember={onRequestRemoveMember}
          />
        )
      })}
    </>
  )
}
