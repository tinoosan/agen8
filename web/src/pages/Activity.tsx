import { useNavigation } from '../lib/routing'
import ActivityFeed from '../components/activity/ActivityFeed'

// Standalone Activity page: a chronological, GitHub-style stream of work
// milestones across the project. The feed itself is a client-side projection
// (see lib/activityFeed.ts) over data already fetched for the dashboard, so it
// stays live for free on the existing SSE invalidation path.
export default function Activity() {
  const { projectId } = useNavigation()

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[760px] flex-col gap-6">
        <div className="flex flex-col gap-1">
          <h1 className="m-0 hidden text-2xl font-bold text-[var(--text-1)] md:block">
            Activity
          </h1>
          <p className="m-0 max-w-prose text-[0.8125rem] leading-relaxed text-[var(--text-3)]">
            A chronological stream of work milestones — tasks created, started,
            and finished; decisions logged; missions launched. Newest first.
          </p>
        </div>

        {!projectId ? (
          <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
            Select a project to view its activity.
          </div>
        ) : (
          <ActivityFeed projectId={projectId} />
        )}
      </div>
    </div>
  )
}
