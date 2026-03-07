import { useState } from 'react'
import { useQueries } from '@tanstack/react-query'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { useTeamManifest } from '../hooks/useTeamStatus'
import { useAgentList, type EnrichedAgent } from '../hooks/useAgentList'
import PulseDot from '../components/PulseDot'
import type { ProjectTeamSummary, TeamGetStatusResult } from '../lib/types'
import { rpcCall } from '../lib/rpc'
import { ChevronRight, Cpu, Coins, ListChecks, Users, Activity } from 'lucide-react'

const DETACHED = 'detached-control'

/* ── Status mapping ──────────────────────────────── */

function agentStatusDisplay(agent: EnrichedAgent): { label: string; color: string; pulse: boolean } {
  const s = agent.effectiveStatus.toLowerCase()
  if (s === 'running' || s === 'working') return { label: 'Running', color: 'var(--green)', pulse: true }
  if (s === 'thinking' || s === 'streaming') return { label: 'Thinking', color: 'var(--accent)', pulse: true }
  if (s === 'pending' || s === 'starting') return { label: 'Pending', color: 'var(--amber)', pulse: false }
  if (s === 'idle' || s === 'waiting') return { label: 'Idle', color: 'var(--text-3)', pulse: false }
  if (s === 'stopped' || s === 'done' || s === 'completed') return { label: 'Done', color: 'var(--blue)', pulse: false }
  if (s === 'failed' || s === 'error') return { label: 'Failed', color: 'var(--red)', pulse: false }
  if (s === 'blocked') return { label: 'Blocked', color: 'var(--red)', pulse: false }
  return { label: agent.effectiveStatus || 'Unknown', color: 'var(--text-3)', pulse: false }
}

function formatCost(usd: number): string {
  if (usd === 0) return '$0.00'
  if (usd < 0.01) return `$${usd.toFixed(4)}`
  return `$${usd.toFixed(2)}`
}

function formatTokens(n: number): string {
  if (n === 0) return '0'
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`
  return `${(n / 1_000_000).toFixed(2)}M`
}

/* ── Summary bar ─────────────────────────────────── */

function SummaryBar({ statuses }: { statuses: (TeamGetStatusResult | undefined)[] }) {
  const totals = statuses.reduce(
    (acc, s) => {
      if (!s) return acc
      acc.tokens += s.totalTokens
      acc.cost += s.totalCostUSD
      acc.pending += s.pending
      acc.active += s.active
      acc.done += s.done
      acc.agents += (s.runIds?.length ?? 0)
      return acc
    },
    { tokens: 0, cost: 0, pending: 0, active: 0, done: 0, agents: 0 },
  )

  const cards = [
    { icon: Users, label: 'Agents', value: String(totals.agents), color: 'var(--accent)' },
    { icon: ListChecks, label: 'Tasks', value: `${totals.active} / ${totals.pending + totals.active + totals.done}`, color: 'var(--green)' },
    { icon: Cpu, label: 'Tokens', value: formatTokens(totals.tokens), color: 'var(--blue)' },
    { icon: Coins, label: 'Cost', value: formatCost(totals.cost), color: 'var(--amber)' },
  ]

  return (
    <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: 28 }}>
      {cards.map(c => (
        <div key={c.label} style={{
          flex: '1 1 140px',
          padding: '14px 16px',
          background: 'var(--bg-panel)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--r-lg)',
          display: 'flex', alignItems: 'center', gap: 12,
        }}>
          <div style={{
            width: 32, height: 32, borderRadius: 8,
            background: 'var(--bg-elevated)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}>
            <c.icon size={15} style={{ color: c.color }} />
          </div>
          <div>
            <div style={{ fontSize: 10, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.06em', fontWeight: 600 }}>
              {c.label}
            </div>
            <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-1)', fontVariantNumeric: 'tabular-nums', letterSpacing: '-0.02em' }}>
              {c.value}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

/* ── Agent table row ─────────────────────────────── */

function AgentRow({ agent }: { agent: EnrichedAgent }) {
  const [expanded, setExpanded] = useState(false)
  const status = agentStatusDisplay(agent)

  return (
    <>
      <tr
        onClick={() => setExpanded(e => !e)}
        style={{ cursor: 'pointer', transition: 'background 0.12s' }}
        className="row-hover"
      >
        <td style={{ padding: '8px 12px', display: 'flex', alignItems: 'center', gap: 6 }}>
          <ChevronRight
            size={10}
            style={{
              color: 'var(--text-3)',
              transform: expanded ? 'rotate(90deg)' : 'none',
              transition: 'transform 0.15s',
            }}
          />
          <span style={{ fontWeight: 600, color: 'var(--text-1)', fontSize: 12 }}>
            {agent.role}
          </span>
        </td>
        <td style={{ padding: '8px 12px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            {status.pulse ? (
              <PulseDot status="active" size={6} />
            ) : (
              <span style={{
                width: 6, height: 6, borderRadius: '50%',
                background: status.color, flexShrink: 0,
              }} />
            )}
            <span style={{ fontSize: 12, color: status.color, fontWeight: 500 }}>
              {status.label}
            </span>
          </div>
        </td>
        <td style={{ padding: '8px 12px', fontSize: 12, color: 'var(--text-2)' }} className="mono">
          {agent.model || '—'}
        </td>
        <td style={{ padding: '8px 12px', fontSize: 12, color: 'var(--amber)', fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
          {formatCost(agent.runTotalCostUSD)}
        </td>
        <td style={{ padding: '8px 12px', fontSize: 12, color: 'var(--text-2)', fontVariantNumeric: 'tabular-nums' }}>
          {formatTokens(agent.runTotalTokens)}
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={5} style={{ padding: '0 12px 10px 30px' }}>
            <div className="animate-fade-in" style={{
              background: 'var(--bg-surface)',
              borderRadius: 'var(--r-md)',
              padding: '10px 14px',
              display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))',
              gap: '8px 20px', fontSize: 11,
            }}>
              <Detail label="Run ID" value={agent.runId.slice(0, 12)} mono />
              {agent.profile && <Detail label="Profile" value={agent.profile} />}
              {agent.model && <Detail label="Model" value={agent.model} mono />}
              <Detail label="Worker" value={agent.workerPresent ? 'Connected' : 'Disconnected'} />
              <Detail label="Cost" value={formatCost(agent.runTotalCostUSD)} />
              <Detail label="Tokens" value={formatTokens(agent.runTotalTokens)} />
              {agent.startedAt && <Detail label="Started" value={new Date(agent.startedAt).toLocaleTimeString()} />}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

function Detail({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <div style={{ fontSize: 9, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.04em', marginBottom: 2 }}>
        {label}
      </div>
      <div className={mono ? 'mono' : ''} style={{ color: 'var(--text-2)' }}>
        {value}
      </div>
    </div>
  )
}

/* ── Team section ────────────────────────────────── */

function TeamSection({ team }: { team: ProjectTeamSummary }) {
  const [open, setOpen] = useState(true)
  const statusQuery = useTeamStatus(team.teamId)
  const manifestQuery = useTeamManifest(team.teamId)
  const manifest = manifestQuery.data
  const sessionIds = manifest?.roles?.map((role) => role.sessionId).filter(Boolean) ?? (team.primarySessionId ? [team.primarySessionId] : [])
  const agentQuery = useAgentList(sessionIds)
  const data = statusQuery.data
  const agents = agentQuery.data ?? []

  const isActive = (data?.active ?? 0) > 0

  return (
    <div style={{
      background: 'var(--bg-panel)',
      border: '1px solid var(--border)',
      borderRadius: 'var(--r-lg)',
      overflow: 'hidden',
      marginBottom: 12,
    }}>
      {/* Team header */}
      <div
        onClick={() => setOpen(o => !o)}
        style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '12px 16px',
          cursor: 'pointer',
          transition: 'background 0.12s',
        }}
        className="row-hover"
      >
        <ChevronRight
          size={12}
          style={{
            color: 'var(--text-3)',
            transform: open ? 'rotate(90deg)' : 'none',
            transition: 'transform 0.15s',
          }}
        />
        <PulseDot status={isActive ? 'active' : 'idle'} size={7} />
        <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
          {manifest?.profileId ?? team.profileId ?? team.teamId.slice(0, 12)}
        </span>

        <div style={{ flex: 1 }} />

        {data && (
          <div style={{ display: 'flex', gap: 16, fontSize: 11, color: 'var(--text-3)', fontVariantNumeric: 'tabular-nums' }}>
            <span>{agents.length} agent{agents.length !== 1 ? 's' : ''}</span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <Activity size={10} />
              {data.active} active
            </span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--amber)', fontWeight: 600 }}>
              <Coins size={10} />
              {formatCost(data.totalCostUSD)}
            </span>
          </div>
        )}
      </div>

      {/* Agent table */}
      {open && (
        <div style={{ borderTop: '1px solid var(--border)' }}>
          {agents.length === 0 ? (
            <div style={{
              padding: '20px 16px',
              textAlign: 'center',
              fontSize: 12,
              color: 'var(--text-3)',
            }}>
              {agentQuery.isLoading ? (
                <span className="spinner spinner-sm" />
              ) : (
                'No agents registered'
              )}
            </div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)' }}>
                  {['Role', 'Status', 'Model', 'Cost', 'Tokens'].map(h => (
                    <th key={h} style={{
                      padding: '6px 12px',
                      fontSize: 9,
                      fontWeight: 600,
                      textTransform: 'uppercase',
                      letterSpacing: '0.06em',
                      color: 'var(--text-3)',
                      textAlign: 'left',
                    }}>
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {agents.map(agent => (
                  <AgentRow key={agent.runId} agent={agent} />
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  )
}

/* ── Dashboard page ──────────────────────────────── */

export default function Dashboard() {
  const teamsQuery = useProjectTeams()
  const teams = teamsQuery.data ?? []

  const statusQueries = useQueries({
    queries: teams.map((team) => ({
      queryKey: ['team.getStatus', team.teamId],
      queryFn: () => rpcCall<TeamGetStatusResult>('team.getStatus', { threadId: DETACHED, teamId: team.teamId }),
      enabled: !!team.teamId,
      refetchInterval: 1500,
      retry: false,
    })),
  })
  const statuses = statusQueries.map((query) => query.data)

  return (
    <div style={{ height: '100%', overflowY: 'auto', padding: '32px 40px' }}>
      {/* Header */}
      <div style={{ marginBottom: 24 }}>
        <h1 style={{
          margin: 0, fontSize: 24, fontWeight: 700,
          letterSpacing: '-0.04em', color: 'var(--text-1)',
        }}>
          Dashboard
        </h1>
        <div style={{ fontSize: 13, color: 'var(--text-3)', marginTop: 4 }}>
          Live operational overview
        </div>
      </div>

      {/* Summary cards */}
      <SummaryBar statuses={statuses} />

      {/* Team sections */}
      {teamsQuery.isLoading && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {[1, 2].map(i => (
            <div key={i} className="skeleton" style={{ height: 60, borderRadius: 'var(--r-lg)' }} />
          ))}
        </div>
      )}

      {!teamsQuery.isLoading && teams.length === 0 && (
        <div style={{
          padding: '40px 20px',
          textAlign: 'center',
          color: 'var(--text-3)',
          fontSize: 13,
        }}>
          No teams running. Start a team with <code className="mono" style={{ color: 'var(--accent)' }}>agen8 team start</code>
        </div>
      )}

      {teams.map(team => (
        <TeamSection key={team.teamId} team={team} />
      ))}
    </div>
  )
}
