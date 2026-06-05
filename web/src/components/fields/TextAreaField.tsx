import { useRef, useEffect } from 'react'
import { cn } from '@/lib/utils'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'

export function TextAreaField({ label, value, onChange, placeholder, mono, rows, description, autoGrow, maxRows, charCount }: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  mono?: boolean
  rows?: number
  description?: string
  autoGrow?: boolean
  maxRows?: number
  charCount?: boolean
}) {
  const ref = useRef<HTMLTextAreaElement>(null)
  const baseRows = rows ?? (autoGrow ? 6 : 4)
  const maxHeight = (maxRows ?? 24) * 20 // ~20px per row

  useEffect(() => {
    if (!autoGrow || !ref.current) return
    const el = ref.current
    el.style.height = 'auto'
    const clamped = Math.min(el.scrollHeight, maxHeight)
    el.style.height = clamped + 'px'
    // Switch to scroll when content exceeds max height
    el.style.overflowY = el.scrollHeight > maxHeight ? 'auto' : 'hidden'
  }, [value, autoGrow, maxHeight])

  return (
    <label>
      <div className="flex justify-between items-baseline">
        <Label className="block text-xs font-medium text-[var(--text-2)] mb-1">{label}</Label>
        {charCount && (
          <span className="text-[0.625rem] text-[var(--text-3)] font-[var(--font-mono,monospace)]">
            {value.length}
          </span>
        )}
      </div>
      <Textarea
        ref={ref}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        rows={baseRows}
        className={cn(
          'h-auto px-2.5 py-[7px] text-[0.8125rem] bg-[var(--bg-elevated)] border-[var(--border)] rounded-[var(--r-md)]',
          mono ? 'font-[var(--font-mono,monospace)]' : 'font-[inherit]',
          autoGrow ? 'resize-none overflow-hidden' : 'resize-y',
        )}
        style={autoGrow || rows ? undefined : { minHeight: 80 }}
      />
      {description && (
        <span className="text-[0.6875rem] text-[var(--text-3)] mt-0.5 block">{description}</span>
      )}
    </label>
  )
}
