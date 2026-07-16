// The mark-weighted boot progress model owns the mapping from boot status
// phases and startup marks to one monotonic 0..1 progress value plus a
// human-readable step label. Every boot surface (the inline boot script's
// static shell, the entrypoint boot-status writer, and the React loading
// screen projection) derives its bar and stage label from this ladder so mapped
// runtime marks advance the bar between coarse boot phase writes. Step
// positions are placeholder weights pending recalibration from measured
// staging traces; keep them monotonic in real emission order. Parallel or
// re-ordered marks are harmless: the projection takes the maximum ladder
// position ever observed, never regresses.
// bootProgressStallDelayMs is the quiet window before an otherwise determinate
// boot bar switches to its indeterminate shimmer without losing accumulated
// progress.
export const bootProgressStallDelayMs = 2000

// BootProgressStatus is the boot status shape shared by every writer. progress
// carries a raw 0..1 download fraction for download-driven phases; the model
// maps it into the phase's ladder window.
export interface BootProgressStatus {
  phase: string
  state: 'loading' | 'error'
  progress?: number
}

// BootProgressMark is the startup mark shape the model consumes; detail is
// used only for the WebView startup-relevance gate.
export interface BootProgressMark {
  label: string
  detail?: Record<string, unknown>
}

// BootProgressStep is the projected result: accumulated progress and the
// label of the furthest boot step observed.
export interface BootProgressStep {
  progress: number
  label: string
}

interface BootProgressLadderStep {
  progress: number
  label: string
}

// bootProgressLadder maps startup mark labels (and boot-status.<phase>
// pseudo-marks) to absolute ladder positions. Ordered by ladder position for
// readability; concurrent marks may arrive in either order.
const bootProgressLadder: Record<string, BootProgressLadderStep> = {
  // prepare: inline boot script loads the browser release manifest.
  'boot-status.loading': { progress: 0.02, label: 'Loading the app shell.' },
  'boot-status.manifest': {
    progress: 0.04,
    label: 'Loading the browser release.',
  },
  'boot-status.manifest-ready': {
    progress: 0.06,
    label: 'Browser release found.',
  },
  // connect: runtime wasm prime plus the app shell bundle download and eval.
  'boot-status.wasm': { progress: 0.07, label: 'Fetching the runtime.' },
  'boot-status.entrypoint': {
    progress: 0.08,
    label: 'Downloading the application.',
  },
  'boot-status.entrypoint-error': {
    progress: 0.08,
    label: 'Downloading the application.',
  },
  'shell.entrypoint-loaded': {
    progress: 0.28,
    label: 'Starting the app shell.',
  },
  'shell.container-resolved': {
    progress: 0.29,
    label: 'Starting the app shell.',
  },
  // runtime: worker, service worker, and runtime channel bring-up marks.
  'boot-status.runtime': {
    progress: 0.3,
    label: 'Connecting the Spacewave runtime.',
  },
  'runtime.wait-start': {
    progress: 0.31,
    label: 'Connecting the Spacewave runtime.',
  },
  'runtime.mode-selected': {
    progress: 0.32,
    label: 'Choosing the runtime mode.',
  },
  'service-worker.install-start': {
    progress: 0.34,
    label: 'Registering the service worker.',
  },
  'service-worker.register-start': {
    progress: 0.34,
    label: 'Registering the service worker.',
  },
  'runtime.client-open-start': {
    progress: 0.35,
    label: 'Opening the runtime channel.',
  },
  'service-worker.install-ready': {
    progress: 0.36,
    label: 'Service worker registered.',
  },
  'service-worker.register-ready': {
    progress: 0.36,
    label: 'Service worker registered.',
  },
  'service-worker.update-ready': {
    progress: 0.38,
    label: 'Service worker updated.',
  },
  'service-worker.activate-ready': {
    progress: 0.4,
    label: 'Service worker active.',
  },
  'service-worker.control-ready': {
    progress: 0.4,
    label: 'Service worker controlling the app.',
  },
  'service-worker.port-started': {
    progress: 0.42,
    label: 'Starting the service worker bridge.',
  },
  'service-worker.port-sent': {
    progress: 0.44,
    label: 'Connecting the service worker bridge.',
  },
  'runtime.worker-create-start': {
    progress: 0.46,
    label: 'Starting the runtime worker.',
  },
  'runtime.worker-created': {
    progress: 0.48,
    label: 'Runtime worker started.',
  },
  'runtime.opfs-bridge-ready': {
    progress: 0.52,
    label: 'Preparing browser storage.',
  },
  'runtime.client-open-sent': {
    progress: 0.56,
    label: 'Opening the runtime channel.',
  },
  'runtime.client-channel-opened': {
    progress: 0.6,
    label: 'Runtime channel opened.',
  },
  'runtime.client-channel-acked': {
    progress: 0.64,
    label: 'Runtime channel connected.',
  },
  'runtime.connected': { progress: 0.68, label: 'Runtime connected.' },
  'runtime.client-connect-ack': { progress: 0.7, label: 'Runtime connected.' },
  'runtime.event-connected': { progress: 0.72, label: 'Runtime connected.' },
  'runtime.wait-conn-ready': { progress: 0.74, label: 'Runtime connected.' },
  'runtime.wait-ready': { progress: 0.76, label: 'Runtime ready.' },
  'shell.deferred-boot-ready': { progress: 0.78, label: 'Runtime ready.' },
  'shell.immediate-boot-ready': { progress: 0.78, label: 'Runtime ready.' },
  'boot-status.ready': { progress: 0.78, label: 'Runtime ready.' },
  // manifest-copy marks carry compact scheduler accounting without changing
  // the startup-ready frontier.
  'manifest-copy.selected': {
    progress: 0.77,
    label: 'Selecting the application manifest.',
  },
  'manifest-copy.waiting-for-running': {
    progress: 0.78,
    label: 'Starting app plugins.',
  },
  'manifest-copy.copying': {
    progress: 0.79,
    label: 'Preparing the manifest cache.',
  },
  'manifest-copy.done': {
    progress: 0.79,
    label: 'Manifest cache ready.',
  },
  'manifest-copy.failed': {
    progress: 0.79,
    label: 'Manifest cache unavailable.',
  },
  // frame: the app frame opens and its plugins, styles, and UI come up.
  'boot-status.app': { progress: 0.8, label: 'Opening the application.' },
  'shell.boot-requested': { progress: 0.8, label: 'Opening the application.' },
  'quickstart.static-handoff-requested': {
    progress: 0.8,
    label: 'Opening the application.',
  },
  'webview.loading-surface-mounted': {
    progress: 0.82,
    label: 'Loading the application interface.',
  },
  'webview.registered': { progress: 0.84, label: 'Preparing the app frame.' },
  'worker.first-ready': { progress: 0.86, label: 'Starting app plugins.' },
  'plugin.frontend-ready': { progress: 0.88, label: 'App plugins ready.' },
  'plugin.capability-ready': { progress: 0.9, label: 'App plugins ready.' },
  'webview.stylesheet-ready': { progress: 0.93, label: 'Loading app styles.' },
  'webview.component-ready': {
    progress: 0.96,
    label: 'Preparing the interface.',
  },
  // done.
  'webview.revealed': { progress: 1, label: 'Spacewave is ready.' },
  'webview.loading-surface-revealed': {
    progress: 1,
    label: 'Spacewave is ready.',
  },
}

// bootDownloadProgressWindows maps a download-driven boot phase's raw 0..1
// status fraction into its absolute ladder window: the entrypoint window
// covers the app shell bundle download, the app window covers app bundle
// download progress reported after boot is requested.
const bootDownloadProgressWindows: Record<string, [number, number]> = {
  entrypoint: [0.08, 0.26],
  'entrypoint-error': [0.08, 0.26],
  app: [0.8, 0.98],
}

// projectBootDownloadProgress maps a phase-local download fraction into its
// absolute ladder window.
function projectBootDownloadProgress(
  phase: string,
  rawProgress: unknown,
): number | undefined {
  const window = bootDownloadProgressWindows[phase]
  if (
    !window ||
    typeof rawProgress !== 'number' ||
    !Number.isFinite(rawProgress)
  ) {
    return undefined
  }
  const fraction = Math.max(0, Math.min(1, rawProgress))
  return window[0] + fraction * (window[1] - window[0])
}

const initialBootProgressStep: BootProgressStep = {
  progress: 0.02,
  label: 'Loading the app shell.',
}

// bootProgressMarkIsStartupRelevant gates WebView marks: nested or later
// WebViews mark with startupRelevant=false (or an unflagged webViewId) and
// must not advance the boot ladder.
function bootProgressMarkIsStartupRelevant(mark: BootProgressMark): boolean {
  if (!mark.label.startsWith('webview.')) {
    return true
  }
  if (typeof mark.detail?.webViewId !== 'string') {
    return mark.detail?.startupRelevant !== false
  }
  return mark.detail.startupRelevant === true
}

// projectBootProgress projects the accumulated boot progress and current step
// label from the boot status plus every startup mark observed so far. The
// result is monotonic over a boot: it is the maximum ladder position of any
// observed mark, the current status phase, and the status download fraction
// mapped into its phase window.
export function projectBootProgress(
  status: BootProgressStatus,
  marks: readonly BootProgressMark[] = [],
): BootProgressStep {
  let best = initialBootProgressStep

  const statusStep = bootProgressLadder[`boot-status.${status.phase}`]
  if (statusStep && statusStep.progress > best.progress) {
    best = statusStep
  }

  const statusProgress = projectBootDownloadProgress(
    status.phase,
    status.progress,
  )
  if (statusProgress !== undefined && statusProgress > best.progress) {
    best = {
      progress: statusProgress,
      label: statusStep?.label ?? best.label,
    }
  }

  for (const mark of marks) {
    if (!bootProgressMarkIsStartupRelevant(mark)) continue

    const step = bootProgressLadder[mark.label]
    if (
      step &&
      (step.progress > best.progress ||
        (step.progress === best.progress &&
          mark.label.startsWith('manifest-copy.')))
    ) {
      best = step
    }

    const phase = mark.label.startsWith('boot-status.')
      ? mark.label.slice('boot-status.'.length)
      : ''
    const progress = projectBootDownloadProgress(phase, mark.detail?.progress)
    if (progress !== undefined && progress > best.progress) {
      best = { progress, label: step?.label ?? best.label }
    }
  }

  return best
}
