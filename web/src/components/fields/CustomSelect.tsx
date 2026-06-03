import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

export interface CustomSelectOption {
  value: string
  label: string
}

/**
 * Radix Select does not allow empty-string values (it uses "" to mean
 * "no selection / show placeholder"). Many consumers pass { value: '', label: 'All ...' }
 * as an "everything" option, so we translate "" ↔ a sentinel internally.
 */
const EMPTY_SENTINEL = '__all__'

function toRadix(v: string) { return v === '' ? EMPTY_SENTINEL : v }
function fromRadix(v: string) { return v === EMPTY_SENTINEL ? '' : v }

export function CustomSelect({ value, onChange, options, className, 'data-testid': testId }: {
  value: string
  onChange: (value: string) => void
  options: CustomSelectOption[]
  className?: string
  'data-testid'?: string
}) {
  return (
    <Select value={toRadix(value)} onValueChange={v => onChange(fromRadix(v))}>
      <SelectTrigger
        data-testid={testId}
        className={cn(
          'h-auto px-3 py-2 text-sm',
          className,
        )}
      >
        <SelectValue placeholder="Select..." />
      </SelectTrigger>
      <SelectContent>
        {options.map(o => (
          <SelectItem key={toRadix(o.value)} value={toRadix(o.value)}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
