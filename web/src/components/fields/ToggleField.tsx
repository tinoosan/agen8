import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

export function ToggleField({ label, description, checked, onChange }: {
  label: string
  description?: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-start gap-2.5">
      <Switch
        checked={checked}
        onCheckedChange={onChange}
        className="shrink-0 mt-0.5"
      />
      <div
        className="cursor-pointer"
        onClick={() => onChange(!checked)}
      >
        <Label className={`block text-xs font-medium text-[var(--text-2)] cursor-pointer ${description ? 'mb-0.5' : 'mb-0'}`}>{label}</Label>
        {description && (
          <div className="text-[0.6875rem] text-[var(--text-3)] leading-[1.4]">{description}</div>
        )}
      </div>
    </div>
  )
}
