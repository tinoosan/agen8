import { useProjects } from '../hooks/useProjects'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { useStore } from '../lib/store'
import { useQueryClient } from '@tanstack/react-query'
import { FolderKanban, RefreshCw, Users } from 'lucide-react'
import type { ProjectRegistrySummary } from '../lib/types'

function projectName(project: ProjectRegistrySummary): string {
  if (project.projectId) return project.projectId
  const segments = project.projectRoot.split('/')
  return segments[segments.length - 1] || project.projectRoot
}

function formatDate(dateStr?: string): string {
  if (!dateStr) return ''
  try {
    return new Date(dateStr).toLocaleDateString(undefined, {
      month: 'short', day: 'numeric', year: 'numeric',
    })
  } catch {
    return dateStr
  }
}

function ProjectCard({ project }: { project: ProjectRegistrySummary }) {
  const setFocusedProjectRoot = useStore(s => s.setFocusedProjectRoot)
  const teamsQuery = useProjectTeams(project.projectRoot)
  const teams = teamsQuery.data ?? []
  const teamCount = teams.length

  return (
    <button
      onClick={() => setFocusedProjectRoot(project.projectRoot)}
      className="card-hover"
      style={{
        width: 320, padding: 20, textAlign: 'left',
        cursor: 'pointer', border: '1px solid var(--border)',
        borderRadius: 'var(--r-xl)',
        background: 'var(--bg-card)',
        transition: 'border-color 0.15s, box-shadow 0.15s',
        display: 'flex', flexDirection: 'column', gap: 12,
        fontFamily: 'inherit',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <div style={{
          width: 32, height: 32, borderRadius: 8,
          background: 'var(--accent-dim)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexShrink: 0,
        }}>
          <FolderKanban size={16} color="var(--accent)" />
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{
            fontWeight: 600, fontSize: 15,
            color: 'var(--text-1)',
            letterSpacing: '-0.02em',
            overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          }}>
            {projectName(project)}
          </div>
        </div>
        <span style={{
          display: 'inline-flex', alignItems: 'center', gap: 5,
          fontSize: 11, fontWeight: 500,
          color: project.enabled ? 'var(--green)' : 'var(--text-3)',
        }}>
          <span style={{
            width: 6, height: 6, borderRadius: '50%', flexShrink: 0,
            background: project.enabled ? 'var(--green)' : 'var(--text-3)',
          }} />
          {project.enabled ? 'Active' : 'Disabled'}
        </span>
      </div>

      <div style={{
        fontSize: 12, color: 'var(--text-3)',
        fontFamily: 'var(--font-mono, monospace)',
        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
      }}>
        {project.projectRoot}
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 12, color: 'var(--text-3)' }}>
        {teamCount > 0 && (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
            <Users size={11} />
            {teamCount} team{teamCount !== 1 ? 's' : ''}
          </span>
        )}
        {project.createdAt && (
          <span>
            Created {formatDate(project.createdAt)}
          </span>
        )}
      </div>
    </button>
  )
}

export default function Project() {
  const projectsQuery = useProjects()
  const projects = projectsQuery.data ?? []
  const isLoading = projectsQuery.isLoading
  const queryClient = useQueryClient()

  return (
    <div style={{ height: '100%', overflowY: 'auto', padding: '40px 44px' }}>
      {/* Header */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 12,
        marginBottom: 36,
      }}>
        <div>
          <h1 style={{
            margin: 0, fontSize: 26, fontWeight: 700,
            letterSpacing: '-0.04em', color: 'var(--text-1)',
            lineHeight: 1.1,
          }}>
            Projects
          </h1>
          {projects.length > 0 && (
            <div style={{ fontSize: 13, color: 'var(--text-3)', marginTop: 4 }}>
              {projects.length} project{projects.length !== 1 ? 's' : ''} registered
            </div>
          )}
        </div>
        <div style={{ flex: 1 }} />
        <button
          className="btn-outline"
          onClick={() => queryClient.invalidateQueries({ queryKey: ['project.listProjects'] })}
          title="Refresh projects list"
        >
          <RefreshCw size={13} />
          Refresh
        </button>
      </div>

      {/* Loading */}
      {isLoading && (
        <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
          {[1, 2, 3].map(i => (
            <div key={i} className="skeleton" style={{
              width: 320, height: 160, borderRadius: 'var(--r-xl)',
            }} />
          ))}
        </div>
      )}

      {/* Empty state */}
      {!isLoading && projects.length === 0 && (
        <div style={{
          display: 'flex', flexDirection: 'column',
          alignItems: 'center', justifyContent: 'center',
          height: 400, gap: 24, textAlign: 'center',
        }}>
          <div style={{
            width: 72, height: 72, borderRadius: 20,
            background: 'var(--accent-dim)',
            border: '1px solid rgba(139,123,248,0.2)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}>
            <FolderKanban size={32} color="var(--accent)" />
          </div>
          <div>
            <div style={{ fontWeight: 600, fontSize: 18, color: 'var(--text-1)', marginBottom: 8, letterSpacing: '-0.02em' }}>
              No projects registered
            </div>
            <div style={{ fontSize: 14, color: 'var(--text-3)', lineHeight: 1.6 }}>
              Initialize a project from your terminal
            </div>
            <code className="mono" style={{
              display: 'inline-block', marginTop: 14,
              fontSize: 13, color: 'var(--accent)',
              background: 'var(--bg-elevated)',
              border: '1px solid var(--border)',
              padding: '8px 18px', borderRadius: 'var(--r-md)',
            }}>
              agen8 project init
            </code>
          </div>
        </div>
      )}

      {/* Project cards grid */}
      {projects.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 18 }}>
          {projects.map(project => (
            <ProjectCard key={project.projectRoot} project={project} />
          ))}
        </div>
      )}
    </div>
  )
}
