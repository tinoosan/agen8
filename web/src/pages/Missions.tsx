import { useNavigation } from '../lib/routing'
import DashboardMissionsPanel from '../components/dashboard/DashboardMissionsPanel'

// Missions — the project's full mission list on its own page: status filters,
// search, and the paginated rows. It used to be a tab in the dashboard's right
// rail; giving it a dedicated left-nav destination keeps the dashboard an
// overview and lets the list have the room a real work list needs.
//
// The list reuses DashboardMissionsPanel in standalone (non-embedded) mode,
// which uses a full-width layout with a larger heading — the same layout the
// panel uses when embedded=false (the default).
export default function Missions() {
  const { projectId, focusedProjectRoot } = useNavigation()

  return (
    <div className="h-full overflow-y-auto">
      {!projectId ? (
        <div className="flex items-center justify-center h-full p-8">
          <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
            Select a project to view its missions.
          </div>
        </div>
      ) : (
        <DashboardMissionsPanel projectId={projectId} focusedProjectRoot={focusedProjectRoot} />
      )}
    </div>
  )
}
