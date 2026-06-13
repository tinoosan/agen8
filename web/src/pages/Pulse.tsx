import { useNavigation } from '../lib/routing'
import DashboardTasksPanel from '../components/dashboard/DashboardTasksPanel'
import MetricsPanel from '../components/metrics/MetricsPanel'
import ActivityFeed from '../components/activity/ActivityFeed'

// Pulse — the project's work surface. It leads with the full task list (the
// actionable work), then the throughput band (how the work is going), then the
// chronological activity feed (what's been happening). Tasks used to live only
// in the dashboard's right rail; folding them in here gives them one home and
// removes the rail/main-column/sidebar duplication.
//
// Everything here is a client-side projection over data already fetched for the
// dashboard (tasks + roster), so it stays live for free on the existing SSE
// invalidation path — see lib/metrics.ts and lib/activityFeed.ts.
export default function Pulse() {
  const { projectId } = useNavigation()

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[880px] flex-col gap-10">
        <h1 className="m-0 hidden text-2xl font-bold text-[var(--text-1)] md:block">
          Pulse
        </h1>

        {!projectId ? (
          <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
            Select a project to view its pulse.
          </div>
        ) : (
          <>
            <DashboardTasksPanel projectId={projectId} />

            <MetricsPanel projectId={projectId} />

            {/* ActivityFeed renders its own section header. */}
            <ActivityFeed projectId={projectId} />
          </>
        )}
      </div>
    </div>
  )
}
