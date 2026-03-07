import type { TeamRoleStatus } from '../lib/types'
import { useState } from 'react'
import { ChevronDown, ChevronRight, Activity, Cpu, DollarSign, Eye } from 'lucide-react'

interface RoleRowProps {
  role: TeamRoleStatus
  stats?: {
    tokens: number
    cost: number
    model?: string
  }
  onViewTranscript?: (role: string) => void
  isActive?: boolean
}

function inferStatus(info: string): 'active' | 'idle' | 'pending' | 'failed' | 'done' {
  const lower = info.toLowerCase()
  if (lower.includes('idle') || lower.includes('waiting') || lower === '') return 'idle'
  if (lower.includes('fail') || lower.includes('error')) return 'failed'
  if (lower.includes('done') || lower.includes('complet')) return 'done'
  if (lower.includes('pending') || lower.includes('queue')) return 'pending'
  return 'active'
}

export default function RoleRow({ role, stats, onViewTranscript, isActive: isFocused }: RoleRowProps) {
  const [isExpanded, setIsExpanded] = useState(false)
  const status = inferStatus(role.info)

  // Determine status color
  const statusColor =
    status === 'active' ? 'var(--accent)' :
      status === 'done' ? 'var(--green)' :
        status === 'failed' ? 'var(--red)' :
          'var(--text-4)'

  return (
    <div
      onClick={() => setIsExpanded(!isExpanded)}
      style={{
        display: 'flex', flexDirection: 'column', gap: 4,
        padding: '10px 14px',
        marginBottom: 6,
        background: isFocused ? 'var(--accent-dim)' : isExpanded ? 'rgba(255, 255, 255, 0.04)' : 'rgba(255, 255, 255, 0.02)',
        border: isFocused ? '1px solid var(--accent)' : '1px solid rgba(255, 255, 255, 0.04)',
        borderRadius: 'var(--r-md)',
        transition: 'all 0.2s cubic-bezier(0.16, 1, 0.3, 1)',
        cursor: 'pointer',
        position: 'relative',
        backdropFilter: 'blur(8px)',
        WebkitBackdropFilter: 'blur(8px)',
      }}
      className="role-glass-hover"
      title={role.info || undefined}
    >
      {/* Top Header line: Status Indicator + Uppercase Role Name */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        {/* Sleek status indicator */}
        <div style={{
          width: 6, height: 6, borderRadius: '50%',
          background: statusColor,
          boxShadow: status === 'active' ? `0 0 8px ${statusColor}, 0 0 4px ${statusColor}` : 'none',
          opacity: status === 'idle' ? 0.3 : 1
        }} />

        {/* Role Name */}
        <div className="truncate" style={{
          fontSize: 10, fontWeight: 700, flex: 1,
          color: status === 'active' ? 'var(--text-1)' : 'var(--text-3)',
          textTransform: 'uppercase',
          letterSpacing: '0.08em',
        }}>
          {role.role.replace(/-/g, ' ')}
        </div>

        {/* Chevron */}
        <div style={{ color: 'var(--text-4)', display: 'flex' }}>
          {isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </div>
      </div>

      {/* Info Line */}
      <div className="truncate" style={{
        fontSize: 12, color: 'var(--text-2)',
        fontWeight: 400,
        paddingLeft: 12, // Indent to align with text above
      }}>
        {role.info || 'Waiting for tasks...'}
      </div>

      {/* Expanded Stats View */}
      {isExpanded && stats && (
        <div style={{
          marginTop: 8,
          paddingTop: 8,
          borderTop: '1px solid rgba(255, 255, 255, 0.06)',
          display: 'flex',
          flexDirection: 'column',
          gap: 6,
          paddingLeft: 12,
        }}>
          {stats.model && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--text-3)' }}>
              <Cpu size={12} style={{ opacity: 0.6 }} />
              <span className="truncate">{stats.model}</span>
            </div>
          )}
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--text-3)' }}>
            <Activity size={12} style={{ opacity: 0.6 }} />
            <span>{stats.tokens.toLocaleString()} tokens</span>
          </div>
          {stats.cost > 0 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--text-3)' }}>
              <DollarSign size={12} style={{ opacity: 0.6 }} />
              <span>${stats.cost.toFixed(4)} total cost</span>
            </div>
          )}
          {onViewTranscript && (
            <button
              onClick={(e) => { e.stopPropagation(); onViewTranscript(role.role) }}
              style={{
                display: 'flex', alignItems: 'center', gap: 5,
                marginTop: 4, padding: '4px 8px', borderRadius: 'var(--r-sm)',
                border: '1px solid var(--border)', background: 'var(--bg-elevated)',
                color: 'var(--accent)', fontSize: 10, fontWeight: 600,
                cursor: 'pointer', fontFamily: 'inherit',
                transition: 'background 0.15s',
              }}
            >
              <Eye size={10} />
              View transcript
            </button>
          )}
        </div>
      )}
    </div>
  )
}
