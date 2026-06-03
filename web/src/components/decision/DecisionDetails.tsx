import type { DecisionView } from '../../lib/types'

function answerText(answer: { selectedOption?: string; selectedOptions?: string[]; freeFormText?: string } | undefined): string {
  if (!answer) return 'No answer recorded'
  const multi = (answer.selectedOptions ?? []).map((v) => v.trim()).filter((v) => v !== '')
  if (multi.length > 0) return multi.join(', ')
  const freeForm = answer.freeFormText?.trim() ?? ''
  if (freeForm) return freeForm
  const selected = answer.selectedOption?.trim() ?? ''
  if (selected) return selected
  return 'No answer recorded'
}

function isAskUserDecision(decision: DecisionView): boolean {
  return (decision.kind ?? '').trim().toLowerCase() === 'ask_user'
}

function Section({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <div className="text-[10px] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)] mb-1">{label}</div>
      {children}
    </>
  )
}

export default function DecisionDetails({ decision }: { decision: DecisionView }) {
  if (isAskUserDecision(decision)) {
    return (
      <div className="text-[11px] text-[var(--text-2)] space-y-3 leading-[1.55]">
        {decision.context && (
          <div>
            <Section label="Context">
              <p className="m-0 whitespace-pre-wrap">{decision.context}</p>
            </Section>
          </div>
        )}
        {(decision.questions?.length ?? 0) > 0 && (
          <div>
            <Section label="Questions">
              <div className="space-y-3">
                {decision.questions!.map((question, index) => {
                  const answer = decision.answers?.find((entry) => entry.questionId === question.id)
                  return (
                    <div key={question.id || `${index}`} className="rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] p-3">
                      <p className="m-0 text-[12px] font-medium text-[var(--text-1)] leading-[1.5]">
                        {question.text}
                        {question.blocking && (
                          <span className="ml-2 align-middle rounded-full bg-[color-mix(in_srgb,var(--amber)_16%,transparent)] px-2 py-0.5 text-[9px] font-semibold uppercase tracking-[0.04em] text-[var(--amber)]">
                            Blocking
                          </span>
                        )}
                      </p>
                      {question.recommendation && (
                        <p className="m-0 mt-1 text-[10px] text-[var(--accent)]">
                          Recommendation: {question.recommendation}
                        </p>
                      )}
                      <p className="m-0 mt-2 text-[11px] text-[var(--text-2)] leading-[1.55]">
                        Answer: {answerText(answer)}
                      </p>
                    </div>
                  )
                })}
              </div>
            </Section>
          </div>
        )}
        {decision.cancelled && (
          <div>
            <Section label="Status">
              <p className="m-0 text-[var(--red)]">Cancelled by operator</p>
            </Section>
          </div>
        )}
      </div>
    )
  }

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
