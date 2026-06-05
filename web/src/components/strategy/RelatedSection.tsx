import { useMemo } from 'react'
import { ChevronRight } from 'lucide-react'
import { NodeLink } from './NodeLink'

export interface RelatedItem {
  nodeId: string
  type: string   // group label: "Task", "Key Result", "Mission", etc.
  title: string
  badge?: string  // optional small badge (e.g., confidence %)
  badgeColor?: string
}

interface Props {
  items: RelatedItem[]
  /** When true, group items by type with section headers + counts.
   *  When false (default), show individual type eyebrows per item. */
  grouped?: boolean
}

// Order types appear in when grouped
const TYPE_ORDER = ['Mission', 'Key Result', 'Task', 'Decision']

function RelatedRow({ item, showType }: { item: RelatedItem; showType: boolean }) {
  return (
    <NodeLink nodeId={item.nodeId}>
      <div
        className="flex items-center gap-2 hover:bg-[var(--bg-elevated)] transition-colors duration-150 rounded-md"
        style={{ padding: '6px 8px' }}
      >
        <div className="flex-1 min-w-0">
          {showType && (
            <div
              className="uppercase"
              style={{
                fontSize: '0.5625rem',
                fontWeight: 600,
                letterSpacing: '0.06em',
                color: 'var(--text-3)',
                marginBottom: 1,
              }}
            >
              {item.type}
            </div>
          )}
          <div
            className="truncate"
            style={{
              fontSize: '0.75rem',
              fontWeight: 400,
              letterSpacing: '-0.12px',
              lineHeight: 1.33,
              color: 'var(--text-2)',
            }}
          >
            {item.title}
          </div>
        </div>
        {item.badge && (
          <span
            className="shrink-0 text-[0.625rem] font-semibold tabular-nums"
            style={{ color: item.badgeColor ?? 'var(--text-3)' }}
          >
            {item.badge}
          </span>
        )}
        <ChevronRight size={10} className="shrink-0 text-muted-foreground" />
      </div>
    </NodeLink>
  )
}

/**
 * Standardised "Related" section for strategy map detail panels.
 * Shows clickable cross-references. In grouped mode, items are
 * organized under type headers with counts.
 */
export function RelatedSection({ items, grouped = false }: Props) {
  const groups = useMemo(() => {
    if (!grouped) return null
    const byType = new Map<string, RelatedItem[]>()
    for (const item of items) {
      const list = byType.get(item.type) ?? []
      list.push(item)
      byType.set(item.type, list)
    }
    return TYPE_ORDER
      .filter((type) => byType.has(type))
      .map((type) => ({ type, items: byType.get(type)! }))
  }, [items, grouped])

  if (items.length === 0) return null

  return (
    <div className="flex flex-col" style={{ gap: '6px' }}>
      <p
        className="uppercase"
        style={{
          fontSize: '0.625rem',
          fontWeight: 500,
          letterSpacing: '0.08em',
          lineHeight: 1.33,
          color: 'var(--text-3)',
          margin: 0,
        }}
      >
        Related
      </p>

      {grouped && groups ? (
        groups.map((group) => (
          <div key={group.type} className="flex flex-col" style={{ gap: '2px' }}>
            <div
              className="uppercase flex items-center gap-1.5"
              style={{
                fontSize: '0.5625rem',
                fontWeight: 600,
                letterSpacing: '0.06em',
                color: 'var(--text-3)',
                padding: '4px 8px 0',
              }}
            >
              {group.type}s
              <span style={{ opacity: 0.5 }}>{group.items.length}</span>
            </div>
            {group.items.map((item) => (
              <RelatedRow key={`${item.type}-${item.nodeId}`} item={item} showType={false} />
            ))}
          </div>
        ))
      ) : (
        items.map((item) => (
          <RelatedRow key={`${item.type}-${item.nodeId}`} item={item} showType={true} />
        ))
      )}
    </div>
  )
}
