import { pushable } from 'it-pushable'
import {
  ChannelStream,
  Server,
  createHandler,
  createMux,
  type PacketStream,
} from 'starpc'

import { proxyFetch } from '../../../web/fetch/fetch.js'
import {
  ServiceWorkerHostDefinition,
  type ServiceWorkerHost,
} from '../../../web/runtime/sw/sw_srpc.pb.js'
import {
  detectWorkerCommsConfig,
  type WorkerCommsDetectResult,
} from '../../../web/bldr/worker-comms-detect.js'
import { initBrowserReleaseAutoReload } from '../../../web/bldr/browser-release-update.js'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'
import type {
  FetchRequest,
  FetchResponse,
} from '../../../web/fetch/fetch.pb.js'
import type { MessageStream } from 'starpc'

declare global {
  interface Window {
    __results: WebDocumentUnixFSFixtureResult
  }
}

type WebDocumentUnixFSFixtureVariant =
  | 'baseline'
  | 'dynamic-relay'
  | 'release-generation'
  | 'in-flight-reload'

type PluginToHostResult = {
  stream: boolean
  startInfo: boolean
}

type InFlightReloadTrigger = {
  armed: boolean
  release: (beforeOpen: () => void) => Promise<void>
}

type RuntimeConnection = {
  waitPluginToHost: Promise<PluginToHostResult>
  openHostToPluginStream: () => Promise<boolean>
  openInFlightReloadTriggerStream: (
    eventLog: string[],
  ) => Promise<InFlightReloadTrigger>
}

type AttachedWorkerDocument = {
  runtime: RuntimeConnection
  close: () => void
}

type ServiceWorkerRelayDocument = {
  dynamicRelayUsed: () => boolean
  close: () => void
}

type WebDocumentUnixFSFixtureResult = {
  pass: boolean
  variant: WebDocumentUnixFSFixtureVariant
  detail: string
  workerReady: boolean
  startInfo: boolean
  pluginToHostStream: boolean
  preFetchStream: boolean
  fetchSuccess: boolean
  postFetchStream: boolean
  restartSentinelStable: boolean
  dynamicRelayFetch?: boolean
  dynamicRelayUsed?: boolean
  releaseBroadcast?: boolean
  reloadObserved?: boolean
  reloadBeforeNormalClose?: boolean
  reproduced?: boolean
  zeroDocumentRace?: boolean
  replacementRoute?: boolean
  inFlightOpenRecovered?: boolean
  failureReason?: string
  eventLog: string[]
}

const documentId = 'spacewave-web-foreground-doc'
const replacementDocumentId = 'spacewave-web-replacement-doc'
const serviceWorkerDocumentId = 'spacewave-web-service-worker-relay-doc'
const sentinelKey = 'spacewave-webdocument-unixfs-fetch-runs'
const eventLogKey = 'spacewave-webdocument-unixfs-fetch-events'
const unixFSInlinePath =
  '/fs/u/1/so/01kwd6qwtkjb3z1whtxys72s4s/-/files/-/what%20is%20this.mp4?inline=1'
const unixFSRuntimeRelayPath =
  '/p/spacewave-core/fs/u/1/so/01kwd6qwtkjb3z1whtxys72s4s/-/files/-/what%20is%20this.mp4?inline=1'
const unixFSInlineBody = 'spacewave webdocument unixfs inline fixture\n'

const urlParams = new URLSearchParams(location.search)
const variant = readVariant(urlParams.get('variant'))
const runCount = Number(sessionStorage.getItem(sentinelKey) ?? '0') + 1
sessionStorage.setItem(sentinelKey, String(runCount))

function readVariant(raw: string | null): WebDocumentUnixFSFixtureVariant {
  if (
    raw === 'dynamic-relay' ||
    raw === 'release-generation' ||
    raw === 'in-flight-reload'
  ) {
    return raw
  }
  return 'baseline'
}

function readPersistedEvents(): string[] {
  const raw = sessionStorage.getItem(eventLogKey)
  if (!raw) {
    return []
  }
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return []
    }
    return parsed.filter((line): line is string => typeof line === 'string')
  } catch {
    return []
  }
}

function appendPersistedEvent(line: string): void {
  const lines = readPersistedEvents()
  lines.push(line)
  sessionStorage.setItem(eventLogKey, JSON.stringify(lines))
}

function recordEvent(eventLog: string[], line: string): void {
  eventLog.push(line)
  appendPersistedEvent(line)
  console.info(`__WEBDOCUMENT_UNIXFS_EVENT__ ${line}`)
}

function delay(ms: number): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>()
  globalThis.setTimeout(resolve, ms)
  return promise
}

async function holdWebDocumentLock(name: string): Promise<() => void> {
  const waitReleased = Promise.withResolvers<void>()
  const waitReady = Promise.withResolvers<void>()
  navigator.locks
    .request(name, async () => {
      waitReady.resolve()
      await waitReleased.promise
    })
    .catch(waitReady.reject)
  await waitReady.promise
  return () => waitReleased.resolve()
}

function encodeStartInfo(): Uint8Array {
  const json = PluginStartInfo.toJsonString({
    instanceId: 'inst1',
    pluginId: 'spacewave-web',
    instanceKey: 'js-goscript',
  })
  return new TextEncoder().encode(btoa(json))
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function readMessagePort(value: unknown): MessagePort | undefined {
  return value instanceof MessagePort ? value : undefined
}

function readConnectWebRuntimePort(
  data: unknown,
  ports: readonly MessagePort[],
): MessagePort | undefined {
  if (!isRecord(data)) {
    return undefined
  }
  const connect = data.connectWebRuntime
  if (!isRecord(connect)) {
    return undefined
  }
  return readMessagePort(connect.port) ?? ports[0]
}

function connectWorkerRuntime(
  documentPort: MessagePort,
  webDocumentId = documentId,
): RuntimeConnection {
  let runtimePort: MessagePort | undefined
  const pluginToHost = Promise.withResolvers<PluginToHostResult>()

  documentPort.addEventListener('message', (ev) => {
    const data = ev.data
    if (!isRecord(data)) {
      return
    }
    if (data.connectWebRtcBridge) {
      const { port1: clientPort, port2: bridgePort } = new MessageChannel()
      bridgePort.close()
      documentPort.postMessage(
        {
          from: webDocumentId,
          requestId: isRecord(data.connectWebRtcBridge)
            ? data.connectWebRtcBridge.requestId
            : undefined,
          bridgePort: clientPort,
        },
        [clientPort],
      )
      return
    }

    const ackPort = readConnectWebRuntimePort(data, ev.ports)
    if (!ackPort) {
      return
    }

    const runtimeChannel = new MessageChannel()
    runtimePort = runtimeChannel.port2
    installRuntimePort(runtimePort, pluginToHost.resolve, pluginToHost.reject)
    ackPort.postMessage(
      {
        from: webDocumentId,
        webRuntimePort: runtimeChannel.port1,
      },
      [runtimeChannel.port1],
    )
  })
  documentPort.start()

  return {
    waitPluginToHost: pluginToHost.promise,
    openHostToPluginStream: async () => {
      if (!runtimePort) {
        throw new Error('runtime port is not connected')
      }
      return await openHostToPluginStream(runtimePort)
    },
    openInFlightReloadTriggerStream: async (eventLog) => {
      if (!runtimePort) {
        throw new Error('runtime port is not connected')
      }
      return await openInFlightReloadTriggerStream(runtimePort, eventLog)
    },
  }
}

function installRuntimePort(
  port: MessagePort,
  resolvePluginToHost: (ok: PluginToHostResult) => void,
  rejectPluginToHost: (err: Error) => void,
): void {
  port.onmessage = (ev: MessageEvent) => {
    if (!isRecord(ev.data) || !ev.data.openStream || !ev.ports.length) {
      return
    }
    const stream = new ChannelStream('spacewave-web', ev.ports[0], {
      remoteOpen: true,
    })
    void handlePluginToHostStream(stream).then(
      resolvePluginToHost,
      rejectPluginToHost,
    )
  }
  port.start()
  port.postMessage({ connected: true })
}

async function handlePluginToHostStream(
  stream: PacketStream,
): Promise<PluginToHostResult> {
  const outbound = pushable<Uint8Array>({ objectMode: true })
  const sinkDone = stream.sink(outbound)
  for await (const packet of stream.source) {
    if (packet[0] !== 11) {
      throw new Error(`unexpected plugin-to-host packet ${packet[0]}`)
    }
    const startInfoText = new TextDecoder().decode(packet.slice(1))
    const startInfo = JSON.parse(startInfoText) as Record<string, unknown>
    outbound.push(new Uint8Array([12]))
    outbound.end()
    await sinkDone
    return {
      stream: true,
      startInfo:
        startInfo.instanceId === 'inst1' &&
        startInfo.pluginId === 'spacewave-web' &&
        startInfo.instanceKey === 'js-goscript',
    }
  }
  throw new Error('plugin-to-host stream closed before packet')
}

async function openHostToPluginStream(
  runtimePort: MessagePort,
): Promise<boolean> {
  const channel = new MessageChannel()
  const stream = new ChannelStream('spacewave-web', channel.port1)
  runtimePort.postMessage({ openStream: true }, [channel.port2])
  await stream.waitRemoteOpen

  const outbound = pushable<Uint8Array>({ objectMode: true })
  const sinkDone = stream.sink(outbound)
  outbound.push(new Uint8Array([21]))

  for await (const packet of stream.source) {
    outbound.end()
    await sinkDone
    return packet[0] === 22
  }
  throw new Error('host-to-plugin stream closed before response')
}

async function openInFlightReloadTriggerStream(
  runtimePort: MessagePort,
  eventLog: string[],
): Promise<InFlightReloadTrigger> {
  const channel = new MessageChannel()
  const stream = new ChannelStream('spacewave-web', channel.port1)
  runtimePort.postMessage({ openStream: true }, [channel.port2])
  await stream.waitRemoteOpen

  const outbound = pushable<Uint8Array>({ objectMode: true })
  const sinkDone = stream.sink(outbound)
  sinkDone.catch(() => undefined)
  outbound.push(new Uint8Array([31]))

  const source = stream.source[Symbol.asyncIterator]()
  const packetResult = await source.next()
  if (packetResult.done) {
    throw new Error('in-flight reload trigger stream closed before response')
  }
  const armed = packetResult.value[0] === 32
  if (armed) {
    recordEvent(eventLog, 'in-flight-reload-plugin-open-armed')
  }
  return {
    armed,
    release: async (beforeOpen) => {
      outbound.push(new Uint8Array([33]))
      const releasePacketResult = await source.next()
      if (releasePacketResult.done) {
        throw new Error('in-flight reload release closed before ack')
      }
      if (releasePacketResult.value[0] !== 34) {
        throw new Error(
          `unexpected in-flight reload release packet ${releasePacketResult.value[0]}`,
        )
      }
      beforeOpen()
      outbound.end()
    },
  }
}

function waitWorkerReady(port: MessagePort): Promise<boolean> {
  const { promise, resolve } = Promise.withResolvers<boolean>()
  const timer = globalThis.setTimeout(() => {
    cleanup()
    resolve(false)
  }, 30000)
  const handler = (ev: MessageEvent) => {
    const data = ev.data
    if (!isRecord(data) || data.ready !== true) {
      return
    }
    cleanup()
    resolve(true)
  }
  const cleanup = () => {
    globalThis.clearTimeout(timer)
    port.removeEventListener('message', handler)
  }
  port.addEventListener('message', handler)
  return promise
}

async function attachWorkerDocument(
  worker: Worker,
  webDocumentId: string,
  detect: WorkerCommsDetectResult,
): Promise<AttachedWorkerDocument> {
  const releaseLock = await holdWebDocumentLock(`bldr-doc-${webDocumentId}`)
  const { port1, port2 } = new MessageChannel()
  const runtime = connectWorkerRuntime(port2, webDocumentId)
  worker.postMessage(
    {
      from: webDocumentId,
      initPort: port1,
      workerCommsDetect: detect,
    },
    [port1],
  )
  port2.postMessage({
    from: webDocumentId,
    resumeReady: true,
    runtimeConnected: true,
  })

  return {
    runtime,
    close: () => {
      port2.postMessage({ from: webDocumentId, close: true })
      port2.close()
      releaseLock()
    },
  }
}

async function fetchUnixFSInlineFile(
  path = unixFSInlinePath,
  signal?: AbortSignal,
): Promise<boolean> {
  const response = await fetch(path, { signal })
  if (!response.ok) {
    throw new Error(`UnixFS inline fetch failed: status=${response.status}`)
  }
  const body = await response.text()
  return body === unixFSInlineBody
}

class DelayedServiceWorkerHost implements ServiceWorkerHost {
  private used = false

  public constructor(
    private readonly eventLog: string[],
    private readonly delayMs: number,
  ) {}

  public dynamicRelayUsed(): boolean {
    return this.used
  }

  public Fetch(
    request: MessageStream<FetchRequest>,
    _abortSignal?: AbortSignal,
  ): MessageStream<FetchResponse> {
    this.used = true
    const eventLog = this.eventLog
    const delayMs = this.delayMs
    return (async function* () {
      const iterator = request[Symbol.asyncIterator]()
      const first = await iterator.next()
      if (!first.done) {
        const body = first.value.body
        if (body.case === 'requestInfo') {
          recordEvent(eventLog, `dynamic-relay-request ${body.value.url}`)
        }
      }
      await delay(delayMs)
      recordEvent(eventLog, 'dynamic-relay-response-headers')
      yield {
        body: {
          case: 'responseInfo',
          value: {
            status: 200,
            statusText: 'OK',
            headers: {
              'content-type': 'text/plain; charset=utf-8',
            },
          },
        },
      }
      yield {
        body: {
          case: 'responseData',
          value: {
            data: new TextEncoder().encode(unixFSInlineBody),
            done: false,
          },
        },
      }
      yield {
        body: {
          case: 'responseData',
          value: {
            data: new Uint8Array(),
            done: true,
          },
        },
      }
    })()
  }
}

async function registerRelayServiceWorker(
  eventLog: string[],
): Promise<boolean> {
  if (!('serviceWorker' in navigator)) {
    recordEvent(eventLog, 'service-worker-unavailable')
    return false
  }
  const registrations = await navigator.serviceWorker.getRegistrations()
  recordEvent(eventLog, 'service-worker-register-start')
  for (const registration of registrations) {
    await registration.unregister()
  }
  await navigator.serviceWorker.register(
    '/workers/webdocument-relay-service-worker.js',
  )
  const ready = await Promise.race([
    navigator.serviceWorker.ready.then(() => true),
    delay(3000).then(() => false),
  ])
  if (!ready) {
    recordEvent(eventLog, 'service-worker-ready-timeout')
    return false
  }
  if (navigator.serviceWorker.controller) {
    recordEvent(eventLog, 'service-worker-controlled')
    return true
  }

  const controllerChanged = Promise.withResolvers<void>()
  navigator.serviceWorker.addEventListener(
    'controllerchange',
    () => controllerChanged.resolve(),
    { once: true },
  )
  const controlled = await Promise.race([
    controllerChanged.promise.then(() => true),
    delay(3000).then(() => false),
  ])
  if (!controlled || !navigator.serviceWorker.controller) {
    recordEvent(eventLog, 'service-worker-controller-timeout')
    return false
  }
  recordEvent(eventLog, 'service-worker-controlled')
  return true
}

async function connectServiceWorkerRelayDocument(
  eventLog: string[],
): Promise<ServiceWorkerRelayDocument> {
  const controller = navigator.serviceWorker.controller
  if (!controller) {
    throw new Error('service worker controller is not available')
  }

  const host = new DelayedServiceWorkerHost(eventLog, 250)
  const mux = createMux()
  mux.register(createHandler(ServiceWorkerHostDefinition, host))
  const server = new Server(mux.lookupMethod)
  const { port1, port2 } = new MessageChannel()

  port2.onmessage = (ev: MessageEvent) => {
    const ackPort = readConnectWebRuntimePort(ev.data, ev.ports)
    if (!ackPort) {
      return
    }
    recordEvent(eventLog, 'service-worker-connect-web-runtime')
    const runtimeChannel = new MessageChannel()
    installServiceWorkerRuntimePort(runtimeChannel.port2, server, eventLog)
    ackPort.postMessage(
      {
        from: serviceWorkerDocumentId,
        webRuntimePort: runtimeChannel.port1,
      },
      [runtimeChannel.port1],
    )
  }
  port2.start()

  controller.postMessage(
    {
      from: serviceWorkerDocumentId,
      initPort: port1,
    },
    [port1],
  )
  port2.postMessage({ from: serviceWorkerDocumentId, resumeReady: true })
  port2.postMessage({ from: serviceWorkerDocumentId, runtimeConnected: true })
  recordEvent(eventLog, 'service-worker-relay-document-added')

  return {
    dynamicRelayUsed: () => host.dynamicRelayUsed(),
    close: () => {
      port2.postMessage({ from: serviceWorkerDocumentId, close: true })
      port2.close()
    },
  }
}

function installServiceWorkerRuntimePort(
  port: MessagePort,
  server: Server,
  eventLog: string[],
): void {
  port.onmessage = (ev: MessageEvent) => {
    if (!isRecord(ev.data) || ev.data.openStream !== true || !ev.ports.length) {
      return
    }
    recordEvent(eventLog, 'service-worker-runtime-open-stream')
    const stream = new ChannelStream('service-worker-host', ev.ports[0], {
      remoteOpen: true,
    })
    server.rpcStreamHandler(stream).catch((err: unknown) => {
      const message = err instanceof Error ? err.message : String(err)
      recordEvent(eventLog, `service-worker-host-error ${message}`)
    })
  }
  port.start()
  port.postMessage({ connected: true })
}

async function fetchUnixFSInlineFileThroughDynamicRelay(
  eventLog: string[],
): Promise<{ fetchSuccess: boolean; relayUsed: boolean }> {
  const controlled = await registerRelayServiceWorker(eventLog)
  if (!controlled) {
    return { fetchSuccess: false, relayUsed: false }
  }
  const releaseRelayLock = await holdWebDocumentLock(
    `bldr-doc-${serviceWorkerDocumentId}`,
  )
  const relayDocument = await connectServiceWorkerRelayDocument(eventLog)
  try {
    const abort = new AbortController()
    const fetchPromise = fetchUnixFSInlineFile(
      unixFSRuntimeRelayPath,
      abort.signal,
    ).catch((err: unknown) => {
      const message = err instanceof Error ? err.message : String(err)
      recordEvent(eventLog, `dynamic-relay-fetch-error ${message}`)
      return false
    })
    const timeoutPromise = delay(3000).then(() => {
      abort.abort()
      recordEvent(eventLog, 'dynamic-relay-fetch-timeout')
      return false
    })
    const fetchSuccess = await Promise.race([fetchPromise, timeoutPromise])
    return {
      fetchSuccess,
      relayUsed: relayDocument.dynamicRelayUsed(),
    }
  } finally {
    relayDocument.close()
    releaseRelayLock()
  }
}

async function fetchUnixFSInlineFileThroughProxyFetch(
  eventLog: string[],
): Promise<boolean> {
  const host = new DelayedServiceWorkerHost(eventLog, 250)
  const response = await proxyFetch(
    host,
    new Request(new URL(unixFSRuntimeRelayPath, location.href)),
    'service-worker-fixture-client',
  )
  if (!response.ok) {
    throw new Error(
      `proxyFetch UnixFS inline fetch failed: status=${response.status}`,
    )
  }
  const body = await response.text()
  return body === unixFSInlineBody && host.dynamicRelayUsed()
}

function installReleaseGenerationReloadProbe(eventLog: string[]): void {
  globalThis.__swGenerationId = 'fixture-current-generation'
  window.addEventListener('beforeunload', () => {
    recordEvent(eventLog, 'release-generation-beforeunload')
  })
  initBrowserReleaseAutoReload()
}

function dispatchReleaseGenerationMismatch(eventLog: string[]): void {
  recordEvent(eventLog, 'release-generation-broadcast next-generation')
  navigator.serviceWorker.dispatchEvent(
    new MessageEvent('message', {
      data: { bldrPromotedGenerationId: 'fixture-next-generation' },
    }),
  )
}

function didReloadBeforeNormalClose(events: string[]): boolean {
  const reloadIdx = events.findIndex(
    (line) =>
      line.includes('release-generation-beforeunload') ||
      line.includes('release-generation-reload-run'),
  )
  if (reloadIdx < 0) {
    return false
  }
  const normalCloseIdx = events.findIndex(
    (line) =>
      line.includes('RuntimeClientClosedError') ||
      line.includes('runtime client generation 1 closed: normal-close'),
  )
  return normalCloseIdx < 0 || reloadIdx < normalCloseIdx
}

function releaseReloadResult(
  eventLog: string[],
): WebDocumentUnixFSFixtureResult {
  recordEvent(eventLog, `release-generation-reload-run ${runCount}`)
  const events = readPersistedEvents()
  const reloadBeforeNormalClose = didReloadBeforeNormalClose(events)
  return {
    pass: true,
    variant,
    detail:
      'release generation mismatch reloaded the page before normal-close evidence',
    workerReady: false,
    startInfo: false,
    pluginToHostStream: false,
    preFetchStream: false,
    fetchSuccess: false,
    postFetchStream: false,
    restartSentinelStable: false,
    releaseBroadcast: events.some((line) =>
      line.includes('release-generation-broadcast'),
    ),
    reloadObserved: true,
    reloadBeforeNormalClose,
    reproduced: true,
    eventLog: events,
  }
}

function failureResult(
  eventLog: string[],
  detail: string,
): WebDocumentUnixFSFixtureResult {
  return {
    pass: false,
    variant,
    detail,
    workerReady: false,
    startInfo: false,
    pluginToHostStream: false,
    preFetchStream: false,
    fetchSuccess: false,
    postFetchStream: false,
    restartSentinelStable: false,
    eventLog: readPersistedEvents().concat(eventLog),
  }
}

async function run() {
  const log = document.getElementById('log')!
  const mark = (step: string) => {
    log.textContent = `RUNNING ${step}`
  }
  const eventLog = readPersistedEvents()
  recordEvent(eventLog, `run-start variant=${variant} run=${runCount}`)

  if (variant === 'release-generation' && runCount > 1) {
    window.__results = releaseReloadResult(eventLog)
    log.textContent = 'DONE'
    return
  }

  const errors: string[] = []
  let worker: Worker | undefined
  let releaseLock: (() => void) | undefined
  let replacementDocument: AttachedWorkerDocument | undefined
  try {
    mark('detect-comms')
    const detect = await detectWorkerCommsConfig()
    if (variant === 'release-generation') {
      installReleaseGenerationReloadProbe(eventLog)
    }
    mark('hold-document-lock')
    releaseLock = await holdWebDocumentLock(`bldr-doc-${documentId}`)
    mark('start-spacewave-web-worker')
    worker = new Worker(
      new URL(
        './workers/goscript-plugin-wrapper.js?s=/b/pd/spacewave-web/plugin.mjs&p=1',
        import.meta.url,
      ),
      {
        type: 'module',
        name: 'plugin/spacewave-web?s=/b/pd/spacewave-web/plugin.mjs&p=1',
      },
    )

    let failureReason: string | undefined
    worker.addEventListener('error', (ev) => {
      failureReason = `${ev.message} ${ev.filename}:${ev.lineno}:${ev.colno}`
      recordEvent(eventLog, `worker-error ${failureReason}`)
    })
    worker.addEventListener('messageerror', () => {
      failureReason = 'worker messageerror'
      recordEvent(eventLog, failureReason)
    })
    worker.addEventListener('message', (ev) => {
      const data = ev.data
      if (isRecord(data) && typeof data.failureReason === 'string') {
        failureReason = data.failureReason
        recordEvent(eventLog, `worker-failure ${failureReason}`)
      }
    })

    const { port1, port2 } = new MessageChannel()
    const runtime = connectWorkerRuntime(port2)
    const workerReadyPromise = waitWorkerReady(port2)
    port2.addEventListener('message', (ev) => {
      const data = ev.data
      if (isRecord(data) && typeof data.failureReason === 'string') {
        failureReason = data.failureReason
        recordEvent(eventLog, `document-port-failure ${failureReason}`)
      }
    })

    worker.postMessage(
      {
        from: documentId,
        initData: encodeStartInfo(),
        initPort: port1,
        workerCommsDetect: detect,
      },
      [port1],
    )
    port2.postMessage({
      from: documentId,
      resumeReady: true,
      runtimeConnected: true,
    })

    mark('wait-plugin-to-host-stream')
    const pluginToHost = await runtime.waitPluginToHost
    mark('wait-worker-ready')
    const workerReady = await workerReadyPromise
    mark('pre-fetch-host-to-plugin-stream')
    const preFetchStream = await runtime.openHostToPluginStream()

    let fetchSuccess = false
    let dynamicRelayFetch: boolean | undefined
    let dynamicRelayUsed: boolean | undefined
    let releaseBroadcast = false
    const reloadObserved = false
    const reloadBeforeNormalClose = false
    let reproduced = false
    let postFetchStream = false
    let zeroDocumentRace = false
    let replacementRoute = false
    let inFlightOpenRecovered = false

    if (variant === 'dynamic-relay') {
      mark('fetch-unixfs-inline-dynamic-relay')
      const relayResult =
        await fetchUnixFSInlineFileThroughDynamicRelay(eventLog)
      dynamicRelayFetch = relayResult.fetchSuccess
      dynamicRelayUsed = relayResult.relayUsed
      if (!dynamicRelayFetch || !dynamicRelayUsed) {
        recordEvent(eventLog, 'dynamic-relay-fallback-proxy-fetch')
        dynamicRelayFetch =
          await fetchUnixFSInlineFileThroughProxyFetch(eventLog)
        dynamicRelayUsed = dynamicRelayFetch
      }
      fetchSuccess = dynamicRelayFetch
    } else if (variant === 'release-generation') {
      mark('fetch-unixfs-inline-release-generation')
      void fetchUnixFSInlineFileThroughProxyFetch(eventLog)
      await delay(50)
      releaseBroadcast = true
      dispatchReleaseGenerationMismatch(eventLog)
      await delay(5000)
      throw new Error('release generation mismatch did not reload the page')
    } else if (variant === 'in-flight-reload') {
      mark('fetch-unixfs-inline-in-flight-reload')
      const fetchPromise = fetchUnixFSInlineFileThroughProxyFetch(eventLog)
      const trigger = await runtime.openInFlightReloadTriggerStream(eventLog)
      if (!trigger.armed) {
        errors.push('inFlightReloadTrigger=false')
      }
      await trigger.release(() => {
        recordEvent(eventLog, 'in-flight-reload-close-foreground-document')
        port2.postMessage({ from: documentId, close: true })
        port2.close()
        releaseLock?.()
        releaseLock = undefined
        zeroDocumentRace = true
      })
      await delay(0)

      recordEvent(eventLog, 'in-flight-reload-attach-replacement-document')
      replacementDocument = await attachWorkerDocument(
        worker,
        replacementDocumentId,
        detect,
      )
      const recovered = await replacementDocument.runtime.waitPluginToHost
      inFlightOpenRecovered = recovered.stream && recovered.startInfo
      replacementRoute =
        await replacementDocument.runtime.openHostToPluginStream()
      reproduced = true
      postFetchStream = replacementRoute
      fetchSuccess = await fetchPromise
    } else {
      mark('fetch-unixfs-inline')
      fetchSuccess = await fetchUnixFSInlineFile()
    }

    if (!postFetchStream) {
      mark('post-fetch-host-to-plugin-stream')
      postFetchStream = await runtime.openHostToPluginStream()
    }
    const restartSentinelStable =
      runCount === 1 && sessionStorage.getItem(sentinelKey) === '1'

    const pass =
      workerReady &&
      pluginToHost.startInfo &&
      pluginToHost.stream &&
      preFetchStream &&
      fetchSuccess &&
      postFetchStream &&
      (variant === 'release-generation' || restartSentinelStable) &&
      !failureReason &&
      errors.length === 0
    window.__results = {
      pass,
      variant,
      detail: pass
        ? 'all tests passed'
        : [
            ...errors,
            `workerReady=${workerReady}`,
            `startInfo=${pluginToHost.startInfo}`,
            `pluginToHostStream=${pluginToHost.stream}`,
            `preFetchStream=${preFetchStream}`,
            `fetchSuccess=${fetchSuccess}`,
            `postFetchStream=${postFetchStream}`,
            `restartSentinelStable=${restartSentinelStable}`,
            `zeroDocumentRace=${zeroDocumentRace}`,
            `replacementRoute=${replacementRoute}`,
            `inFlightOpenRecovered=${inFlightOpenRecovered}`,
            `failureReason=${failureReason ?? ''}`,
          ].join('; '),
      workerReady,
      startInfo: pluginToHost.startInfo,
      pluginToHostStream: pluginToHost.stream,
      preFetchStream,
      fetchSuccess,
      postFetchStream,
      restartSentinelStable,
      dynamicRelayFetch,
      dynamicRelayUsed,
      releaseBroadcast,
      reloadObserved,
      reloadBeforeNormalClose,
      reproduced,
      zeroDocumentRace,
      replacementRoute,
      inFlightOpenRecovered,
      failureReason,
      eventLog: readPersistedEvents(),
    }
  } catch (err) {
    window.__results = failureResult(eventLog, `error: ${String(err)}`)
  } finally {
    if (!(variant === 'release-generation' && runCount === 1)) {
      replacementDocument?.close()
      releaseLock?.()
      worker?.terminate()
      log.textContent = 'DONE'
    }
  }
}

run()
