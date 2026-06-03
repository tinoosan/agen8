import { Command } from 'cmdk'
import { ClipboardList, Home, Network, Target, UserRound, AlertTriangle } from 'lucide-react'
import { useLocation } from 'wouter'
import { useFocusTrap } from '../hooks/useFocusTrap'
import { usePendingEscalations } from '../hooks/useEscalations'
import { useMissions } from '../hooks/useMissions'
import { usePendingOpActions } from '../hooks/useOpActions'
import { useProjectSpaces } from '../hooks/useProjectSpaces'
import { actionDetailLink, actionsPanelLink, dashboardLink, strategyMapLink, useNavigation } from '../lib/routing'
import { spaceDisplayName } from '../lib/spaceDisplayName'
import { useStore } from '../lib/store'
import PulseDot from './PulseDot'
import { useStrategyMapStore } from './strategy/strategyMapStore'

export default function CommandPalette() {
  const dialogRef = useFocusTrap<HTMLDivElement>()
  const { setPaletteOpen } = useStore()
  const { setFocusedSpaceId, projectId } = useNavigation()
  const [, navigate] = useLocation()
  const spacesQuery = useProjectSpaces(projectId)
  const spaces = spacesQuery.data ?? []
  const { data: missions } = useMissions(projectId)
  const { data: pendingOAs } = usePendingOpActions(projectId)
  const { data: pendingEscalations } = usePendingEscalations(projectId)

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
            placeholder="Search spaces, context, missions, and requests..."
            autoFocus
            className="flex-1 py-[15px] border-none outline-none bg-transparent text-[var(--text-1)] text-sm font-[inherit]"
          />
          <kbd
            onClick={close}
            className="text-[11px] cursor-pointer bg-[var(--bg-elevated)] text-[var(--text-3)] px-[7px] py-[2px] rounded-[5px] border border-[var(--border)] font-[inherit] transition-colors duration-150"
          >
            ESC
          </kbd>
        </div>

        <Command.List className="max-h-[360px] overflow-y-auto py-1.5">
          <Command.Empty className="px-4 py-8 text-center text-[var(--text-3)] text-[13px] leading-[1.6]">
            No results found<br />
            <span className="text-[11px]">Try searching for spaces, context, missions, or requests</span>
          </Command.Empty>

          {spaces.length > 0 && (
            <Command.Group heading="Spaces">
              {spaces.map(space => {
                const isActive = space.status === 'running' || space.lifecyclePhase === 'ready' || space.lifecyclePhase === 'progressing'
                const status = isActive ? 'active' : space.status === 'failed' || space.lifecyclePhase === 'degraded' ? 'failed' : 'idle'
                return (
                  <Command.Item
                    key={space.spaceId}
                    value={`space-${space.spaceId}-${space.spaceName}`}
                    onSelect={() => {
                      if (space.spaceId) setFocusedSpaceId(space.spaceId)
                      close()
                    }}
                    className="py-2.5 px-3.5 cursor-pointer flex items-center gap-2.5 text-[13px] text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
                  >
                    <PulseDot status={status} size={7} />
                    <span className="flex-1 font-medium">
                      {spaceDisplayName(space.spaceId, space.spaceName)}
                    </span>
                    <span className="text-[11px] text-[var(--text-3)] capitalize">
                      {space.status}
                    </span>
                  </Command.Item>
                )
              })}
            </Command.Group>
          )}

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
                  className="py-2.5 px-3.5 cursor-pointer flex items-center gap-2.5 text-[13px] text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
                >
                  <Target size={13} className="text-[var(--text-3)] shrink-0" />
                  <span className="flex-1 truncate">{m.title}</span>
                  <span className="text-[11px] text-[var(--text-3)] capitalize">{m.status}</span>
                </Command.Item>
              ))}
            </Command.Group>
          )}

          {pendingOAs && pendingOAs.length > 0 && (
            <Command.Group heading="Requests">
              {pendingOAs.map(oa => (
                <Command.Item
                  key={oa.id}
                  value={`request-${oa.id}-${oa.title}-${oa.urgency}-${oa.status}`}
                  onSelect={() => {
                    if (projectId) navigate(actionDetailLink(projectId, oa.id))
                    close()
                  }}
                  className="py-2.5 px-3.5 cursor-pointer flex items-center gap-2.5 text-[13px] text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
                >
                  <ClipboardList size={13} className="text-[var(--text-3)] shrink-0" />
                  <span className="flex-1 truncate">{oa.title}</span>
                  <span className="text-[11px] text-[var(--text-3)] capitalize">{oa.urgency}</span>
                </Command.Item>
              ))}
            </Command.Group>
          )}

          {pendingEscalations && pendingEscalations.length > 0 && (
            <Command.Group heading="Escalations">
              {pendingEscalations.map(esc => (
                <Command.Item
                  key={esc.id}
                  value={`escalation-${esc.id}-${esc.title}-${esc.urgency}-${esc.category}`}
                  onSelect={() => {
                    if (projectId) navigate(actionsPanelLink(projectId, 'escalation'))
                    close()
                  }}
                  className="py-2.5 px-3.5 cursor-pointer flex items-center gap-2.5 text-[13px] text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
                >
                  <AlertTriangle size={13} className="text-[var(--amber)] shrink-0" />
                  <span className="flex-1 truncate">{esc.title}</span>
                  <span className="text-[11px] text-[var(--text-3)] capitalize">{esc.urgency}</span>
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
              className="py-2.5 px-3.5 cursor-pointer text-[13px] flex items-center gap-2.5 text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
            >
              <Home size={13} className="text-[var(--text-3)]" />
              Dashboard
            </Command.Item>
            <Command.Item
              value="context map strategy graph"
              onSelect={() => {
                if (projectId) navigate(strategyMapLink(projectId))
                close()
              }}
              className="py-2.5 px-3.5 cursor-pointer text-[13px] flex items-center gap-2.5 text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
            >
              <Network size={13} className="text-[var(--text-3)]" />
              Context map
            </Command.Item>
            <Command.Item
              value="requests actions inbox operator"
              onSelect={() => {
                if (projectId) navigate(actionsPanelLink(projectId))
                close()
              }}
              className="py-2.5 px-3.5 cursor-pointer text-[13px] flex items-center gap-2.5 text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
            >
              <ClipboardList size={13} className="text-[var(--text-3)]" />
              Requests
            </Command.Item>
            <Command.Item
              value="account setup settings user"
              onSelect={() => {
                navigate('/account')
                close()
              }}
              className="py-2.5 px-3.5 cursor-pointer text-[13px] flex items-center gap-2.5 text-[var(--text-1)] rounded-[var(--r-md)] mx-1.5 my-px"
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
