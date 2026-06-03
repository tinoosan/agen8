import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ArtifactNode, FilesListDirResult } from '../lib/types'

interface ArtifactListResult {
  nodes: ArtifactNode[]
}

export function useArtifactFiles(spaceId: string | null) {
  return useQuery<ArtifactNode[]>({
    queryKey: ['artifact.list.files', spaceId],
    queryFn: async () => {
      const res = await rpcCall<ArtifactListResult>('artifact.list', {
        spaceId: spaceId ?? undefined,
      })
      return (res.nodes ?? []).filter((node) => node.kind === 'file' && (node.vpath ?? '').startsWith('/workspace/'))
    },
    enabled: !!spaceId,
    refetchInterval: 5000,
    staleTime: 2000,
    retry: false,
  })
}

export function useProjectArtifactFiles(projectId: string | null, projectRoot: string | null, spaceId: string | null) {
  return useQuery<ArtifactNode[]>({
    queryKey: ['artifact.list.files.project', projectId, projectRoot, spaceId],
    queryFn: async () => {
      const res = await rpcCall<ArtifactListResult>('artifact.list', {
        projectId: projectId ?? undefined,
        projectRoot: projectRoot ?? undefined,
        spaceId: spaceId ?? undefined,
      })
      return (res.nodes ?? []).filter((node) => node.kind === 'file' && (node.vpath ?? '').startsWith('/workspace/'))
    },
    enabled: !!projectId && !!projectRoot,
    refetchInterval: 5000,
    staleTime: 2000,
    retry: false,
  })
}

export function useProjectFileRefs(projectId: string | null, projectRoot: string | null) {
  return useQuery<ArtifactNode[]>({
    queryKey: ['artifact.file-refs.project-root', projectId, projectRoot],
    queryFn: async () => {
      if (!projectId || !projectRoot) return []
      const filePaths = await listAllProjectFiles(projectId, projectRoot)
      return filePaths.map((vpath) => syntheticFileArtifact(vpath))
    },
    enabled: !!projectId && !!projectRoot,
    staleTime: 5 * 60_000,
    gcTime: 15 * 60_000,
    retry: false,
  })
}

async function listAllProjectFiles(projectId: string, projectRoot: string): Promise<string[]> {
  const queue: string[] = ['/project']
  const visited = new Set<string>()
  const files: string[] = []

  while (queue.length > 0) {
    const dir = queue.shift()
    if (!dir || visited.has(dir)) continue
    visited.add(dir)

    const result = await rpcCall<FilesListDirResult>('files.listDir', {
      projectId,
      projectRoot,
      path: dir,
    })

    for (const entry of result.entries ?? []) {
      if (entry.isDir) {
        if (!visited.has(entry.path)) queue.push(entry.path)
        continue
      }
      files.push(entry.path)
    }
  }

  return files
}

function syntheticFileArtifact(vpath: string): ArtifactNode {
  const label = vpath.split('/').filter(Boolean).pop() ?? vpath
  return {
    nodeKey: `file:${vpath}`,
    kind: 'file',
    label,
    displayName: label,
    vpath,
  }
}
