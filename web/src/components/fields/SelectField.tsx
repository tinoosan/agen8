import { labelStyle } from './fieldStyles'
import { CustomSelect } from './CustomSelect'

export function SelectField({ label, value, onChange, options, description }: {
  label: string
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  description?: string
}) {
  return (
    <div>
      <span className={labelStyle}>{label}</span>
      <CustomSelect
        value={value}
        onChange={onChange}
        options={options}
        className="w-full max-w-[280px] border-[var(--border)] bg-transparent"
      />
      {description && (
        <span className="text-[0.6875rem] text-[var(--text-3)] mt-1 block">{description}</span>
      )}
    </div>
  )
}
