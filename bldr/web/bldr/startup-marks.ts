export const startupMarkPrefix = 'spacewave.startup.'
export const startupMarkEvent = 'spacewave-startup-mark'

export interface StartupMarkDetail {
  documentId?: string
  from?: string
  mode?: string
  phase?: string
  runtimeId?: string
  sequence?: number
  shared?: boolean
  source?: string
  workerId?: string
  [key: string]: unknown
}

declare global {
  var __swStartupMarks:
    | Array<{
        name: string
        label: string
        sequence: number
        detail: Record<string, unknown>
      }>
    | undefined
  var __swStartupMarkSequence: number | undefined
}

let nextStartupMarkSequence = 1

function getPerformance(): Performance | undefined {
  return typeof globalThis.performance?.mark === 'function'
    ? globalThis.performance
    : undefined
}

function nextSequence(): number {
  const next = globalThis.__swStartupMarkSequence ?? nextStartupMarkSequence
  globalThis.__swStartupMarkSequence = next + 1
  nextStartupMarkSequence = next + 1
  return next
}

export function markStartupBoundary(
  label: string,
  detail: StartupMarkDetail = {},
): string {
  const name = `${startupMarkPrefix}${label}`
  const sequence = nextSequence()
  const markDetail: StartupMarkDetail = {
    ...detail,
    label,
    sequence,
  }
  globalThis.__swStartupMarks = [
    ...(globalThis.__swStartupMarks ?? []),
    { name, label, sequence, detail: markDetail },
  ]
  const perf = getPerformance()
  if (perf) {
    try {
      perf.mark(name, { detail: markDetail })
    } catch {
      perf.mark(name)
    }
  }
  if (
    typeof globalThis.dispatchEvent === 'function' &&
    typeof globalThis.CustomEvent === 'function'
  ) {
    globalThis.dispatchEvent(
      new CustomEvent(startupMarkEvent, {
        detail: {
          name,
          detail: markDetail,
        },
      }),
    )
  }
  return name
}

export function resetStartupMarksForTest(): void {
  nextStartupMarkSequence = 1
  globalThis.__swStartupMarkSequence = undefined
  globalThis.__swStartupMarks = undefined
}
