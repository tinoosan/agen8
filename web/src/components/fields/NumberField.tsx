import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function NumberField({ label, value, onChange, min, max, step, description }: {
  label: string
  value: number | undefined
  onChange: (v: number | undefined) => void
  min?: number
  max?: number
  step?: number
  description?: string
}) {
  return (
    <label>
      <Label className="block text-xs font-medium text-[var(--text-2)] mb-1">{label}</Label>
      <Input
        type="number"
        value={value ?? ''}
        onChange={e => {
          const v = e.target.value
          onChange(v === '' ? undefined : Number(v))
        }}
        min={min}
        max={max}
        step={step}
        className={cn(
          'h-auto px-2.5 py-[7px] text-[0.8125rem] bg-[var(--bg-elevated)] border-[var(--border)] rounded-[var(--r-md)] w-[120px]',
        )}
      />
      {description && (
        <span className="text-[0.6875rem] text-[var(--text-3)] mt-0.5 block">{description}</span>
      )}
    </label>
  )
}
