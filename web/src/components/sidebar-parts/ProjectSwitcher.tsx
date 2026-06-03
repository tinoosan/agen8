/**
 * ProjectSwitcher — dropdown for selecting the active project.
 * Uses shadcn Popover for keyboard navigation, focus trap, and
 * outside-click dismissal (replacing the old manual mousedown listener).
 */
import { useState } from 'react'
import { useLocation } from 'wouter'
import { ChevronDown, Check, Plus, LayoutGrid } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useProjects } from '../../hooks/useProjects'
import { useNavigation } from '../../lib/routing'
import { projectDisplayName } from '../../lib/spaceHelpers'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

export function ProjectSwitcher() {
  const [open, setOpen] = useState(false)
  const projectsQuery = useProjects()
  const projects = projectsQuery.data ?? []
  const { projectId, setFocusedProjectRoot, setActiveView } = useNavigation()
  const [, navigate] = useLocation()

  const currentProject = projects.find(p => p.id === projectId) ?? null
  const pickerLabel = currentProject ? projectDisplayName(currentProject) : 'Select a project'

  const handleSelect = (project: { id: string }) => {
    navigate(`/project/${encodeURIComponent(project.id)}`)
    setOpen(false)
  }

  const handleAllProjects = () => {
    setFocusedProjectRoot(null)
    setActiveView('project')
    setOpen(false)
  }

  const handleNewProject = () => {
    navigate('/?new=true')
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          className="flex items-center gap-1 w-full border-none bg-transparent cursor-pointer font-[inherit] text-left"
        >
          <span
            className={cn(
              'flex-1 min-w-0 truncate',
              currentProject ? 'text-[var(--text-1)]' : 'text-[var(--text-3)]',
            )}
            style={{ fontSize: '14px', fontWeight: 600, letterSpacing: '-0.02em' }}
          >
            {pickerLabel}
          </span>
          <ChevronDown
            size={10}
            className={cn(
              'text-[var(--text-3)] shrink-0 transition-transform duration-150',
              open && 'rotate-180',
            )}
          />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        side="bottom"
        sideOffset={6}
        className="w-[var(--radix-popover-trigger-width)] min-w-[220px] p-1 border-[var(--border-strong,rgba(255,255,255,0.16))] bg-[var(--bg-elevated)] rounded-[12px] shadow-[var(--shadow-xl,0_8px_32px_rgba(0,0,0,0.5))]"
      >
        {/* Scrollable project list */}
        <div className="max-h-[40vh] overflow-y-auto">
          {projects.map(project => {
            const isCurrent = project.id === projectId
            return (
              <button
                key={project.id}
                onClick={() => handleSelect(project)}
                className={cn(
                  'flex items-center gap-2 w-full px-2.5 py-[7px] text-[12px] font-medium border-none bg-transparent cursor-pointer font-[inherit] rounded-[8px] transition-[background] duration-75',
                  isCurrent
                    ? 'bg-[var(--accent-dim,rgba(59,130,246,0.14))] text-[var(--accent)]'
                    : 'text-[var(--text-2)] hover:bg-[var(--bg-hover)]',
                )}
              >
                <span
                  className="w-1.5 h-1.5 rounded-full shrink-0"
                  style={{
                    backgroundColor: project.status === 'open' ? 'var(--green)' : 'var(--text-3)',
                    opacity: project.status === 'open' ? 1 : 0.4,
                  }}
                />
                <span className="flex-1 min-w-0 truncate">{projectDisplayName(project)}</span>
                {isCurrent && <Check size={12} className="text-[var(--accent)] shrink-0" />}
              </button>
            )
          })}
          {projects.length === 0 && (
            <div className="px-3 py-3 text-[11px] text-[var(--text-3)] text-center">
              No projects yet
            </div>
          )}
        </div>

        {/* Divider + actions */}
        <div className="h-px my-1 bg-[var(--border)]" />
        <button
          onClick={handleNewProject}
          className="flex items-center gap-2 w-full px-2.5 py-[7px] text-[12px] border-none bg-transparent cursor-pointer font-[inherit] rounded-[8px] text-[var(--text-3)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-2)] transition-colors"
        >
          <Plus size={12} className="shrink-0" />
          New project
        </button>
        <button
          onClick={handleAllProjects}
          className="flex items-center gap-2 w-full px-2.5 py-[7px] text-[12px] border-none bg-transparent cursor-pointer font-[inherit] rounded-[8px] text-[var(--text-3)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-2)] transition-colors"
        >
          <LayoutGrid size={12} className="shrink-0" />
          Manage projects
        </button>
      </PopoverContent>
    </Popover>
  )
}
