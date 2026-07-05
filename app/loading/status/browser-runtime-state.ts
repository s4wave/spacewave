export type BrowserRuntimeStartupPhaseID =
  | 'prepare'
  | 'connect'
  | 'runtime'
  | 'frame'
  | 'done'

export type BrowserDocumentState =
  | 'unknown'
  | 'constructing'
  | 'visible'
  | 'hidden'
  | 'resume-ready'
  | 'closed'

export type BrowserRuntimeClientState =
  | 'unknown'
  | 'opening'
  | 'connected'
  | 'failed'
  | 'closed'

export type BrowserServiceWorkerState =
  | 'unknown'
  | 'installing'
  | 'registered'
  | 'controlled'
  | 'connected'
  | 'failed'
  | 'closed'

export type BrowserPluginGenerationState =
  | 'idle'
  | 'worker-requested'
  | 'worker-created'
  | 'startup-running'
  | 'frontend-ready'
  | 'capability-ready'
  | 'running'
  | 'normal-stop'
  | 'terminal-failure'

export type BrowserFrameRevealState =
  | 'idle'
  | 'registered'
  | 'stylesheet-ready'
  | 'component-ready'
  | 'revealed'

export interface BrowserRuntimeStartupMark {
  label: string
  sequence: number
  detail: Record<string, unknown>
}

export interface BrowserRuntimeBootStatus {
  phase: string
  detail: string
  state: 'loading' | 'error'
}

export interface BrowserRuntimeFailure {
  owner:
    | 'browser-boot'
    | 'document'
    | 'runtime-client'
    | 'service-worker'
    | 'plugin-generation'
  phase: BrowserRuntimeStartupPhaseID
  detail: string
}

export interface BrowserRuntimeState {
  startup: {
    phase: BrowserRuntimeStartupPhaseID
  }
  document: {
    state: BrowserDocumentState
  }
  runtimeClient: {
    state: BrowserRuntimeClientState
  }
  serviceWorker: {
    state: BrowserServiceWorkerState
  }
  pluginGeneration: {
    state: BrowserPluginGenerationState
  }
  frame: {
    state: BrowserFrameRevealState
  }
  terminalFailure?: BrowserRuntimeFailure
}

const documentStateRank: Record<BrowserDocumentState, number> = {
  unknown: 0,
  constructing: 1,
  visible: 2,
  hidden: 2,
  'resume-ready': 3,
  closed: 4,
}

const runtimeClientStateRank: Record<BrowserRuntimeClientState, number> = {
  unknown: 0,
  opening: 1,
  connected: 2,
  failed: 3,
  closed: 4,
}

const serviceWorkerStateRank: Record<BrowserServiceWorkerState, number> = {
  unknown: 0,
  installing: 1,
  registered: 2,
  controlled: 3,
  connected: 4,
  failed: 5,
  closed: 6,
}

const pluginGenerationStateRank: Record<BrowserPluginGenerationState, number> =
  {
    idle: 0,
    'worker-requested': 1,
    'worker-created': 2,
    'startup-running': 3,
    'frontend-ready': 4,
    'capability-ready': 5,
    running: 6,
    'normal-stop': 7,
    'terminal-failure': 8,
  }

const frameRevealStateRank: Record<BrowserFrameRevealState, number> = {
  idle: 0,
  registered: 1,
  'stylesheet-ready': 2,
  'component-ready': 3,
  revealed: 4,
}

const startupPhaseRank: Record<BrowserRuntimeStartupPhaseID, number> = {
  prepare: 0,
  connect: 1,
  runtime: 2,
  frame: 3,
  done: 4,
}

const bootPhaseToStartupPhase: Record<string, BrowserRuntimeStartupPhaseID> = {
  loading: 'prepare',
  manifest: 'prepare',
  'manifest-ready': 'prepare',
  'manifest-error': 'prepare',
  wasm: 'connect',
  entrypoint: 'connect',
  'entrypoint-error': 'connect',
  runtime: 'runtime',
  ready: 'runtime',
  'runtime-error': 'runtime',
  app: 'frame',
}

const bootFailurePhase: Record<string, BrowserRuntimeStartupPhaseID> = {
  'manifest-error': 'prepare',
  'entrypoint-error': 'connect',
  'runtime-error': 'runtime',
}

export function browserBootStatusStartupPhase(
  phase: string,
): BrowserRuntimeStartupPhaseID | undefined {
  return bootPhaseToStartupPhase[phase]
}

const startupMarkToPhase: Record<string, BrowserRuntimeStartupPhaseID> = {
  'boot-status.loading': 'prepare',
  'boot-status.manifest': 'prepare',
  'boot-status.manifest-ready': 'prepare',
  'boot-status.manifest-error': 'prepare',
  'boot-status.wasm': 'connect',
  'boot-status.entrypoint': 'connect',
  'boot-status.entrypoint-error': 'connect',
  'shell.entrypoint-loaded': 'connect',
  'shell.container-resolved': 'connect',
  'runtime.wait-start': 'runtime',
  'runtime.wait-ready': 'runtime',
  'runtime.wait-conn-start': 'runtime',
  'runtime.wait-conn-ready': 'runtime',
  'runtime.event-connected': 'runtime',
  'runtime.client-channel-acked': 'runtime',
  'runtime.client-connect-ack': 'runtime',
  'runtime.connected': 'runtime',
  'shell.deferred-boot-ready': 'runtime',
  'boot-status.runtime': 'runtime',
  'boot-status.ready': 'runtime',
  'boot-status.runtime-error': 'runtime',
  'boot-status.app': 'frame',
  'shell.boot-requested': 'frame',
  'quickstart.static-handoff-requested': 'frame',
  'webview.registered': 'frame',
  'webview.stylesheet-ready': 'frame',
  'webview.component-ready': 'frame',
  'webview.revealed': 'done',
  'webview.loading-surface-mounted': 'frame',
  'webview.loading-surface-revealed': 'done',
}

export function createBrowserRuntimeState(): BrowserRuntimeState {
  return {
    startup: { phase: 'prepare' },
    document: { state: 'unknown' },
    runtimeClient: { state: 'unknown' },
    serviceWorker: { state: 'unknown' },
    pluginGeneration: { state: 'idle' },
    frame: { state: 'idle' },
  }
}

export function buildBrowserRuntimeState(
  status: BrowserRuntimeBootStatus,
  marks: readonly BrowserRuntimeStartupMark[] = [],
): BrowserRuntimeState {
  const state = createBrowserRuntimeState()
  applyBootStatus(state, status)

  if (status.state === 'error') {
    state.terminalFailure = {
      owner:
        status.phase === 'runtime-error' ? 'runtime-client' : 'browser-boot',
      phase: bootFailurePhase[status.phase] ?? 'prepare',
      detail: status.detail,
    }
    return state
  }

  for (const mark of marks) {
    applyStartupMark(state, mark)
  }
  return state
}

export function projectBrowserRuntimeStartupPhase(
  state: BrowserRuntimeState,
): BrowserRuntimeStartupPhaseID {
  if (state.terminalFailure) {
    return state.terminalFailure.phase
  }
  let phase = state.startup.phase
  if (state.frame.state === 'revealed') {
    phase = maxStartupPhase(phase, 'done')
  }
  if (
    frameRevealStateRank[state.frame.state] >=
      frameRevealStateRank.registered ||
    pluginGenerationStateRank[state.pluginGeneration.state] >=
      pluginGenerationStateRank['frontend-ready']
  ) {
    phase = maxStartupPhase(phase, 'frame')
  }
  if (
    runtimeClientStateRank[state.runtimeClient.state] >=
      runtimeClientStateRank.connected ||
    serviceWorkerStateRank[state.serviceWorker.state] >=
      serviceWorkerStateRank.controlled ||
    pluginGenerationStateRank[state.pluginGeneration.state] >=
      pluginGenerationStateRank['worker-requested']
  ) {
    phase = maxStartupPhase(phase, 'runtime')
  }
  if (state.runtimeClient.state === 'opening') {
    phase = maxStartupPhase(phase, 'connect')
  }
  return phase
}

function applyBootStatus(
  state: BrowserRuntimeState,
  status: BrowserRuntimeBootStatus,
): void {
  const phase = bootPhaseToStartupPhase[status.phase]
  if (phase) {
    advanceStartupPhase(state, phase)
  }
  switch (status.phase) {
    case 'wasm':
    case 'entrypoint':
    case 'entrypoint-error':
      advanceRuntimeClient(state, 'opening')
      break
    case 'runtime':
      advanceRuntimeClient(state, 'opening')
      break
    case 'ready':
      advanceRuntimeClient(state, 'connected')
      break
    case 'runtime-error':
      advanceRuntimeClient(state, 'failed')
      break
    case 'app':
      advanceRuntimeClient(state, 'connected')
      advanceFrame(state, 'registered')
      break
  }
}

function applyStartupMark(
  state: BrowserRuntimeState,
  mark: BrowserRuntimeStartupMark,
): void {
  const phase = startupMarkToPhase[mark.label]
  if (phase && startupMarkCanAdvanceFrame(mark)) {
    advanceStartupPhase(state, phase)
  }
  switch (mark.label) {
    case 'web-document.construct-start':
      advanceDocument(state, 'constructing')
      break
    case 'web-document.resume-ready':
      advanceDocument(state, 'resume-ready')
      break
    case 'web-document.resume-not-ready':
      advanceDocument(state, 'hidden')
      break
    case 'runtime.wait-start':
    case 'runtime.wait-conn-start':
    case 'runtime.client-open-start':
    case 'runtime.client-channel-opened':
    case 'runtime.worker-create-start':
      advanceRuntimeClient(state, 'opening')
      break
    case 'runtime.wait-ready':
    case 'runtime.wait-conn-ready':
    case 'runtime.event-connected':
    case 'runtime.client-channel-acked':
    case 'runtime.client-connect-ack':
    case 'runtime.connected':
      advanceRuntimeClient(state, 'connected')
      break
    case 'service-worker.install-start':
    case 'service-worker.register-start':
      advanceServiceWorker(state, 'installing')
      break
    case 'service-worker.install-ready':
    case 'service-worker.register-ready':
    case 'service-worker.update-ready':
      advanceServiceWorker(state, 'registered')
      break
    case 'service-worker.activate-ready':
    case 'service-worker.control-ready':
      advanceServiceWorker(state, 'controlled')
      break
    case 'service-worker.port-started':
    case 'service-worker.port-sent':
    case 'service-worker.first-document-message':
      advanceServiceWorker(state, 'connected')
      break
    case 'worker.construct-start':
    case 'worker.first-create-start':
      advancePluginGeneration(state, 'worker-requested')
      break
    case 'worker.shared-created':
    case 'worker.shared-fallback-created':
    case 'worker.dedicated-created':
    case 'worker.create-ready':
      advancePluginGeneration(state, 'worker-created')
      break
    case 'worker.ready':
    case 'worker.first-ready':
    case 'plugin.frontend-ready':
      advancePluginGeneration(state, 'frontend-ready')
      break
    case 'plugin.capability-ready':
      advancePluginGeneration(state, 'capability-ready')
      break
    case 'plugin.running':
      advancePluginGeneration(state, 'running')
      break
    case 'plugin.normal-stop':
      advancePluginGeneration(state, 'normal-stop')
      break
    case 'plugin.terminal-failure':
      advancePluginGeneration(state, 'terminal-failure')
      state.terminalFailure = {
        owner: 'plugin-generation',
        phase: 'runtime',
        detail: formatFailureDetail(mark.detail.reason ?? mark.detail.error),
      }
      break
    case 'shell.boot-requested':
    case 'quickstart.static-handoff-requested':
    case 'webview.registered':
    case 'webview.loading-surface-mounted':
      if (startupMarkCanAdvanceFrame(mark)) {
        advanceFrame(state, 'registered')
      }
      break
    case 'webview.stylesheet-ready':
      if (startupMarkCanAdvanceFrame(mark)) {
        advanceFrame(state, 'stylesheet-ready')
      }
      break
    case 'webview.component-ready':
      if (startupMarkCanAdvanceFrame(mark)) {
        advanceFrame(state, 'component-ready')
      }
      break
    case 'webview.revealed':
    case 'webview.loading-surface-revealed':
      if (startupMarkCanAdvanceFrame(mark)) {
        advanceFrame(state, 'revealed')
      }
      break
  }
}

function startupMarkCanAdvanceFrame(mark: BrowserRuntimeStartupMark): boolean {
  if (!mark.label.startsWith('webview.')) {
    return true
  }
  if (typeof mark.detail.webViewId !== 'string') {
    return mark.detail.startupRelevant !== false
  }
  return mark.detail.startupRelevant === true
}

function formatFailureDetail(value: unknown): string {
  if (typeof value === 'string' && value.trim()) {
    return value
  }
  if (value instanceof Error && value.message.trim()) {
    return value.message
  }
  return 'plugin.terminal-failure'
}

function advanceDocument(
  state: BrowserRuntimeState,
  next: BrowserDocumentState,
): void {
  if (next === 'hidden' || next === 'closed') {
    state.document.state = next
    return
  }
  if (documentStateRank[next] >= documentStateRank[state.document.state]) {
    state.document.state = next
  }
}

function advanceStartupPhase(
  state: BrowserRuntimeState,
  next: BrowserRuntimeStartupPhaseID,
): void {
  state.startup.phase = maxStartupPhase(state.startup.phase, next)
}

function maxStartupPhase(
  current: BrowserRuntimeStartupPhaseID,
  next: BrowserRuntimeStartupPhaseID,
): BrowserRuntimeStartupPhaseID {
  return startupPhaseRank[next] > startupPhaseRank[current] ? next : current
}

function advanceRuntimeClient(
  state: BrowserRuntimeState,
  next: BrowserRuntimeClientState,
): void {
  if (next === 'failed' || next === 'closed') {
    state.runtimeClient.state = next
    return
  }
  if (
    runtimeClientStateRank[next] >=
    runtimeClientStateRank[state.runtimeClient.state]
  ) {
    state.runtimeClient.state = next
  }
}

function advanceServiceWorker(
  state: BrowserRuntimeState,
  next: BrowserServiceWorkerState,
): void {
  if (next === 'failed' || next === 'closed') {
    state.serviceWorker.state = next
    return
  }
  if (
    serviceWorkerStateRank[next] >=
    serviceWorkerStateRank[state.serviceWorker.state]
  ) {
    state.serviceWorker.state = next
  }
}

function advancePluginGeneration(
  state: BrowserRuntimeState,
  next: BrowserPluginGenerationState,
): void {
  if (next === 'normal-stop' || next === 'terminal-failure') {
    state.pluginGeneration.state = next
    return
  }
  if (
    pluginGenerationStateRank[next] >=
    pluginGenerationStateRank[state.pluginGeneration.state]
  ) {
    state.pluginGeneration.state = next
  }
}

function advanceFrame(
  state: BrowserRuntimeState,
  next: BrowserFrameRevealState,
): void {
  if (frameRevealStateRank[next] >= frameRevealStateRank[state.frame.state]) {
    state.frame.state = next
  }
}
