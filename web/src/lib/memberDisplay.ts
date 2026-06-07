import { isPrefixedId } from './displaySanitizers'

export function memberDisplayName(label?: string, id?: string): string | undefined {
  const cleanLabel = label?.trim()
  if (cleanLabel && !isPrefixedId(cleanLabel)) return cleanLabel.replaceAll('_', ' ')

  const cleanId = id?.trim()
  if (!cleanId) return undefined

  return cleanId.replaceAll('_', ' ')
}
