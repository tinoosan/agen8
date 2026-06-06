import { Command } from 'cmdk'
import { Home, Network, ScrollText, Target, UserRound } from 'lucide-react'
import { useLocation } from 'wouter'
import { useFocusTrap } from '../hooks/useFocusTrap'
import { useMissions } from '../hooks/useMissions'
import { dashboardLink, decisionsLink, strategyMapLink, useNavigation } from '../lib/routing'
import { useStore } from '../lib/store'
import { useStrategyMapStore } from './strategy/strategyMapStore'

export default function CommandPalette() {
  const dialogRef = useFocusTrap<HTMLDivElement>()
  const { setPaletteOpen } = useStore()
  const { projectId } = useNavigation()
  const [, navigate] = useLocation()
  const { data: missions } = useMissions(projectId)

  function close() {
    setPaletteOpen(false)
  }

  return (
    <div className="dialog-overlay" onClick={close}>
      <Command
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
        onClick={e => e.stopPropagation()}
        className="animate-scale-in dialog-container w-[min(90vw,540px)]"
      >
        <div className="flex items-center px-4 border-b border-[var(--border)]">
          <Command.Input
            placeholder="Search missions, decisions, and project context..."
            autoFocus
            className="flex-1 py-[15px] border-none outline-none bg-transparent text-[var(--text-1)] text-sm font-[inherit]"
          />
          <kbd
            onClick={close}
            className="text-[0.6875rem] cursor-pointer bg-[var(--bg-elevated)] text-[var(--text-3)] px-[7px] py-[2px] rounded-[5px] border border-[var(--border)] font-[inherit] transition-colors duration-150"
          >
            ESC
          </kbd>
        </div>

        <Command.List className="max-h-[360px] overflow-y-auto py-1.5">
          <Command.Empty className="px-4 py-8 text-center text-[var(--text-3)] text-[0.8125rem] leading-[1.6]">
            No results found<br />
            <span className="text-[0.6875rem]">Try searching for missions, decisions, or graph context</span>
          </Command.Empty>

          {missions && missions.filter(m => m.status !== 'archived').length > 0 && (
            <Command.Group heading="Missions">
              {missions.filter(m => m.status !== 'archived').map(m => (
                <Command.Item
                  key={m.id}
                  value={`mission-${m.id}-${m.title}-${m.status}`}
                  onSelect={() => {
                    useStrategyMapStore.setState({ pendingFocusNodeId: m.id })
                    if (projectId) navigate(strategyMapLink(projectId, m.id))
                    close()
                  }}
                  className="py-2.5 px-3.5 cursor-pointer flex items-center gap-2.5 text-[0.8125rem] text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
                >
                  <Target size={13} className="text-[var(--text-3)] shrink-0" />
                  <span className="flex-1 truncate">{m.title}</span>
                  <span className="text-[0.6875rem] text-[var(--text-3)] capitalize">{m.status}</span>
                </Command.Item>
              ))}
            </Command.Group>
          )}

          <Command.Group heading="Navigate">
            <Command.Item
              value="project dashboard overview"
              onSelect={() => {
                if (projectId) navigate(dashboardLink(projectId))
                close()
              }}
              className="py-2.5 px-3.5 cursor-pointer text-[0.8125rem] flex items-center gap-2.5 text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
            >
              <Home size={13} className="text-[var(--text-3)]" />
              Dashboard
            </Command.Item>
            <Command.Item
              value="strategy map context graph missions key results tasks"
              onSelect={() => {
                if (projectId) navigate(strategyMapLink(projectId))
                close()
              }}
              className="py-2.5 px-3.5 cursor-pointer text-[0.8125rem] flex items-center gap-2.5 text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
            >
              <Network size={13} className="text-[var(--text-3)]" />
              Strategy map
            </Command.Item>
            <Command.Item
              value="decisions log record choices"
              onSelect={() => {
                if (projectId) navigate(decisionsLink(projectId))
                close()
              }}
              className="py-2.5 px-3.5 cursor-pointer text-[0.8125rem] flex items-center gap-2.5 text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
            >
              <ScrollText size={13} className="text-[var(--text-3)]" />
              Decisions
            </Command.Item>
            <Command.Item
              value="account setup settings user"
              onSelect={() => {
                navigate('/account')
                close()
              }}
              className="py-2.5 px-3.5 cursor-pointer text-[0.8125rem] flex items-center gap-2.5 text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
            >
              <UserRound size={13} className="text-[var(--text-3)]" />
              Account setup
            </Command.Item>
          </Command.Group>
        </Command.List>
      </Command>
    </div>
  )
}
