import {
  binaryWriteMessage,
  type BinaryWriteOptions,
} from '@aptre/protobuf-es-lite/binary'

import {
  BootAttachmentKind,
  BootBuildType,
  BootCacheState,
  BootCounterUnit,
  BootEnvironmentClass,
  BootPhase,
  BootReport,
  BootRecoveryDecision,
  BootReportState,
  BootRuntimeKind,
  BootServiceWorkerState,
  BootShareDestination,
  BootSpanResult,
  BootValidation,
  BootValidationViolationKind,
  BootWorkerMode,
  BootWorkClass,
  type BootGap,
  type BootSpan,
  type BootValidation as BootValidationMessage,
  type BootValidationViolation,
  type BootValue,
} from './report.pb.js'

const noGapDivisor = 20n
const completePartsPerMillion = 1_000_000n
const maxBootDurationMicros = 24n * 60n * 60n * 1_000_000n
const maxBootStringBytes = 128
const maxBootRecordBytes = 4 << 20
const maxBootDetailFields = 32
const maxBootMarks = 4096
const maxBootSpans = 4096
const maxBootSamples = 4096
const maxBootAttachments = 64
const maxBootPrivacyFields = 256
const maxBootRecords = 8192
const maxBootSpanDepth = 64
const maxUint64 = (1n << 64n) - 1n
const maxInt64 = (1n << 63n) - 1n
const minInt64 = -(1n << 63n)

const bootMarkLabels = new Set([
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

const bootOwners = new Set([
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

const bootOperations = new Set([
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

const bootDetailKeys = new Set([
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

const bootDetailStrings = new Set([
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

const bootCounterNames = new Set([
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

const bootTerminalErrorCodes = new Set([
  'boot-interrupted',
  'entrypoint-load-failed',
  'manifest-copy-failed',
  'runtime-init-failed',
  'startup-aborted',
  'startup-timeout',
])

const bootEntrypoints = new Set([
  'canvas',
  'computers',
  'drive',
  'forge',
  'space',
])

const bootProjects = new Set(['spacewave'])

const bootBrowserEngines = new Set(['chromium', 'webkit'])

const bootOSFamilies = new Set(['android', 'darwin', 'ios', 'linux', 'windows'])

const bootArchitectures = new Set(['amd64', 'arm64', 'wasm'])

type Interval = readonly [bigint, bigint]
type SpanFrame = Readonly<{
  span: BootSpan
  start: bigint
  end: bigint
}>

function intervalUnionDuration(intervals: Interval[]): bigint {
  if (intervals.length === 0) return 0n
  const ordered = intervals.toSorted((a, b) =>
    a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : a[1] < b[1] ? -1 : a[1] > b[1] ? 1 : 0,
  )
  let start = ordered[0][0]
  let end = ordered[0][1]
  let duration = 0n
  for (const interval of ordered.slice(1)) {
    if (interval[0] > end) {
      duration += end - start
      start = interval[0]
      end = interval[1]
      continue
    }
    if (interval[1] > end) end = interval[1]
  }
  return duration + end - start
}

function clippedInterval(
  span: BootSpan,
  start: bigint,
  end: bigint,
): Interval | undefined {
  const clippedStart =
    (span.startMonotonicMicros ?? 0n) > start
      ? (span.startMonotonicMicros ?? 0n)
      : start
  const clippedEnd =
    (span.endMonotonicMicros ?? 0n) < end
      ? (span.endMonotonicMicros ?? 0n)
      : end
  return clippedEnd > clippedStart ? [clippedStart, clippedEnd] : undefined
}

function validIdentifier(value: string | undefined): boolean {
  if (!value || value.length > maxBootStringBytes || validHex(value, 32)) {
    return false
  }
  let previousSeparator = false
  for (let index = 0; index < value.length; index++) {
    const char = value.charCodeAt(index)
    const letter = char >= 97 && char <= 122
    const digit = char >= 48 && char <= 57
    const separator = char === 45
    if (
      (!letter && !digit && !separator) ||
      (index === 0 && !letter) ||
      (separator && (previousSeparator || index === value.length - 1))
    ) {
      return false
    }
    previousSeparator = separator
  }
  return true
}

function validReportId(value: string | undefined): boolean {
  if (value === 'fixture-report') return true
  const prefix = 'boot-report-'
  return (
    !!value?.startsWith(prefix) &&
    validIdentifier(`id-${value.slice(prefix.length)}`)
  )
}

function validOptionalReportId(value: string | undefined): boolean {
  return !value || validReportId(value)
}

function validScopeId(value: string | undefined): boolean {
  const prefix = 'boot-candidate'
  return (
    value === prefix ||
    (!!value?.startsWith(`${prefix}-`) &&
      validIdentifier(`id-${value.slice(prefix.length + 1)}`))
  )
}

function validSpanId(value: string | undefined): boolean {
  if (
    ['network', 'runtime', 'scheduler', 'storage', 'wait'].includes(value ?? '')
  ) {
    return true
  }
  const prefix = 'span-'
  return (
    !!value?.startsWith(prefix) &&
    validIdentifier(`id-${value.slice(prefix.length)}`)
  )
}

function validOptionalSpanId(value: string | undefined): boolean {
  return !value || validSpanId(value)
}

function validArtifactId(value: string | undefined): boolean {
  const prefix = 'boot-artifact-'
  return (
    !!value?.startsWith(prefix) &&
    validIdentifier(`id-${value.slice(prefix.length)}`)
  )
}

function validFieldPath(value: string): boolean {
  if (!value || value.length > maxBootStringBytes) return false
  const parts = value.split('.')
  if (parts.length > maxBootPrivacyFields) return false
  for (const part of parts) {
    if (!part) return false
    for (let index = 0; index < part.length; index++) {
      const char = part.charCodeAt(index)
      const letter = char >= 97 && char <= 122
      const digit = index > 0 && char >= 48 && char <= 57
      const underscore = index > 0 && char === 95
      if (!letter && !digit && !underscore) return false
    }
  }
  return true
}

function validCommit(value: string | undefined): boolean {
  return !value || validHex(value, 7)
}

function validHash(value: string): boolean {
  return validHex(value, 32)
}

function validHex(value: string, minimum: number): boolean {
  return (
    value.length >= minimum &&
    value.length <= maxBootStringBytes &&
    /^[0-9a-f]+$/.test(value)
  )
}

function spanIsActionable(span: BootSpan): boolean {
  if (
    !bootOwners.has(span.owner ?? '') ||
    !bootOperations.has(span.operation ?? '')
  ) {
    return false
  }
  switch (span.workClass) {
    case BootWorkClass.COMPUTE:
      return bootOperations.has(span.sourceTask ?? '')
    case BootWorkClass.STORAGE_IO:
    case BootWorkClass.NETWORK_IO:
    case BootWorkClass.SCHEDULER:
      return true
    case BootWorkClass.LOCK_WAIT:
    case BootWorkClass.DEPENDENCY_WAIT:
      return bootOperations.has(span.causalWaitTarget ?? '')
    default:
      return false
  }
}

function violation(
  kind: BootValidationViolationKind,
  values: Partial<BootValidationViolation> = {},
): BootValidationViolation {
  return { kind, ...values }
}

function validInt64(value: bigint | undefined): boolean {
  const scalar = value ?? 0n
  return scalar >= minInt64 && scalar <= maxInt64
}

function validUint64(value: bigint | undefined): boolean {
  const scalar = value ?? 0n
  return scalar >= 0n && scalar <= maxUint64
}

function validBootValue(value: BootValue | undefined): boolean {
  switch (value?.value?.case) {
    case 'stringValue':
      return bootDetailStrings.has(value.value.value)
    case 'numberValue':
      return Number.isFinite(value.value.value)
    case 'signedValue':
      return validInt64(value.value.value)
    case 'unsignedValue':
      return validUint64(value.value.value)
    case 'boolValue':
      return true
    default:
      return false
  }
}

function validEnum(
  value: number | undefined,
  minimum: number,
  maximum: number,
): boolean {
  return (
    value !== undefined &&
    Number.isInteger(value) &&
    value >= minimum &&
    value <= maximum
  )
}

function reportExceedsCollectionBounds(report: BootReport): boolean {
  const marks = report.marks?.length ?? 0
  const spans = report.spans?.length ?? 0
  const samples = report.accounting?.samples?.length ?? 0
  const attachments = report.attachments?.length ?? 0
  const validation = report.validation
  return (
    marks > maxBootMarks ||
    spans > maxBootSpans ||
    samples > maxBootSamples ||
    attachments > maxBootAttachments ||
    (report.privacy?.removedFields?.length ?? 0) > maxBootPrivacyFields ||
    (report.privacy?.bucketedFields?.length ?? 0) > maxBootPrivacyFields ||
    (validation?.phaseDurations?.length ?? 0) > BootPhase.DONE ||
    (validation?.longestGaps?.length ?? 0) > maxBootMarks ||
    (validation?.violations?.length ?? 0) > maxBootRecords ||
    marks + spans + samples + attachments > maxBootRecords
  )
}

type BinaryWriter = ReturnType<BinaryWriteOptions['writerFactory']>

class BoundedSizeWriter implements BinaryWriter {
  private count = 0
  private readonly stack: number[] = []

  constructor(private readonly limit: number) {}

  get size(): number {
    return this.count
  }

  finish(): Uint8Array {
    return new Uint8Array()
  }

  fork(): this {
    this.stack.push(this.count)
    this.count = 0
    return this
  }

  join(): this {
    const nested = this.count
    const parent = this.stack.pop() ?? 0
    this.count = Math.min(
      this.limit + 1,
      parent + unsignedVarintSize(BigInt(nested)) + nested,
    )
    return this
  }

  tag(fieldNo: number, type: number): this {
    return this.uint32((fieldNo << 3) | type)
  }

  raw(chunk: Uint8Array): this {
    this.add(chunk.byteLength)
    return this
  }

  uint32(value: number): this {
    this.add(unsignedVarintSize(BigInt(value >>> 0)))
    return this
  }

  int32(value: number): this {
    this.add(value < 0 ? 10 : unsignedVarintSize(BigInt(value)))
    return this
  }

  sint32(value: number): this {
    this.add(unsignedVarintSize(BigInt((value << 1) ^ (value >> 31))))
    return this
  }

  int64(value: string | number | bigint): this {
    const bigint = BigInt(value)
    this.add(bigint < 0n ? 10 : unsignedVarintSize(bigint))
    return this
  }

  uint64(value: string | number | bigint): this {
    this.add(unsignedVarintSize(BigInt(value)))
    return this
  }

  sint64(value: string | number | bigint): this {
    const bigint = BigInt(value)
    this.add(unsignedVarintSize((bigint << 1n) ^ (bigint >> 63n)))
    return this
  }

  fixed64(_value: string | number | bigint): this {
    this.add(8)
    return this
  }

  sfixed64(_value: string | number | bigint): this {
    this.add(8)
    return this
  }

  bool(_value: boolean): this {
    this.add(1)
    return this
  }

  fixed32(_value: number): this {
    this.add(4)
    return this
  }

  sfixed32(_value: number): this {
    this.add(4)
    return this
  }

  float(_value: number): this {
    this.add(4)
    return this
  }

  double(_value: number): this {
    this.add(8)
    return this
  }

  bytes(value: Uint8Array): this {
    this.add(unsignedVarintSize(BigInt(value.byteLength)) + value.byteLength)
    return this
  }

  string(value: string): this {
    const length = utf8Length(value)
    this.add(unsignedVarintSize(BigInt(length)) + length)
    return this
  }

  private add(length: number): void {
    this.count = Math.min(this.limit + 1, this.count + length)
  }
}

function unsignedVarintSize(value: bigint): number {
  let size = 1
  for (let current = value; current >= 128n; current >>= 7n) size++
  return size
}

function utf8Length(value: string): number {
  let length = 0
  for (let index = 0; index < value.length; index++) {
    const char = value.charCodeAt(index)
    if (char < 0x80) length++
    else if (char < 0x800) length += 2
    else if (char >= 0xd800 && char <= 0xdbff && index + 1 < value.length) {
      length += 4
      index++
    } else length += 3
  }
  return length
}

function boundedEncodedSize(report: BootReport): number {
  const writer = new BoundedSizeWriter(maxBootRecordBytes)
  binaryWriteMessage(report, BootReport.fields, writer, {
    writeUnknownFields: true,
    writerFactory: () => writer,
  })
  return writer.size
}

function validReportContract(report: BootReport, total: bigint): boolean {
  if (boundedEncodedSize(report) > maxBootRecordBytes) return false
  if (
    report.schemaVersion !== 1 ||
    !validReportId(report.reportId) ||
    !validOptionalReportId(report.parentReportId) ||
    !bootEntrypoints.has(report.entrypointId ?? '') ||
    !bootMarkLabels.has(report.usableMark ?? '') ||
    !validInt64(report.startedUnixMicros) ||
    (report.startedUnixMicros ?? 0n) <= 0n ||
    !validInt64(report.monotonicOriginUnixMicros) ||
    (report.monotonicOriginUnixMicros ?? 0n) <= 0n ||
    !validUint64(total) ||
    total > maxBootDurationMicros ||
    !report.build ||
    !report.environment ||
    !report.accounting ||
    !report.privacy ||
    report.privacy.exportPolicyVersion !== 1
  ) {
    return false
  }
  const build = report.build
  if (
    !validCommit(build.commit) ||
    !validIdentifier(build.releaseGeneration) ||
    !bootProjects.has(build.projectId ?? '') ||
    !validEnum(build.buildType, BootBuildType.DEBUG, BootBuildType.RELEASE) ||
    !validEnum(
      build.runtimeKind,
      BootRuntimeKind.GOSCRIPT,
      BootRuntimeKind.NATIVE,
    ) ||
    !validEnum(build.workerMode, BootWorkerMode.SHARED, BootWorkerMode.INLINE)
  ) {
    return false
  }
  const environment = report.environment
  if (
    !validEnum(
      environment.class,
      BootEnvironmentClass.LOCAL,
      BootEnvironmentClass.PRODUCTION,
    ) ||
    !bootBrowserEngines.has(environment.browserEngine ?? '') ||
    !bootOSFamilies.has(environment.osFamily ?? '') ||
    !bootArchitectures.has(environment.architecture ?? '') ||
    !validEnum(
      environment.serviceWorkerState,
      BootServiceWorkerState.UNAVAILABLE,
      BootServiceWorkerState.REDUNDANT,
    ) ||
    !validEnum(
      environment.cacheState,
      BootCacheState.COLD,
      BootCacheState.HOT,
    ) ||
    !validEnum(
      environment.recoveryDecision,
      BootRecoveryDecision.NONE,
      BootRecoveryDecision.ABORT_STALE,
    )
  ) {
    return false
  }
  const marks = report.marks ?? []
  const spans = report.spans ?? []
  const samples = report.accounting.samples ?? []
  const attachments = report.attachments ?? []
  const privacy = report.privacy
  const markTimes = new Set(marks.map((mark) => mark.monotonicMicros ?? 0n))
  for (const mark of marks) {
    if (
      !validUint64(mark.sequence) ||
      !validUint64(mark.monotonicMicros) ||
      (mark.detail?.length ?? 0) > maxBootDetailFields ||
      !bootMarkLabels.has(mark.label ?? '') ||
      !bootOwners.has(mark.sourceOwner ?? '')
    ) {
      return false
    }
    let previous = ''
    for (const detail of mark.detail ?? []) {
      if (
        !bootDetailKeys.has(detail.key ?? '') ||
        (detail.key ?? '') <= previous ||
        !validBootValue(detail.value)
      ) {
        return false
      }
      previous = detail.key ?? ''
    }
  }
  let previousSampleTime = 0n
  for (let index = 0; index < samples.length; index++) {
    const sample = samples[index]
    const time = sample.monotonicMicros ?? 0n
    if (
      !validScopeId(sample.scopeId) ||
      !bootOwners.has(sample.owner ?? '') ||
      !bootCounterNames.has(sample.name ?? '') ||
      !validEnum(
        sample.unit,
        BootCounterUnit.COUNT,
        BootCounterUnit.MICROSECONDS,
      ) ||
      !validUint64(sample.value) ||
      !validUint64(time) ||
      time > total ||
      !markTimes.has(time) ||
      (!sample.known && (sample.value ?? 0n) !== 0n) ||
      (index !== 0 && time < previousSampleTime)
    ) {
      return false
    }
    previousSampleTime = time
  }
  for (const attachment of attachments) {
    if (
      !validArtifactId(attachment.artifactId) ||
      !validEnum(
        attachment.kind,
        BootAttachmentKind.SOURCE_MAP,
        BootAttachmentKind.NETWORK_PROFILE,
      ) ||
      !validHash(attachment.contentHash ?? '') ||
      !validUint64(attachment.sizeBytes) ||
      (attachment.sizeBytes ?? 0n) === 0n ||
      !validIdentifier(attachment.releaseGeneration) ||
      attachment.releaseGeneration !== build.releaseGeneration
    ) {
      return false
    }
  }
  for (const field of [
    ...(privacy.removedFields ?? []),
    ...(privacy.bucketedFields ?? []),
  ]) {
    if (!validFieldPath(field)) return false
  }
  const shared = privacy.sharedUnixMicros ?? 0n
  const destination = privacy.shareDestination ?? BootShareDestination.UNKNOWN
  if (
    (shared === 0n) !== (destination === BootShareDestination.UNKNOWN) ||
    !validInt64(shared) ||
    (destination !== BootShareDestination.UNKNOWN &&
      !validEnum(
        destination,
        BootShareDestination.CLIPBOARD,
        BootShareDestination.SPACE,
      ))
  ) {
    return false
  }
  if (
    !bootMarkLabels.has(report.terminalMark ?? '') ||
    (!!report.terminalErrorCode &&
      !bootTerminalErrorCodes.has(report.terminalErrorCode))
  ) {
    return true
  }
  for (const span of spans) {
    if (
      !validSpanId(span.spanId) ||
      !validOptionalSpanId(span.parentSpanId) ||
      !bootOwners.has(span.owner ?? '') ||
      !bootOperations.has(span.operation ?? '') ||
      (!!span.causalWaitTarget && !bootOperations.has(span.causalWaitTarget)) ||
      (!!span.sourceTask && !bootOperations.has(span.sourceTask))
    ) {
      return true
    }
  }
  return true
}

function validateSpanStructure(
  spans: BootSpan[],
  total: bigint,
): Readonly<{ children: Map<string, BootSpan[]>; invalid: boolean }> {
  const spanById = new Map<string, BootSpan>()
  const children = new Map<string, BootSpan[]>()
  let invalid = false
  for (const span of spans) {
    const id = span.spanId ?? ''
    const duplicate = spanById.has(id)
    if (
      !validSpanId(id) ||
      duplicate ||
      !bootOwners.has(span.owner ?? '') ||
      !bootOperations.has(span.operation ?? '') ||
      !validEnum(
        span.result,
        BootSpanResult.SUCCEEDED,
        BootSpanResult.CANCELED,
      ) ||
      !validEnum(
        span.workClass,
        BootWorkClass.COMPUTE,
        BootWorkClass.SCHEDULER,
      ) ||
      !validUint64(span.startMonotonicMicros) ||
      !validUint64(span.endMonotonicMicros) ||
      (span.endMonotonicMicros ?? 0n) <= (span.startMonotonicMicros ?? 0n) ||
      (span.endMonotonicMicros ?? 0n) > total ||
      !validOptionalSpanId(span.parentSpanId) ||
      (!!span.causalWaitTarget && !bootOperations.has(span.causalWaitTarget)) ||
      (!!span.sourceTask && !bootOperations.has(span.sourceTask))
    ) {
      invalid = true
    }
    if (!duplicate && id) spanById.set(id, span)
    const siblings = children.get(span.parentSpanId ?? '') ?? []
    siblings.push(span)
    children.set(span.parentSpanId ?? '', siblings)
  }
  for (const span of spans) {
    if (!span.parentSpanId) continue
    const parent = spanById.get(span.parentSpanId)
    if (
      !parent ||
      (span.startMonotonicMicros ?? 0n) < (parent.startMonotonicMicros ?? 0n) ||
      (span.endMonotonicMicros ?? 0n) > (parent.endMonotonicMicros ?? 0n)
    ) {
      invalid = true
    }
  }
  for (const span of spans) {
    const visited = new Set<string>()
    let current: BootSpan | undefined = span
    for (let depth = 0; current?.parentSpanId; depth++) {
      if (depth >= maxBootSpanDepth || visited.has(current.parentSpanId)) {
        invalid = true
        break
      }
      visited.add(current.parentSpanId)
      current = spanById.get(current.parentSpanId)
    }
  }
  return { children, invalid }
}

function validateSpanTree(
  start: bigint,
  end: bigint,
  threshold: bigint,
  roots: BootSpan[],
  children: Map<string, BootSpan[]>,
  violations: BootValidationViolation[],
): Interval[] {
  const stack: SpanFrame[] = roots
    .toReversed()
    .map((span) => ({ span, start, end }))
  const actionable: Interval[] = []
  const visited = new Set<string>()
  while (stack.length !== 0) {
    const frame = stack.pop()
    if (!frame) break
    const interval = clippedInterval(frame.span, frame.start, frame.end)
    if (!interval) continue
    const id = frame.span.spanId ?? ''
    if (visited.has(id)) continue
    visited.add(id)
    const nested = children.get(id) ?? []
    if (nested.length === 0) {
      if (spanIsActionable(frame.span)) {
        actionable.push(interval)
      } else if (interval[1] - interval[0] > threshold) {
        violations.push(
          violation(BootValidationViolationKind.GENERIC_LEAF, {
            startMonotonicMicros: interval[0],
            endMonotonicMicros: interval[1],
            spanId: id,
          }),
        )
      }
      continue
    }
    const childIntervals = nested
      .map((child) => clippedInterval(child, interval[0], interval[1]))
      .filter((child): child is Interval => child !== undefined)
    const duration = interval[1] - interval[0]
    const covered = intervalUnionDuration(childIntervals)
    if (
      duration > threshold &&
      (duration - covered > duration / noGapDivisor ||
        duration - covered > threshold)
    ) {
      violations.push(
        violation(BootValidationViolationKind.GAP_COVERAGE, {
          startMonotonicMicros: interval[0],
          endMonotonicMicros: interval[1],
          spanId: id,
        }),
      )
    }
    for (const child of nested.toReversed()) {
      stack.push({ span: child, start: interval[0], end: interval[1] })
    }
  }
  return actionable
}

// validateBootReport derives phase totals and enforces the recursive five-percent
// attribution contract over one terminal BootReport.
export function validateBootReport(report: BootReport): BootValidationMessage {
  const violations: BootValidationViolation[] = []
  const total = report.terminalMonotonicMicros ?? 0n
  if (reportExceedsCollectionBounds(report)) {
    return BootValidation.create({
      totalDurationMicros: total,
      violations: [violation(BootValidationViolationKind.REPORT_CONTRACT)],
    })
  }
  if (!validReportContract(report, total)) {
    violations.push(violation(BootValidationViolationKind.REPORT_CONTRACT))
  }

  const marks = report.marks ?? []
  const usableMarks = marks.filter((mark) => mark.label === report.usableMark)
  const terminalMarks = marks.filter(
    (mark) => mark.label === report.terminalMark,
  )
  const terminalState =
    report.state === BootReportState.READY ||
    report.state === BootReportState.FAILED ||
    report.state === BootReportState.ABORTED
  let terminalInvalid =
    !terminalState ||
    total === 0n ||
    !bootMarkLabels.has(report.terminalMark ?? '') ||
    terminalMarks.length !== 1 ||
    (terminalMarks[0]?.monotonicMicros ?? 0n) !== total
  switch (report.state) {
    case BootReportState.READY:
      terminalInvalid ||=
        report.terminalMark !== report.usableMark ||
        usableMarks.length !== 1 ||
        !!report.terminalErrorCode
      break
    case BootReportState.FAILED:
      terminalInvalid ||= usableMarks.length !== 0
      terminalInvalid ||= !bootTerminalErrorCodes.has(
        report.terminalErrorCode ?? '',
      )
      break
    case BootReportState.ABORTED:
      terminalInvalid ||= usableMarks.length !== 0
      terminalInvalid ||=
        !!report.terminalErrorCode &&
        !bootTerminalErrorCodes.has(report.terminalErrorCode)
      break
  }
  if (terminalInvalid) {
    violations.push(violation(BootValidationViolationKind.TERMINAL_CONTRACT))
  }

  let markOrderInvalid =
    marks.length < 2 || (marks[0]?.monotonicMicros ?? 0n) !== 0n
  for (let index = 0; index < marks.length; index++) {
    const mark = marks[index]
    const previous = marks[index - 1]
    if (
      (mark.sequence ?? 0n) === 0n ||
      !bootMarkLabels.has(mark.label ?? '') ||
      !bootOwners.has(mark.sourceOwner ?? '') ||
      !validEnum(mark.phase, BootPhase.PREPARE, BootPhase.DONE) ||
      (mark.monotonicMicros ?? 0n) > total ||
      (previous !== undefined &&
        ((mark.sequence ?? 0n) <= (previous.sequence ?? 0n) ||
          (mark.monotonicMicros ?? 0n) < (previous.monotonicMicros ?? 0n)))
    ) {
      markOrderInvalid = true
      break
    }
  }
  if ((marks.at(-1)?.monotonicMicros ?? 0n) !== total) markOrderInvalid = true
  if (markOrderInvalid) {
    violations.push(violation(BootValidationViolationKind.MARK_ORDER))
  }

  const spanStructure = validateSpanStructure(report.spans ?? [], total)
  if (spanStructure.invalid) {
    violations.push(violation(BootValidationViolationKind.SPAN_CONTRACT))
  }

  const phaseTotals = new Map<BootPhase, bigint>()
  const gaps: BootGap[] = []
  const threshold = total / noGapDivisor
  let unknown = 0n
  if (!markOrderInvalid && !spanStructure.invalid && total > 0n) {
    for (let index = 0; index < marks.length - 1; index++) {
      const before = marks[index]
      const after = marks[index + 1]
      const start = before.monotonicMicros ?? 0n
      const end = after.monotonicMicros ?? 0n
      const duration = end - start
      phaseTotals.set(
        before.phase ?? BootPhase.UNKNOWN,
        (phaseTotals.get(before.phase ?? BootPhase.UNKNOWN) ?? 0n) + duration,
      )
      if (duration <= threshold) continue

      const topLevel = spanStructure.children.get('') ?? []
      const topIntervals = topLevel
        .map((span) => clippedInterval(span, start, end))
        .filter((interval): interval is Interval => interval !== undefined)
      const covered = intervalUnionDuration(topIntervals)
      gaps.push({
        beforeSequence: before.sequence,
        afterSequence: after.sequence,
        startMonotonicMicros: start,
        endMonotonicMicros: end,
        durationMicros: duration,
        coveredDurationMicros: covered,
      })
      if (
        duration - covered > duration / noGapDivisor ||
        duration - covered > threshold
      ) {
        violations.push(
          violation(BootValidationViolationKind.GAP_COVERAGE, {
            startMonotonicMicros: start,
            endMonotonicMicros: end,
            markSequence: before.sequence,
          }),
        )
      }
      const actionable = validateSpanTree(
        start,
        end,
        threshold,
        topLevel,
        spanStructure.children,
        violations,
      )
      unknown += duration - intervalUnionDuration(actionable)
    }
  }

  const unknownPartsPerMillion =
    total === 0n ? 0 : Number((unknown * completePartsPerMillion) / total)
  if (!spanStructure.invalid && unknown > threshold) {
    violations.push(violation(BootValidationViolationKind.UNKNOWN_SHARE))
  }
  const phaseDurations = [...phaseTotals.entries()]
    .filter(([phase]) => phase !== BootPhase.UNKNOWN)
    .toSorted(([a], [b]) => a - b)
    .map(([phase, durationMicros]) => ({ phase, durationMicros }))
  const longestGaps = gaps.toSorted((a, b) => {
    const aDuration = a.durationMicros ?? 0n
    const bDuration = b.durationMicros ?? 0n
    if (aDuration !== bDuration) return aDuration > bDuration ? -1 : 1
    return (a.beforeSequence ?? 0n) < (b.beforeSequence ?? 0n) ? -1 : 1
  })
  return BootValidation.create({
    pass: violations.length === 0,
    totalDurationMicros: total,
    unknownDurationMicros: unknown,
    unknownPartsPerMillion,
    phaseDurations,
    longestGaps,
    violations,
  })
}
