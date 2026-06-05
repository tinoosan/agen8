export const inputStyle = 'w-full px-2.5 py-[7px] text-[0.8125rem] font-[inherit] bg-[var(--bg-elevated)] text-[var(--text-1)] border border-[var(--border)] rounded-[var(--r-md)] outline-none box-border'

export const labelStyle = 'block text-xs font-medium text-[var(--text-2)] mb-1'

export function jsonEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}
