import { useState, useMemo } from 'react'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { useNavigation } from '../../lib/routing'
import { rpcCall } from '../../lib/rpc'
import { artifactIdentity, dedupeArtifactNodes } from '../../lib/artifactNodes'
import { ChevronLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useArtifactFiles, useProjectArtifactFiles } from '../../hooks/useArtifactFiles'
import type { ArtifactGetResult, ArtifactNode } from '../../lib/types'
import ArtifactViewer from '../files/ArtifactViewer'
import ArtifactTree from '../files/ArtifactTree'

const ARTIFACT_PREVIEW_MAX_BYTES = 8 * 1024 * 1024

interface SpaceFilesTabProps {
  spaceId: string
}

export default function SpaceFilesTab({ spaceId }: SpaceFilesTabProps) {
  const { projectId, focusedProjectRoot } = useNavigation()

  const projectQuery = useProjectArtifactFiles(projectId, focusedProjectRoot ?? '', spaceId)
  const spaceQuery = useArtifactFiles(spaceId)
  const query = focusedProjectRoot ? projectQuery : spaceQuery
  const artifacts = useMemo(() => dedupeArtifactNodes(query.data ?? []), [query.data])

  const [selectedFile, setSelectedFile] = useState<ArtifactNode | null>(null)

  const effectiveSelectedFile = useMemo(() => {
    if (!selectedFile) return null
    const id = artifactIdentity(selectedFile)
    return artifacts.find(a => artifactIdentity(a) === id) ?? null
  }, [artifacts, selectedFile])

  const previewVPath = effectiveSelectedFile?.vpath ?? ''
  const previewArtifactId = previewVPath ? undefined : effectiveSelectedFile?.artifactId
  const selectedKey = effectiveSelectedFile
    ? (effectiveSelectedFile.nodeKey ?? artifactIdentity(effectiveSelectedFile))
    : null

  const previewQuery = useQuery<ArtifactGetResult>({
    queryKey: ['artifact.get', spaceId, selectedKey, previewVPath, previewArtifactId ?? ''],
    queryFn: async () => rpcCall<ArtifactGetResult>('artifact.get', {
      projectRoot: focusedProjectRoot ?? undefined,
      spaceId,
      artifactId: previewArtifactId,
      vpath: previewVPath || undefined,
      maxBytes: ARTIFACT_PREVIEW_MAX_BYTES,
    }),
    enabled: !!spaceId && !!effectiveSelectedFile,
    retry: false,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  })

  const isOpen = !!effectiveSelectedFile

  return (
    <div className="flex h-full min-h-0">
      {/* File list */}
      <div
        className="shrink-0 flex flex-col min-h-0 border-r border-[var(--border)]"
        style={{ width: isOpen ? 260 : '100%' }}
      >
        {/* Header */}
        <div className="flex items-center gap-2 px-4 py-3 border-b border-[var(--border)] shrink-0">
          {isOpen && (
            <Button
              variant="ghost"
              size="icon"
              className="mr-0.5 h-7 w-7"
              onClick={() => setSelectedFile(null)}
              title="Back to file list"
              aria-label="Back to file list"
            >
              <ChevronLeft size={16} />
            </Button>
          )}
          <span className="font-semibold text-[13px] text-[var(--text-1)] tracking-[-0.02em]">Files</span>
          {artifacts.length > 0 && (
            <span className="text-[11px] text-[var(--text-3)] bg-[var(--bg-elevated)] border border-[var(--border)] rounded-full px-[7px] tabular-nums">
              {artifacts.length}
            </span>
          )}
        </div>

        {/* Tree */}
        <ArtifactTree
          artifacts={artifacts}
          selectedIdentity={effectiveSelectedFile ? artifactIdentity(effectiveSelectedFile) : null}
          onSelectFile={setSelectedFile}
          isLoading={query.isLoading}
          className="flex-1"
        />
      </div>

      {/* Content panel */}
      {isOpen && (
        <div className="flex-1 min-h-0 min-w-0">
          <ArtifactViewer
            file={effectiveSelectedFile!}
            preview={previewQuery.data}
            isLoading={previewQuery.isLoading && !previewQuery.isPlaceholderData}
            error={!!previewQuery.error}
            variant="page"
          />
        </div>
      )}
    </div>
  )
}
