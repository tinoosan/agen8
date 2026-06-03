/**
 * SpaceList — the expandable list of spaces in the sidebar. Each space
 * row shows its icon, name, member count, and hover actions (customize,
 * rename, delete). Expanding a row reveals the MemberSubList.
 *
 * Dialog state is managed here but rendered via extracted SpaceDialogs
 * components so this file stays focused on list logic.
 */
import React, { useCallback, useMemo, useState } from 'react'
import clsx from 'clsx'
import { toast } from 'sonner'
import { ChevronDown, MoreHorizontal, Palette, SquarePen, Trash2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useNavigation } from '../../lib/routing'
import { useSpaceList, useSpaceMemberList } from '../../hooks/useSpace'
import { useCreateSpace } from '../../hooks/useCreateSpace'
import { sanitizeSpaceTitle } from '../../lib/displaySanitizers'
import { isCanonicalSpaceId } from '../../lib/spaceHelpers'
import { resolveSpaceIcon, spaceColorVar } from '../../lib/spaceCustomization'
import { SpaceCustomizationPicker } from '../SpaceCustomizationPicker'
import { MemberSubList } from './MemberList'
import { SpaceRenameDialog, SpaceDeleteDialog, MemberRemoveDialog } from './SpaceDialogs'
import { Skeleton } from '@/components/ui/skeleton'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from '@/components/ui/sidebar'

/* ── SpaceRowMeta: compact member count + status dot ─ */

function SpaceRowMeta({ spaceId }: { spaceId: string }) {
  const membersQuery = useSpaceMemberList({ spaceId, enabled: !!spaceId })
  const members = membersQuery.data ?? []
  const activeCount = members.filter(m => (m.lifecycleState ?? '').toLowerCase() === 'active').length
  if (membersQuery.isLoading || activeCount === 0) return null
  return (
    <span className="ml-auto flex items-center gap-1 shrink-0">
      <span className="text-[10px] tabular-nums text-[var(--text-3)]">{activeCount}</span>
      <span
        className="w-[5px] h-[5px] rounded-full shrink-0"
        style={{ backgroundColor: 'var(--green)' }}
      />
    </span>
  )
}

/* ── SpaceList ─────────────────────────────────────── */

export function SpaceList() {
  const { focusedProjectRoot, focusedSpaceId, setFocusedSpaceId, activeView, projectId } = useNavigation()
  const spacesQuery = useSpaceList({ projectId, status: 'open', enabled: !!projectId })
  const spaces = useMemo(() => {
    const list = spacesQuery.data ?? []
    return [...list].sort((a, b) => (b.updatedAt ?? '').localeCompare(a.updatedAt ?? ''))
  }, [spacesQuery.data])

  // Expand/collapse member sublists
  const [expandedMembers, setExpandedMembers] = useState<Set<string>>(new Set())
  const toggleMembers = useCallback((spaceId: string) => {
    setExpandedMembers(prev => {
      const next = new Set(prev)
      if (next.has(spaceId)) next.delete(spaceId)
      else next.add(spaceId)
      return next
    })
  }, [])

  // First-time setup flow: rename → customize chain
  const [firstSetupSpaceId, setFirstSetupSpaceId] = useState<string | null>(null)

  const { createSpace, creating } = useCreateSpace({
    onCreated: (newSpaceId) => {
      setFirstSetupSpaceId(newSpaceId)
      setRenameTarget({ spaceId: newSpaceId, name: '' })
    },
  })

  // Dialog state — thin targets, heavy logic lives in SpaceDialogs
  const [renameTarget, setRenameTarget] = useState<{ spaceId: string; name: string } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [memberToRemove, setMemberToRemove] = useState<{ spaceId: string; memberId: string; displayName: string } | null>(null)
  const [customizingSpaceId, setCustomizingSpaceId] = useState<string | null>(null)

  const handleRenameClose = useCallback(() => {
    const setupId = firstSetupSpaceId
    setRenameTarget(null)
    if (setupId) {
      setFirstSetupSpaceId(null)
      setCustomizingSpaceId(setupId)
    }
  }, [firstSetupSpaceId, setCustomizingSpaceId, setFirstSetupSpaceId, setRenameTarget])

  const handleDeleted = useCallback((deletedId: string) => {
    if (focusedSpaceId === deletedId) setFocusedSpaceId(null)
  }, [focusedSpaceId, setFocusedSpaceId])

  if (!focusedProjectRoot) return null

  if (spacesQuery.isLoading) {
    return (
      <div className="pl-9 pr-4 py-2 flex flex-col gap-1">
        {[1, 2, 3].map(i => <Skeleton key={i} className="h-[22px] rounded" />)}
      </div>
    )
  }

  if (spaces.length === 0) {
    return (
      <div className="pl-9 pr-4 py-2 text-[11px] text-[var(--text-3)]">
        No spaces yet.{' '}
        <button
          type="button"
          className="underline cursor-pointer bg-transparent border-0 p-0 font-inherit text-[var(--accent)] leading-none"
          onClick={() => void createSpace()}
          disabled={creating}
        >
          {creating ? 'Creating…' : 'Create one'}
        </button>
      </div>
    )
  }

  return (
    <>
      <SidebarMenuSub className="max-h-[30vh] gap-0 overflow-y-auto !border-l-0 mx-0 px-1.5">
        {spaces.map(space => {
          const isFocused = activeView === 'space' && space.id === focusedSpaceId
          const rawTitle = (space.title ?? '').trim()
          const sanitized = sanitizeSpaceTitle(rawTitle)
          const name = sanitized || 'Untitled space'
          const isUnnamed = !sanitized
          const customization = space.customization ?? null
          const SpaceIcon = resolveSpaceIcon(customization?.icon)
          const accentVar = spaceColorVar(customization?.color)
          const iconColor = accentVar ?? 'currentColor'

          return (
            <React.Fragment key={space.id}>
              <SidebarMenuSubItem className="group/space relative flex items-center">
                {/* Accent bar on active row */}
                {isFocused && accentVar && (
                  <span
                    aria-hidden
                    className="absolute left-0 top-1 bottom-1 w-[2px] rounded-full"
                    style={{ backgroundColor: accentVar }}
                  />
                )}
                <SidebarMenuSubButton
                  asChild
                  isActive={isFocused}
                  size="sm"
                  className="flex-1 min-w-0 h-[24px]"
                >
                  <button
                    onClick={() => {
                      if (!isCanonicalSpaceId(space.id)) {
                        toast.error('Invalid space id')
                        return
                      }
                      setFocusedSpaceId(space.id)
                    }}
                    className={cn(
                      'cursor-pointer w-full text-left justify-start gap-1.5 rounded-[9px] bg-transparent transition-colors',
                      isFocused
                        ? 'bg-white/[0.08] hover:bg-white/[0.08]'
                        : 'hover:bg-transparent',
                    )}
                    data-testid={`space-row-${space.id}`}
                  >
                    {/* Chevron toggle */}
                    <span
                      role="button"
                      aria-label={expandedMembers.has(space.id) ? 'Collapse members' : 'Expand members'}
                      data-testid={`space-member-toggle-${space.id}`}
                      onClick={e => { e.stopPropagation(); toggleMembers(space.id) }}
                      className="shrink-0 flex items-center justify-center w-[14px] h-[14px] rounded text-[var(--text-3)] hover:text-[var(--text-1)] cursor-pointer"
                    >
                      <ChevronDown
                        size={10}
                        className={clsx('transition-transform duration-150', !expandedMembers.has(space.id) && '-rotate-90')}
                      />
                    </span>
                    <SpaceIcon
                      size={13}
                      strokeWidth={1.75}
                      aria-hidden
                      className="shrink-0"
                      style={{ color: iconColor }}
                    />
                    <span className="min-w-0 flex-1 text-left leading-tight">
                      <span
                        className={cn(
                          'block truncate text-[13px] font-medium',
                          isUnnamed ? 'italic text-[var(--text-3)]' : 'text-[var(--text-1)]',
                        )}
                      >
                        {name}
                      </span>
                    </span>
                    <SpaceRowMeta spaceId={space.id} />
                  </button>
                </SidebarMenuSubButton>

                {/* Customize hover-button */}
                <SpaceCustomizationPicker
                  spaceId={space.id}
                  current={customization}
                  open={customizingSpaceId === space.id}
                  onOpenChange={open => setCustomizingSpaceId(open ? space.id : null)}
                >
                  <button
                    type="button"
                    className="shrink-0 h-5 w-5 flex items-center justify-center rounded text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] opacity-0 group-hover/space:opacity-100 focus:opacity-100 transition-opacity cursor-pointer border-0 bg-transparent"
                    aria-label="Customize space"
                    data-testid={`space-customize-${space.id}`}
                    onClick={e => e.stopPropagation()}
                  >
                    <Palette size={12} aria-hidden />
                  </button>
                </SpaceCustomizationPicker>

                {/* Kebab menu */}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button
                      className="shrink-0 h-5 w-5 flex items-center justify-center rounded text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] opacity-0 group-hover/space:opacity-100 focus:opacity-100 transition-opacity cursor-pointer border-0 bg-transparent"
                      aria-label="Space actions"
                      data-testid={`space-actions-${space.id}`}
                    >
                      <MoreHorizontal size={12} aria-hidden />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent side="right" align="start" className="text-xs">
                    <DropdownMenuItem
                      onSelect={() => setRenameTarget({ spaceId: space.id, name })}
                      data-testid={`rename-space-${space.id}`}
                    >
                      <SquarePen size={12} className="mr-2" aria-hidden />
                      Rename space
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onSelect={() => setDeleteTarget(space.id)}
                      className="text-[var(--red)] focus:text-[var(--red)]"
                      data-testid={`delete-space-${space.id}`}
                    >
                      <Trash2 size={12} className="mr-2" aria-hidden />
                      Delete space
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </SidebarMenuSubItem>

              {/* Expanded member sublist */}
              {expandedMembers.has(space.id) && (
                <div
                  className="animate-in fade-in-0 slide-in-from-top-1 duration-150"
                  data-testid={`member-sublist-${space.id}`}
                >
                  <MemberSubList
                    spaceId={space.id}
                    accentVar={accentVar}
                    onRequestRemoveMember={(memberId, displayName) =>
                      setMemberToRemove({ spaceId: space.id, memberId, displayName })
                    }
                  />
                </div>
              )}
            </React.Fragment>
          )
        })}
      </SidebarMenuSub>

      {/* Dialogs — rendered once, controlled by state */}
      <SpaceRenameDialog
        open={renameTarget !== null}
        spaceId={renameTarget?.spaceId ?? null}
        initialName={renameTarget?.name ?? ''}
        onClose={handleRenameClose}
      />
      <SpaceDeleteDialog
        open={deleteTarget !== null}
        spaceId={deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onDeleted={handleDeleted}
      />
      <MemberRemoveDialog
        open={memberToRemove !== null}
        spaceId={memberToRemove?.spaceId ?? null}
        memberId={memberToRemove?.memberId ?? null}
        displayName={memberToRemove?.displayName ?? ''}
        onClose={() => setMemberToRemove(null)}
      />
    </>
  )
}
