import { useNavigation, type ActiveView } from '../lib/routing'
import { useStore } from '../lib/store'
import agen8IconDark from '../assets/agen8-app-icon-dark.svg'
import agen8IconLight from '../assets/agen8-app-icon-light.svg'

const VIEW_TITLES: Partial<Record<ActiveView, string>> = {
  project: 'Projects',
  dashboard: 'Dashboard',
  missions: 'Missions',
  decisions: 'Decision Log',
  strategy: 'Strategy',
}

export default function TopBar() {
  const { activeView } = useNavigation()
  const theme = useStore((s) => s.theme)
  const title = VIEW_TITLES[activeView] ?? activeView

  return (
    <div className="h-12 border-b border-[var(--border)] bg-[var(--bg-panel)] flex items-center shrink-0">
      {/* Logo section — matches sidebar width */}
      <div className="w-[var(--sidebar-width)] shrink-0 flex items-center gap-2 px-4 h-full">
        <img src={theme === 'light' ? agen8IconLight : agen8IconDark} alt="" className="w-6 h-6 shrink-0 rounded-[7px]" aria-hidden="true" />
        <span className="font-semibold text-[15px] tracking-[-0.03em] text-[var(--text-1)]">agen8</span>
      </div>

      {/* Page context section */}
      <div className="flex-1 flex items-center gap-2.5 px-6 min-w-0 h-full">
        <span className="text-[14px] font-semibold text-[var(--text-1)]">{title}</span>
      </div>

      {/* Right section */}
      <div className="flex items-center gap-2 px-4 h-full">
      </div>
    </div>
  )
}
