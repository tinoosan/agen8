/**
 * Sidebar — the main navigation shell. Composes extracted sub-components
 * (ProjectSwitcher, SpaceList, MissionSection, AccountChip) into the
 * shadcn sidebar layout with header, scrollable content, and footer.
 *
 * This file owns only:
 * - The overall layout (ShadcnSidebar + Header + Content + Footer)
 * - Navigation link rows (Dashboard, Strategy)
 * - Section labels and the collapsed state
 * - Wiring of the CreateMissionDialog
 *
 * All heavy sub-components live in `./sidebar/`.
 */
import { useState } from 'react'
import { useLocation } from 'wouter'
import {
  BarChart3, Network, Plus, PanelLeft,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useNavigation, dashboardLink } from '../lib/routing'
import { useStore } from '../lib/store'
import CreateMissionDialog from './mission/CreateMissionDialog'
import { ProjectSwitcher, AccountChip, MissionsSidebarSection, GlobalSidebarContent } from './sidebar-parts'
import {
  Sidebar as ShadcnSidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'

import agen8IconDark from '../assets/agen8-app-icon-dark.svg'
import agen8IconLight from '../assets/agen8-app-icon-light.svg'

/* ── Style constants ─────────────────────────────────── */

const ROW_BASE = 'mx-1 rounded-[6px] px-2.5 py-[5px] text-[13px] font-normal'
const ROW_STYLE = { letterSpacing: '-0.08px' } as const
const ROW_ACTIVE = 'bg-[var(--bg-active)] text-[var(--text-1)]'
const ROW_IDLE = 'text-[var(--text-3)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-2)]'

/* ── Shared building blocks ──────────────────────────── */

function SectionAddButton({ onClick, label, tourAnchor }: { onClick: () => void; label: string; tourAnchor?: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      data-tour={tourAnchor}
      className="shrink-0 flex items-center justify-center text-[var(--text-3)] hover:text-[var(--accent)] cursor-pointer border-0 bg-transparent p-0 opacity-60 hover:opacity-100 transition-opacity"
      aria-label={label}
    >
      <Plus size={12} />
    </button>
  )
}

function SidebarSectionLabel({ children, action }: { children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="mx-3.5 mb-1 mt-2.5 flex items-center gap-2">
      <span className="flex-1 text-[10px] font-semibold uppercase text-[var(--text-3)]" style={{ letterSpacing: '0.06em' }}>
        {children}
      </span>
      {action}
    </div>
  )
}

function SidebarCollapseToggle() {
  const { toggleSidebar, state } = useSidebar()
  const collapsed = state === 'collapsed'
  return (
    <button
      type="button"
      onClick={toggleSidebar}
      className="shrink-0 h-7 w-7 flex items-center justify-center rounded-[6px] border-none bg-transparent cursor-pointer text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors"
      title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
      aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
    >
      <PanelLeft size={14} />
    </button>
  )
}

/* ── Sidebar ─────────────────────────────────────────── */

export default function Sidebar() {
  const [, navigate] = useLocation()
  const theme = useStore((s) => s.theme)
  const { state } = useSidebar()
  const collapsed = state === 'collapsed'
  const { activeView, setActiveView, focusedProjectRoot, projectId } = useNavigation()
  const hasProject = !!focusedProjectRoot

  const [createMissionOpen, setCreateMissionOpen] = useState(false)

  const SIDEBAR_CARD_CHROME = [
    'agen8-sidebar-shell',
    'bg-[var(--sidebar-shell-bg)] text-[var(--text-1)]',
    'border-r border-[var(--sidebar-shell-border)]',
    'shadow-[var(--sidebar-shell-shadow)]',
    'backdrop-blur-xl',
  ].join(' ')

  // Collapsed: thin icon column
  if (collapsed) {
    return (
      <nav
        className="agen8-sidebar-collapsed-overlay absolute left-2 top-3 z-30 flex flex-col items-center"
        aria-label="Sidebar (collapsed)"
      >
        <SidebarCollapseToggle />
      </nav>
    )
  }

  return (
    <ShadcnSidebar
      collapsible="none"
      className={cn('!w-[272px] overflow-hidden', SIDEBAR_CARD_CHROME)}
    >
      <SidebarHeader className="p-0 gap-0 shrink-0">
        <div className="flex items-center gap-1.5 px-3.5 h-[46px] shrink-0">
          <img
            src={theme === 'light' ? agen8IconLight : agen8IconDark}
            alt=""
            className="w-5 h-5 shrink-0 rounded-[5px] select-none"
            draggable={false}
            aria-hidden="true"
          />
          <div className="flex-1 min-w-0">
            <ProjectSwitcher />
          </div>
          <SidebarCollapseToggle />
        </div>
      </SidebarHeader>

      <SidebarContent className="min-h-0 flex-1 !overflow-x-hidden">
        {hasProject ? (
          <>
            {/* Navigation links */}
            <SidebarGroup className="py-0 px-0 mt-0">
              <SidebarMenu className="gap-0 px-1.5 pt-0.5">
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={activeView === 'dashboard'}
                    onClick={projectId ? () => navigate(dashboardLink(projectId)) : undefined}
                    className={cn(ROW_BASE, activeView === 'dashboard' ? ROW_ACTIVE : ROW_IDLE)}
                    style={ROW_STYLE}
                  >
                    <BarChart3 size={15} className="shrink-0" />
                    <span>Dashboard</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={activeView === 'strategy'}
                    onClick={() => setActiveView('strategy')}
                    className={cn(ROW_BASE, activeView === 'strategy' ? ROW_ACTIVE : ROW_IDLE)}
                    style={ROW_STYLE}
                  >
                    <Network size={15} className="shrink-0" />
                    <span>Strategy</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroup>

            <div className="h-px mx-3.5 my-1 bg-[var(--border)]" />

            {/* Missions section */}
            <SidebarGroup className="py-0 px-0 mt-0">
              <SidebarSectionLabel action={
                <SectionAddButton onClick={() => setCreateMissionOpen(true)} label="New mission" />
              }>Missions</SidebarSectionLabel>
              <MissionsSidebarSection projectId={projectId} />
            </SidebarGroup>
          </>
        ) : (
          <GlobalSidebarContent />
        )}
      </SidebarContent>

      <SidebarFooter className={cn('p-0 gap-0 shrink-0', collapsed && 'hidden')}>
        <div className="px-3.5 py-2.5">
          <AccountChip />
        </div>
      </SidebarFooter>

      {projectId && (
        <CreateMissionDialog
          projectId={projectId}
          open={createMissionOpen}
          onOpenChange={setCreateMissionOpen}
        />
      )}
    </ShadcnSidebar>
  )
}
