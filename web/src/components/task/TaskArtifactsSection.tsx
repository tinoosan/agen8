import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { FileText, Paperclip } from 'lucide-react'
import { toast } from 'sonner'
import { rpcCall } from '../../lib/rpc'
import { qk } from '../../lib/queryKeys'
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

/** Bare file name for the attachment path: no separators, no traversal. */
function safeAttachmentName(name: string): string {
  const base = name.split(/[/\\]/).pop() ?? ''
  const cleaned = base.replace(/\.\./g, '.').trim()
  return cleaned || `attachment-${Date.now()}`
}

function fileToBase64(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error ?? new Error('read failed'))
    reader.onload = () => {
      const url = String(reader.result)
      resolve(url.slice(url.indexOf(',') + 1)) // strip the data:...;base64, prefix
    }
    reader.readAsDataURL(file)
  })
}

interface TaskArtifactsSectionProps {
  task: Task
  projectId: string | null
  /** Called with the vpath when a file artifact row is clicked. The parent owns the viewer host (sheet or split panel). */
  onOpenArtifact: (vpath: string) => void
}

export function TaskArtifactsSection({ task, projectId, onOpenArtifact }: TaskArtifactsSectionProps) {
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [uploading, setUploading] = useState(false)
  const canUpload = !!projectId && task.status !== 'canceled'

  const uploadAttachment = useCallback(async (file: Blob, name: string) => {
    if (!projectId || uploading) return
    setUploading(true)
    try {
      const fileName = safeAttachmentName(name)
      const vpath = `/project/.agen8/attachments/${task.id}/${fileName}`
      const bytesB64 = await fileToBase64(file)
      await rpcCall('files.upload', { projectId, path: vpath, bytesB64 })
      // Server-side append (task.attachArtifact -> Service.AttachArtifact):
      // never read-modify-write the artifacts array in the client.
      await rpcCall('task.attachArtifact', { taskId: task.id, ref: FILE_REF_PREFIX + vpath })
      await queryClient.invalidateQueries({ queryKey: qk.taskGet(task.id) })
      toast.success(`Attached ${fileName}`)
    } catch (err) {
      toast.error(`Attach failed: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      setUploading(false)
    }
  }, [projectId, task.id, uploading, queryClient])

  // Paste-to-attach: the fastest path for the screenshot-review flow.
  useEffect(() => {
    if (!canUpload) return
    const onPaste = (e: ClipboardEvent) => {
      // Don't hijack pastes aimed at editable fields (inline edit, dialogs).
      const target = e.target as HTMLElement | null
      if (target && typeof target.closest === 'function' && target.closest('input, textarea, [contenteditable="true"]')) return
      const image = [...(e.clipboardData?.items ?? [])].find((item) => item.type.startsWith('image/'))
      if (!image) return
      const blob = image.getAsFile()
      if (!blob) return
      e.preventDefault()
      const ext = image.type.split('/')[1] || 'png'
      void uploadAttachment(blob, `pasted-${new Date().toISOString().replace(/[:.]/g, '-')}.${ext}`)
    }
    document.addEventListener('paste', onPaste)
    return () => document.removeEventListener('paste', onPaste)
  }, [canUpload, uploadAttachment])

  const artifacts = task.artifacts ?? []
  if (artifacts.length === 0 && !canUpload) return null

  return (
    <CollapsibleSection
      storageKey="task-detail-artifacts"
      defaultOpen={false}
      label={<>Artifacts <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>({artifacts.length})</span></>}
    >
      <div style={{ borderTop: artifacts.length > 0 ? '1px solid var(--border)' : 'none' }}>
        {artifacts.map((ref, i) => {
          const vpath = fileArtifactVPath(ref)
          return (
            <div
              key={i}
              style={{
                paddingTop: 8,
                paddingBottom: 8,
                borderBottom: i < artifacts.length - 1 ? '1px solid var(--border)' : 'none',
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
      {canUpload && (
        <div style={{ paddingTop: 8 }}>
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            aria-label="Attachment file"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) void uploadAttachment(file, file.name)
              e.target.value = ''
            }}
          />
          <button
            type="button"
            disabled={uploading}
            onClick={() => fileInputRef.current?.click()}
            className="inline-flex items-center gap-1.5 border-none cursor-pointer bg-transparent text-[var(--text-2)] hover:text-[var(--text-1)] transition-colors disabled:opacity-50"
            style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px', padding: 0 }}
          >
            <Paperclip size={13} />
            {uploading ? 'Attaching…' : 'Attach file'}
          </button>
          <span className="text-[var(--text-3)]" style={{ fontSize: '0.6875rem', marginLeft: 8 }}>
            or paste an image
          </span>
        </div>
      )}
    </CollapsibleSection>
  )
}
