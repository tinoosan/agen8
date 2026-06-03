/**
 * SpaceCustomizationPicker — popover that lets users set a space's
 * Lucide icon and palette color.
 *
 * Uses shadcn Popover so focus-trap, escape-to-close, and outside-click
 * dismissal come for free. The trigger is provided by the caller
 * (typically a small icon button on the sidebar row); we render the
 * grid + color swatches inside the popover content.
 *
 * Calls space.update with a partial customization patch on every
 * selection. Server-side validation catches invalid colors / icon
 * shapes; we don't double-validate here, but we do trim the input.
 */
import { useMemo, useState } from 'react'
import { Search, X } from 'lucide-react'
import { toast } from 'sonner'

import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { useSpaceUpdate } from '@/hooks/useSpace'
import {
  SPACE_ICON_GROUPS,
  SPACE_COLOR_PALETTE,
  resolveSpaceIcon,
  searchSpaceIcons,
  spaceColorVar,
} from '@/lib/spaceCustomization'
import type { SpaceCustomization } from '@/lib/types'

/**
 * Sentinel value the backend interprets as "explicitly clear this
 * field" (vs the empty string which means "preserve existing"). Kept
 * in lockstep with clearCustomizationFieldSentinel in the Go RPC layer.
 */
const CLEAR_SENTINEL = 'none'

interface SpaceCustomizationPickerProps {
  spaceId: string
  /** Current customization on the space, used to highlight selected entries. */
  current: SpaceCustomization | null | undefined
  /** Render-prop trigger so callers control where the popover anchors. */
  children: React.ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
  /** Optional callback fired after a successful update. */
  onCustomized?: (next: SpaceCustomization | null) => void
}

export function SpaceCustomizationPicker({
  spaceId,
  current,
  children,
  open,
  onOpenChange,
  onCustomized,
}: SpaceCustomizationPickerProps) {
  const update = useSpaceUpdate()
  const [query, setQuery] = useState('')
  const searchResults = useMemo(() => searchSpaceIcons(query), [query])

  /**
   * Optimistic local override of the customization. We render selection
   * highlights from this when set, so clicks feel instant even before
   * the server round-trip completes. Cleared back to undefined on
   * success (the parent's `current` prop now reflects the server state)
   * or on error (rolling back to whatever was real).
   */
  const [optimistic, setOptimistic] = useState<SpaceCustomization | null>(null)
  const effective = optimistic ?? current ?? null
  const currentIcon = effective?.icon ?? ''
  const currentColor = effective?.color ?? ''

  /**
   * Apply a partial customization patch with an optimistic UI update.
   * The optimistic state mirrors the patch onto the current customization
   * so selection highlights move immediately. Errors surface via a
   * toast so silent server failures don't leave the user confused.
   */
  const applyPatch = (patch: SpaceCustomization) => {
    // Compute what the customization will look like after the patch is
    // applied, mirroring the server's "" preserve / "none" clear rules.
    const next: SpaceCustomization = { ...(effective ?? {}) }
    if (patch.icon !== undefined) {
      next.icon = patch.icon === CLEAR_SENTINEL ? '' : patch.icon || next.icon
    }
    if (patch.color !== undefined) {
      next.color = patch.color === CLEAR_SENTINEL ? '' : patch.color || next.color
    }
    setOptimistic(next)

    update.mutate(
      { spaceId, customization: patch },
      {
        onSuccess: space => {
          // Server is authoritative — drop the optimistic override so
          // the parent's refetched `current` takes over.
          setOptimistic(null)
          onCustomized?.(space.customization ?? null)
        },
        onError: err => {
          // Roll back the optimistic state and surface the failure.
          setOptimistic(null)
          const message = err instanceof Error ? err.message : 'Failed to update customization'
          toast.error(message)
        },
      },
    )
  }

  /**
   * Clearing both fields means "no customization" — send the clear
   * sentinel for both so the backend collapses to a nil customization.
   */
  const clearAll = () => {
    applyPatch({ icon: CLEAR_SENTINEL, color: CLEAR_SENTINEL })
  }

  return (
    <Popover open={open} onOpenChange={(v) => { if (v) setQuery(''); onOpenChange?.(v) }}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        align="start"
        side="right"
        sideOffset={6}
        className="w-[320px] p-0 border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text-1)]"
      >
        {/* Header: search input + close button */}
        <div className="flex items-center gap-2 border-b border-[var(--border)] px-2 py-2">
          <Search size={14} className="shrink-0 text-[var(--text-3)]" aria-hidden />
          <Input
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Search icons"
            className="h-7 border-none bg-transparent px-1 text-[13px] focus-visible:ring-0 focus-visible:ring-offset-0"
            autoFocus
          />
          <button
            type="button"
            onClick={() => onOpenChange?.(false)}
            className="flex h-6 w-6 items-center justify-center rounded-[6px] text-[var(--text-3)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-1)]"
            title="Close"
            aria-label="Close"
          >
            <X size={14} />
          </button>
        </div>

        {/* Icon grid */}
        <div className="max-h-[280px] overflow-y-auto px-2 py-2">
          {searchResults === null ? (
            // Default view: grouped grid
            SPACE_ICON_GROUPS.map(group => (
              <div key={group.label} className="mb-2 last:mb-0">
                <div className="mb-1 px-1 text-[10px] font-semibold uppercase text-[var(--text-3)]" style={{ letterSpacing: '0.06em' }}>
                  {group.label}
                </div>
                <IconGrid
                  icons={group.icons}
                  currentIcon={currentIcon}
                  accentVar={spaceColorVar(currentColor)}
                  onPick={name => applyPatch({ icon: name })}
                  disabled={update.isPending}
                />
              </div>
            ))
          ) : searchResults.length === 0 ? (
            <div className="px-2 py-6 text-center text-[12px] text-[var(--text-3)]">
              No icons match "{query}"
            </div>
          ) : (
            <IconGrid
              icons={searchResults}
              currentIcon={currentIcon}
              accentVar={spaceColorVar(currentColor)}
              onPick={name => applyPatch({ icon: name })}
              disabled={update.isPending}
            />
          )}
        </div>

        {/* Color palette + reset */}
        <div className="border-t border-[var(--border)] px-2 py-2">
          <div className="mb-1 px-1 text-[10px] font-semibold uppercase text-[var(--text-3)]" style={{ letterSpacing: '0.06em' }}>
            Color
          </div>
          <div className="flex items-center gap-1.5">
            {SPACE_COLOR_PALETTE.map(({ key, label }) => {
              const selected = currentColor === key
              return (
                <button
                  key={key}
                  type="button"
                  onClick={() => applyPatch({ color: key })}
                  disabled={update.isPending}
                  title={label}
                  aria-label={`Set color to ${label}`}
                  aria-pressed={selected}
                  className={cn(
                    'h-6 w-6 rounded-full border transition disabled:opacity-50',
                    selected
                      ? 'border-[var(--text-1)] ring-2 ring-[var(--text-1)]/20'
                      : 'border-transparent hover:scale-110',
                  )}
                  style={{ backgroundColor: `var(--space-color-${key})` }}
                />
              )
            })}
          </div>
          {(currentIcon || currentColor) && (
            <button
              type="button"
              onClick={clearAll}
              disabled={update.isPending}
              className="mt-2 px-1 text-[11px] text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors disabled:opacity-50"
            >
              Reset to default
            </button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

/* ------------------------------------------------------------------ */
/* Icon grid sub-component                                             */
/* ------------------------------------------------------------------ */

function IconGrid({
  icons,
  currentIcon,
  accentVar,
  onPick,
  disabled,
}: {
  icons: ReadonlyArray<{ name: string; component: ReturnType<typeof resolveSpaceIcon> }>
  currentIcon: string
  accentVar: string | undefined
  onPick: (name: string) => void
  disabled: boolean
}) {
  return (
    <div className="grid grid-cols-8 gap-0.5">
      {icons.map(({ name, component: Icon }) => {
        const selected = currentIcon === name
        return (
          <button
            key={name}
            type="button"
            onClick={() => onPick(name)}
            disabled={disabled}
            title={name}
            aria-label={`Set icon to ${name}`}
            aria-pressed={selected}
            className={cn(
              'flex h-7 w-7 items-center justify-center rounded-[6px] transition disabled:opacity-50',
              selected
                ? 'bg-[var(--bg-active)] text-[var(--text-1)]'
                : 'text-[var(--text-2)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-1)]',
            )}
            style={selected && accentVar ? { color: accentVar } : undefined}
          >
            <Icon size={15} strokeWidth={1.75} />
          </button>
        )
      })}
    </div>
  )
}
