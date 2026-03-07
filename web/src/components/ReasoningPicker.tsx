import { useState } from 'react'
import { useStore } from '../lib/store'
import { rpcCall } from '../lib/rpc'
import { X, Check, AlertCircle } from 'lucide-react'
import type { ReasoningEffort, ReasoningSummary } from '../lib/types'

const EFFORTS: ReasoningEffort[] = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh']
const SUMMARIES: ReasoningSummary[] = ['off', 'auto', 'concise', 'detailed']

export default function ReasoningPicker() {
  const { reasoningPickerTarget, setReasoningPickerTarget } = useStore()
  const [effort, setEffort] = useState<ReasoningEffort>('medium')
  const [summary, setSummary] = useState<ReasoningSummary>('auto')
  const [status, setStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [errorMsg, setErrorMsg] = useState('')

  const threadId = reasoningPickerTarget?.threadId ?? ''
  const role = reasoningPickerTarget?.role ?? null

  function close() {
    setReasoningPickerTarget(null)
  }

  async function apply() {
    setStatus('loading')
    try {
      await rpcCall('control.setReasoning', {
        threadId,
        effort,
        summary,
        ...(role != null ? { target: role } : {}),
      })
      setStatus('success')
      setTimeout(() => close(), 600)
    } catch (err: unknown) {
      setStatus('error')
      setErrorMsg(err instanceof Error ? err.message : 'Failed to set reasoning')
    }
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
          width: 420,
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
        }}>
          <span style={{ fontWeight: 600, fontSize: 13, color: 'var(--text-1)' }}>
            {role != null
              ? <>Set reasoning for <span style={{ color: 'var(--accent)', textTransform: 'uppercase', fontSize: 11, letterSpacing: '0.04em' }}>{role}</span></>
              : 'Set reasoning (all roles)'}
          </span>
          <button
            onClick={close}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-3)', display: 'flex', padding: 4 }}
          >
            <X size={14} />
          </button>
        </div>

        {/* Status feedback */}
        {status === 'success' && (
          <div style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '8px 16px', background: 'var(--green-dim)', color: 'var(--green)',
            fontSize: 12,
          }}>
            <Check size={14} /> Reasoning updated successfully
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

        {/* Effort section */}
        <div style={{ padding: '14px 16px 8px' }}>
          <div style={{ fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-3)', marginBottom: 8 }}>
            Effort
          </div>
          <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            {EFFORTS.map(e => (
              <button
                key={e}
                onClick={() => setEffort(e)}
                style={{
                  padding: '5px 12px', borderRadius: 999,
                  border: effort === e ? '1px solid var(--accent)' : '1px solid var(--border)',
                  background: effort === e ? 'var(--accent-dim)' : 'var(--bg-elevated)',
                  color: effort === e ? 'var(--accent)' : 'var(--text-2)',
                  fontSize: 12, fontWeight: 500, cursor: 'pointer',
                  fontFamily: 'inherit', transition: 'all 0.15s',
                }}
              >
                {e}
              </button>
            ))}
          </div>
        </div>

        {/* Summary section */}
        <div style={{ padding: '8px 16px 14px' }}>
          <div style={{ fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-3)', marginBottom: 8 }}>
            Summary
          </div>
          <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            {SUMMARIES.map(s => (
              <button
                key={s}
                onClick={() => setSummary(s)}
                style={{
                  padding: '5px 12px', borderRadius: 999,
                  border: summary === s ? '1px solid var(--accent)' : '1px solid var(--border)',
                  background: summary === s ? 'var(--accent-dim)' : 'var(--bg-elevated)',
                  color: summary === s ? 'var(--accent)' : 'var(--text-2)',
                  fontSize: 12, fontWeight: 500, cursor: 'pointer',
                  fontFamily: 'inherit', transition: 'all 0.15s',
                }}
              >
                {s}
              </button>
            ))}
          </div>
        </div>

        {/* Apply button */}
        <div style={{ padding: '0 16px 14px', display: 'flex', justifyContent: 'flex-end' }}>
          <button
            onClick={apply}
            disabled={status === 'loading'}
            style={{
              padding: '7px 20px', borderRadius: 'var(--r-sm)',
              border: '1px solid var(--accent)', background: 'var(--accent-dim)',
              color: 'var(--accent)', fontSize: 12, fontWeight: 600,
              cursor: status === 'loading' ? 'wait' : 'pointer',
              fontFamily: 'inherit', transition: 'background 0.15s',
            }}
          >
            Apply
          </button>
        </div>
      </div>
    </div>
  )
}
