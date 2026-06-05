import type { DecisionView } from '../../lib/types'

function Section({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <div className="text-[10px] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)] mb-1">{label}</div>
      {children}
    </>
  )
}

export default function DecisionDetails({ decision }: { decision: DecisionView }) {
  return (
    <div className="text-[11px] text-[var(--text-2)] space-y-3 leading-[1.55]">
      <div>
        <Section label="Rationale">
          <p className="m-0 whitespace-pre-wrap">{decision.rationale}</p>
        </Section>
      </div>
      {decision.alternativesRejected && (
        <div>
          <Section label="Alternatives">
            <p className="m-0 whitespace-pre-wrap">{decision.alternativesRejected}</p>
          </Section>
        </div>
      )}
      {(decision.invalidationConditions?.length ?? 0) > 0 && (
        <div>
          <Section label="Invalidation conditions">
            <ul className="m-0 list-disc space-y-1 pl-4">
              {decision.invalidationConditions!.map((condition) => (
                <li key={condition}>{condition}</li>
              ))}
            </ul>
          </Section>
        </div>
      )}
    </div>
  )
}
