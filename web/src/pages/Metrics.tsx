import { useNavigation } from '../lib/routing'
import MetricsPanel from '../components/metrics/MetricsPanel'

// Standalone Metrics page: a few high-value numbers derived entirely from data
// already loaded for the dashboard (the task list + member roster) — no metrics
// table, RPC, or migration. The projection lives in lib/metrics.ts and rides the
// existing SSE invalidation, so the figures refresh as work moves.
export default function Metrics() {
  const { projectId } = useNavigation()

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[880px] flex-col gap-6">
        <div className="flex flex-col gap-1">
          <h1 className="m-0 hidden text-2xl font-bold text-[var(--text-1)] md:block">
            Metrics
          </h1>
          <p className="m-0 max-w-prose text-[0.8125rem] leading-relaxed text-[var(--text-3)]">
            How the work is going — derived from tasks and the member roster.
            Pickup latency is time to awareness (queued → claimed); work time is
            in-progress duration (claimed → done). Leaderboards attribute
            completed tasks to the model and harness that did them.
          </p>
        </div>

        {!projectId ? (
          <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
            Select a project to view its metrics.
          </div>
        ) : (
          <MetricsPanel projectId={projectId} />
        )}
      </div>
    </div>
  )
}
