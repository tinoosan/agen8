import { FileText } from 'lucide-react'
import { basename } from '../files/filePreviewUtils'
import { CollapsibleSection } from '../strategy/CollapsibleSection'
import type { Task } from '../../lib/types'

const FILE_REF_PREFIX = 'file:'

/** Extracts the vpath from a file:<vpath> artifact ref, or null for any other ref shape. */
export function fileArtifactVPath(ref: string): string | null {
  if (!ref.startsWith(FILE_REF_PREFIX)) return null
  const vpath = ref.slice(FILE_REF_PREFIX.length).trim()
  return vpath ? vpath : null
}

interface TaskArtifactsSectionProps {
  task: Task
  /** Called with the vpath when a file artifact row is clicked. The parent owns the viewer host (sheet or split panel). */
  onOpenArtifact: (vpath: string) => void
}

export function TaskArtifactsSection({ task, onOpenArtifact }: TaskArtifactsSectionProps) {
  if (!task.artifacts || task.artifacts.length === 0) return null

  return (
    <CollapsibleSection
      storageKey="task-detail-artifacts"
      defaultOpen={false}
      label={<>Artifacts <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>({task.artifacts.length})</span></>}
    >
      <div style={{ borderTop: '1px solid var(--border)' }}>
        {task.artifacts.map((ref, i) => {
          const vpath = fileArtifactVPath(ref)
          return (
            <div
              key={i}
              style={{
                paddingTop: 8,
                paddingBottom: 8,
                borderBottom: i < task.artifacts!.length - 1 ? '1px solid var(--border)' : 'none',
              }}
            >
              {vpath ? (
                <button
                  type="button"
                  onClick={() => onOpenArtifact(vpath)}
                  className="group inline-flex items-start gap-1.5 border-none cursor-pointer bg-transparent p-0 text-left"
                  aria-label={`View ${basename(vpath)}`}
                >
                  <FileText size={13} className="shrink-0 mt-px text-[var(--text-3)] group-hover:text-[var(--text-1)] transition-colors" />
                  <span style={{ fontSize: '0.75rem', wordBreak: 'break-all' }}>
                    <span className="text-[var(--accent)] group-hover:underline underline-offset-2" style={{ fontFamily: 'monospace' }}>
                      {basename(vpath)}
                    </span>
                    <span className="text-[var(--text-3)]" style={{ fontFamily: 'monospace' }}> {vpath}</span>
                  </span>
                </button>
              ) : (
                <span style={{ fontSize: '0.75rem', color: 'var(--accent)', fontFamily: 'monospace', wordBreak: 'break-all' }}>{ref}</span>
              )}
            </div>
          )
        })}
      </div>
    </CollapsibleSection>
  )
}
