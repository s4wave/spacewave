import { pushable } from 'it-pushable'
import {
  ChannelStream,
  Server,
  createHandler,
  createMux,
  type PacketStream,
} from 'starpc'

import { channelPacketStream } from '../../../web/bldr/channel-packet-stream.js'
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
import { timeoutPromise } from '../../../web/bldr/timeout.js'
import { waitWorkerReady } from './wait-worker-ready.js'
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
  | 'plugin-host-replacement'
  | 'service-worker-fetch-route-timing'
  | 'deliberate-worker-replacement'

type PluginToHostResult = {
  stream: boolean
  startInfo: boolean
}

type InFlightReloadTrigger = {
  armed: boolean
  release: (beforeOpen: () => Promise<void>) => Promise<void>
}

type TerminalOrphanOutcome = {
  failedFast: boolean
  err: string
}

type TerminalOrphanTrigger = {
  armed: boolean
  waitOutcome: () => Promise<TerminalOrphanOutcome>
}

type RuntimeConnection = {
  waitPluginToHost: Promise<PluginToHostResult>
  openHostToPluginStream: (
    eventLog?: string[],
    label?: string,
  ) => Promise<boolean>
  openInFlightReloadTriggerStream: (
    eventLog: string[],
  ) => Promise<InFlightReloadTrigger>
  openTerminalOrphanStream: (
    eventLog: string[],
  ) => Promise<TerminalOrphanTrigger>
}

type RelayResponseControl = {
  requestStarted: Promise<void>
  releaseResponse: () => void
}

type ControlledProxyFetch = RelayResponseControl & {
  result: Promise<boolean>
}

type AttachedWorkerDocument = {
  runtime: RuntimeConnection
  close: () => Promise<void>
}

type ServiceWorkerRelayDocument = RelayResponseControl & {
  dynamicRelayUsed: () => boolean
  close: () => Promise<void>
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
  pluginHostReplacement?: boolean
  serviceWorkerRouteTiming?: boolean
  deliberateWorkerReplacement?: boolean
  orphanFailedFast?: boolean
  orphanTerminalNotRerouted?: boolean
  orphanOutcomeErr?: string
  orphanElapsedMs?: number
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
const fixtureTimeoutMs = 5000

const urlParams = new URLSearchParams(location.search)
const variant = readVariant(urlParams.get('variant'))
const runCount = Number(sessionStorage.getItem(sentinelKey) ?? '0') + 1
sessionStorage.setItem(sentinelKey, String(runCount))

function readVariant(raw: string | null): WebDocumentUnixFSFixtureVariant {
  if (
    raw === 'dynamic-relay' ||
    raw === 'release-generation' ||
    raw === 'in-flight-reload' ||
    raw === 'plugin-host-replacement' ||
    raw === 'service-worker-fetch-route-timing' ||
    raw === 'deliberate-worker-replacement'
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
  const timedLine = `t=${performance.now().toFixed(3)} ${line}`
  eventLog.push(timedLine)
  appendPersistedEvent(timedLine)
  console.info(`__WEBDOCUMENT_UNIXFS_EVENT__ ${timedLine}`)
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
  let released = false
  return () => {
    if (released) {
      return
    }
    released = true
    waitReleased.resolve()
  }
}

function closeWebDocument(
  port: MessagePort,
  webDocumentId: string,
  terminal = false,
): Promise<void> {
  const { port1: ackPort, port2: ownerAckPort } = new MessageChannel()
  const { promise, resolve } = Promise.withResolvers<void>()
  ackPort.addEventListener('message', () => resolve(), { once: true })
  ackPort.start()
  port.postMessage(
    {
      from: webDocumentId,
      close: true,
      terminal: terminal || undefined,
      closeAckPort: ownerAckPort,
    },
    [ownerAckPort],
  )
  return promise.finally(() => {
    ackPort.close()
    port.close()
  })
}

function encodeStartInfo(): Uint8Array {
  const json = PluginStartInfo.toJsonString({
    instanceId: 'inst1',
    pluginId: 'spacewave-web',
    instanceKey: 'js-goscript',
  })
  return new TextEncoder().encode(btoa(json))
}

function createSpacewaveWebWorker(): Worker {
  return new Worker(
    new URL(
      './workers/goscript-plugin-wrapper.js?s=/b/pd/spacewave-web/plugin.mjs&p=1',
      import.meta.url,
    ),
    {
      type: 'module',
      name: 'plugin/spacewave-web?s=/b/pd/spacewave-web/plugin.mjs&p=1',
    },
  )
}

function recordTrackerCloseReceipt(
  eventLog: string[],
  owner: string,
  webDocumentId: string,
  remainingDocumentCount: number,
): void {
  recordEvent(
    eventLog,
    `tracker-close-receipt owner=${owner} webDocumentId=${webDocumentId} remainingDocumentCount=${remainingDocumentCount}`,
  )
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
    openHostToPluginStream: async (eventLog, label) => {
      if (!runtimePort) {
        throw new Error('runtime port is not connected')
      }
      return await openHostToPluginStream(runtimePort, eventLog, label)
    },
    openInFlightReloadTriggerStream: async (eventLog) => {
      if (!runtimePort) {
        throw new Error('runtime port is not connected')
      }
      return await openInFlightReloadTriggerStream(runtimePort, eventLog)
    },
    openTerminalOrphanStream: async (eventLog) => {
      if (!runtimePort) {
        throw new Error('runtime port is not connected')
      }
      return await openTerminalOrphanStream(runtimePort, eventLog)
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
    void handlePluginToHostStream(channelPacketStream(stream)).then(
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
    if (packet[0] === 51) {
      // Orphan hold stream: keep it open with no response so it stays active on
      // the plugin runtime client until the deliberate terminal teardown fails
      // it. The plugin observes that failure and reports the outcome.
      await new Promise<void>(() => undefined)
    }
    if (packet[0] !== 11) {
      throw new Error(`unexpected plugin-to-host packet ${packet[0]}`)
    }
    const startInfoText = new TextDecoder().decode(packet.slice(1))
    const parsedStartInfo: unknown = JSON.parse(startInfoText)
    if (!isRecord(parsedStartInfo)) {
      throw new Error('plugin-to-host start info is not an object')
    }
    outbound.push(new Uint8Array([12]))
    outbound.end()
    await sinkDone
    return {
      stream: true,
      startInfo:
        parsedStartInfo.instanceId === 'inst1' &&
        parsedStartInfo.pluginId === 'spacewave-web' &&
        parsedStartInfo.instanceKey === 'js-goscript',
    }
  }
  throw new Error('plugin-to-host stream closed before packet')
}

async function openHostToPluginStream(
  runtimePort: MessagePort,
  eventLog?: string[],
  label = 'host-to-plugin',
): Promise<boolean> {
  const channel = new MessageChannel()
  const stream = new ChannelStream('spacewave-web', channel.port1)
  recordEvent(eventLog ?? [], `${label}-open-send`)
  runtimePort.postMessage({ openStream: true }, [channel.port2])
  await stream.waitRemoteOpen
  recordEvent(eventLog ?? [], `${label}-remote-open`)

  const outbound = pushable<Uint8Array>({ objectMode: true })
  const sinkDone = stream.sink(outbound)
  outbound.push(new Uint8Array([21]))

  for await (const packet of stream.source) {
    outbound.end()
    await sinkDone
    recordEvent(eventLog ?? [], `${label}-response ${packet[0]}`)
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
      await beforeOpen()
      outbound.push(new Uint8Array([35]))
      outbound.end()
      await sinkDone
    },
  }
}

// openTerminalOrphanStream arms an active plugin-to-runtime stream through the
// accept-stream bridge, then waits for the plugin's fail-fast outcome after the
// fixture discards the last WebDocument with a terminal close.
async function openTerminalOrphanStream(
  runtimePort: MessagePort,
  eventLog: string[],
): Promise<TerminalOrphanTrigger> {
  const channel = new MessageChannel()
  const stream = new ChannelStream('spacewave-web', channel.port1)
  runtimePort.postMessage({ openStream: true }, [channel.port2])
  await stream.waitRemoteOpen

  const outbound = pushable<Uint8Array>({ objectMode: true })
  const sinkDone = stream.sink(outbound)
  sinkDone.catch(() => undefined)
  outbound.push(new Uint8Array([41]))

  const source = stream.source[Symbol.asyncIterator]()
  const armedResult = await source.next()
  if (armedResult.done) {
    throw new Error('terminal orphan stream closed before armed')
  }
  const armed = armedResult.value[0] === 42
  if (armed) {
    recordEvent(eventLog, 'terminal-orphan-plugin-open-armed')
  }
  return {
    armed,
    waitOutcome: async () => {
      try {
        const outcomeResult = await source.next()
        if (outcomeResult.done) {
          return { failedFast: true, err: 'orphan stream closed' }
        }
        const packet = outcomeResult.value
        if (packet[0] === 44) {
          return {
            failedFast: true,
            err: new TextDecoder().decode(packet.slice(1)),
          }
        }
        if (packet[0] === 45) {
          return { failedFast: false, err: 'host answered orphaned stream' }
        }
        return { failedFast: false, err: `unexpected packet ${packet[0]}` }
      } catch (err) {
        // Tearing down the orphaned client closes this relayed trigger stream
        // with the terminal client-closed error. That rejection is the fast-fail
        // signal: a lingering reroute would keep the stream open until the
        // timeout instead.
        return {
          failedFast: true,
          err: err instanceof Error ? err.message : String(err),
        }
      } finally {
        outbound.end()
      }
    },
  }
}

async function attachWorkerDocument(
  worker: Worker,
  webDocumentId: string,
  detect: WorkerCommsDetectResult,
  eventLog?: string[],
  includeStartInfo = false,
): Promise<AttachedWorkerDocument> {
  const releaseLock = await holdWebDocumentLock(`bldr-doc-${webDocumentId}`)
  recordEvent(
    eventLog ?? [],
    `lock-acquire ${webDocumentId} lock=bldr-doc-${webDocumentId}`,
  )
  const { port1, port2 } = new MessageChannel()
  const runtime = connectWorkerRuntime(port2, webDocumentId)
  const message: Record<string, unknown> = {
    from: webDocumentId,
    initPort: port1,
    workerCommsDetect: detect,
  }
  if (includeStartInfo) {
    message.initData = encodeStartInfo()
  }
  worker.postMessage(message, [port1])
  port2.postMessage({
    from: webDocumentId,
    resumeReady: true,
    runtimeConnected: true,
  })

  return {
    runtime,
    close: async () => {
      try {
        await closeWebDocument(port2, webDocumentId)
      } finally {
        releaseLock()
      }
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

class ControlledServiceWorkerHost implements ServiceWorkerHost {
  private used = false
  private readonly request = Promise.withResolvers<void>()
  private readonly response = Promise.withResolvers<void>()
  public readonly requestStarted = this.request.promise
  public readonly releaseResponse = this.response.resolve

  public constructor(private readonly eventLog: string[]) {}

  public dynamicRelayUsed(): boolean {
    return this.used
  }

  public Fetch(
    request: MessageStream<FetchRequest>,
    _abortSignal?: AbortSignal,
  ): MessageStream<FetchResponse> {
    this.used = true
    const eventLog = this.eventLog
    const requestSignal = this.request
    const responseGate = this.response
    return (async function* () {
      const iterator = request[Symbol.asyncIterator]()
      const first = await iterator.next()
      if (!first.done) {
        const body = first.value.body
        if (body.case === 'requestInfo') {
          recordEvent(eventLog, `dynamic-relay-request ${body.value.url}`)
        }
      }
      requestSignal.resolve()
      await responseGate.promise
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
    timeoutPromise(3000).then(() => false),
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
    timeoutPromise(3000).then(() => false),
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

  const host = new ControlledServiceWorkerHost(eventLog)
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
    requestStarted: host.requestStarted,
    releaseResponse: host.releaseResponse,
    dynamicRelayUsed: () => host.dynamicRelayUsed(),
    close: () => closeWebDocument(port2, serviceWorkerDocumentId),
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
    server.rpcStreamHandler(channelPacketStream(stream)).catch((err: unknown) => {
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
  recordEvent(
    eventLog,
    `lock-acquire ${serviceWorkerDocumentId} lock=bldr-doc-${serviceWorkerDocumentId}`,
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
    const deadline = timeoutPromise(3000).then(() => {
      abort.abort()
      recordEvent(eventLog, 'dynamic-relay-fetch-timeout')
      return false
    })
    const requestStarted = await Promise.race([
      relayDocument.requestStarted.then(() => true),
      deadline,
    ])
    if (!requestStarted) {
      return {
        fetchSuccess: false,
        relayUsed: relayDocument.dynamicRelayUsed(),
      }
    }
    relayDocument.releaseResponse()
    const fetchSuccess = await Promise.race([fetchPromise, deadline])
    return {
      fetchSuccess,
      relayUsed: relayDocument.dynamicRelayUsed(),
    }
  } finally {
    await relayDocument.close()
    releaseRelayLock()
  }
}

function startUnixFSInlineFileThroughProxyFetch(
  eventLog: string[],
): ControlledProxyFetch {
  const host = new ControlledServiceWorkerHost(eventLog)
  const result = (async () => {
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
  })()
  return {
    requestStarted: host.requestStarted,
    releaseResponse: host.releaseResponse,
    result,
  }
}

async function fetchUnixFSInlineFileThroughProxyFetch(
  eventLog: string[],
): Promise<boolean> {
  const fetch = startUnixFSInlineFileThroughProxyFetch(eventLog)
  fetch.releaseResponse()
  return await fetch.result
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
    releaseLock = await holdWebDocumentLock(`bldr-doc-${documentId}`)
    recordEvent(
      eventLog,
      `lock-acquire ${documentId} lock=bldr-doc-${documentId}`,
    )
    mark('start-spacewave-web-worker')
    worker = createSpacewaveWebWorker()

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
    const preFetchStream = await runtime.openHostToPluginStream(
      eventLog,
      'pre-fetch-host-to-plugin-stream',
    )

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
    let pluginHostReplacement = false
    let serviceWorkerRouteTiming = false
    let deliberateWorkerReplacement = false
    let orphanFailedFast = false
    let orphanTerminalNotRerouted = false
    let orphanOutcomeErr = ''
    let orphanElapsedMs = 0

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
      const fetch = startUnixFSInlineFileThroughProxyFetch(eventLog)
      await fetch.requestStarted
      releaseBroadcast = true
      dispatchReleaseGenerationMismatch(eventLog)
      await timeoutPromise(fixtureTimeoutMs)
      throw new Error('release generation mismatch did not reload the page')
    } else if (variant === 'in-flight-reload') {
      mark('fetch-unixfs-inline-in-flight-reload')
      const fetch = startUnixFSInlineFileThroughProxyFetch(eventLog)
      await fetch.requestStarted
      const trigger = await runtime.openInFlightReloadTriggerStream(eventLog)
      if (!trigger.armed) {
        errors.push('inFlightReloadTrigger=false')
      }
      await trigger.release(async () => {
        recordEvent(eventLog, 'in-flight-reload-close-foreground-document')
        await closeWebDocument(port2, documentId)
        recordTrackerCloseReceipt(
          eventLog,
          'fixture-in-flight-reload',
          documentId,
          0,
        )
        recordEvent(
          eventLog,
          `lock-release ${documentId} lock=bldr-doc-${documentId}`,
        )
        releaseLock?.()
        releaseLock = undefined
        zeroDocumentRace = true
      })

      recordEvent(eventLog, 'in-flight-reload-attach-replacement-document')
      replacementDocument = await attachWorkerDocument(
        worker,
        replacementDocumentId,
        detect,
        eventLog,
      )
      const recovered = await replacementDocument.runtime.waitPluginToHost
      inFlightOpenRecovered = recovered.stream && recovered.startInfo
      replacementRoute =
        await replacementDocument.runtime.openHostToPluginStream(
          eventLog,
          'in-flight-reload-replacement-route',
        )
      fetch.releaseResponse()
      reproduced = true
      postFetchStream = replacementRoute
      fetchSuccess = await fetch.result
    } else if (variant === 'plugin-host-replacement') {
      mark('fetch-unixfs-inline-plugin-host-replacement')
      recordEvent(eventLog, 'plugin-host-replacement-fetch-route-start')
      const fetch = startUnixFSInlineFileThroughProxyFetch(eventLog)
      await fetch.requestStarted
      recordEvent(
        eventLog,
        'PluginHost RemoveWebWorker owner=fixture-plugin-host-replacement plugin=spacewave-web',
      )
      await closeWebDocument(port2, documentId)
      recordTrackerCloseReceipt(
        eventLog,
        'PluginHost.RemoveWebWorker',
        documentId,
        0,
      )
      recordEvent(
        eventLog,
        `lock-release ${documentId} lock=bldr-doc-${documentId}`,
      )
      releaseLock?.()
      releaseLock = undefined
      worker?.terminate()
      worker = createSpacewaveWebWorker()
      recordEvent(eventLog, 'plugin-host-replacement-createWebWorker')
      replacementDocument = await attachWorkerDocument(
        worker,
        replacementDocumentId,
        detect,
        eventLog,
        true,
      )
      const recovered = await replacementDocument.runtime.waitPluginToHost
      inFlightOpenRecovered = recovered.stream && recovered.startInfo
      replacementRoute =
        await replacementDocument.runtime.openHostToPluginStream(
          eventLog,
          'plugin-host-replacement-route',
        )
      fetch.releaseResponse()
      pluginHostReplacement = true
      reproduced = true
      postFetchStream = replacementRoute
      fetchSuccess = await fetch.result
    } else if (variant === 'service-worker-fetch-route-timing') {
      mark('fetch-unixfs-inline-service-worker-route-timing')
      recordEvent(eventLog, 'service-worker-fetch-route-start')
      const fetch = startUnixFSInlineFileThroughProxyFetch(eventLog)
      await fetch.requestStarted
      const trigger = await runtime.openInFlightReloadTriggerStream(eventLog)
      if (!trigger.armed) {
        errors.push('inFlightReloadTrigger=false')
      }
      await trigger.release(async () => {
        recordEvent(
          eventLog,
          'service-worker-fetch-route-before-last-document-removal',
        )
        await closeWebDocument(port2, documentId)
        recordTrackerCloseReceipt(
          eventLog,
          'service-worker-fetch-route-timing',
          documentId,
          0,
        )
        recordEvent(
          eventLog,
          `lock-release ${documentId} lock=bldr-doc-${documentId}`,
        )
        releaseLock?.()
        releaseLock = undefined
        zeroDocumentRace = true
      })
      recordEvent(
        eventLog,
        'service-worker-fetch-route-attach-replacement-document',
      )
      replacementDocument = await attachWorkerDocument(
        worker,
        replacementDocumentId,
        detect,
        eventLog,
      )
      const recovered = await replacementDocument.runtime.waitPluginToHost
      inFlightOpenRecovered = recovered.stream && recovered.startInfo
      replacementRoute =
        await replacementDocument.runtime.openHostToPluginStream(
          eventLog,
          'service-worker-route-timing-replacement-route',
        )
      fetch.releaseResponse()
      serviceWorkerRouteTiming = true
      reproduced = true
      postFetchStream = replacementRoute
      fetchSuccess = await fetch.result
    } else if (variant === 'deliberate-worker-replacement') {
      mark('deliberate-worker-replacement')
      recordEvent(eventLog, 'deliberate-worker-replacement-arm')
      const trigger = await runtime.openTerminalOrphanStream(eventLog)
      if (!trigger.armed) {
        errors.push('terminalOrphanArmed=false')
      }

      // The plugin holds an active runtime stream. Deliberately discard the last
      // WebDocument with a terminal close: the orphaned client must fail the
      // stream fast, not wait for a replacement WebDocument that never attaches.
      recordEvent(eventLog, 'deliberate-worker-replacement-terminal-close')
      const closeStartMs = performance.now()
      await closeWebDocument(port2, documentId, true)
      recordTrackerCloseReceipt(
        eventLog,
        'fixture-deliberate-worker-replacement',
        documentId,
        0,
      )
      recordEvent(
        eventLog,
        `lock-release ${documentId} lock=bldr-doc-${documentId}`,
      )
      releaseLock?.()
      releaseLock = undefined

      const outcome = await Promise.race([
        trigger.waitOutcome(),
        timeoutPromise(fixtureTimeoutMs).then(
          (): TerminalOrphanOutcome => ({
            failedFast: false,
            err: 'terminal-orphan-timeout',
          }),
        ),
      ])
      orphanElapsedMs = performance.now() - closeStartMs
      orphanOutcomeErr = outcome.err
      // Fast fail: the orphaned stream settled well within the CI teardown-latency
      // window, not after the multi-second hang the discriminator removes.
      orphanFailedFast = outcome.failedFast && orphanElapsedMs < 2000
      // A terminal client close, not a relay-rerouted retry that would keep the
      // client alive waiting for a replacement WebDocument.
      orphanTerminalNotRerouted =
        outcome.failedFast &&
        !outcome.err.includes('relay-rerouted') &&
        (outcome.err.includes('normal-close') || outcome.err.includes('closed'))
      recordEvent(
        eventLog,
        `deliberate-worker-replacement-outcome failedFast=${outcome.failedFast} elapsedMs=${orphanElapsedMs.toFixed(1)} err=${orphanOutcomeErr}`,
      )
      if (!orphanFailedFast) {
        errors.push(
          `orphanFailedFast=false elapsedMs=${orphanElapsedMs.toFixed(1)}`,
        )
      }
      if (!orphanTerminalNotRerouted) {
        errors.push(`orphanTerminalNotRerouted=false err=${orphanOutcomeErr}`)
      }
      deliberateWorkerReplacement = true
      reproduced = true
      // This variant exercises orphan teardown, not the fetch/post-fetch route.
      fetchSuccess = true
      postFetchStream = true
    } else {
      mark('fetch-unixfs-inline')
      fetchSuccess = await fetchUnixFSInlineFile()
    }

    if (!postFetchStream) {
      mark('post-fetch-host-to-plugin-stream')
      postFetchStream = await runtime.openHostToPluginStream(
        eventLog,
        'post-fetch-host-to-plugin-stream',
      )
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
            `pluginHostReplacement=${pluginHostReplacement}`,
            `failureReason=${failureReason ?? ''}`,
            `serviceWorkerRouteTiming=${serviceWorkerRouteTiming}`,
            `deliberateWorkerReplacement=${deliberateWorkerReplacement}`,
            `orphanFailedFast=${orphanFailedFast}`,
            `orphanTerminalNotRerouted=${orphanTerminalNotRerouted}`,
            `orphanElapsedMs=${orphanElapsedMs.toFixed(1)}`,
            `orphanOutcomeErr=${orphanOutcomeErr}`,
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
      pluginHostReplacement,
      serviceWorkerRouteTiming,
      deliberateWorkerReplacement,
      orphanFailedFast,
      orphanTerminalNotRerouted,
      orphanOutcomeErr,
      orphanElapsedMs,
      failureReason,
      eventLog: readPersistedEvents(),
    }
  } catch (err) {
    window.__results = failureResult(eventLog, `error: ${String(err)}`)
  } finally {
    if (!(variant === 'release-generation' && runCount === 1)) {
      await replacementDocument?.close()
      releaseLock?.()
      worker?.terminate()
      log.textContent = 'DONE'
    }
  }
}

run()
