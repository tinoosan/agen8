import { useState, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function TagField({ label, tags, onChange, placeholder }: {
  label: string
  tags: string[]
  onChange: (tags: string[]) => void
  placeholder?: string
}) {
  const [input, setInput] = useState('')

  const commit = () => {
    const val = input.trim()
    if (val && !tags.includes(val)) {
      onChange([...tags, val])
    }
    setInput('')
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') { e.preventDefault(); commit() }
    if (e.key === 'Backspace' && input === '' && tags.length > 0) {
      onChange(tags.slice(0, -1))
    }
  }

  return (
    <div>
      <Label className="block text-xs font-medium text-[var(--text-2)] mb-1">{label}</Label>
      {tags.length > 0 && (
        <div className="flex flex-wrap gap-1 mb-1.5">
          {tags.map((tag, i) => (
            <span key={i} className="inline-flex items-center gap-1 px-2 py-[2px] text-[11px] font-medium text-[var(--text-1)] bg-[var(--bg-surface)] border border-[var(--border)] rounded-[var(--r-sm)] font-[var(--font-mono,monospace)]">
              {tag}
              <button
                type="button"
                onClick={() => onChange(tags.filter((_, j) => j !== i))}
                className="bg-transparent border-none cursor-pointer text-[var(--text-3)] p-0 flex leading-none"
              >
                <X size={10} />
              </button>
            </span>
          ))}
        </div>
      )}
      <Input
        type="text"
        value={input}
        onChange={e => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={commit}
        placeholder={placeholder ?? 'Type and press Enter to add'}
        className="h-auto px-2.5 py-[7px] text-[13px] bg-[var(--bg-elevated)] border-[var(--border)] rounded-[var(--r-md)]"
      />
    </div>
  )
}
