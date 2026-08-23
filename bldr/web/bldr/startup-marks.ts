// startupMarkPrefix prefixes every startup performance mark name.
export const startupMarkPrefix = 'spacewave.startup.'

// startupMarkEvent is the DOM event dispatched on each startup mark.
export const startupMarkEvent = 'spacewave-startup-mark'

// StartupMarkDetail carries optional context for one startup mark.
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
  var __swStartupMarkOverflows: number | undefined
}

let nextStartupMarkSequence = 1

function getPerformance(): Performance | undefined {
  return typeof globalThis.performance?.mark === 'function'
    ? globalThis.performance
    : undefined
}

// startupMarkBufferLimit bounds the document-global mark store. The inline
// shell seeds the first mark and shares this bound; appends past the limit
// increment __swStartupMarkOverflows instead of pushing, and the BootReport
// collector turns that count into a persisted validation failure.
export const startupMarkBufferLimit = 4096

function nextSequence(): number {
  if (globalThis.__swStartupMarkSequence !== undefined) {
    const seeded = globalThis.__swStartupMarkSequence
    globalThis.__swStartupMarkSequence = seeded + 1
    nextStartupMarkSequence = seeded + 1
    return seeded
  }
  // The inline shell's first mark already used sequence 1, so continue
  // after the last buffered mark instead of restarting at 1.
  const lastBuffered = globalThis.__swStartupMarks?.at(-1)?.sequence
  const seeded =
    lastBuffered !== undefined ? lastBuffered + 1 : nextStartupMarkSequence
  globalThis.__swStartupMarkSequence = seeded
  nextStartupMarkSequence = seeded + 1
  return seeded
}

// markStartupBoundary records a performance mark for a startup boundary and
// returns the generated mark name.
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
  const perf = getPerformance()
  if (perf) {
    try {
      perf.mark(name, { detail: markDetail })
    } catch {
      perf.mark(name)
    }
  }
  const bufferedMarks =
    globalThis.__swStartupMarks ??
    (globalThis.__swStartupMarks = [])
  if (bufferedMarks.length >= startupMarkBufferLimit) {
    globalThis.__swStartupMarkOverflows =
      (globalThis.__swStartupMarkOverflows ?? 0) + 1
  } else {
    bufferedMarks.push({
      name,
      label,
      sequence,
      detail: markDetail,
    })
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

// readStartupMarks returns every startup mark recorded so far in this
// document, across all mark producers sharing the global store.
export function readStartupMarks(): Array<{
  name: string
  label: string
  sequence: number
  detail: Record<string, unknown>
}> {
  return globalThis.__swStartupMarks ?? []
}

export function resetStartupMarksForTest(): void {
  nextStartupMarkSequence = 1
  globalThis.__swStartupMarkSequence = undefined
  globalThis.__swStartupMarks = undefined
}
