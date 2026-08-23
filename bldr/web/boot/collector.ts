import {
  BootAccounting,
  BootBuild,
  BootValidationViolationKind,
  BootBuildType,
  BootEnvironment,
  BootEnvironmentClass,
  BootCacheState,
  BootPrivacy,
  BootRecoveryDecision,
  BootReport,
  BootReportState,
  BootRuntimeKind,
  BootServiceWorkerState,
  BootWorkerMode,
} from './report.pb.js'
import type {
  BootValidationViolation,
} from './report.pb.js'
import { validateBootReport } from './validator.js'
import { bootMarkErrorCodes, bootMarkLabels, bootMarkPhases } from './vocabulary.js'

// The collector composes the browser's existing startup-mark stream into one
// durable BootReport using the frozen validator contract. It installs at
// entrypoint-bundle evaluation, replays the inline shell's earlier marks, then
// journals live marks through the BootReportStore until the entrypoint's
// declared usable mark seals the report READY or a failure mark seals it
// FAILED.

export const bootReportGlobalKey = '__swBootReport'

// BootReportStoreLike is the persistence surface the collector needs. The
// durable implementation comes from store.ts; tests may pass a double.
export interface BootReportStoreLike {
  recoverOnStartup(): Promise<void>
  put(report: BootReport): Promise<unknown>
  // HoldBootLock resolves true only after the Web Locks grant, so attachStore
  // awaits it before journaling a recording boot.
  holdBootLock?(reportId: string): Promise<boolean>
  sealReport?(
    report: BootReport,
    state: BootReportState,
    terminalErrorCode?: string,
  ): Promise<unknown>
  applyRetention?(): Promise<unknown>
}

export interface BootReportCollectorOptions {
  // EntrypointId is the public entrypoint contract name (canvas, computers,
  // drive, forge, or space).
  entrypointId: string
  // UsableMark is the single mark label that completes this entrypoint's
  // startup. The first occurrence seals the report READY.
  usableMark: string
  // Commit is the public source commit when available.
  commit?: string
  // ReleaseGeneration is the immutable public release generation when known.
  releaseGeneration?: string
}

export interface BootReportCollectorDeps {
  // Store persists recording journals and sealed reports. When omitted, the
  // collector stays memory-only and opens the real store lazily in a browser
  // that provides IndexedDB.
  store?: BootReportStoreLike
}

// Optional persistence extensions live on the durable store only; memory
// doubles in tests omit them.

interface StartupMarkRecord {
  name: string
  label: string
  sequence: number
  detail: Record<string, unknown>
}

type CollectorGlobals = typeof globalThis & {
  __swStartupMarks?: StartupMarkRecord[]
  __swStartupMarkOverflows?: number
  __swBootReport?: BootReport
  __swGenerationId?: string
  performance: Performance
}

// This module is one instance per document per bundle evaluation. A hot
// module replacement would create a second module instance, but the collector
// remains a single document-global owner through liveCollector and the
// __swBootReport seam; no second global owner exists or is added.


// readReleaseGeneration returns the validator-compatible release-generation
// identifier from the inline shell's loaded browser-release manifest. Asset
// path characters outside the report vocabulary are folded to hyphens so the
// value stays a stable coarse identity.
function readReleaseGeneration(globals: CollectorGlobals): string {
  const raw = globals.__swGenerationId ?? ''
  const folded = raw
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return /^[a-z]/.test(folded) ? folded.slice(0, 128) : 'unknown'
}

// randomBootReportId returns a device-local report id that satisfies the
// validator's `boot-report-` identity contract without any account meaning.
function randomBootReportId(): string {
  const random =
    globalThis.crypto?.randomUUID?.().replaceAll('-', '') ??
    Math.random().toString(16).slice(2).padEnd(12, '0')
  return `boot-report-${random}`
}

// readStartupMonotonicMicros returns the monotonic offset of one startup mark
// in microseconds relative to the collector origin.
function readStartupMonotonicMicros(
  perf: Performance,
  originMs: number,
  record: StartupMarkRecord,
): bigint {
  let elapsedMs: number | undefined
  try {
    const entries = perf.getEntriesByName(record.name)
    elapsedMs = entries.at(-1)?.startTime
  } catch {
    elapsedMs = undefined
  }
  if (elapsedMs === undefined) return BigInt(Math.round(perf.now() * 1000))
  const micros =
    BigInt(Math.round(elapsedMs * 1000)) - BigInt(Math.round(originMs * 1000))
  return micros > 0n ? micros : 0n
}

// detectBrowserEngine reports the lowercase engine name the validator accepts.
function detectBrowserEngine(): string {
  const nav = globalThis.navigator
  if (!nav) return 'chromium'
  const webkit = /applewebkit/i.test(nav.userAgent)
  const blink = /chrome|chromium|crios/i.test(nav.userAgent)
  return webkit && !blink ? 'webkit' : 'chromium'
}

export class BootReportCollector {
  private readonly options: BootReportCollectorOptions
  private readonly globals: CollectorGlobals
  private deps: BootReportCollectorDeps
  private readonly perf: Performance
  private readonly originMs: number
  private readonly report: BootReport
  private readonly inlineOverflowMarks: number
  private sealed = false
  // leaseHeld gates every durable RECORDING write: without this report's
  // held boot lease the collector stays memory-only until the terminal seal.
  private leaseHeld = false

  constructor(
    options: BootReportCollectorOptions,
    globals: CollectorGlobals,
    deps: BootReportCollectorDeps,
  ) {
    this.options = options
    this.globals = globals
    this.deps = deps
    this.perf = globals.performance
    this.originMs = this.perf.now()
    this.inlineOverflowMarks = Number(globals.__swStartupMarkOverflows ?? 0)
    if (this.inlineOverflowMarks > 0) {
      // Overflow is never silent: the exact count is logged here and becomes
      // a persisted REPORT_CONTRACT validation failure at seal time.
      console.warn(
        `bootreport: inline startup-mark buffer overflowed; ` +
          `${this.inlineOverflowMarks} mark(s) dropped before collection`,
      )
    }
    const nowUnixMicros = BigInt(Date.now()) * 1000n
    this.report = BootReport.create({
      schemaVersion: 1,
      reportId: randomBootReportId(),
      startedUnixMicros: nowUnixMicros,
      monotonicOriginUnixMicros:
        nowUnixMicros - BigInt(Math.round(this.originMs * 1000)),
      entrypointId: options.entrypointId,
      usableMark: options.usableMark,
      state: BootReportState.RECORDING,
      build: BootBuild.create({
        commit: options.commit,
        releaseGeneration:
          options.releaseGeneration ?? readReleaseGeneration(globals),
        projectId: 'spacewave',
        buildType: this.detectBuildType(),
        runtimeKind: this.detectRuntimeKind(),
        // The browser runtime defaults to a dedicated worker today.
        workerMode: BootWorkerMode.DEDICATED,
      }),
      environment: BootEnvironment.create({
        class: this.detectEnvironmentClass(),
        browserEngine: detectBrowserEngine(),
        osFamily: this.detectOsFamily(),
        architecture: 'wasm',
        serviceWorkerState: BootServiceWorkerState.UNAVAILABLE,
        cacheState: BootCacheState.COLD,
        recoveryDecision: BootRecoveryDecision.NONE,
      }),
      // Accounting and privacy start empty; counter samples arrive with the
      // later attribution phases, and local-only capture keeps the v1 export
      // policy with nothing shared.
      accounting: BootAccounting.create({}),
      privacy: BootPrivacy.create({ exportPolicyVersion: 1 }),
    })
    this.replayStartupMarks()
  }

  // start subscribes to the live startup-mark stream, recovers stale boots,
  // takes this report's recording lease before any durable write, and
  // publishes the recording report on the document-global diagnostic seam.
  start(): void {
    globalThis.addEventListener(
      'spacewave-startup-mark',
      this.onStartupMark as EventListener,
    )
    void this.takeLeaseAndJournal().catch((cause: unknown) => {
      console.warn('bootreport: startup recovery failed', cause)
    })
    this.publish()
  }

  // takeLeaseAndJournal orders the durable lifecycle: recover stale boots,
  // hold this report's lease, then allow durable RECORDING writes. A denied
  // or unavailable lease leaves no durable RECORDING row; collection keeps
  // running in memory until the terminal seal.
  private async takeLeaseAndJournal(): Promise<void> {
    const store = this.deps.store
    if (!store) return
    await store.recoverOnStartup()
    if (this.deps.store !== store || this.sealed) return
    if (!store.holdBootLock) return
    this.leaseHeld = await store.holdBootLock(this.report.reportId ?? '')
    if (this.leaseHeld) {
      this.journal()
    } else {
      console.warn('bootreport: recording memory-only without a boot lease')
    }
  }

  stop(): void {
    globalThis.removeEventListener(
      'spacewave-startup-mark',
      this.onStartupMark as EventListener,
    )
  }

  // IsSealed reports whether this collector already wrote its terminal state.
  isSealed(): boolean {
    return this.sealed
  }

  // AttachStore binds persistence after construction so the entrypoint can
  // install synchronously while the durable store opens asynchronously. The
  // collector owns exactly one report: on attachment it recovers stale boots,
  // then either takes its unique Web Lock before the first durable RECORDING
  // write (memory-only when the lease is denied) or, when this document
  // already sealed, persists the complete sealed snapshot and applies
  // retention. A terminal snapshot needs no lease: the report key is a
  // device-local random id minted once per boot, so the write cannot
  // overwrite or race a live recorder's row.
  attachStore(store: BootReportStoreLike): void {
    if (this.deps.store === store) return
    this.deps = { ...this.deps, store }
    void store
      .recoverOnStartup()
      .then(async () => {
        if (this.deps.store !== store) return
        if (this.sealed) {
          await store.put({ ...this.report })
          await store.applyRetention?.()
          return
        }
        if (!store.holdBootLock) return
        this.leaseHeld = await store.holdBootLock(
          this.report.reportId ?? '',
        )
        if (!this.leaseHeld) {
          console.warn(
            'bootreport: recording memory-only without a boot lease',
          )
          return
        }
        await store.put({ ...this.report })
        this.journal()
      })
      .catch((cause: unknown) => {
        console.warn('bootreport: durable attach failed', cause)
      })
  }

  // journal persists one snapshot of the recording report without blocking
  // the boot: producers enqueue and never await storage on the critical path.
  // Durable RECORDING writes require the held lease; without it the report
  // stays memory-only until seal persists its terminal record.
  private journal(): void {
    if (!this.sealed && this.leaseHeld) {
      void this.deps.store
        ?.put({ ...this.report })
        .catch((cause: unknown) => {
          console.warn('bootreport: journal write failed', cause)
        })
    }
  }

  // seal completes the report, validates it against the frozen contract,
  // writes the complete sealed record atomically, applies retention after the
  // seal, and publishes the sealed result. Later marks are ignored after
  // sealing and repeated seals are no-ops.
  seal(state: BootReportState, terminalErrorCode?: string): BootReport {
    if (this.sealed) return this.report
    this.sealed = true
    if (liveCollector === this) liveCollector = undefined
    this.stop()
    const marks = this.report.marks ?? []
    const terminal = marks.at(-1)
    this.report.state = state
    this.report.terminalMark = terminal?.label
    this.report.terminalErrorCode = terminalErrorCode
    this.report.terminalMonotonicMicros = terminal?.monotonicMicros ?? 0n
    this.report.validation = validateBootReport(this.report)
    if (this.inlineOverflowMarks > 0) {
      // Dropped inline marks are a report contract failure, persisted with
      // the sealed record so overflow can never pass silently.
      const violation: BootValidationViolation = {
        kind: BootValidationViolationKind.REPORT_CONTRACT,
        markSequence: terminal?.sequence ?? 0n,
      }
      this.report.validation.pass = false
      this.report.validation.violations = [
        ...(this.report.validation.violations ?? []),
        violation,
      ]
    }
    const sealedCopy: BootReport = { ...this.report }
    void this.deps.store
      ?.sealReport?.(sealedCopy, state, terminalErrorCode)
      .then((sealed) => {
        this.publish(sealed ?? sealedCopy)
      })
      .catch((cause: unknown) => {
        console.warn('bootreport: seal write failed', cause)
      })
    void this.deps.store?.applyRetention?.().catch((cause: unknown) => {
      console.warn('bootreport: retention failed', cause)
    })
    this.publish()
    return this.report
  }

  private readonly onStartupMark = (event: Event): void => {
    if (this.sealed) return
    const detail = (
      event as CustomEvent<{ detail: Record<string, unknown> }>
    ).detail?.detail
    if (!detail || typeof detail.label !== 'string') return
    // Live marks share the document-global sequence counter with the inline
    // shell's earlier marks, so an incoming sequence at or below the last
    // accepted one is bumped to keep the timeline strictly increasing.
    const lastSequence = Number((this.report.marks ?? []).at(-1)?.sequence ?? 0n)
    const sequence = Math.max(Number(detail.sequence ?? 0), lastSequence + 1)
    const monotonicMicros = BigInt(Math.round(this.perf.now() * 1000)) -
      BigInt(Math.round(this.originMs * 1000))
    this.acceptMark(detail.label, sequence, monotonicMicros < 0n ? 0n : monotonicMicros, detail)
  }

  // replayStartupMarks converts the inline shell's already-emitted marks into
  // accepted report marks before the live subscription begins, then clears
  // the inline buffer so a re-install cannot replay it twice.
  private replayStartupMarks(): void {
    const records = this.globals.__swStartupMarks ?? []
    for (const record of records) {
      const micros = readStartupMonotonicMicros(
        this.perf,
        this.originMs,
        record,
      )
      this.acceptMark(record.label, record.sequence, micros, record.detail)
      if (this.sealed) break
    }
    delete this.globals.__swStartupMarks
  }

  private acceptMark(
    label: string,
    sequence: number,
    monotonicMicros: bigint,
    detail: Record<string, unknown>,
  ): void {
    if (this.sealed) return
    if (!bootMarkLabels.has(label)) return
    const marks = this.report.marks ?? []
    const previous = marks.at(-1)
    if (previous && BigInt(sequence) <= (previous.sequence ?? 0n)) return
    const mark = {
      sequence: BigInt(sequence),
      label,
      sourceOwner: this.markOwner(label, detail),
      monotonicMicros:
        previous && monotonicMicros < (previous.monotonicMicros ?? 0n)
          ? previous.monotonicMicros
          : monotonicMicros,
      phase: bootMarkPhases.get(label),
    }
    marks.push(mark)
    this.report.marks = marks

    if (label === this.options.usableMark) {
      this.seal(BootReportState.READY)
      return
    }
    const errorCode = bootMarkErrorCodes.get(label)
    if (errorCode !== undefined) this.seal(BootReportState.FAILED, errorCode)
    this.journal()
  }

  // markOwner maps the emitting producer to the validator's stable owner set.
  private markOwner(label: string, detail: Record<string, unknown>): string {
    const source = typeof detail.source === 'string' ? detail.source : ''
    switch (source) {
      case 'browser':
      case 'runtime':
      case 'scheduler':
      case 'service-worker':
      case 'webview':
      case 'worker':
        return source
      default:
        break
    }
    if (label.startsWith('service-worker.')) return 'service-worker'
    if (label.startsWith('manifest-copy.')) return 'scheduler'
    if (label.startsWith('webview.')) return 'webview'
    if (label.startsWith('runtime.')) return 'runtime'
    if (label.startsWith('worker.')) return 'worker'
    if (label.startsWith('shell.') || label.startsWith('boot-status.')) {
      return 'browser'
    }
    return 'browser'
  }

  private detectBuildType(): BootBuildType {
    // The bundled app always ships minified release bytes today.
    return BootBuildType.RELEASE
  }

  private detectRuntimeKind(): BootRuntimeKind {
    // The GoScript runtime worker sets its own mode mark before connect.
    return BootRuntimeKind.GOSCRIPT
  }

  private detectEnvironmentClass(): BootEnvironmentClass {
    const host = globalThis.location?.hostname ?? ''
    if (host === 'localhost' || host === '127.0.0.1') {
      return BootEnvironmentClass.LOCAL
    }
    if (host.endsWith('.spacewave.app')) return BootEnvironmentClass.PRODUCTION
    return BootEnvironmentClass.DEV
  }

  private detectOsFamily(): string {
    // Android reports the Linux platform string, so classify it from the
    // user agent before any platform fallback.
    const nav = globalThis.navigator
    const platform = nav?.platform ?? ''
    if (/android/i.test(nav?.userAgent ?? '')) return 'android'
    if (/mac/i.test(platform)) return 'darwin'
    if (/win/i.test(platform)) return 'windows'
    if (/iphone|ipad|ios/i.test(platform)) return 'ios'
    if (/linux/i.test(platform)) return 'linux'
    return 'linux'
  }

  private publish(sealed?: BootReport): void {
    ;(globalThis as { [bootReportGlobalKey]?: BootReport })[bootReportGlobalKey] =
      sealed ?? this.report
  }
}

// readSealedBootReport returns this document's sealed report, or undefined
// while startup is still recording or before the collector installed. This
// document-global handle is a diagnostic test seam only and never the durable
// owner.
export function readSealedBootReport(): BootReport | undefined {
  const report = (globalThis as CollectorGlobals)[bootReportGlobalKey]
  if (!report || report.state === BootReportState.RECORDING) return undefined
  return report
}

// liveCollector holds this document's recording collector so repeated
// initBootReportCollector calls reuse one subscriber instead of installing a
// second listener on the same startup-mark stream.
let liveCollector: BootReportCollector | undefined

// initBootReportCollector installs the production collector for this document.
// The collector replays the inline shell's marks, records live marks, journals
// them through the BootReportStore, and seals the report at the declared
// usable boundary. Only one collector runs per document: a later call returns
// the live collector while startup is still recording, and keeps the sealed
// report once one exists. Without an injected store the collector opens the
// real IndexedDB-backed store lazily when the document provides IndexedDB and
// otherwise stays memory-only with a warning.
export function initBootReportCollector(
  options: BootReportCollectorOptions,
  deps: BootReportCollectorDeps = {},
): BootReportCollector | undefined {
  const globals = globalThis as CollectorGlobals
  const existing = globals[bootReportGlobalKey]
  if (existing && existing.state !== BootReportState.RECORDING) return undefined
  if (liveCollector) return liveCollector
  const collector = new BootReportCollector(options, globals, deps)
  liveCollector = collector
  collector.start()
  if (!deps.store && globals.indexedDB) {
    import('./store.js')
      .then((storeModule) =>
        storeModule.openBootReportStore({
          locks: globals.navigator?.locks,
        }),
      )
      .then((store) => {
        // A fast seal can land before the store finishes opening; attach
        // anyway so attachStore persists the complete sealed snapshot. Skip
        // only when a different live collector took over this document.
        if (liveCollector && liveCollector !== collector) return
        collector.attachStore(store)
      })
      .catch((cause: unknown) => {
        console.warn('bootreport: durable store unavailable', cause)
      })
  }
  return collector
}
