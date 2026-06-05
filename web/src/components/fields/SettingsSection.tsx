import type { ReactNode } from 'react'

export function SettingsSection({ title, description, restartRequired, children }: {
  title: string
  description?: string
  restartRequired?: boolean
  children: ReactNode
}) {
  return (
    <div className="py-5 border-b border-[var(--border)] last:border-b-0">
      <div className={`flex items-baseline gap-2 ${description ? 'mb-0.5' : 'mb-4'}`}>
        <h3 className="m-0 text-sm font-semibold text-[var(--text-1)] tracking-[-0.02em]">
          {title}
        </h3>
        {restartRequired && (
          <span className="text-[0.625rem] font-semibold px-[7px] py-px rounded-full bg-[var(--amber-dim)] text-[var(--amber)] tracking-[0.01em] whitespace-nowrap">
            restart required
          </span>
        )}
      </div>
      {description && (
        <p className="m-0 mb-4 text-xs text-[var(--text-3)] leading-normal">{description}</p>
      )}
      <div className="flex flex-col gap-3.5">
        {children}
      </div>
    </div>
  )
}
