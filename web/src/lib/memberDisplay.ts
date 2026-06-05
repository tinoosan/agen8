export function memberDisplayName(label?: string, id?: string): string | undefined {
  const cleanLabel = label?.trim()
  if (cleanLabel) return cleanLabel.replaceAll('_', ' ')

  const cleanId = id?.trim()
  if (!cleanId) return undefined

  if (cleanId.startsWith('member-') && cleanId.length > 'member-'.length + 6) {
    return `Member ${cleanId.slice(-6)}`
  }

  return cleanId.replaceAll('_', ' ')
}
