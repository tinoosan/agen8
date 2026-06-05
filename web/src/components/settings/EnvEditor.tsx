import clsx from 'clsx'
import { Plus, Trash2 } from 'lucide-react'
import { inputStyle, labelStyle } from '../fields'

export function EnvEditor({ env, onChange }: {
  env: Record<string, string>
  onChange: (env: Record<string, string>) => void
}) {
  const entries = Object.entries(env)

  const updateKey = (oldKey: string, newKey: string) => {
    const next: Record<string, string> = {}
    for (const [k, v] of entries) next[k === oldKey ? newKey : k] = v
    onChange(next)
  }

  const updateValue = (key: string, value: string) => {
    onChange({ ...env, [key]: value })
  }

  const remove = (key: string) => {
    const next = { ...env }
    delete next[key]
    onChange(next)
  }

  const add = () => {
    let key = 'NEW_VAR'
    let n = 1
    while (Object.prototype.hasOwnProperty.call(env, key)) key = `NEW_VAR_${n++}`
    onChange({ ...env, [key]: '' })
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <span className={labelStyle}>Environment Variables</span>
        <button
          type="button"
          onClick={add}
          className="inline-flex items-center gap-1 px-2.5 py-[3px] text-[0.6875rem] font-medium bg-[var(--bg-surface)] border border-[var(--border)] rounded-[var(--r-sm)] text-[var(--text-2)] cursor-pointer font-[inherit]"
        >
          <Plus size={11} />
          Add
        </button>
      </div>
      {entries.length === 0 ? (
        <p className="text-xs text-[var(--text-3)] m-0">No environment variables configured.</p>
      ) : (
        <div className="flex flex-col gap-1.5">
          {entries.map(([key, value]) => (
            <div key={key} className="flex gap-1.5 items-center">
              <input
                type="text"
                value={key}
                onChange={e => updateKey(key, e.target.value)}
                placeholder="KEY"
                className={clsx(inputStyle, 'flex-[0_0_40%] font-[var(--font-mono,monospace)] text-xs')}
              />
              <input
                type="text"
                value={value}
                onChange={e => updateValue(key, e.target.value)}
                placeholder="value"
                className={clsx(inputStyle, 'flex-1')}
              />
              <button
                type="button"
                onClick={() => remove(key)}
                className="bg-none border-none cursor-pointer text-[var(--text-3)] p-1 flex shrink-0"
              >
                <Trash2 size={13} />
              </button>
            </div>
          ))}
        </div>
      )}
      <p className="text-[0.6875rem] text-[var(--text-3)] mt-2 mb-0 leading-[1.5]">
        Non-secret defaults only. API keys should use the OS keychain.
      </p>
    </div>
  )
}
