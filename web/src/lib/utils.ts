import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// Write text to the clipboard. Throws when the API is unavailable so callers
// can surface a failure; the caller owns any success/error feedback.
export async function copyText(text: string): Promise<void> {
  if (typeof navigator === 'undefined' || !navigator.clipboard) {
    throw new Error('Clipboard is unavailable in this browser')
  }
  await navigator.clipboard.writeText(text)
}
