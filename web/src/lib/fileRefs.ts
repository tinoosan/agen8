import type { ArtifactNode } from './types'

export function normalizeReferencePath(path: string): string {
  return path
    .trim()
    .replace(/^@/, '')
    .replace(/^\/?(workspace|project)\/?/, '')
    .replace(/^\/+/, '')
}

export function artifactReferencePath(file: ArtifactNode): string {
  return normalizeReferencePath(file.vpath ?? file.displayName ?? file.label)
}

function artifactSearchText(file: ArtifactNode): string {
  return [
    artifactReferencePath(file),
    normalizeReferencePath(file.displayName ?? ''),
    normalizeReferencePath(file.label ?? ''),
    normalizeReferencePath(file.vpath ?? ''),
  ].join(' ').toLowerCase()
}

export function filterArtifactsByRefQuery(files: ArtifactNode[], query: string, limit = 8): ArtifactNode[] {
  const normalizedQuery = normalizeReferencePath(query).toLowerCase()
  const candidates = files.filter((file) => file.kind === 'file')
  if (!normalizedQuery) return candidates.slice(0, limit)

  return candidates
    .filter((file) => artifactSearchText(file).includes(normalizedQuery))
    .slice(0, limit)
}

export function findArtifactByRef(files: ArtifactNode[], ref: string): ArtifactNode | null {
  const normalizedRef = normalizeReferencePath(ref).toLowerCase()
  if (!normalizedRef) return null

  return files.find((file) => {
    const candidates = [
      artifactReferencePath(file),
      normalizeReferencePath(file.displayName ?? ''),
      normalizeReferencePath(file.label ?? ''),
      normalizeReferencePath(file.vpath ?? ''),
    ].filter(Boolean).map((value) => value.toLowerCase())
    return candidates.includes(normalizedRef)
  }) ?? null
}
