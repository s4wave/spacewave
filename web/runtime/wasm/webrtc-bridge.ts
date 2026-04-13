// WebRTC bridge shim for worker-hosted Go WASM.
//
// RTCPeerConnection is unavailable in DedicatedWorker contexts. This shim
// provides a ProxyRTCPeerConnection that forwards signaling commands to the
// main thread via a bridge MessagePort. Data channels are transferred back
// to the worker as real RTCDataChannel objects.

// Bridge command sent from worker to main thread.
export interface BridgeCommand {
  type: string
  cmdId: number
  pcId?: string
  config?: RTCConfiguration
  sdp?: RTCSessionDescriptionInit
  candidate?: RTCIceCandidateInit
  label?: string
  options?: RTCDataChannelInit
}

// Bridge response sent from main thread to worker.
export interface BridgeResponse {
  type: string
  cmdId: number
  pcId?: string
  error?: string
  sdp?: RTCSessionDescriptionInit
  dc?: RTCDataChannel
  snapshot?: PeerConnectionSnapshot
}

// Bridge event sent from main thread to worker (no cmdId).
// The candidate field carries full RTCIceCandidate properties (protocol,
// address, port, type, foundation, etc.) in addition to the standard
// RTCIceCandidateInit fields so that pion/webrtc's valueToICECandidate
// takes the standard code path.
export interface BridgeEvent {
  type: string
  pcId: string
  candidate?: RTCIceCandidateInit & Record<string, unknown>
  dc?: RTCDataChannel
  label?: string
  snapshot?: PeerConnectionSnapshot
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

// DataChannelWrapper is a synchronous stub that looks like an RTCDataChannel.
// Returned by createDataChannel before the real DC arrives from the main
// thread via transfer. Queues send() calls and stores event handlers until
// the real DC is attached.
export class DataChannelWrapper {
  // Properties known at creation time
  readonly label: string
  readonly ordered: boolean
  readonly protocol: string
  readonly negotiated: boolean
  readonly id: number | null
  readonly maxPacketLifeTime: number | null
  readonly maxRetransmits: number | null

  // Mutable state
  private _readyState: RTCDataChannelState = 'connecting'
  private _bufferedAmount = 0
  private _bufferedAmountLowThreshold = 0
  private _closed = false
  private _realDC: RTCDataChannel | null = null

  // Queued sends before the real DC arrives
  private sendQueue: (string | ArrayBuffer | ArrayBufferView)[] = []

  // Stored event handlers. After attach(), setters forward to the real DC
  // so that pion's Detach() -> OnMessage() wiring takes effect.
  private _onopen: ((ev: Event) => void) | null = null
  private _onmessage: ((ev: MessageEvent) => void) | null = null
  private _onclose: ((ev: Event) => void) | null = null
  private _onerror: ((ev: Event) => void) | null = null
  private _onbufferedamountlow: ((ev: Event) => void) | null = null
  private _onclosing: ((ev: Event) => void) | null = null

  get onopen() {
    return this._realDC ? this._realDC.onopen : this._onopen
  }
  set onopen(v: ((ev: Event) => void) | null) {
    this._onopen = v
    if (this._realDC) this._realDC.onopen = v
  }
  get onmessage() {
    return this._realDC ? this._realDC.onmessage : this._onmessage
  }
  set onmessage(v: ((ev: MessageEvent) => void) | null) {
    this._onmessage = v
    if (this._realDC) this._realDC.onmessage = v
  }
  get onclose() {
    return this._realDC ? this._realDC.onclose : this._onclose
  }
  set onclose(v: ((ev: Event) => void) | null) {
    this._onclose = v
    if (this._realDC) this._realDC.onclose = v
  }
  get onerror(): ((ev: Event) => void) | null {
    return this._onerror
  }
  set onerror(v: ((ev: Event) => void) | null) {
    this._onerror = v
    if (this._realDC) this._realDC.onerror = v as any
  }
  get onbufferedamountlow() {
    return this._realDC ? this._realDC.onbufferedamountlow : this._onbufferedamountlow
  }
  set onbufferedamountlow(v: ((ev: Event) => void) | null) {
    this._onbufferedamountlow = v
    if (this._realDC) this._realDC.onbufferedamountlow = v
  }
  get onclosing() {
    return this._onclosing
  }
  set onclosing(v: ((ev: Event) => void) | null) {
    this._onclosing = v
  }

  constructor(label: string, options?: RTCDataChannelInit) {
    this.label = label
    this.ordered = options?.ordered ?? true
    this.protocol = options?.protocol ?? ''
    this.negotiated = options?.negotiated ?? false
    this.id = options?.id ?? null
    this.maxPacketLifeTime = options?.maxPacketLifeTime ?? null
    this.maxRetransmits = options?.maxRetransmits ?? null
  }

  get readyState(): RTCDataChannelState {
    if (this._realDC) return this._realDC.readyState
    return this._readyState
  }

  get bufferedAmount(): number {
    if (this._realDC) return this._realDC.bufferedAmount
    return this._bufferedAmount
  }

  get bufferedAmountLowThreshold(): number {
    if (this._realDC) return this._realDC.bufferedAmountLowThreshold
    return this._bufferedAmountLowThreshold
  }

  set bufferedAmountLowThreshold(v: number) {
    if (this._realDC) {
      this._realDC.bufferedAmountLowThreshold = v
    }
    this._bufferedAmountLowThreshold = v
  }

  // maxRetransmitTime is a deprecated alias used by older pion-webrtc
  get maxRetransmitTime(): number | null {
    return this.maxPacketLifeTime
  }

  send(data: string | ArrayBuffer | ArrayBufferView): void {
    if (this._realDC) {
      this._realDC.send(data as any)
      return
    }
    if (this._closed) return
    this.sendQueue.push(data)
    // Track buffered amount for pre-attach reads
    if (typeof data === 'string') {
      this._bufferedAmount += data.length
    } else if (data instanceof ArrayBuffer) {
      this._bufferedAmount += data.byteLength
    } else {
      this._bufferedAmount += data.byteLength
    }
  }

  close(): void {
    if (this._realDC) {
      this._realDC.close()
      return
    }
    this._closed = true
    this._readyState = 'closed'
    this.sendQueue.length = 0
    this._bufferedAmount = 0
  }

  // attach swaps in the real transferred DC. Called by ProxyRTCPeerConnection
  // when the main thread transfers the DC back to the worker.
  attach(dc: RTCDataChannel) {
    if (this._closed) {
      dc.close()
      return
    }

    this._realDC = dc

    // Set bufferedAmountLowThreshold before replaying sends
    dc.bufferedAmountLowThreshold = this._bufferedAmountLowThreshold

    // Re-attach stored event handlers to the real DC
    if (this._onopen) dc.onopen = this._onopen
    if (this._onmessage) dc.onmessage = this._onmessage
    if (this._onclose) dc.onclose = this._onclose
    if (this._onerror) dc.onerror = this._onerror as any
    if (this._onbufferedamountlow)
      dc.onbufferedamountlow = this._onbufferedamountlow

    // Replay queued sends
    for (const data of this.sendQueue) {
      dc.send(data as any)
    }
    this.sendQueue.length = 0
    this._bufferedAmount = 0

    // If the real DC is already open, fire the stored onopen handler
    if (dc.readyState === 'open' && this._onopen) {
      this._onopen(new Event('open'))
    }
  }

  // bridgeDied is called when the bridge port closes before the DC arrives.
  bridgeDied() {
    if (this._realDC) return
    this._readyState = 'closed'
    this.sendQueue.length = 0
    this._bufferedAmount = 0
    if (this._onerror) {
      this._onerror(new Event('error'))
    }
    if (this._onclose) {
      this._onclose(new Event('close'))
    }
  }
}

// Stub implementations for supporting WebRTC objects. pion-webrtc accesses
// pc.sctp, RTCDtlsTransport, RTCIceTransport, and transceiver properties
// via syscall/js .Get(). These stubs return plausible defaults.

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
  direction: RTCRtpTransceiverDirection = 'sendrecv'

  get currentDirection(): RTCRtpTransceiverDirection | null {
    return this.direction
  }

  get mid(): string | null {
    return null
  }
}

// BridgeDispatcher manages the single bridge MessagePort shared by all
// ProxyRTCPeerConnection instances in this worker. It owns the port's
// onmessage handler, allocates globally unique command IDs, and routes
// responses by cmdId and events by pcId.
class BridgeDispatcher {
  private nextCmdId = 1
  private pending = new Map<
    number,
    { resolve: (v: BridgeResponse) => void; reject: (e: Error) => void }
  >()
  private pcs = new Map<string, ProxyRTCPeerConnection>()

  constructor(private port: MessagePort) {
    this.port.onmessage = (e: MessageEvent<BridgeMessage>) =>
      this.handleMessage(e.data)
    this.port.start()
  }

  // allocCmdId returns a globally unique command ID.
  allocCmdId(): number {
    return this.nextCmdId++
  }

  // sendCommand sends a command and returns a Promise for the response.
  sendCommand(
    type: string,
    payload: Partial<BridgeCommand> = {},
  ): Promise<BridgeResponse> {
    return new Promise((resolve, reject) => {
      const cmdId = this.nextCmdId++
      this.pending.set(cmdId, { resolve, reject })
      const msg: BridgeCommand = { type, cmdId, ...payload }
      console.log(
        `WebRTC bridge worker: send cmd=${type} cmdId=${cmdId} pcId=${payload.pcId ?? ''}`,
      )
      this.port.postMessage(msg)
    })
  }

  // postRaw posts a pre-built command on the bridge port.
  postRaw(cmd: BridgeCommand) {
    console.log(
      `WebRTC bridge worker: send raw cmd=${cmd.type} cmdId=${cmd.cmdId} pcId=${cmd.pcId ?? ''}`,
    )
    this.port.postMessage(cmd)
  }

  // registerPending registers a custom response handler for a cmdId.
  registerPending(
    cmdId: number,
    handler: {
      resolve: (v: BridgeResponse) => void
      reject: (e: Error) => void
    },
  ) {
    this.pending.set(cmdId, handler)
  }

  // registerPC registers a PC for event routing by pcId.
  registerPC(pcId: string, pc: ProxyRTCPeerConnection) {
    this.pcs.set(pcId, pc)
  }

  // unregisterPC removes a PC from event routing.
  unregisterPC(pcId: string) {
    this.pcs.delete(pcId)
  }

  private handleMessage(data: BridgeMessage) {
    // Command response (has cmdId)
    if ('cmdId' in data && data.cmdId != null) {
      const entry = this.pending.get(data.cmdId)
      if (entry) {
        this.pending.delete(data.cmdId)
        console.log(
          `WebRTC bridge worker: recv response type=${data.type} cmdId=${data.cmdId} pcId=${data.pcId ?? ''} err=${data.error ?? ''}`,
        )
        if (data.error) {
          entry.reject(new Error(data.error))
        } else {
          entry.resolve(data as BridgeResponse)
        }
      }
      return
    }

    // Event (type starts with "event:") - route by pcId
    if (data.type?.startsWith('event:') && (data as BridgeEvent).pcId) {
      const event = data as BridgeEvent
      console.log(
        `WebRTC bridge worker: recv event type=${event.type} pcId=${event.pcId}`,
      )
      const pc = this.pcs.get(event.pcId)
      if (pc) {
        pc.handleBridgeEvent(event)
      }
    }
  }
}

// ProxyRTCPeerConnection proxies RTCPeerConnection operations to the main
// thread via a bridge MessagePort. Signaling methods send commands and await
// responses. Data channels are transferred back as real objects.
//
// Multiple instances share a single BridgeDispatcher (and thus a single
// bridge MessagePort). Command IDs are globally unique to avoid collisions.
export class ProxyRTCPeerConnection {
  private dispatcher: BridgeDispatcher
  private pcId: string | null = null
  private pcIdPromise: Promise<string>
  private _closed = false

  // DC wrappers awaiting the real transferred DC, keyed by cmdId
  private pendingDCs = new Map<number, DataChannelWrapper>()

  // Event handlers
  onicecandidate: ((ev: { candidate: RTCIceCandidate | null }) => void) | null =
    null
  ondatachannel: ((ev: { channel: RTCDataChannel }) => void) | null = null
  onsignalingstatechange: (() => void) | null = null
  oniceconnectionstatechange: (() => void) | null = null
  onconnectionstatechange: (() => void) | null = null
  onicegatheringstatechange: (() => void) | null = null
  onnegotiationneeded: (() => void) | null = null

  // Cached state (updated from snapshots)
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
    if (!dispatcher) {
      throw new Error('WebRTC bridge port not available')
    }
    this.dispatcher = dispatcher

    // Send createPC command and register this PC once pcId arrives.
    this.pcIdPromise = this.dispatcher
      .sendCommand('createPC', { config })
      .then((r) => {
        this.pcId = r.pcId!
        if (this._closed) {
          // close() was called before createPC resolved. Send close to
          // clean up the main-thread PC and skip event registration.
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
        // createPC failed. Tear down any pending DC wrappers that were
        // queued before the failure arrived.
        this.teardownPendingDCs()
        throw err
      })
  }

  // sendCommand sends a command after the pcId is available.
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

  // handleBridgeEvent is called by BridgeDispatcher for events routed by pcId.
  handleBridgeEvent(event: BridgeEvent) {
    if (event.snapshot) this.updateSnapshot(event.snapshot)
    this.dispatchEvent(event)
  }

  private dispatchEvent(event: BridgeEvent) {
    const eventType = event.type.slice(6) // strip "event:"
    switch (eventType) {
      case 'icecandidate':
        if (this.onicecandidate) {
          // Pass the candidate as a plain object (or null for gathering
          // complete). Workers don't have RTCIceCandidate constructor,
          // but pion-webrtc only reads properties via .Get().
          this.onicecandidate({
            candidate: event.candidate ? (event.candidate as any) : null,
          })
        }
        break
      case 'datachannel':
        if (this.ondatachannel && event.dc) {
          this.ondatachannel({ channel: event.dc })
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

  // Signaling methods

  async createOffer(
    options?: RTCOfferOptions,
  ): Promise<RTCSessionDescriptionInit> {
    const r = await this.sendCommand('createOffer', {
      options: options as any,
    })
    return r.sdp!
  }

  async createAnswer(
    options?: RTCAnswerOptions,
  ): Promise<RTCSessionDescriptionInit> {
    const r = await this.sendCommand('createAnswer', {
      options: options as any,
    })
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

  // createDataChannel returns a synchronous DataChannelWrapper. The real DC
  // will arrive via transfer from the main thread and be attached to the
  // wrapper. pion-webrtc calls this synchronously via js.Value.Call().
  //
  // The command is queued until the createPC response provides a pcId.
  createDataChannel(
    label: string,
    options?: RTCDataChannelInit,
  ): DataChannelWrapper {
    console.log(`WebRTC bridge worker: createDataChannel label=${label}`)
    const wrapper = new DataChannelWrapper(label, options)
    const cmdId = this.dispatcher.allocCmdId()

    this.pendingDCs.set(cmdId, wrapper)
    this.dispatcher.registerPending(cmdId, {
      resolve: (r: BridgeResponse) => {
        if (r.snapshot) this.updateSnapshot(r.snapshot)
        if (r.dc) wrapper.attach(r.dc)
        this.pendingDCs.delete(cmdId)
      },
      reject: () => {
        this.pendingDCs.delete(cmdId)
      },
    })

    // Queue the command until pcId is available.
    this.pcIdPromise
      .then((pcId) => {
        if (this._closed) return
        this.dispatcher.postRaw({
          type: 'createDataChannel',
          cmdId,
          pcId,
          label,
          options,
        })
      })
      .catch(() => {
        // createPC failed - clean up this wrapper if not already done.
        if (this.pendingDCs.delete(cmdId)) {
          wrapper.bridgeDied()
        }
      })

    return wrapper
  }

  // Property accessors -- return cached values from snapshot

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

  // Description accessors return plain objects with type/sdp properties.
  // Workers don't have RTCSessionDescription constructor, but pion-webrtc
  // only reads properties via syscall/js .Get("type") and .Get("sdp").

  get localDescription(): { type: string; sdp: string } | null {
    return this._snapshot.localDescription as any
  }

  get remoteDescription(): { type: string; sdp: string } | null {
    return this._snapshot.remoteDescription as any
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

  // Supporting object accessors

  get sctp(): StubRTCSctpTransport | null {
    // Return sctp only after connection is established
    if (
      this._snapshot.connectionState === 'connected' ||
      this._snapshot.connectionState === 'connecting'
    ) {
      return new StubRTCSctpTransport()
    }
    return null
  }

  // pion-webrtc calls these but they're not critical for the bridge
  getConfiguration(): RTCConfiguration {
    return {}
  }

  setConfiguration(_config: RTCConfiguration): void {
    // no-op: config was passed at construction time to the real PC
  }

  addTransceiver(
    _trackOrKind: string | MediaStreamTrack,
    _init?: RTCRtpTransceiverInit,
  ): StubRTCRtpTransceiver {
    return new StubRTCRtpTransceiver()
  }

  getTransceivers(): StubRTCRtpTransceiver[] {
    return []
  }

  setIdentityProvider(_provider: string): void {
    // no-op
  }

  private teardownPendingDCs() {
    for (const wrapper of this.pendingDCs.values()) {
      wrapper.bridgeDied()
    }
    this.pendingDCs.clear()
  }

  // close sends a close command to the main thread and cleans up.
  // If called before createPC resolves, the pcIdPromise handler will
  // send the close command when it completes.
  close() {
    if (this._closed) return
    this._closed = true

    if (this.pcId) {
      this.dispatcher.sendCommand('close', { pcId: this.pcId }).catch(() => {})
      this.dispatcher.unregisterPC(this.pcId)
    }
    this._snapshot.connectionState = 'closed'
    this._snapshot.signalingState = 'closed'
    this.teardownPendingDCs()
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
  const globals = globalThis as any
  if (!globals.window) {
    globals.window = globalThis
  }
  globals.window.RTCPeerConnection = ProxyRTCPeerConnection
  globals.RTCPeerConnection = ProxyRTCPeerConnection
}
