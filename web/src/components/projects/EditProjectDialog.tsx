import { useState } from 'react'
import { Check, Pencil, X } from 'lucide-react'
import type { Project, ProjectCustomization } from '../../lib/types'
import { updateProject } from '../../lib/projectClient'
import {
  PROJECT_COLORS, PROJECT_ICON_NAMES, PROJECT_ICONS,
} from '../../lib/projectCustomization'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'

/* ── Edit a project ───────────────────────────────────────────
   A project's identity (id, root) is fixed, but its display name
   and presentation (icon + color) are editable. The name field is
   pre-filled with the current title; the folder name is the
   placeholder so clearing it falls back to the folder rather than
   leaving the project nameless. Icon and color are chosen from
   curated sets so they stay consistent and legible in both themes. */

export default function EditProjectDialog({
  project,
  onClose,
  onSaved,
}: {
  project: Project
  onClose: () => void
  onSaved: (project: Project) => void
}) {
  const folderName = project.root.split('/').filter(Boolean).at(-1) || project.id
  const [name, setName] = useState(project.title ?? '')
  const [icon, setIcon] = useState(project.customization?.icon?.trim() ?? '')
  const [color, setColor] = useState(project.customization?.color?.trim() ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const initialName = (project.title ?? '').trim()
  const initialIcon = project.customization?.icon?.trim() ?? ''
  const initialColor = project.customization?.color?.trim() ?? ''
  const dirty =
    name.trim() !== initialName || icon !== initialIcon || color !== initialColor

  async function handleSave() {
    setBusy(true)
    setError(null)
    try {
      // Always send customization so clearing a field is a real edit: empty
      // strings drop the icon/color back to the neutral monogram default,
      // mirroring how an empty title falls back to the folder name.
      const customization: ProjectCustomization = { icon, color }
      const updated = await updateProject(project.id, { title: name.trim(), customization })
      onSaved(updated)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save project')
      setBusy(false)
    }
  }

  // Live preview of the avatar as it will render once saved. Index the curated
  // map directly so the glyph is a stable component, not one created on render.
  const PreviewIcon = icon ? PROJECT_ICONS[icon] : undefined
  const previewColor = color || 'var(--text-3)'
  const monogram = (name.trim() || folderName).charAt(0).toUpperCase() || '?'

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
      <DialogContent className="flex max-h-[calc(100vh-2rem)] max-w-[min(92vw,440px)] flex-col overflow-hidden rounded-[var(--r-xl)] border-[var(--border)] bg-[var(--bg-panel)] p-0 shadow-[var(--shadow-lg)] gap-0">
        <DialogHeader className="shrink-0 border-b border-[var(--border)] px-5 pt-5 pb-3">
          <DialogTitle className="flex items-center gap-2 text-[0.9375rem] font-semibold text-[var(--text-1)]">
            <Pencil size={14} className="text-[var(--accent)]" />
            Edit project
          </DialogTitle>
          <DialogDescription className="text-[0.75rem] text-[var(--text-3)]">
            Change the display name, icon, and color for{' '}
            <span className="font-mono text-[var(--text-2)]">{folderName}</span>.
            This does not move or rename the folder on disk.
          </DialogDescription>
        </DialogHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-4">
          {/* Name + live avatar preview */}
          <div className="flex items-end gap-3">
            <span
              data-testid="avatar-preview"
              aria-hidden
              className="mb-0.5 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--r-md)]"
              style={{
                color: previewColor,
                backgroundColor: `color-mix(in srgb, ${previewColor} 16%, transparent)`,
              }}
            >
              {PreviewIcon ? (
                <PreviewIcon size={18} strokeWidth={2} />
              ) : (
                <span style={{ fontSize: 16, fontWeight: 600, lineHeight: 1 }}>{monogram}</span>
              )}
            </span>
            <div className="min-w-0 flex-1">
              <label className="mb-2 block text-[0.6875rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
                Display name
              </label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={folderName}
                className="h-9 text-[0.8125rem]"
                autoFocus
                onKeyDown={(e) => { if (e.key === 'Enter' && !busy && dirty) handleSave() }}
              />
            </div>
          </div>
          <p className="-mt-2 text-[0.6875rem] text-[var(--text-4)]">
            Leave the name blank to use the folder name.
          </p>

          {/* Icon picker */}
          <div>
            <label className="mb-2 block text-[0.6875rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
              Icon
            </label>
            <div className="flex flex-wrap gap-1.5">
              {/* "No icon" clears back to the monogram. */}
              <button
                type="button"
                aria-label="No icon"
                aria-pressed={icon === ''}
                onClick={() => setIcon('')}
                className={cn(
                  'flex h-8 w-8 items-center justify-center rounded-[var(--r-md)] border text-[var(--text-3)] transition-colors hover:bg-[var(--bg-hover)]',
                  icon === '' ? 'border-[var(--accent)] bg-[var(--bg-active)]' : 'border-[var(--border)]',
                )}
              >
                <X size={14} />
              </button>
              {PROJECT_ICON_NAMES.map((iconName) => {
                const Glyph = PROJECT_ICONS[iconName]
                const selected = icon === iconName
                return (
                  <button
                    key={iconName}
                    type="button"
                    aria-label={iconName}
                    aria-pressed={selected}
                    onClick={() => setIcon(iconName)}
                    className={cn(
                      'flex h-8 w-8 items-center justify-center rounded-[var(--r-md)] border transition-colors hover:bg-[var(--bg-hover)]',
                      selected
                        ? 'border-[var(--accent)] bg-[var(--bg-active)] text-[var(--text-1)]'
                        : 'border-[var(--border)] text-[var(--text-2)]',
                    )}
                  >
                    <Glyph size={15} />
                  </button>
                )
              })}
            </div>
          </div>

          {/* Color picker */}
          <div>
            <label className="mb-2 block text-[0.6875rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
              Color
            </label>
            <div className="flex flex-wrap gap-2">
              {/* "Default" clears back to the neutral token. */}
              <button
                type="button"
                aria-label="Default color"
                aria-pressed={color === ''}
                onClick={() => setColor('')}
                className={cn(
                  'flex h-7 w-7 items-center justify-center rounded-full border text-[var(--text-3)] transition-transform hover:scale-110',
                  color === '' ? 'border-[var(--accent)]' : 'border-[var(--border)]',
                )}
              >
                <X size={12} />
              </button>
              {PROJECT_COLORS.map((swatch) => {
                const selected = color === swatch.value
                return (
                  <button
                    key={swatch.value}
                    type="button"
                    aria-label={swatch.name}
                    aria-pressed={selected}
                    onClick={() => setColor(swatch.value)}
                    className={cn(
                      'flex h-7 w-7 items-center justify-center rounded-full border-2 text-white transition-transform hover:scale-110',
                      selected ? 'border-[var(--text-1)]' : 'border-transparent',
                    )}
                    style={{ backgroundColor: swatch.value }}
                  >
                    {selected && <Check size={13} />}
                  </button>
                )
              })}
            </div>
          </div>

          {error && <div className="text-[0.75rem] text-[var(--red)]">{error}</div>}
        </div>

        <div className="flex shrink-0 items-center justify-end gap-2 border-t border-[var(--border)] px-5 py-3">
          <Button variant="ghost" size="sm" onClick={onClose} disabled={busy} className="text-[var(--text-3)]">
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={busy || !dirty}
            className="gap-1.5"
          >
            {busy ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
