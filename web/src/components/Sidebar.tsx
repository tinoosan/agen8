/**
 * Sidebar — the main navigation shell. Composes extracted sub-components
 * (MissionSection, GlobalContent, AccountChip) into the
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
import { useEffect, useState } from 'react'
import { useLocation } from 'wouter'
import {
  BarChart3, Network, Plus, PanelLeft, Users, Activity, ListChecks,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useNavigation, dashboardLink } from '../lib/routing'
import { useStore } from '../lib/store'
import { brandIconFor } from '../lib/brandIcon'
import CreateMissionDialog from './mission/CreateMissionDialog'
import NotificationInbox from './notifications/NotificationInbox'
import { AccountChip, MissionsSidebarSection, GlobalSidebarContent } from './sidebar-parts'
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
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

/* ── Style constants ─────────────────────────────────── */

const ROW_BASE = 'mx-1 rounded-[6px] px-2.5 py-[5px] text-[0.8125rem] font-normal'
const ROW_STYLE = { letterSpacing: '-0.08px' } as const
const BRAND_STYLE = { fontSize: '0.875rem', fontWeight: 600, letterSpacing: '-0.02em' } as const
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
      <span className="flex-1 text-[0.625rem] font-semibold uppercase text-[var(--text-3)]" style={{ letterSpacing: '0.06em' }}>
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
  const [location, navigate] = useLocation()
  const theme = useStore((s) => s.theme)
  const { state, isMobile, openMobile, setOpenMobile } = useSidebar()
  const collapsed = state === 'collapsed'
  const { activeView, setActiveView, focusedProjectRoot, projectId } = useNavigation()
  const hasProject = !!focusedProjectRoot

  const [createMissionOpen, setCreateMissionOpen] = useState(false)

  // Close the mobile drawer whenever the route changes (e.g. tapping a nav link).
  // Depend only on `location` so opening the drawer doesn't immediately re-close it.
  useEffect(() => {
    setOpenMobile(false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location])

  const SIDEBAR_CARD_CHROME = [
    'agen8-sidebar-shell',
    'bg-[var(--sidebar-shell-bg)] text-[var(--text-1)]',
    'border-r border-[var(--sidebar-shell-border)]',
    'shadow-[var(--sidebar-shell-shadow)]',
    'backdrop-blur-xl',
  ].join(' ')

  const body = (
    <>
      <SidebarHeader className="p-0 gap-0 shrink-0">
        <div className="flex items-center gap-1.5 px-3.5 h-[46px] shrink-0">
          <button
            type="button"
            onClick={() => navigate('/')}
            className="group flex flex-1 min-w-0 items-center gap-1.5 border-none bg-transparent p-0 text-left cursor-pointer"
            aria-label="All projects"
            title="All projects"
          >
            <img
              src={brandIconFor(theme)}
              alt=""
              className="w-5 h-5 shrink-0 rounded-[5px] select-none"
              draggable={false}
              aria-hidden="true"
            />
            <span
              className="flex-1 min-w-0 truncate text-[var(--text-1)] group-hover:text-[var(--accent)] transition-colors"
              style={BRAND_STYLE}
            >
              agen8
            </span>
          </button>
          <button
            type="button"
            onClick={() => navigate('/?new=true')}
            className="shrink-0 h-7 w-7 flex items-center justify-center rounded-[6px] border-none bg-transparent cursor-pointer text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors"
            title="New project"
            aria-label="New project"
          >
            <Plus size={15} />
          </button>
          {hasProject && <NotificationInbox projectId={projectId} />}
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
                    isActive={activeView === 'tasks'}
                    onClick={() => setActiveView('tasks')}
                    className={cn(ROW_BASE, activeView === 'tasks' ? ROW_ACTIVE : ROW_IDLE)}
                    style={ROW_STYLE}
                  >
                    <ListChecks size={15} className="shrink-0" />
                    <span>Tasks</span>
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
                    <span>Context Map</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={activeView === 'pulse'}
                    onClick={() => setActiveView('pulse')}
                    className={cn(ROW_BASE, activeView === 'pulse' ? ROW_ACTIVE : ROW_IDLE)}
                    style={ROW_STYLE}
                  >
                    <Activity size={15} className="shrink-0" />
                    <span>Pulse</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={activeView === 'members'}
                    onClick={() => setActiveView('members')}
                    className={cn(ROW_BASE, activeView === 'members' ? ROW_ACTIVE : ROW_IDLE)}
                    style={ROW_STYLE}
                  >
                    <Users size={15} className="shrink-0" />
                    <span>Members</span>
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

      <SidebarFooter className={cn('p-0 gap-0 shrink-0', !isMobile && collapsed && 'hidden')}>
        <div className="px-2.5 pt-1 pb-2">
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
    </>
  )

  // Mobile: off-canvas drawer (the desktop collapsed-overlay never applies here)
  if (isMobile) {
    return (
      <Sheet open={openMobile} onOpenChange={setOpenMobile}>
        <SheetContent
          side="left"
          className={cn('w-screen max-w-none p-0 gap-0 [&>button]:hidden', SIDEBAR_CARD_CHROME)}
        >
          <SheetHeader className="sr-only">
            <SheetTitle>Navigation</SheetTitle>
          </SheetHeader>
          <div className="flex h-full w-full flex-col">{body}</div>
        </SheetContent>
      </Sheet>
    )
  }

  // Collapsed: thin icon column (desktop only)
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
      className={cn('!w-[272px] overflow-hidden md:pt-[env(safe-area-inset-top)]', SIDEBAR_CARD_CHROME)}
    >
      {body}
    </ShadcnSidebar>
  )
}
