import { useNavigation } from '../lib/routing'
import MetricsPanel from '../components/metrics/MetricsPanel'
import ActivityFeed from '../components/activity/ActivityFeed'

// Pulse — the consolidated observability surface. It blends the two former
// pages into one scrolling view: the throughput metrics band on top (how the
// work is going) and the chronological activity feed below (what's been
// happening). Both are client-side projections over data already fetched for
// the dashboard (tasks + roster), so they stay live for free on the existing
// SSE invalidation path — see lib/metrics.ts and lib/activityFeed.ts.
export default function Pulse() {
  const { projectId } = useNavigation()

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[880px] flex-col gap-8">
        <div className="flex flex-col gap-1">
          <h1 className="m-0 hidden text-2xl font-bold text-[var(--text-1)] md:block">
            Pulse
          </h1>
          <p className="m-0 max-w-prose text-[0.8125rem] leading-relaxed text-[var(--text-3)]">
            How the work is going, and what's been happening. The tiles summarize
            current throughput — pickup latency is time to awareness (queued →
            claimed), work time is in-progress duration (claimed → done). The
            stream below is a chronological log of work milestones, newest first.
          </p>
        </div>

        {!projectId ? (
          <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
            Select a project to view its pulse.
          </div>
        ) : (
          <>
            <MetricsPanel projectId={projectId} />

            <div className="flex flex-col gap-4">
              <div className="flex flex-col gap-1">
                <h2 className="m-0 text-base font-semibold text-[var(--text-1)]">
                  Activity
                </h2>
                <p className="m-0 max-w-prose text-[0.8125rem] leading-relaxed text-[var(--text-3)]">
                  Tasks created, started, and finished; decisions logged;
                  missions launched.
                </p>
              </div>
              <ActivityFeed projectId={projectId} />
            </div>
          </>
        )}
      </div>
    </div>
  )
}
