import { useState } from 'react'
import type { CSSProperties } from 'react'
import { ChevronDown, TrendingUp, TrendingDown, Minus } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useProgressHistory } from '../../hooks/useMissions'
import type { KeyResultView } from '../../lib/types'

const SF_TEXT = 'SF Pro Text, SF Pro Icons, Helvetica Neue, Helvetica, Arial, sans-serif'

const LABEL_STYLE: CSSProperties = {
  fontSize: '0.625rem',
  fontWeight: 500,
  letterSpacing: '0.08em',
  lineHeight: 1.33,
  color: 'var(--text-3)',
  margin: 0,
}

const VALUE_STYLE: CSSProperties = {
  fontFamily: SF_TEXT,
  fontSize: '0.9375rem',
  fontWeight: 500,
  letterSpacing: '-0.224px',
  lineHeight: 1.24,
  color: 'var(--text-1)',
  fontVariantNumeric: 'tabular-nums',
  margin: 0,
}

const SECONDARY_STYLE: CSSProperties = {
  fontFamily: SF_TEXT,
  fontSize: '0.8125rem',
  fontWeight: 400,
  letterSpacing: '-0.224px',
  lineHeight: 1.43,
  color: 'var(--text-2)',
  margin: 0,
}

interface KRDetailBodyProps {
  kr: KeyResultView
}

/**
 * Shared read-only KR detail content used by both:
 *   - `KeyResultRow` (inside MissionPanel's KR list, when a row is expanded)
 *   - `KRPanel` (the standalone slide-over for KR nodes on the strategy map)
 *
 * Renders only the inner sections (description, current/target with direction
 * arrow, baseline, last update, collapsible scrollable progress history).
 * The parent owns the wrapping container chrome (background, padding, radius).
 */
export default function KRDetailBody({ kr }: KRDetailBodyProps) {
  const [showHistory, setShowHistory] = useState(false)

  const updatedBy = kr.lastUpdatedBy?.trim() ?? ''

  // Lazy fetch: history only loads when the user opens the section.
  const historyQuery = useProgressHistory(showHistory ? kr.id : null)
  const historyEntries = historyQuery.data ?? []

  const formattedUpdatedAt = kr.updatedAt ? kr.updatedAt.slice(0, 10) : ''

  const DirectionIcon =
    kr.direction === 'increase' ? TrendingUp :
      kr.direction === 'decrease' ? TrendingDown :
        Minus

  return (
    <>
      {/* Description (markdown) */}
      {kr.description && (
        <div
          className="md-prose"
          style={{
            fontFamily: SF_TEXT,
            fontSize: '0.8125rem',
            fontWeight: 400,
            lineHeight: 1.43,
            letterSpacing: '-0.224px',
            color: 'var(--text-2)',
          }}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {kr.description}
          </ReactMarkdown>
        </div>
      )}

      {/* Current → direction arrow → Target */}
      <div
        className="grid"
        style={{
          gridTemplateColumns: '1fr auto 1fr',
          gridTemplateRows: 'auto auto',
          columnGap: '14px',
          rowGap: '4px',
          alignItems: 'baseline',
        }}
      >
        <p className="uppercase" style={LABEL_STYLE}>Current</p>
        <span />
        <p className="uppercase" style={LABEL_STYLE}>Target</p>

        <p style={VALUE_STYLE}>
          {kr.currentValue}{kr.unit ? ` ${kr.unit}` : ''}
        </p>
        <DirectionIcon
          size={14}
          style={{
            color: 'var(--text-3)',
            alignSelf: 'center',
            justifySelf: 'center',
          }}
          aria-label={`Direction: ${kr.direction}`}
        />
        <p style={VALUE_STYLE}>
          {kr.targetValue}{kr.unit ? ` ${kr.unit}` : ''}
        </p>
      </div>

      {/* Baseline (if present) */}
      {kr.baseline != null && (
        <div className="flex flex-col" style={{ gap: '4px' }}>
          <p className="uppercase" style={LABEL_STYLE}>Baseline</p>
          <p
            style={{
              ...SECONDARY_STYLE,
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            {kr.baseline}{kr.unit ? ` ${kr.unit}` : ''}
          </p>
        </div>
      )}

      {/* Last update — quick at-a-glance summary */}
      {(kr.lastUpdateNote || kr.lastUpdatedBy) && (
        <div className="flex flex-col" style={{ gap: '6px' }}>
          <p className="uppercase" style={LABEL_STYLE}>Last update</p>
          {kr.lastUpdateNote && (
            <p style={SECONDARY_STYLE}>{kr.lastUpdateNote}</p>
          )}
          {(kr.lastUpdatedBy || formattedUpdatedAt) && (
            <p
              style={{
                fontFamily: SF_TEXT,
                fontSize: '0.6875rem',
                fontWeight: 400,
                letterSpacing: '-0.12px',
                lineHeight: 1.33,
                color: 'var(--text-3)',
                margin: 0,
              }}
            >
              {updatedBy && `by ${updatedBy}`}
              {updatedBy && formattedUpdatedAt && ' · '}
              {formattedUpdatedAt}
            </p>
          )}
        </div>
      )}

      {/* Progress history — collapsible, scrollable, lazy-fetched */}
      <div className="flex flex-col" style={{ gap: '8px' }}>
        <button
          type="button"
          onClick={() => setShowHistory((prev) => !prev)}
          className="flex items-center cursor-pointer bg-transparent border-0 p-0 text-left"
          style={{ gap: '6px' }}
          aria-expanded={showHistory}
        >
          <ChevronDown
            size={10}
            className="shrink-0 transition-transform duration-150"
            style={{
              color: 'var(--text-3)',
              transform: showHistory ? 'rotate(0deg)' : 'rotate(-90deg)',
            }}
          />
          <p className="uppercase" style={LABEL_STYLE}>
            Progress history
          </p>
        </button>
        {showHistory && (
          <div
            className="flex flex-col overflow-y-auto"
            style={{
              gap: '12px',
              maxHeight: '200px',
              paddingRight: '4px',
            }}
          >
            {historyQuery.isLoading && (
              <p
                style={{
                  fontFamily: SF_TEXT,
                  fontSize: '0.6875rem',
                  letterSpacing: '-0.12px',
                  color: 'var(--text-3)',
                  margin: 0,
                }}
              >
                Loading…
              </p>
            )}
            {!historyQuery.isLoading && historyEntries.length === 0 && (
              <p
                style={{
                  fontFamily: SF_TEXT,
                  fontSize: '0.6875rem',
                  letterSpacing: '-0.12px',
                  color: 'var(--text-3)',
                  fontStyle: 'italic',
                  margin: 0,
                }}
              >
                No progress recorded yet
              </p>
            )}
            {historyEntries.map((entry) => {
              const entryUpdatedBy = entry.updatedBy?.trim() ?? ''
              return (
                <div key={entry.id} className="flex flex-col" style={{ gap: '2px' }}>
                <div className="flex justify-between items-baseline">
                  <span
                    style={{
                      fontFamily: SF_TEXT,
                      fontSize: '0.75rem',
                      fontWeight: 500,
                      letterSpacing: '-0.12px',
                      lineHeight: 1.33,
                      color: 'var(--text-1)',
                      fontVariantNumeric: 'tabular-nums',
                    }}
                  >
                    {entry.value}{kr.unit ? ` ${kr.unit}` : ''}
                  </span>
                  <span
                    style={{
                      fontFamily: SF_TEXT,
                      fontSize: '0.625rem',
                      fontWeight: 600,
                      letterSpacing: '-0.08px',
                      color: 'var(--text-2)',
                      fontVariantNumeric: 'tabular-nums',
                    }}
                  >
                    {entry.progress}%
                  </span>
                </div>
                {entry.note && (
                  <p
                    style={{
                      fontFamily: SF_TEXT,
                      fontSize: '0.6875rem',
                      fontWeight: 400,
                      lineHeight: 1.43,
                      letterSpacing: '-0.12px',
                      color: 'var(--text-2)',
                      margin: 0,
                    }}
                  >
                    {entry.note}
                  </p>
                )}
                <p
                  style={{
                    fontFamily: SF_TEXT,
                    fontSize: '0.625rem',
                    fontWeight: 400,
                    letterSpacing: '-0.08px',
                    lineHeight: 1.33,
                    color: 'var(--text-3)',
                    margin: 0,
                  }}
                >
                  {entryUpdatedBy && `by ${entryUpdatedBy}`}
                  {entryUpdatedBy && entry.createdAt && ' · '}
                  {entry.createdAt && entry.createdAt.slice(0, 10)}
                </p>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </>
  )
}
