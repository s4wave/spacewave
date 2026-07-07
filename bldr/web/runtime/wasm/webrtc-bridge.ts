// WebRTC bridge shim for worker-hosted Go WASM.
//
// RTCPeerConnection is unavailable in DedicatedWorker contexts. This shim
// provides a ProxyRTCPeerConnection that forwards signaling commands to the
// main thread via a bridge MessagePort. RTCDataChannel byte flow is proxied
// over that same port so QUIC-over-DataChannel does not depend on browser
// support for transferring RTCDataChannel objects to workers.

export type DataChannelBridgePayload = string | ArrayBuffer

// Bridge command sent from worker to main thread.
export interface BridgeCommand {
  type: string
  cmdId?: number
  pcId?: string
  dcId?: string
  config?: RTCConfiguration
  sdp?: RTCSessionDescriptionInit
  candidate?: RTCIceCandidateInit
  label?: string
  options?: RTCDataChannelInit | RTCOfferOptions | RTCAnswerOptions
  data?: DataChannelBridgePayload
  bufferedAmountLowThreshold?: number
}

// Snapshot of RTCDataChannel state, cached in worker-side wrappers.
export interface DataChannelSnapshot {
  label: string
  ordered: boolean
  protocol: string
  negotiated: boolean
  id: number | null
  maxPacketLifeTime: number | null
  maxRetransmits: number | null
  readyState: RTCDataChannelState
  bufferedAmount: number
  bufferedAmountLowThreshold: number
}

// Bridge response sent from main thread to worker.
export interface BridgeResponse {
  type: string
  cmdId: number
  pcId?: string
  dcId?: string
  error?: string
  sdp?: RTCSessionDescriptionInit
  channel?: DataChannelSnapshot
  snapshot?: PeerConnectionSnapshot
}

// Bridge event sent from main thread to worker (no cmdId).
// The candidate field carries full RTCIceCandidate properties (protocol,
// address, port, type, foundation, etc.) in addition to the standard
// RTCIceCandidateInit fields so that pion/webrtc's valueToICECandidate
// takes the standard code path.
export interface BridgeEvent {
  type: string
  pcId?: string
  dcId?: string
  candidate?: RTCIceCandidateInit & Record<string, unknown>
  label?: string
  snapshot?: PeerConnectionSnapshot
  channel?: DataChannelSnapshot
  data?: DataChannelBridgePayload
  error?: string
}

// Snapshot of RTCPeerConnection state, cached in the proxy.
export interface PeerConnectionSnapshot {
  connectionState: string
  signalingState: string
  iceConnectionState: string
  iceGatheringState: string
  localDescription: RTCSessionDescriptionInit | null
  remoteDescription: RTCSessionDescriptionInit | null
}

type BridgeMessage = BridgeResponse | BridgeEvent
type IceCandidateLike = RTCIceCandidateInit & Record<string, unknown>
type QueuedDataChannelPayload = string | ArrayBuffer

type PionErrorEvent = Event & { error?: Error }

const textEncoder = new TextEncoder()

function payloadByteLength(data: QueuedDataChannelPayload): number {
  if (typeof data === 'string') return textEncoder.encode(data).byteLength
  return data.byteLength
}

function copyArrayBufferView(data: ArrayBufferView): ArrayBuffer {
  const copy = new Uint8Array(data.byteLength)
  copy.set(new Uint8Array(data.buffer, data.byteOffset, data.byteLength))
  return copy.buffer
}

function copyDataChannelPayload(
  data: string | ArrayBuffer | ArrayBufferView,
): QueuedDataChannelPayload {
  if (typeof data === 'string') return data
  if (data instanceof ArrayBuffer) return data.slice(0)
  return copyArrayBufferView(data)
}

function payloadTransfers(data: QueuedDataChannelPayload): Transferable[] {
  if (typeof data === 'string') return []
  return [data]
}

function makeEvent(type: string): Event {
  if (typeof Event === 'function') return new Event(type)
  return { type } as Event
}

function makeMessageEvent(data: DataChannelBridgePayload): MessageEvent {
  if (typeof MessageEvent === 'function') {
    return new MessageEvent('message', { data })
  }
  return { type: 'message', data } as MessageEvent
}

function makeErrorEvent(message: string): PionErrorEvent {
  return { type: 'error', error: new Error(message) } as PionErrorEvent
}

function channelOptionsFromSnapshot(
  snapshot: DataChannelSnapshot,
): RTCDataChannelInit {
  return {
    ordered: snapshot.ordered,
    protocol: snapshot.protocol,
    negotiated: snapshot.negotiated,
    id: snapshot.id ?? undefined,
    maxPacketLifeTime: snapshot.maxPacketLifeTime ?? undefined,
    maxRetransmits: snapshot.maxRetransmits ?? undefined,
  }
}

// DataChannelWrapper is the synchronous RTCDataChannel-shaped object returned
// to pion/webrtc in the worker. It queues sends until the main-thread channel is
// open, then proxies one bridge message per application packet to preserve QUIC
// packet boundaries.
export class DataChannelWrapper {
  label: string
  ordered: boolean
  protocol: string
  negotiated: boolean
  id: number | null
  maxPacketLifeTime: number | null
  maxRetransmits: number | null
  binaryType: BinaryType = 'arraybuffer'

  private _readyState: RTCDataChannelState = 'connecting'
  private _bufferedAmount = 0
  private _bufferedAmountLowThreshold = 0
  private _closed = false
  private _dcId: string | null = null
  private _pcId: string | null = null
  private _openFired = false
  private _closeFired = false
  private thresholdDirty = false
  private sendQueue: QueuedDataChannelPayload[] = []

  private _onopen: ((ev: Event) => void) | null = null
  private _onmessage: ((ev: MessageEvent) => void) | null = null
  private _onclose: ((ev: Event) => void) | null = null
  private _onerror: ((ev: PionErrorEvent) => void) | null = null
  private _onbufferedamountlow: ((ev: Event) => void) | null = null
  private _onclosing: ((ev: Event) => void) | null = null

  constructor(
    label: string,
    options?: RTCDataChannelInit,
    private dispatcher?: BridgeDispatcher,
  ) {
    this.label = label
    this.ordered = options?.ordered ?? true
    this.protocol = options?.protocol ?? ''
    this.negotiated = options?.negotiated ?? false
    this.id = options?.id ?? null
    this.maxPacketLifeTime = options?.maxPacketLifeTime ?? null
    this.maxRetransmits = options?.maxRetransmits ?? null
  }

  get onopen() {
    return this._onopen
  }
  set onopen(v: ((ev: Event) => void) | null) {
    this._onopen = v
  }

  get onmessage() {
    return this._onmessage
  }
  set onmessage(v: ((ev: MessageEvent) => void) | null) {
    this._onmessage = v
  }

  get onclose() {
    return this._onclose
  }
  set onclose(v: ((ev: Event) => void) | null) {
    this._onclose = v
  }

  get onerror(): ((ev: PionErrorEvent) => void) | null {
    return this._onerror
  }
  set onerror(v: ((ev: PionErrorEvent) => void) | null) {
    this._onerror = v
  }

  get onbufferedamountlow() {
    return this._onbufferedamountlow
  }
  set onbufferedamountlow(v: ((ev: Event) => void) | null) {
    this._onbufferedamountlow = v
  }

  get onclosing() {
    return this._onclosing
  }
  set onclosing(v: ((ev: Event) => void) | null) {
    this._onclosing = v
  }

  get readyState(): RTCDataChannelState {
    return this._readyState
  }

  get bufferedAmount(): number {
    return this._bufferedAmount
  }

  get bufferedAmountLowThreshold(): number {
    return this._bufferedAmountLowThreshold
  }

  set bufferedAmountLowThreshold(v: number) {
    this._bufferedAmountLowThreshold = v
    this.thresholdDirty = true
    this.syncBufferedAmountLowThreshold()
  }

  get maxRetransmitTime(): number | null {
    return this.maxPacketLifeTime
  }

  get bridgeId(): string | null {
    return this._dcId
  }

  send(data: string | ArrayBuffer | ArrayBufferView): void {
    if (this._closed || this._readyState === 'closing') {
      throw new Error('RTCDataChannel is closed')
    }
    const payload = copyDataChannelPayload(data)
    if (!this._dcId || this._readyState !== 'open') {
      this.queuePayload(payload)
      return
    }
    this.postPayload(payload)
  }

  close(): void {
    if (this._closed) return
    this._closed = true
    this.sendQueue.length = 0
    this._bufferedAmount = 0

    if (this._dcId && this.dispatcher) {
      this._readyState = 'closing'
      try {
        this.dispatcher.postRaw({
          type: 'dc:close',
          dcId: this._dcId,
          pcId: this._pcId ?? undefined,
        })
      } catch {
        this.finishClose()
      }
      return
    }

    this.finishClose()
  }

  attachBridge(
    pcId: string,
    dcId: string,
    snapshot: DataChannelSnapshot,
    dispatcher: BridgeDispatcher,
  ) {
    this._pcId = pcId
    this._dcId = dcId
    this.dispatcher = dispatcher

    if (this._closed) {
      try {
        this.dispatcher.postRaw({ type: 'dc:close', pcId, dcId })
      } catch {
        // The local side is already closed; bridge death is handled elsewhere.
      }
      return
    }

    const desiredThreshold = this._bufferedAmountLowThreshold
    const thresholdDirty = this.thresholdDirty
    this.applySnapshot(snapshot)
    if (thresholdDirty) {
      this._bufferedAmountLowThreshold = desiredThreshold
      this.thresholdDirty = true
      this.syncBufferedAmountLowThreshold()
    }
    if (this._readyState === 'open') this.markOpen()
    if (this._readyState === 'closed') this.finishClose()
  }

  handleBridgeEvent(event: BridgeEvent) {
    if (event.channel) this.applySnapshot(event.channel)

    switch (event.type) {
      case 'event:dcopen':
        this.markOpen()
        break
      case 'event:dcmessage':
        if (event.data !== undefined) {
          this._onmessage?.(makeMessageEvent(event.data))
        }
        break
      case 'event:dcclosing':
        if (!this._closed) {
          this._readyState = 'closing'
          this._onclosing?.(makeEvent('closing'))
        }
        break
      case 'event:dcerror':
        this._onerror?.(makeErrorEvent(event.error ?? 'RTCDataChannel error'))
        break
      case 'event:dcbufferedamountlow':
        if (this._readyState === 'open') {
          this._onbufferedamountlow?.(makeEvent('bufferedamountlow'))
        }
        break
      case 'event:dcclose':
        this._closed = true
        this.finishClose()
        break
    }
  }

  bridgeDied(message = 'WebRTC bridge closed') {
    if (this._closeFired) return
    this._closed = true
    this._readyState = 'closed'
    this.sendQueue.length = 0
    this._bufferedAmount = 0
    this._onerror?.(makeErrorEvent(message))
    this.fireClose()
  }

  private queuePayload(payload: QueuedDataChannelPayload) {
    this.sendQueue.push(payload)
    this._bufferedAmount += payloadByteLength(payload)
  }

  private postPayload(payload: QueuedDataChannelPayload) {
    if (!this.dispatcher || !this._dcId) {
      this.queuePayload(payload)
      return
    }
    try {
      this.dispatcher.postRaw(
        {
          type: 'dc:send',
          pcId: this._pcId ?? undefined,
          dcId: this._dcId,
          data: payload,
        },
        payloadTransfers(payload),
      )
    } catch (err) {
      this._onerror?.(
        makeErrorEvent(err instanceof Error ? err.message : String(err)),
      )
    }
  }

  private flushQueuedSends() {
    if (!this._dcId || this._readyState !== 'open') return
    const queued = this.sendQueue.splice(0)
    this._bufferedAmount = 0
    for (const payload of queued) {
      this.postPayload(payload)
    }
  }

  private applySnapshot(snapshot: DataChannelSnapshot) {
    this.label = snapshot.label
    this.ordered = snapshot.ordered
    this.protocol = snapshot.protocol
    this.negotiated = snapshot.negotiated
    this.id = snapshot.id
    this.maxPacketLifeTime = snapshot.maxPacketLifeTime
    this.maxRetransmits = snapshot.maxRetransmits
    this._readyState = snapshot.readyState
    this._bufferedAmount = Math.max(
      this._bufferedAmount,
      snapshot.bufferedAmount,
    )
    this._bufferedAmountLowThreshold = snapshot.bufferedAmountLowThreshold
  }

  private markOpen() {
    if (this._closed) return
    this._readyState = 'open'
    if (!this._openFired) {
      this._openFired = true
      this._onopen?.(makeEvent('open'))
    }
    this.flushQueuedSends()
  }

  private finishClose() {
    this._closed = true
    this._readyState = 'closed'
    this.sendQueue.length = 0
    this._bufferedAmount = 0
    this.fireClose()
  }

  private fireClose() {
    if (this._closeFired) return
    this._closeFired = true
    this._onclose?.(makeEvent('close'))
  }

  private syncBufferedAmountLowThreshold() {
    if (!this.dispatcher || !this._dcId) return
    try {
      this.dispatcher.postRaw({
        type: 'dc:setBufferedAmountLowThreshold',
        pcId: this._pcId ?? undefined,
        dcId: this._dcId,
        bufferedAmountLowThreshold: this._bufferedAmountLowThreshold,
      })
      this.thresholdDirty = false
    } catch (err) {
      this._onerror?.(
        makeErrorEvent(err instanceof Error ? err.message : String(err)),
      )
    }
  }
}

// Stub implementations for supporting WebRTC objects. pion-webrtc accesses
// pc.sctp, RTCDtlsTransport, RTCIceTransport, and transceiver properties via
// syscall/js .Get(). These stubs return stable browser-shaped objects.

class StubRTCIceTransport {
  getSelectedCandidatePair(): RTCIceCandidatePair | null {
    return null
  }

  get state(): RTCIceTransportState {
    return 'connected'
  }
}

class StubRTCDtlsTransport {
  readonly iceTransport = new StubRTCIceTransport()

  getRemoteCertificates(): ArrayBuffer[] {
    return []
  }

  get state(): RTCDtlsTransportState {
    return 'connected'
  }
}

class StubRTCSctpTransport {
  readonly transport = new StubRTCDtlsTransport()

  get maxMessageSize(): number {
    return 65536
  }

  get state(): RTCSctpTransportState {
    return 'connected'
  }
}

class StubRTCRtpSender {
  get track(): MediaStreamTrack | null {
    return null
  }

  get dtmf(): RTCDTMFSender | null {
    return null
  }
}

class StubRTCRtpReceiver {
  get track(): MediaStreamTrack | null {
    return null
  }
}

class StubRTCRtpTransceiver {
  readonly sender = new StubRTCRtpSender()
  readonly receiver = new StubRTCRtpReceiver()
  direction: RTCRtpTransceiverDirection

  constructor(init?: RTCRtpTransceiverInit) {
    this.direction = init?.direction ?? 'sendrecv'
  }

  get currentDirection(): RTCRtpTransceiverDirection | null {
    return this.direction
  }

  get mid(): string | null {
    return null
  }

  stop(): void {
    this.direction = 'inactive'
  }

  setCodecPreferences(_codecs: unknown[]): void {
    // Browser workers never carry media in this bridge path.
  }
}

// BridgeDispatcher manages the single bridge MessagePort shared by all
// ProxyRTCPeerConnection instances in this worker. It owns the port's message
// handler, allocates command IDs, and routes responses/events.
class BridgeDispatcher {
  private nextCmdId = 1
  private pending = new Map<
    number,
    { resolve: (v: BridgeResponse) => void; reject: (e: Error) => void }
  >()
  private pcs = new Map<string, ProxyRTCPeerConnection>()
  private failed = false

  constructor(private port: MessagePort) {
    this.port.onmessage = (e: MessageEvent<BridgeMessage>) =>
      this.handleMessage(e.data)
    this.port.onmessageerror = () =>
      this.handleBridgeClosed(new Error('WebRTC bridge message error'))
    this.port.start()
  }

  allocCmdId(): number {
    return this.nextCmdId++
  }

  sendCommand(
    type: string,
    payload: Partial<BridgeCommand> = {},
    transfer: Transferable[] = [],
  ): Promise<BridgeResponse> {
    if (this.failed) {
      return Promise.reject(new Error('WebRTC bridge closed'))
    }
    const { promise, resolve, reject } = Promise.withResolvers<BridgeResponse>()
    const cmdId = this.nextCmdId++
    this.pending.set(cmdId, { resolve, reject })
    const msg: BridgeCommand = { type, cmdId, ...payload }
    try {
      this.port.postMessage(msg, transfer)
    } catch (err) {
      this.pending.delete(cmdId)
      const error = err instanceof Error ? err : new Error(String(err))
      reject(error)
      this.handleBridgeClosed(error)
    }
    return promise
  }

  postRaw(cmd: BridgeCommand, transfer: Transferable[] = []) {
    if (this.failed) throw new Error('WebRTC bridge closed')
    try {
      this.port.postMessage(cmd, transfer)
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err))
      this.handleBridgeClosed(error)
      throw error
    }
  }

  registerPending(
    cmdId: number,
    handler: {
      resolve: (v: BridgeResponse) => void
      reject: (e: Error) => void
    },
  ) {
    this.pending.set(cmdId, handler)
  }

  unregisterPending(cmdId: number) {
    this.pending.delete(cmdId)
  }

  registerPC(pcId: string, pc: ProxyRTCPeerConnection) {
    this.pcs.set(pcId, pc)
  }

  unregisterPC(pcId: string) {
    this.pcs.delete(pcId)
  }

  private handleMessage(data: BridgeMessage) {
    if ('cmdId' in data && data.cmdId != null) {
      const entry = this.pending.get(data.cmdId)
      if (entry) {
        this.pending.delete(data.cmdId)
        if (data.error) {
          entry.reject(new Error(data.error))
        } else {
          entry.resolve(data)
        }
      }
      return
    }

    if (data.type === 'event:bridgeclose') {
      this.handleBridgeClosed(
        new Error((data as BridgeEvent).error ?? 'WebRTC bridge closed'),
      )
      return
    }

    if (data.type?.startsWith('event:') && (data as BridgeEvent).pcId) {
      const event = data as BridgeEvent
      const pc = this.pcs.get(event.pcId!)
      if (pc) pc.handleBridgeEvent(event)
    }
  }

  private handleBridgeClosed(err: Error) {
    if (this.failed) return
    this.failed = true
    const pending = Array.from(this.pending.values())
    this.pending.clear()
    for (const entry of pending) entry.reject(err)

    const pcs = Array.from(this.pcs.values())
    this.pcs.clear()
    for (const pc of pcs) pc.handleBridgeClosed(err)
  }
}

// ProxyRTCPeerConnection proxies RTCPeerConnection operations to the main
// thread via a bridge MessagePort. Signaling methods send commands and await
// responses. Data channels are worker-side wrappers backed by bridge messages.
export class ProxyRTCPeerConnection {
  static async generateCertificate(
    _keygenAlgorithm: AlgorithmIdentifier,
  ): Promise<RTCCertificate> {
    throw new Error('RTCPeerConnection.generateCertificate is unavailable')
  }

  private dispatcher: BridgeDispatcher
  private pcId: string | null = null
  private pcIdPromise: Promise<string>
  private _closed = false
  private pendingDCs = new Map<number, DataChannelWrapper>()
  private dataChannels = new Map<string, DataChannelWrapper>()
  private readonly sctpTransport = new StubRTCSctpTransport()
  private readonly transceivers: StubRTCRtpTransceiver[] = []

  onicecandidate:
    | ((ev: { candidate: IceCandidateLike | null }) => void)
    | null = null
  ondatachannel: ((ev: { channel: RTCDataChannel }) => void) | null = null
  onsignalingstatechange: (() => void) | null = null
  oniceconnectionstatechange: (() => void) | null = null
  onconnectionstatechange: (() => void) | null = null
  onicegatheringstatechange: (() => void) | null = null
  onnegotiationneeded: (() => void) | null = null

  private _snapshot: PeerConnectionSnapshot = {
    connectionState: 'new',
    signalingState: 'stable',
    iceConnectionState: 'new',
    iceGatheringState: 'new',
    localDescription: null,
    remoteDescription: null,
  }

  constructor(config?: RTCConfiguration) {
    const dispatcher = getDispatcher()
    if (!dispatcher) throw new Error('WebRTC bridge port not available')
    this.dispatcher = dispatcher

    this.pcIdPromise = this.dispatcher
      .sendCommand('createPC', { config })
      .then((r) => {
        this.pcId = r.pcId!
        if (this._closed) {
          this.dispatcher
            .sendCommand('close', { pcId: this.pcId })
            .catch(() => {})
          return this.pcId
        }
        if (r.snapshot) this.updateSnapshot(r.snapshot)
        this.dispatcher.registerPC(this.pcId, this)
        return this.pcId
      })
      .catch((err) => {
        this.teardownPendingDCs(
          err instanceof Error ? err.message : String(err),
        )
        throw err
      })
  }

  private async sendCommand(
    type: string,
    payload: Partial<BridgeCommand> = {},
  ): Promise<BridgeResponse> {
    const pcId = await this.pcIdPromise
    const r = await this.dispatcher.sendCommand(type, { pcId, ...payload })
    if (r.snapshot) this.updateSnapshot(r.snapshot)
    return r
  }

  private updateSnapshot(snapshot: PeerConnectionSnapshot) {
    this._snapshot = snapshot
  }

  handleBridgeEvent(event: BridgeEvent) {
    if (event.snapshot) this.updateSnapshot(event.snapshot)
    this.dispatchEvent(event)
  }

  handleBridgeClosed(err: Error) {
    this._closed = true
    this._snapshot.connectionState = 'closed'
    this._snapshot.signalingState = 'closed'
    this.teardownPendingDCs(err.message)
    this.teardownDataChannels(err.message)
  }

  private dispatchEvent(event: BridgeEvent) {
    const eventType = event.type.slice(6)
    switch (eventType) {
      case 'icecandidate':
        if (this.onicecandidate) {
          this.onicecandidate({ candidate: event.candidate ?? null })
        }
        break
      case 'datachannel':
        if (event.dcId && event.channel) {
          const wrapper = new DataChannelWrapper(
            event.channel.label,
            channelOptionsFromSnapshot(event.channel),
            this.dispatcher,
          )
          wrapper.attachBridge(
            event.pcId!,
            event.dcId,
            event.channel,
            this.dispatcher,
          )
          this.dataChannels.set(event.dcId, wrapper)
          this.ondatachannel?.({
            channel: wrapper as unknown as RTCDataChannel,
          })
        }
        break
      case 'dcopen':
      case 'dcmessage':
      case 'dcclosing':
      case 'dcerror':
      case 'dcbufferedamountlow':
      case 'dcclose':
        if (event.dcId) {
          const wrapper = this.dataChannels.get(event.dcId)
          wrapper?.handleBridgeEvent(event)
          if (eventType === 'dcclose') this.dataChannels.delete(event.dcId)
        }
        break
      case 'signalingstatechange':
        this.onsignalingstatechange?.()
        break
      case 'iceconnectionstatechange':
        this.oniceconnectionstatechange?.()
        break
      case 'connectionstatechange':
        this.onconnectionstatechange?.()
        break
      case 'icegatheringstatechange':
        this.onicegatheringstatechange?.()
        break
      case 'negotiationneeded':
        this.onnegotiationneeded?.()
        break
    }
  }

  async createOffer(
    options?: RTCOfferOptions,
  ): Promise<RTCSessionDescriptionInit> {
    const r = await this.sendCommand('createOffer', { options })
    return r.sdp!
  }

  async createAnswer(
    options?: RTCAnswerOptions,
  ): Promise<RTCSessionDescriptionInit> {
    const r = await this.sendCommand('createAnswer', { options })
    return r.sdp!
  }

  async setLocalDescription(desc?: RTCSessionDescriptionInit): Promise<void> {
    await this.sendCommand('setLocalDescription', { sdp: desc })
  }

  async setRemoteDescription(desc: RTCSessionDescriptionInit): Promise<void> {
    await this.sendCommand('setRemoteDescription', { sdp: desc })
  }

  async addIceCandidate(candidate?: RTCIceCandidateInit): Promise<void> {
    await this.sendCommand('addIceCandidate', { candidate })
  }

  createDataChannel(
    label: string,
    options?: RTCDataChannelInit,
  ): DataChannelWrapper {
    const wrapper = new DataChannelWrapper(label, options, this.dispatcher)
    const cmdId = this.dispatcher.allocCmdId()

    this.pendingDCs.set(cmdId, wrapper)
    this.dispatcher.registerPending(cmdId, {
      resolve: (r: BridgeResponse) => {
        if (r.snapshot) this.updateSnapshot(r.snapshot)
        if (r.dcId && r.channel && r.pcId) {
          wrapper.attachBridge(r.pcId, r.dcId, r.channel, this.dispatcher)
          this.dataChannels.set(r.dcId, wrapper)
        } else {
          wrapper.bridgeDied('createDataChannel response missing channel')
        }
        this.pendingDCs.delete(cmdId)
      },
      reject: (err: Error) => {
        this.pendingDCs.delete(cmdId)
        wrapper.bridgeDied(err.message)
      },
    })

    this.pcIdPromise
      .then((pcId) => {
        if (this._closed) {
          if (this.pendingDCs.delete(cmdId)) {
            this.dispatcher.unregisterPending(cmdId)
            wrapper.bridgeDied('RTCPeerConnection closed')
          }
          return
        }
        this.dispatcher.postRaw({
          type: 'createDataChannel',
          cmdId,
          pcId,
          label,
          options,
        })
      })
      .catch((err) => {
        if (this.pendingDCs.delete(cmdId)) {
          this.dispatcher.unregisterPending(cmdId)
          wrapper.bridgeDied(err instanceof Error ? err.message : String(err))
        }
      })

    return wrapper
  }

  get connectionState(): RTCPeerConnectionState {
    return this._snapshot.connectionState as RTCPeerConnectionState
  }

  get signalingState(): RTCSignalingState {
    return this._snapshot.signalingState as RTCSignalingState
  }

  get iceConnectionState(): RTCIceConnectionState {
    return this._snapshot.iceConnectionState as RTCIceConnectionState
  }

  get iceGatheringState(): RTCIceGatheringState {
    return this._snapshot.iceGatheringState as RTCIceGatheringState
  }

  get localDescription(): { type: string; sdp: string } | null {
    return this._snapshot.localDescription
      ? {
          type: this._snapshot.localDescription.type,
          sdp: this._snapshot.localDescription.sdp ?? '',
        }
      : null
  }

  get remoteDescription(): { type: string; sdp: string } | null {
    return this._snapshot.remoteDescription
      ? {
          type: this._snapshot.remoteDescription.type,
          sdp: this._snapshot.remoteDescription.sdp ?? '',
        }
      : null
  }

  get currentLocalDescription(): { type: string; sdp: string } | null {
    return this.localDescription
  }

  get pendingLocalDescription(): { type: string; sdp: string } | null {
    return null
  }

  get currentRemoteDescription(): { type: string; sdp: string } | null {
    return this.remoteDescription
  }

  get pendingRemoteDescription(): { type: string; sdp: string } | null {
    return null
  }

  get canTrickleIceCandidates(): boolean | null {
    return true
  }

  get sctp(): StubRTCSctpTransport | null {
    if (this._snapshot.connectionState === 'closed') return null
    return this.sctpTransport
  }

  getConfiguration(): RTCConfiguration {
    return {}
  }

  setConfiguration(_config: RTCConfiguration): void {
    // The real main-thread PeerConnection received the initial configuration.
  }

  addTransceiver(
    _trackOrKind: string | MediaStreamTrack,
    init?: RTCRtpTransceiverInit,
  ): StubRTCRtpTransceiver {
    const transceiver = new StubRTCRtpTransceiver(init)
    this.transceivers.push(transceiver)
    return transceiver
  }

  getTransceivers(): StubRTCRtpTransceiver[] {
    return [...this.transceivers]
  }

  setIdentityProvider(_provider: string): void {
    // No-op: browser identity providers are not available in worker bridge mode.
  }

  private teardownPendingDCs(message = 'WebRTC bridge closed') {
    for (const [cmdId, wrapper] of this.pendingDCs) {
      this.dispatcher.unregisterPending(cmdId)
      wrapper.bridgeDied(message)
    }
    this.pendingDCs.clear()
  }

  private teardownDataChannels(message = 'WebRTC bridge closed') {
    for (const wrapper of this.dataChannels.values()) {
      wrapper.bridgeDied(message)
    }
    this.dataChannels.clear()
  }

  close() {
    if (this._closed) return
    this._closed = true

    if (this.pcId) {
      this.dispatcher.sendCommand('close', { pcId: this.pcId }).catch(() => {})
      this.dispatcher.unregisterPC(this.pcId)
    }
    this._snapshot.connectionState = 'closed'
    this._snapshot.signalingState = 'closed'
    this.teardownPendingDCs('RTCPeerConnection closed')
    this.teardownDataChannels('RTCPeerConnection closed')
  }
}

interface WebRtcBridgeGlobals {
  __bldrWebRtcBridgePort?: MessagePort | null
  __bldrWebRtcBridgeDispatcher?: BridgeDispatcher | null
}

function getWebRtcBridgeGlobals(): WebRtcBridgeGlobals {
  return globalThis as typeof globalThis & WebRtcBridgeGlobals
}

// setBridgePort sets the bridge MessagePort for WebRTC proxying.
// Creates a BridgeDispatcher to manage the port.
export function setBridgePort(port: MessagePort) {
  const globals = getWebRtcBridgeGlobals()
  globals.__bldrWebRtcBridgePort = port
  globals.__bldrWebRtcBridgeDispatcher = new BridgeDispatcher(port)
}

// getBridgePort returns the current bridge MessagePort, or null.
export function getBridgePort(): MessagePort | null {
  return getWebRtcBridgeGlobals().__bldrWebRtcBridgePort ?? null
}

// getDispatcher returns the BridgeDispatcher, or null if no bridge port is set.
function getDispatcher(): BridgeDispatcher | null {
  return getWebRtcBridgeGlobals().__bldrWebRtcBridgeDispatcher ?? null
}

// installWebRTCShim installs ProxyRTCPeerConnection as the global
// RTCPeerConnection. Call after setBridgePort.
export function installWebRTCShim() {
  const globals = globalThis as typeof globalThis & {
    RTCPeerConnection?: typeof RTCPeerConnection
    window?: typeof globalThis & {
      RTCPeerConnection?: typeof RTCPeerConnection
    }
  }

  globals.RTCPeerConnection =
    ProxyRTCPeerConnection as unknown as typeof RTCPeerConnection

  if (globals.window) {
    globals.window.RTCPeerConnection =
      ProxyRTCPeerConnection as unknown as typeof RTCPeerConnection
  }
}
