import { AlertCircle } from 'lucide-react'

/* ── Shared empty / error states for entity detail pages ── */

// `entity` is the lowercase singular noun ("mission" / "task" / "decision").
// Not-found capitalizes it ("Mission not found."); the error keeps it lowercase
// ("Failed to load mission: …") — matching the existing copy on every detail page.
// Callers pass the already-computed `message` because error extraction differs per
// page (a single query error vs. a two-query fallback on MissionDetail).

export function DetailNotFound({ entity }: { entity: string }) {
  const Entity = entity.charAt(0).toUpperCase() + entity.slice(1)
  return <div className="max-w-4xl mx-auto px-6 pt-8 text-[var(--text-3)] text-sm">{Entity} not found.</div>
}

export function DetailError({ entity, message }: { entity: string; message: string }) {
  return (
    <div className="max-w-4xl mx-auto px-6 pt-8">
      <div className="flex items-center gap-2 text-[var(--red)] text-sm">
        <AlertCircle size={15} />
        <span>Failed to load {entity}: {message}</span>
      </div>
    </div>
  )
}
