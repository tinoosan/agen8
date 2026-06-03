import type { ArtifactNode } from './types'

export function artifactIdentity(file: ArtifactNode): string {
  const diskPath = file.diskPath?.trim() ?? ''
  if (diskPath) return `disk:${diskPath}`

  const vpath = file.vpath?.trim() ?? ''
  if (vpath) return `vpath:${vpath}`

  const artifactId = file.artifactId?.trim() ?? ''
  if (artifactId) return `artifact:${artifactId}`

  return file.nodeKey ?? `${file.displayName ?? file.label ?? ''}`
}

function artifactVersionRank(file: ArtifactNode): number {
  const producedAt = file.producedAt ? Date.parse(file.producedAt) : 0
  return Number.isFinite(producedAt) ? producedAt : 0
}

export function dedupeArtifactNodes(files: ArtifactNode[]): ArtifactNode[] {
  const byIdentity = new Map<string, ArtifactNode>()

  for (const file of files) {
    const key = artifactIdentity(file)
    const existing = byIdentity.get(key)
    if (!existing) {
      byIdentity.set(key, file)
      continue
    }

    const candidateRank = artifactVersionRank(file)
    const existingRank = artifactVersionRank(existing)
    if (candidateRank > existingRank) {
      byIdentity.set(key, file)
      continue
    }

    if (candidateRank === existingRank) {
      const candidateArtifactId = file.artifactId ?? ''
      const existingArtifactId = existing.artifactId ?? ''
      if (candidateArtifactId > existingArtifactId) {
        byIdentity.set(key, file)
      }
    }
  }

  return Array.from(byIdentity.values())
}
