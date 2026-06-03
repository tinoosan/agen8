interface ResolveEdgeStrokeColorInput {
  isDirect: boolean
  isAmbient: boolean
  focusNodeId: string | null
  source: string
  target: string
  sourceColor?: string
  targetColor?: string
}

/**
 * Chooses edge stroke colour using focus direction so incoming links are
 * highlighted with the selected node's cluster colour, not only the source.
 */
export function resolveEdgeStrokeColor(input: ResolveEdgeStrokeColorInput): string {
  const {
    isDirect,
    isAmbient,
    focusNodeId,
    source,
    target,
    sourceColor,
    targetColor,
  } = input

  if (!isDirect && !isAmbient) return 'var(--text-3)'

  if (isDirect && focusNodeId) {
    if (focusNodeId === source && sourceColor) return sourceColor
    if (focusNodeId === target && targetColor) return targetColor
  }

  return sourceColor ?? targetColor ?? 'var(--accent)'
}
