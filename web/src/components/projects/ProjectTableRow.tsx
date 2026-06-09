import { useLocation } from 'wouter'
import { cn } from '@/lib/utils'
import { formatDate, formatRelative } from '@/lib/format'
import { projectDisplayName } from '@/lib/projectHelpers'
import { shortenPath } from '../../lib/projectFormat'
import { Badge } from '@/components/ui/badge'
import {
  TableCell, TableRow,
} from '@/components/ui/table'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Archive, Link as LinkIcon, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import type { Project } from '../../lib/types'
import type { ProjectRemoveAction } from './RemoveProjectDialog'
import { ProjectAvatar } from './ProjectAvatar'

/* ── A single row in the projects table ───────────────────────
   Click navigates into the project (unless archived); the kebab
   exposes link/archive/delete via the parent's callbacks. */

export default function ProjectTableRow({
  project,
  onRemove,
  onLink,
  onEdit,
}: {
  project: Project
  onRemove: (action: ProjectRemoveAction) => void
  onLink: () => void
  onEdit: () => void
}) {
  const [, navigate] = useLocation()
  const active = project.status === 'open'
  const archived = project.status === 'archived'
  const lastActivity = project.updatedAt

  return (
    <TableRow
      onClick={() => { if (!archived) navigate(`/project/${encodeURIComponent(project.id)}`) }}
      className={cn(
        'group cursor-pointer border-[var(--border)] transition-colors hover:bg-[var(--bg-hover)]',
        archived && 'opacity-70',
      )}
    >
      {/* Status dot */}
      <TableCell className="w-[28px] px-3 py-2.5">
        <span
          className={cn(
            'block h-[6px] w-[6px] rounded-full',
            active ? 'bg-[var(--green)]' : 'bg-[var(--text-4)]',
          )}
        />
      </TableCell>

      {/* Project avatar + name + path (stacked) */}
      <TableCell className="px-3 py-2.5">
        <div className="flex min-w-0 items-center gap-2.5">
          <ProjectAvatar project={project} size={26} className={cn(archived && 'opacity-70')} />
          <div className="min-w-0">
            <div className={cn(
              'truncate text-[0.8125rem] font-semibold leading-tight',
              archived ? 'text-[var(--text-3)]' : 'text-[var(--text-1)]',
            )}>
              {projectDisplayName(project)}
            </div>
            <div
              className="mt-0.5 truncate font-mono text-[0.6875rem] leading-tight text-[var(--text-3)]"
              title={project.root}
            >
              {shortenPath(project.root)}
            </div>
          </div>
        </div>
      </TableCell>

      {/* Status badge — hidden below 640px */}
      <TableCell className="hidden w-[80px] px-3 py-2.5 text-center @[640px]:table-cell">
        <Badge
          variant={active ? 'success' : 'secondary'}
          className="text-[0.625rem] px-1.5 py-0"
        >
          {active ? 'Active' : 'Archived'}
        </Badge>
      </TableCell>

      {/* Activity */}
      <TableCell className="w-[80px] px-3 py-2.5 text-right">
        <span
          className="whitespace-nowrap tabular-nums text-[0.75rem] text-[var(--text-3)]"
          title={lastActivity ? `Updated ${formatDate(lastActivity)}` : undefined}
        >
          {lastActivity ? formatRelative(lastActivity) : ''}
        </span>
      </TableCell>

      {/* Actions kebab */}
      <TableCell className="w-[36px] px-2 py-2.5" onClick={e => e.stopPropagation()}>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label="Project actions"
              className="flex h-6 w-6 items-center justify-center rounded-[var(--r-sm)] border-0 bg-transparent text-[var(--text-3)] opacity-0 transition-opacity cursor-pointer hover:bg-[var(--bg-active)] hover:text-[var(--text-1)] group-hover:opacity-100 focus:opacity-100"
            >
              <MoreHorizontal size={13} />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="text-xs min-w-[160px]">
            {!archived && (
              <>
                <DropdownMenuItem onSelect={onEdit}>
                  <Pencil size={12} className="mr-2" />
                  Edit project
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={onLink}>
                  <LinkIcon size={12} className="mr-2" />
                  Link this folder
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onSelect={() => onRemove('archive')}
                  className="text-[var(--red)] focus:text-[var(--red)]"
                >
                  <Archive size={12} className="mr-2" />
                  Archive project
                </DropdownMenuItem>
              </>
            )}
            <DropdownMenuItem
              onSelect={() => onRemove('delete')}
              className="text-[var(--red)] focus:text-[var(--red)]"
            >
              <Trash2 size={12} className="mr-2" />
              Delete project
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  )
}
