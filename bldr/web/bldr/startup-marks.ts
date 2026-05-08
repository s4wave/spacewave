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
  workerType?: string | number
  [key: string]: unknown
}

let nextStartupMarkSequence = 1

function getPerformance(): Performance | undefined {
  return typeof globalThis.performance?.mark === 'function' ?
      globalThis.performance
    : undefined
}

export function markStartupBoundary(
  label: string,
  detail: StartupMarkDetail = {},
): string {
  const name = `${startupMarkPrefix}${label}`
  const markDetail: StartupMarkDetail = {
    ...detail,
    label,
    sequence: nextStartupMarkSequence++,
  }
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
}
