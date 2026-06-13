import { useNavigation } from '../lib/routing'
import DashboardTasksPanel from '../components/dashboard/DashboardTasksPanel'

// Tasks — the project's full task list on its own page: status filters, search,
// and the paginated rows. It used to be a tab in the dashboard's right rail;
// giving it a dedicated left-nav destination keeps the dashboard an overview
// and lets the list have the room a real work list needs.
//
// The list reuses DashboardTasksPanel, which reads its status/page filters from
// the URL (?status=, ?page=) and navigates route-relative, so deep links and
// pagination work here exactly as they did in the rail.
export default function Tasks() {
  const { projectId } = useNavigation()

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[880px] flex-col">
        {!projectId ? (
          <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
            Select a project to view its tasks.
          </div>
        ) : (
          <DashboardTasksPanel projectId={projectId} />
        )}
      </div>
    </div>
  )
}
