import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function TextField({ label, value, onChange, placeholder, mono, description, type = 'text' }: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  mono?: boolean
  description?: string
  type?: string
}) {
  return (
    <label>
      <Label className="block text-xs font-medium text-[var(--text-2)] mb-1">{label}</Label>
      <Input
        type={type}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className={cn(
          'h-auto px-2.5 py-[7px] text-[0.8125rem] bg-[var(--bg-elevated)] border-[var(--border)] rounded-[var(--r-md)]',
          mono ? 'font-[var(--font-mono,monospace)]' : 'font-[inherit]',
        )}
      />
      {description && (
        <span className="text-[0.6875rem] text-[var(--text-3)] mt-0.5 block">{description}</span>
      )}
    </label>
  )
}
