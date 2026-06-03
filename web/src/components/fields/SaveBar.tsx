import clsx from 'clsx'
import { Button } from '../ui/button'

export function SaveBar({ isDirty, saving, onSave, onDiscard }: {
  isDirty: boolean
  saving: boolean
  onSave: () => void
  onDiscard: () => void
}) {
  return (
    <div
      className={clsx(
        'shrink-0 px-10 py-3 flex items-center justify-between',
        'border-t border-[var(--border)] backdrop-blur-[16px] transition-opacity duration-150',
        isDirty ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none',
      )}
      style={{
        background: 'color-mix(in srgb, var(--bg-app) 95%, transparent)',
        WebkitBackdropFilter: 'blur(16px)',
      }}
    >
      <span className="text-xs text-[var(--text-3)]">
        You have unsaved changes.
      </span>
      <div className="flex gap-2">
        <Button variant="outline" size="sm" onClick={onDiscard} disabled={saving}>
          Discard
        </Button>
        <Button size="sm" onClick={onSave} disabled={saving}>
          {saving ? 'Saving...' : 'Save changes'}
        </Button>
      </div>
    </div>
  )
}
