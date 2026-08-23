import { BootPhase } from './report.pb.js'

// This module is the single TypeScript source of the BootReport startup
// vocabulary. The validator and the collector both consume it so accepted
// labels, owners, operations, and phases cannot drift between them.

export const bootMarkLabels = new Set([
  'boot.started',
  'boot.aborted',
  'boot.sealed',
  'content-ready',
  'runtime.started',
  'runtime.failed',
  'runtime.last-observed',
  'worker.ready',
  'boot-status.loading',
  'boot-status.manifest',
  'boot-status.manifest-ready',
  'boot-status.wasm',
  'boot-status.entrypoint',
  'boot-status.entrypoint-error',
  'shell.entrypoint-loaded',
  'shell.container-resolved',
  'boot-status.runtime',
  'runtime.wait-start',
  'runtime.mode-selected',
  'service-worker.install-start',
  'service-worker.register-start',
  'runtime.client-open-start',
  'service-worker.install-ready',
  'service-worker.register-ready',
  'service-worker.update-ready',
  'service-worker.activate-ready',
  'service-worker.control-ready',
  'service-worker.port-started',
  'service-worker.port-sent',
  'runtime.worker-create-start',
  'runtime.worker-created',
  'runtime.opfs-bridge-ready',
  'runtime.client-open-sent',
  'runtime.client-channel-opened',
  'runtime.client-channel-acked',
  'runtime.connected',
  'runtime.client-connect-ack',
  'runtime.event-connected',
  'runtime.wait-conn-ready',
  'runtime.wait-ready',
  'shell.deferred-boot-ready',
  'shell.immediate-boot-ready',
  'boot-status.ready',
  'manifest-copy.selected',
  'manifest-copy.waiting-for-running',
  'manifest-copy.copying',
  'manifest-copy.done',
  'manifest-copy.failed',
  'boot-status.app',
  'shell.boot-requested',
  'quickstart.static-handoff-requested',
  'webview.loading-surface-mounted',
  'webview.registered',
  'worker.first-ready',
  'plugin.frontend-ready',
  'plugin.capability-ready',
  'webview.stylesheet-ready',
  'webview.component-ready',
  'webview.revealed',
  'webview.loading-surface-revealed',
])

export const bootOwners = new Set([
  'bldr',
  'browser',
  'cdn',
  'entrypoint',
  'fixture',
  'manifest-materializer',
  'network',
  'opfs',
  'plugin-host',
  'runtime',
  'scheduler',
  'service-worker',
  'shell',
  'webview',
  'worker',
])

export const bootOperations = new Set([
  'access-manifest',
  'checksum',
  'copy-manifest',
  'decode',
  'dist-main',
  'download-entrypoint',
  'execute-plugin',
  'load-dist-main',
  'open-release-world',
  'opfs-read',
  'opfs-write',
  'publish-local-ref',
  'read-release-blocks',
  'release-block-request',
  'release-pack-range',
  'release-world-provider',
  'runtime',
  'select-startup-manifests',
  'sync-manifest',
  'wait-for-provider',
  'wait-for-running',
  'worker-request',
])

export const bootDetailKeys = new Set([
  'attempt-count',
  'blocks-copied',
  'blocks-deduped',
  'blocks-existing',
  'blocks-seen',
  'blocks-written',
  'cache-mode',
  'candidate-count',
  'copied-bytes',
  'demand-read-bytes',
  'demand-read-count',
  'duration-ratio',
  'logical-source-bytes',
  'phase',
  'revision-count',
  'skipped-subtrees',
  'written-bytes',
])

export const bootDetailStrings = new Set([
  'cold',
  'warm',
  'hot',
  'selected',
  'waiting-for-running',
  'copying',
  'local-ref-publication',
  'sync',
  'done',
  'failed',
])

export const bootCounterNames = new Set([
  'blocks-copied',
  'blocks-deduped',
  'blocks-existing',
  'blocks-seen',
  'blocks-written',
  'copied-bytes',
  'demand-read-bytes',
  'demand-read-count',
  'download-bytes',
  'logical-source-bytes',
  'revision-count',
  'selected-candidates',
  'skipped-subtrees',
  'written-bytes',
])

export const bootTerminalErrorCodes = new Set([
  'boot-interrupted',
  'entrypoint-load-failed',
  'manifest-copy-failed',
  'runtime-init-failed',
  'startup-aborted',
  'startup-timeout',
])

export const bootEntrypoints = new Set([
  'canvas',
  'computers',
  'drive',
  'forge',
  'space',
])

export const bootProjects = new Set(['spacewave'])

export const bootBrowserEngines = new Set(['chromium', 'webkit'])

export const bootOSFamilies = new Set(['android', 'darwin', 'ios', 'linux', 'windows'])

export const bootArchitectures = new Set(['amd64', 'arm64', 'wasm'])

// bootMarkPhases partitions every accepted startup mark into the report's
// phase timeline. The mapping mirrors the app's startup-mark phase ladder in
// app/loading/status/browser-runtime-state.ts.
export const bootMarkPhases: ReadonlyMap<string, BootPhase> = new Map([
  ['boot.started', BootPhase.PREPARE],
  ['boot.aborted', BootPhase.DONE],
  ['boot.sealed', BootPhase.DONE],
  ['content-ready', BootPhase.DONE],
  ['runtime.started', BootPhase.RUNTIME],
  ['runtime.failed', BootPhase.RUNTIME],
  ['runtime.last-observed', BootPhase.RUNTIME],
  ['worker.ready', BootPhase.RUNTIME],
  ['boot-status.loading', BootPhase.PREPARE],
  ['boot-status.manifest', BootPhase.PREPARE],
  ['boot-status.manifest-ready', BootPhase.PREPARE],
  ['boot-status.wasm', BootPhase.CONNECT],
  ['boot-status.entrypoint', BootPhase.CONNECT],
  ['boot-status.entrypoint-error', BootPhase.CONNECT],
  ['shell.entrypoint-loaded', BootPhase.CONNECT],
  ['shell.container-resolved', BootPhase.CONNECT],
  ['boot-status.runtime', BootPhase.RUNTIME],
  ['runtime.wait-start', BootPhase.RUNTIME],
  ['runtime.mode-selected', BootPhase.RUNTIME],
  ['service-worker.install-start', BootPhase.CONNECT],
  ['service-worker.register-start', BootPhase.CONNECT],
  ['runtime.client-open-start', BootPhase.CONNECT],
  ['service-worker.install-ready', BootPhase.CONNECT],
  ['service-worker.register-ready', BootPhase.CONNECT],
  ['service-worker.update-ready', BootPhase.CONNECT],
  ['service-worker.activate-ready', BootPhase.CONNECT],
  ['service-worker.control-ready', BootPhase.CONNECT],
  ['service-worker.port-started', BootPhase.CONNECT],
  ['service-worker.port-sent', BootPhase.CONNECT],
  ['runtime.worker-create-start', BootPhase.RUNTIME],
  ['runtime.worker-created', BootPhase.RUNTIME],
  ['runtime.opfs-bridge-ready', BootPhase.RUNTIME],
  ['runtime.client-open-sent', BootPhase.RUNTIME],
  ['runtime.client-channel-opened', BootPhase.RUNTIME],
  ['runtime.client-channel-acked', BootPhase.RUNTIME],
  ['runtime.connected', BootPhase.RUNTIME],
  ['runtime.client-connect-ack', BootPhase.RUNTIME],
  ['runtime.event-connected', BootPhase.RUNTIME],
  ['runtime.wait-conn-ready', BootPhase.RUNTIME],
  ['runtime.wait-ready', BootPhase.RUNTIME],
  ['shell.deferred-boot-ready', BootPhase.RUNTIME],
  ['shell.immediate-boot-ready', BootPhase.RUNTIME],
  ['boot-status.ready', BootPhase.RUNTIME],
  ['manifest-copy.selected', BootPhase.RUNTIME],
  ['manifest-copy.waiting-for-running', BootPhase.RUNTIME],
  ['manifest-copy.copying', BootPhase.RUNTIME],
  ['manifest-copy.done', BootPhase.RUNTIME],
  ['manifest-copy.failed', BootPhase.RUNTIME],
  ['boot-status.app', BootPhase.FRAME],
  ['shell.boot-requested', BootPhase.FRAME],
  ['quickstart.static-handoff-requested', BootPhase.FRAME],
  ['webview.loading-surface-mounted', BootPhase.FRAME],
  ['webview.registered', BootPhase.FRAME],
  ['worker.first-ready', BootPhase.FRAME],
  ['plugin.frontend-ready', BootPhase.FRAME],
  ['plugin.capability-ready', BootPhase.FRAME],
  ['webview.stylesheet-ready', BootPhase.FRAME],
  ['webview.component-ready', BootPhase.FRAME],
  ['webview.revealed', BootPhase.DONE],
  ['webview.loading-surface-revealed', BootPhase.DONE],
])

// bootMarkErrorCodes maps failure marks to the stable terminal error codes the
// validator accepts. Marks absent from the map do not fail a boot.
// boot-status.entrypoint-error and manifest-copy.failed are timeline labels
// only today: their producers recover or retry, so sealing on them would make
// transient failures terminal. A mark joins this map only when an explicitly
// non-retryable producer owns it.
export const bootMarkErrorCodes: ReadonlyMap<string, string> = new Map([
  ['runtime.failed', 'runtime-init-failed'],
])
