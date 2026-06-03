import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { HumanInputAnswer, HumanInputQuestion, QuestionsPayload } from '../../hooks/useHumanInput'

interface Props {
  payload: QuestionsPayload
  busy: boolean
  onSubmit: (answers: HumanInputAnswer[]) => void
  onCancel: () => void
  onCancelRequest?: () => void
}

function buildInitialAnswers(questions: HumanInputQuestion[]): HumanInputAnswer[] {
  return questions.map((question) => ({ questionId: question.id }))
}

export default function QuestionsPanel({ payload, busy, onSubmit, onCancel, onCancelRequest }: Props) {
  const questions = payload.questions ?? []
  const [index, setIndex] = useState(0)
  const [answers, setAnswers] = useState<HumanInputAnswer[]>(() => buildInitialAnswers(questions))
  const [validationError, setValidationError] = useState<string | null>(null)

  const questionCount = questions.length
  const current = questions[index]
  const currentAnswer = useMemo(() => answers[index] ?? { questionId: current?.id ?? '' }, [answers, current?.id, index])

  if (!current || questionCount === 0) {
    return (
      <div className="rounded-2xl border border-[var(--border)] bg-[var(--bg-panel)] px-5 py-4 text-[13px] text-[var(--text-2)] shadow-[var(--shadow-lg)]">
        No questions available.
      </div>
    )
  }

  function updateCurrent(next: HumanInputAnswer) {
    setAnswers((existing) => {
      const out = existing.slice()
      out[index] = next
      return out
    })
  }

  function isAnswered(question: HumanInputQuestion, answer: HumanInputAnswer): boolean {
    const selected = (answer.selectedOption ?? '').trim()
    const selectedMany = (answer.selectedOptions ?? []).filter((opt) => opt.trim() !== '')
    const freeForm = (answer.freeFormText ?? '').trim()
    if (question.type === 'free_form') return freeForm !== ''
    if (question.type === 'multi_select') {
      // multi_select requires at least one checked option, OR a
      // free-form fallback if the question allows it.
      if (selectedMany.length > 0) return true
      if (question.allowFreeForm && freeForm !== '') return true
      return false
    }
    if (selected !== '') return true
    if (freeForm !== '') return true  // free-form always accepted on multiple_choice
    return false
  }

  function handleNext() {
    if (!isAnswered(current, currentAnswer)) {
      setValidationError(current.type === 'multi_select'
        ? 'Pick at least one option before continuing.'
        : 'Select or enter an answer before continuing.')
      return
    }
    setValidationError(null)
    if (index >= questionCount - 1) {
      onSubmit(answers.map((answer, answerIndex) => ({
        questionId: questions[answerIndex]?.id ?? answer.questionId,
        ...(answer.selectedOption ? { selectedOption: answer.selectedOption } : {}),
        ...((answer.selectedOptions ?? []).length > 0
          ? { selectedOptions: (answer.selectedOptions ?? []).filter((opt) => opt.trim() !== '') }
          : {}),
        ...(answer.freeFormText ? { freeFormText: answer.freeFormText } : {}),
      })))
      return
    }
    setIndex((v) => v + 1)
  }

  const selectedOption = (currentAnswer.selectedOption ?? '').trim()
  const selectedOptions = currentAnswer.selectedOptions ?? []
  const freeFormText = currentAnswer.freeFormText ?? ''
  const primaryLabel = index >= questionCount - 1 ? 'Submit' : 'Next'
  const isBlocking = current.blocking === true

  return (
    <div className={cn(
      'rounded-2xl border bg-[var(--bg-panel)] shadow-[var(--shadow-lg)] overflow-hidden',
      isBlocking ? 'border-[color-mix(in_srgb,var(--amber)_45%,var(--border))]' : 'border-[var(--border)]',
    )}>

      {/* ── Header ── */}
      <div className="px-5 pt-5 pb-4">
        {/* Step + title breadcrumb */}
        <div className="flex items-center gap-1.5 mb-3">
          <span className="text-[11px] tabular-nums text-[var(--text-3)]" style={{ letterSpacing: '-0.04px' }}>
            {index + 1} / {questionCount}
          </span>
          {/* Accessible full label for tests/screen-readers */}
          <span className="sr-only">Question {index + 1} of {questionCount}</span>
          {payload.title && (
            <>
              <span className="text-[var(--border)] select-none">·</span>
              <span className="text-[11px] text-[var(--text-3)] truncate" style={{ letterSpacing: '-0.04px' }}>
                {payload.title}
              </span>
            </>
          )}
          {isBlocking && (
            <span className="ml-auto rounded-full bg-[color-mix(in_srgb,var(--amber)_16%,transparent)] px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.04em] text-[var(--amber)]">
              Blocking
            </span>
          )}
        </div>

        {/* Question */}
        <p className="m-0 text-[15px] font-semibold leading-snug text-[var(--text-1)]" style={{ letterSpacing: '-0.02em' }}>
          {current.text}
        </p>

        {/* Context */}
        {payload.context && (
          <p className="m-0 mt-2 text-[12px] leading-relaxed text-[var(--text-3)]" style={{ letterSpacing: '-0.06px' }}>
            {payload.context}
          </p>
        )}
        {isBlocking && (
          <p className="m-0 mt-2 rounded-[var(--r-sm)] bg-[color-mix(in_srgb,var(--amber)_9%,transparent)] px-3 py-2 text-[11px] font-medium leading-relaxed text-[color-mix(in_srgb,var(--amber)_82%,var(--text-2))]">
            Answer required before dependent work can continue.
          </p>
        )}
      </div>

      {/* ── Options ── */}
      {current.type === 'multiple_choice' && (
        <div className="px-3 pb-3 flex flex-col gap-0.5">
          {(current.options ?? []).map((option) => {
            const active = selectedOption === option
            const recommended = current.recommendation?.trim() === option.trim()
            return (
              <button
                key={option}
                type="button"
                disabled={busy}
                onClick={() => {
                  setValidationError(null)
                  updateCurrent({
                    questionId: current.id,
                    selectedOption: option,
                    ...(freeFormText.trim() ? { freeFormText } : {}),
                  })
                }}
                className={cn(
                  'w-full flex items-center gap-3 rounded-[var(--r-md)] px-3 py-2.5 text-left transition-colors duration-100 focus:outline-none',
                  active
                    ? 'bg-[color-mix(in_srgb,var(--accent)_10%,var(--bg-surface))]'
                    : 'hover:bg-[var(--bg-hover)]',
                  busy && 'cursor-default opacity-60',
                )}
              >
                {/* Radio dot */}
                <span className={cn(
                  'flex shrink-0 items-center justify-center w-4 h-4 rounded-full border-2 transition-colors duration-100',
                  active ? 'border-[var(--accent)] bg-[var(--accent)]' : 'border-[var(--border)]',
                )}>
                  {active && <span className="w-1.5 h-1.5 rounded-full bg-white" />}
                </span>

                <span
                  className={cn(
                    'flex-1 text-[13px] font-medium leading-snug',
                    active ? 'text-[var(--text-1)]' : 'text-[var(--text-2)]',
                  )}
                  style={{ letterSpacing: '-0.08px' }}
                >
                  {option}
                </span>

                {recommended && (
                  <span
                    className="shrink-0 text-[10px] font-semibold text-[var(--accent)] bg-[color-mix(in_srgb,var(--accent)_12%,transparent)] rounded-full px-2 py-0.5"
                    style={{ letterSpacing: '-0.04px' }}
                  >
                    Recommended
                  </span>
                )}
              </button>
            )
          })}
        </div>
      )}

      {/* ── Multi-select checkbox list ── */}
      {current.type === 'multi_select' && (
        <div className="px-3 pb-3 flex flex-col gap-0.5">
          {(current.options ?? []).map((option) => {
            const active = selectedOptions.includes(option)
            const recommended = current.recommendation?.trim() === option.trim()
            return (
              <button
                key={option}
                type="button"
                disabled={busy}
                onClick={() => {
                  setValidationError(null)
                  const next = active
                    ? selectedOptions.filter((opt) => opt !== option)
                    : [...selectedOptions, option]
                  updateCurrent({
                    questionId: current.id,
                    ...(next.length > 0 ? { selectedOptions: next } : {}),
                    ...(freeFormText.trim() ? { freeFormText } : {}),
                  })
                }}
                className={cn(
                  'w-full flex items-center gap-3 rounded-[var(--r-md)] px-3 py-2.5 text-left transition-colors duration-100 focus:outline-none',
                  active
                    ? 'bg-[color-mix(in_srgb,var(--accent)_10%,var(--bg-surface))]'
                    : 'hover:bg-[var(--bg-hover)]',
                  busy && 'cursor-default opacity-60',
                )}
              >
                {/* Checkbox square */}
                <span className={cn(
                  'flex shrink-0 items-center justify-center w-4 h-4 rounded-[4px] border-2 transition-colors duration-100',
                  active ? 'border-[var(--accent)] bg-[var(--accent)]' : 'border-[var(--border)]',
                )}>
                  {active && (
                    <svg viewBox="0 0 12 12" className="w-3 h-3 text-white" aria-hidden="true">
                      <path d="M2.5 6.2 L5 8.5 L9.5 4" stroke="currentColor" strokeWidth="1.6" fill="none" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  )}
                </span>

                <span
                  className={cn(
                    'flex-1 text-[13px] font-medium leading-snug',
                    active ? 'text-[var(--text-1)]' : 'text-[var(--text-2)]',
                  )}
                  style={{ letterSpacing: '-0.08px' }}
                >
                  {option}
                </span>

                {recommended && (
                  <span
                    className="shrink-0 text-[10px] font-semibold text-[var(--accent)] bg-[color-mix(in_srgb,var(--accent)_12%,transparent)] rounded-full px-2 py-0.5"
                    style={{ letterSpacing: '-0.04px' }}
                  >
                    Recommended
                  </span>
                )}
              </button>
            )
          })}
        </div>
      )}

      {/* ── Free-form textarea ── */}
      {(current.type === 'free_form' || current.type === 'multiple_choice' || (current.type === 'multi_select' && current.allowFreeForm) || current.allowFreeForm) && (
        <div className={cn('px-4', current.type === 'free_form' ? 'pb-3' : 'pt-0 pb-3')}>
          {current.type !== 'free_form' && (
            <p className="m-0 mb-1.5 text-[11px] text-[var(--text-3)]" style={{ letterSpacing: '-0.04px' }}>
              Or enter a different answer
            </p>
          )}
          <Textarea
            value={freeFormText}
            disabled={busy}
            rows={current.type === 'free_form' ? 4 : 2}
            onChange={(e) => {
              setValidationError(null)
              updateCurrent({
                questionId: current.id,
                ...(selectedOption ? { selectedOption } : {}),
                ...(selectedOptions.length > 0 ? { selectedOptions } : {}),
                freeFormText: e.target.value,
              })
            }}
            placeholder={current.type === 'free_form' ? 'Type your answer…' : 'Type your own answer…'}
            className="rounded-[var(--r-md)] border-[var(--border)] bg-[var(--bg-surface)] text-[13px] px-3 py-2.5 resize-none"
          />
        </div>
      )}

      {/* ── Validation error ── */}
      {validationError && (
        <p className="m-0 mx-5 mb-3 text-[11px] text-[var(--red)]" style={{ letterSpacing: '-0.04px' }}>
          {validationError}
        </p>
      )}

      {/* ── Footer ── */}
      <div className="flex items-center justify-between gap-3 px-4 py-3 border-t border-[var(--border)]/50">
        <div className="flex items-center gap-1">
          <Button type="button" variant="ghost" size="sm" onClick={onCancel} disabled={busy}
            className="text-[var(--text-3)] hover:text-[var(--text-2)]">
            Dismiss
          </Button>
          {onCancelRequest && (
            <Button type="button" variant="ghost" size="sm" onClick={onCancelRequest} disabled={busy}
              className="text-[var(--red)]/60 hover:text-[var(--red)]">
              Cancel request
            </Button>
          )}
          {index > 0 && (
            <Button type="button" variant="ghost" size="sm" disabled={busy}
              onClick={() => { setValidationError(null); setIndex((v) => v - 1) }}
              className="text-[var(--text-3)] hover:text-[var(--text-2)]">
              Back
            </Button>
          )}
        </div>
        <Button type="button" size="sm" onClick={handleNext} disabled={busy}>
          {primaryLabel}
        </Button>
      </div>
    </div>
  )
}
