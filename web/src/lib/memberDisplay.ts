export function memberDisplayName(label?: string, id?: string): string | undefined {
  const cleanLabel = label?.trim()
  if (cleanLabel && !isRawIdentity(cleanLabel)) return cleanLabel.replaceAll('_', ' ')

  const cleanId = id?.trim()
  if (!cleanId) return undefined

  if (isRawIdentity(cleanId)) return undefined

  return cleanId.replaceAll('_', ' ')
}

function isRawIdentity(value: string): boolean {
  return /^(member|user|session|thread|channel|space|project|task|kr|mission|dec)-[a-z0-9-]{4,}$/i.test(value.trim())
}
