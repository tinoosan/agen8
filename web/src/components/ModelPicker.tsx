import { useState, useRef, useEffect } from 'react'
import { useStore } from '../lib/store'
import { useModelList } from '../hooks/useModelList'
import { rpcCall } from '../lib/rpc'
import { X, Search, Check, AlertCircle } from 'lucide-react'
import type { ModelEntry } from '../lib/types'

export default function ModelPicker() {
  const { modelPickerTarget, setModelPickerTarget } = useStore()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [errorMsg, setErrorMsg] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const threadId = modelPickerTarget?.threadId ?? null
  const roleName = modelPickerTarget?.role ?? ''

  const modelQuery = useModelList(threadId)
  const models = modelQuery.data?.models ?? []

  // Filter by search query
  const filtered = query
    ? models.filter(m =>
      m.id.toLowerCase().includes(query.toLowerCase()) ||
      m.provider.toLowerCase().includes(query.toLowerCase())
    )
    : models

  // Group by provider
  const grouped: Record<string, ModelEntry[]> = {}
  for (const m of filtered) {
    const key = m.provider || 'other'
    if (!grouped[key]) grouped[key] = []
    grouped[key].push(m)
  }

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  async function selectModel(model: ModelEntry) {
    setStatus('loading')
    try {
      await rpcCall('control.setModel', { threadId, model: model.id, target: roleName })
      setStatus('success')
      setTimeout(() => setModelPickerTarget(null), 600)
    } catch (err: unknown) {
      setStatus('error')
      setErrorMsg(err instanceof Error ? err.message : 'Failed to set model')
    }
  }

  function close() {
    setModelPickerTarget(null)
  }

  function formatPrice(perM?: number): string {
    if (perM == null) return ''
    return `$${perM.toFixed(2)}/M`
  }

  return (
    <div
      style={{
        position: 'fixed', inset: 0, zIndex: 100,
        background: 'rgba(0,0,0,0.6)',
        display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
        paddingTop: '14vh',
        backdropFilter: 'blur(8px)',
      }}
      onClick={close}
    >
      <div
        onClick={e => e.stopPropagation()}
        className="animate-scale-in"
        style={{
          width: 480, maxHeight: '60vh',
          background: 'var(--bg-surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--r-xl)',
          boxShadow: '0 24px 80px rgba(0,0,0,0.5)',
          display: 'flex', flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '12px 16px',
          borderBottom: '1px solid var(--border)',
          flexShrink: 0,
        }}>
          <span style={{ fontWeight: 600, fontSize: 13, color: 'var(--text-1)' }}>
            Change model for <span style={{ color: 'var(--accent)', textTransform: 'uppercase', fontSize: 11, letterSpacing: '0.04em' }}>{roleName}</span>
          </span>
          <button
            onClick={close}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-3)', display: 'flex', padding: 4 }}
          >
            <X size={14} />
          </button>
        </div>

        {/* Search */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '8px 16px',
          borderBottom: '1px solid var(--border)',
        }}>
          <Search size={14} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Search models..."
            style={{
              background: 'transparent', border: 'none', outline: 'none',
              color: 'var(--text-1)', fontSize: 13, fontFamily: 'inherit', width: '100%',
            }}
          />
        </div>

        {/* Status feedback */}
        {status === 'success' && (
          <div style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '8px 16px', background: 'var(--green-dim)', color: 'var(--green)',
            fontSize: 12,
          }}>
            <Check size={14} /> Model updated successfully
          </div>
        )}
        {status === 'error' && (
          <div style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '8px 16px', background: 'var(--red-dim)', color: 'var(--red)',
            fontSize: 12,
          }}>
            <AlertCircle size={14} /> {errorMsg}
          </div>
        )}

        {/* Model list */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '8px 0' }}>
          {modelQuery.isLoading ? (
            <div style={{ padding: '20px 16px', display: 'flex', flexDirection: 'column', gap: 8 }}>
              <div className="skeleton" style={{ width: '100%', height: 32 }} />
              <div className="skeleton" style={{ width: '100%', height: 32 }} />
              <div className="skeleton" style={{ width: '100%', height: 32 }} />
            </div>
          ) : filtered.length === 0 ? (
            <div style={{ padding: '20px 16px', textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>
              {models.length > 0 ? 'No models match your search' : 'No models available'}
            </div>
          ) : (
            Object.entries(grouped).map(([provider, providerModels]) => (
              <div key={provider}>
                <div style={{
                  padding: '6px 16px', fontSize: 10, fontWeight: 600,
                  textTransform: 'uppercase', letterSpacing: '0.06em',
                  color: 'var(--text-3)',
                }}>
                  {provider}
                </div>
                {providerModels.map(model => (
                  <button
                    key={model.id}
                    onClick={() => selectModel(model)}
                    disabled={status === 'loading'}
                    style={{
                      display: 'flex', alignItems: 'center', gap: 8,
                      width: '100%', padding: '8px 16px',
                      background: 'transparent', border: 'none',
                      color: 'var(--text-1)', fontSize: 12,
                      cursor: status === 'loading' ? 'wait' : 'pointer',
                      fontFamily: 'inherit', textAlign: 'left',
                      transition: 'background 0.1s',
                    }}
                    className="row-hover"
                  >
                    <span style={{ flex: 1 }} className="mono truncate">{model.id}</span>
                    {model.isReasoning && (
                      <span style={{
                        fontSize: 9, fontWeight: 600, padding: '0 5px',
                        borderRadius: 999, background: 'var(--amber-dim)', color: 'var(--amber)',
                      }}>
                        reasoning
                      </span>
                    )}
                    {(model.inputPerM != null || model.outputPerM != null) && (
                      <span style={{
                        display: 'flex', alignItems: 'center', gap: 4,
                        fontSize: 10, whiteSpace: 'nowrap',
                        background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                        borderRadius: 'var(--r-sm)', padding: '1px 6px',
                      }}>
                        <span style={{ color: 'var(--green)' }}>{formatPrice(model.inputPerM)}</span>
                        <span style={{ color: 'var(--text-4)' }}>/</span>
                        <span style={{ color: 'var(--amber)' }}>{formatPrice(model.outputPerM)}</span>
                      </span>
                    )}
                  </button>
                ))}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
