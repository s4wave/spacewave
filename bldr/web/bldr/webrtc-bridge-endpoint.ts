// WebRTC bridge endpoint for the main thread (WebDocument side).
//
// Receives signaling commands from the worker's ProxyRTCPeerConnection via a
// bridge MessagePort, drives real RTCPeerConnection instances, and proxies
// RTCDataChannel data/events back to the worker.

import type {
  BridgeCommand,
  BridgeResponse,
  BridgeEvent,
  DataChannelBridgePayload,
  DataChannelSnapshot,
  PeerConnectionSnapshot,
} from '../runtime/wasm/webrtc-bridge.js'

type IceCandidateStats = RTCStats & {
  address?: string
  port?: number
  protocol?: string
  candidateType?: string
}

type TrackedDataChannel = {
  pcId: string
  dc: RTCDataChannel
}

type PionCompatibleErrorEvent = Event & { error?: { message?: string } }

function isIceCandidateStats(stat: RTCStats): stat is IceCandidateStats {
  return stat.type === 'local-candidate' || stat.type === 'remote-candidate'
}

function copyArrayBufferView(data: ArrayBufferView): ArrayBuffer {
  const copy = new Uint8Array(data.byteLength)
  copy.set(new Uint8Array(data.buffer, data.byteOffset, data.byteLength))
  return copy.buffer
}

function normalizeBridgePayload(
  data: string | ArrayBuffer | ArrayBufferView,
): DataChannelBridgePayload {
  if (typeof data === 'string') return data
  if (data instanceof ArrayBuffer) return data
  return copyArrayBufferView(data)
}

function sendRtcDataChannelPayload(
  dc: RTCDataChannel,
  data: string | ArrayBuffer | ArrayBufferView,
) {
  const payload = normalizeBridgePayload(data)
  if (typeof payload === 'string') {
    dc.send(payload)
    return
  }
  dc.send(payload)
}

function dataChannelErrorMessage(ev: Event): string {
  const error = (ev as PionCompatibleErrorEvent).error
  return error?.message ?? 'RTCDataChannel error'
}

async function toBridgePayload(
  data: unknown,
): Promise<{ data: DataChannelBridgePayload; transfer: Transferable[] }> {
  if (typeof data === 'string') return { data, transfer: [] }
  if (data instanceof ArrayBuffer) return { data, transfer: [data] }
  if (ArrayBuffer.isView(data)) {
    const copied = copyArrayBufferView(data)
    return { data: copied, transfer: [copied] }
  }
  if (typeof Blob !== 'undefined' && data instanceof Blob) {
    const copied = await data.arrayBuffer()
    return { data: copied, transfer: [copied] }
  }
  return { data: String(data), transfer: [] }
}

// WebRTCBridgeEndpoint handles a single bridge MessagePort connection from a
// worker. It manages real RTCPeerConnection and RTCDataChannel objects on the
// main thread and forwards browser-shaped events to the worker.
export class WebRTCBridgeEndpoint {
  private port: MessagePort
  private pcs = new Map<string, RTCPeerConnection>()
  private dataChannels = new Map<string, TrackedDataChannel>()
  private closed = false
  private nextDataChannelNumber = 1
  // Pending stats promises keyed by pcId, awaited before close to avoid
  // collecting stats on an already-closed PC.
  private pendingStats = new Map<string, Promise<void>>()

  constructor(port: MessagePort) {
    this.port = port
    this.port.onmessage = (e: MessageEvent<BridgeCommand>) =>
      this.handleCommand(e.data)
    this.port.onmessageerror = () => this.close()
    this.port.start()
  }

  private getSnapshot(pc: RTCPeerConnection): PeerConnectionSnapshot {
    return {
      connectionState: pc.connectionState,
      signalingState: pc.signalingState,
      iceConnectionState: pc.iceConnectionState,
      iceGatheringState: pc.iceGatheringState,
      localDescription: pc.localDescription
        ? { type: pc.localDescription.type, sdp: pc.localDescription.sdp }
        : null,
      remoteDescription: pc.remoteDescription
        ? { type: pc.remoteDescription.type, sdp: pc.remoteDescription.sdp }
        : null,
    }
  }

  private getDataChannelSnapshot(dc: RTCDataChannel): DataChannelSnapshot {
    return {
      label: dc.label,
      ordered: dc.ordered,
      protocol: dc.protocol,
      negotiated: dc.negotiated,
      id: dc.id,
      maxPacketLifeTime: dc.maxPacketLifeTime,
      maxRetransmits: dc.maxRetransmits,
      readyState: dc.readyState,
      bufferedAmount: dc.bufferedAmount,
      bufferedAmountLowThreshold: dc.bufferedAmountLowThreshold,
    }
  }

  private getPcSnapshot(pcId: string): PeerConnectionSnapshot | undefined {
    const pc = this.pcs.get(pcId)
    return pc ? this.getSnapshot(pc) : undefined
  }

  private safePostMessage(
    message: BridgeResponse | BridgeEvent,
    transfer: Transferable[] = [],
  ) {
    if (this.closed) return false
    try {
      this.port.postMessage(message, transfer)
      return true
    } catch (err) {
      console.warn(
        `WebRTCBridgeEndpoint: failed to post bridge message: ${err instanceof Error ? err.message : String(err)}`,
      )
      return false
    }
  }

  private async logIceFailureStats(pc: RTCPeerConnection, pcId: string) {
    try {
      const report = await pc.getStats()
      const local = new Map<string, IceCandidateStats>()
      const remote = new Map<string, IceCandidateStats>()
      const pairs: string[] = []

      report.forEach((stat) => {
        if (isIceCandidateStats(stat)) {
          if (stat.type === 'local-candidate') local.set(stat.id, stat)
          if (stat.type === 'remote-candidate') remote.set(stat.id, stat)
        }
      })

      report.forEach((stat) => {
        if (stat.type !== 'candidate-pair') return
        const pair = stat as RTCIceCandidatePairStats
        const localCandidate = pair.localCandidateId
          ? local.get(pair.localCandidateId)
          : undefined
        const remoteCandidate = pair.remoteCandidateId
          ? remote.get(pair.remoteCandidateId)
          : undefined

        pairs.push(
          JSON.stringify({
            id: pair.id,
            state: pair.state,
            nominated: pair.nominated,
            bytesSent: pair.bytesSent,
            bytesReceived: pair.bytesReceived,
            currentRoundTripTime: pair.currentRoundTripTime,
            totalRoundTripTime: pair.totalRoundTripTime,
            requestsReceived: pair.requestsReceived,
            requestsSent: pair.requestsSent,
            responsesReceived: pair.responsesReceived,
            responsesSent: pair.responsesSent,
            local: localCandidate
              ? {
                  id: localCandidate.id,
                  address: localCandidate.address,
                  port: localCandidate.port,
                  protocol: localCandidate.protocol,
                  candidateType: localCandidate.candidateType,
                }
              : undefined,
            remote: remoteCandidate
              ? {
                  id: remoteCandidate.id,
                  address: remoteCandidate.address,
                  port: remoteCandidate.port,
                  protocol: remoteCandidate.protocol,
                  candidateType: remoteCandidate.candidateType,
                }
              : undefined,
          }),
        )
      })

      console.log(
        `WebRTCBridgeEndpoint: ice failure stats pc=${pcId} pairs=${pairs.join(' | ')}`,
      )
    } catch (err) {
      console.log(
        `WebRTCBridgeEndpoint: ice failure stats pc=${pcId} error=${err instanceof Error ? err.message : String(err)}`,
      )
    }
  }

  private async logIceStats(
    pc: RTCPeerConnection,
    pcId: string,
    label: string,
  ) {
    try {
      const report = await pc.getStats()
      const locals: string[] = []
      const remotes: string[] = []
      const pairs: string[] = []
      report.forEach((stat) => {
        if (isIceCandidateStats(stat) && stat.type === 'local-candidate') {
          locals.push(
            `${stat.candidateType} ${stat.protocol ?? '?'}://${stat.address}:${stat.port}`,
          )
        }
        if (isIceCandidateStats(stat) && stat.type === 'remote-candidate') {
          remotes.push(
            `${stat.candidateType} ${stat.protocol ?? '?'}://${stat.address}:${stat.port}`,
          )
        }
        if (stat.type === 'candidate-pair') {
          const p = stat as RTCIceCandidatePairStats
          pairs.push(`${p.state} nominated=${p.nominated}`)
        }
      })
      console.log(
        `WebRTCBridgeEndpoint: ice stats [${label}] pc=${pcId} local=[${locals.join(', ')}] remote=[${remotes.join(', ')}] pairs=[${pairs.join(', ')}]`,
      )
    } catch {
      // Stats are diagnostic only.
    }
  }

  private wireEvents(pc: RTCPeerConnection, pcId: string) {
    pc.onicecandidate = (e) => {
      if (this.closed) return
      const event: BridgeEvent = {
        type: 'event:icecandidate',
        pcId,
        candidate: e.candidate
          ? {
              candidate: e.candidate.candidate,
              sdpMid: e.candidate.sdpMid ?? undefined,
              sdpMLineIndex: e.candidate.sdpMLineIndex ?? undefined,
              usernameFragment: e.candidate.usernameFragment ?? undefined,
              protocol: e.candidate.protocol ?? undefined,
              address: e.candidate.address ?? undefined,
              port: e.candidate.port ?? undefined,
              type: e.candidate.type ?? undefined,
              foundation: e.candidate.foundation ?? undefined,
              component: e.candidate.component ?? undefined,
              priority: e.candidate.priority ?? undefined,
              relatedAddress: e.candidate.relatedAddress ?? undefined,
              relatedPort: e.candidate.relatedPort ?? undefined,
              tcpType: e.candidate.tcpType ?? undefined,
            }
          : undefined,
        snapshot: this.getSnapshot(pc),
      }
      this.safePostMessage(event)
    }

    pc.onconnectionstatechange = () => {
      if (this.closed) return
      if (pc.connectionState === 'failed') {
        const p = this.logIceFailureStats(pc, pcId)
        this.pendingStats.set(pcId, p)
        p.finally(() => this.pendingStats.delete(pcId))
      }
      this.safePostMessage({
        type: 'event:connectionstatechange',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.onsignalingstatechange = () => {
      if (this.closed) return
      this.safePostMessage({
        type: 'event:signalingstatechange',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.oniceconnectionstatechange = () => {
      if (this.closed) return
      if (pc.iceConnectionState === 'checking') {
        void this.logIceStats(pc, pcId, 'checking')
      }
      this.safePostMessage({
        type: 'event:iceconnectionstatechange',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.onicegatheringstatechange = () => {
      if (this.closed) return
      this.safePostMessage({
        type: 'event:icegatheringstatechange',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.onicecandidateerror = (e) => {
      if (this.closed) return
      console.log(
        `WebRTCBridgeEndpoint: event icecandidateerror pc=${pcId} address=${e.address ?? ''} port=${e.port ?? 0} url=${e.url ?? ''} errorCode=${e.errorCode} errorText=${e.errorText}`,
      )
    }

    pc.onnegotiationneeded = () => {
      if (this.closed) return
      this.safePostMessage({
        type: 'event:negotiationneeded',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.ondatachannel = (e) => {
      if (this.closed) return
      const dc = e.channel
      const dcId = this.registerDataChannel(pcId, dc)
      this.safePostMessage({
        type: 'event:datachannel',
        pcId,
        dcId,
        label: dc.label,
        channel: this.getDataChannelSnapshot(dc),
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }
  }

  private registerDataChannel(pcId: string, dc: RTCDataChannel): string {
    dc.binaryType = 'arraybuffer'
    const dcId = `${pcId}-dc-${this.nextDataChannelNumber++}`
    this.dataChannels.set(dcId, { pcId, dc })
    this.wireDataChannel(pcId, dcId, dc)
    return dcId
  }

  private wireDataChannel(pcId: string, dcId: string, dc: RTCDataChannel) {
    dc.onopen = () => {
      if (this.closed || !this.dataChannels.has(dcId)) return
      this.safePostMessage({
        type: 'event:dcopen',
        pcId,
        dcId,
        channel: this.getDataChannelSnapshot(dc),
        snapshot: this.getPcSnapshot(pcId),
      } satisfies BridgeEvent)
    }

    dc.onmessage = (e) => {
      if (this.closed || !this.dataChannels.has(dcId)) return
      void this.postDataChannelMessage(pcId, dcId, dc, e.data)
    }

    dc.onclose = () => {
      if (this.closed || !this.dataChannels.has(dcId)) return
      this.dataChannels.delete(dcId)
      this.safePostMessage({
        type: 'event:dcclose',
        pcId,
        dcId,
        channel: {
          ...this.getDataChannelSnapshot(dc),
          readyState: 'closed',
        },
        snapshot: this.getPcSnapshot(pcId),
      } satisfies BridgeEvent)
    }

    dc.onerror = (e) => {
      if (this.closed || !this.dataChannels.has(dcId)) return
      this.safePostMessage({
        type: 'event:dcerror',
        pcId,
        dcId,
        error: dataChannelErrorMessage(e),
        channel: this.getDataChannelSnapshot(dc),
        snapshot: this.getPcSnapshot(pcId),
      } satisfies BridgeEvent)
    }

    ;(
      dc as RTCDataChannel & { onclosing: ((ev: Event) => void) | null }
    ).onclosing = () => {
      if (this.closed || !this.dataChannels.has(dcId)) return
      this.safePostMessage({
        type: 'event:dcclosing',
        pcId,
        dcId,
        channel: this.getDataChannelSnapshot(dc),
        snapshot: this.getPcSnapshot(pcId),
      } satisfies BridgeEvent)
    }

    dc.onbufferedamountlow = () => {
      if (this.closed || !this.dataChannels.has(dcId)) return
      this.safePostMessage({
        type: 'event:dcbufferedamountlow',
        pcId,
        dcId,
        channel: this.getDataChannelSnapshot(dc),
        snapshot: this.getPcSnapshot(pcId),
      } satisfies BridgeEvent)
    }
  }

  private async postDataChannelMessage(
    pcId: string,
    dcId: string,
    dc: RTCDataChannel,
    data: unknown,
  ) {
    try {
      const bridgePayload = await toBridgePayload(data)
      this.safePostMessage(
        {
          type: 'event:dcmessage',
          pcId,
          dcId,
          data: bridgePayload.data,
          channel: this.getDataChannelSnapshot(dc),
          snapshot: this.getPcSnapshot(pcId),
        } satisfies BridgeEvent,
        bridgePayload.transfer,
      )
    } catch (err) {
      this.safePostMessage({
        type: 'event:dcerror',
        pcId,
        dcId,
        error: err instanceof Error ? err.message : String(err),
        channel: this.getDataChannelSnapshot(dc),
        snapshot: this.getPcSnapshot(pcId),
      } satisfies BridgeEvent)
    }
  }

  private releaseDataChannel(dcId: string) {
    const tracked = this.dataChannels.get(dcId)
    if (!tracked) return
    this.dataChannels.delete(dcId)
    const { pcId, dc } = tracked
    const channel = {
      ...this.getDataChannelSnapshot(dc),
      readyState: 'closed' as RTCDataChannelState,
    }
    try {
      if (dc.readyState !== 'closed') dc.close()
    } catch {
      // The channel is already unusable; the worker still needs closure.
    }
    this.safePostMessage({
      type: 'event:dcclose',
      pcId,
      dcId,
      channel,
      snapshot: this.getPcSnapshot(pcId),
    } satisfies BridgeEvent)
  }

  private releaseDataChannelsForPc(pcId: string) {
    for (const [dcId, tracked] of Array.from(this.dataChannels)) {
      if (tracked.pcId === pcId) this.releaseDataChannel(dcId)
    }
  }

  private handleDataChannelCommand(cmd: BridgeCommand): boolean {
    if (!cmd.type.startsWith('dc:')) return false

    const dcId = cmd.dcId
    const tracked = dcId ? this.dataChannels.get(dcId) : undefined
    if (!dcId || !tracked) {
      this.safePostMessage({
        type: 'event:dcerror',
        pcId: cmd.pcId,
        dcId,
        error: 'unknown dcId: ' + dcId,
      } satisfies BridgeEvent)
      return true
    }

    try {
      switch (cmd.type) {
        case 'dc:send':
          if (cmd.data === undefined) {
            throw new Error('dc:send missing data')
          }
          sendRtcDataChannelPayload(tracked.dc, cmd.data)
          break
        case 'dc:close':
          this.releaseDataChannel(dcId)
          break
        case 'dc:setBufferedAmountLowThreshold':
          tracked.dc.bufferedAmountLowThreshold =
            cmd.bufferedAmountLowThreshold ?? 0
          break
        default:
          throw new Error('unknown datachannel command: ' + cmd.type)
      }
    } catch (err) {
      this.safePostMessage({
        type: 'event:dcerror',
        pcId: tracked.pcId,
        dcId,
        error: err instanceof Error ? err.message : String(err),
        channel: this.getDataChannelSnapshot(tracked.dc),
        snapshot: this.getPcSnapshot(tracked.pcId),
      } satisfies BridgeEvent)
    }
    return true
  }

  private async handleCommand(cmd: BridgeCommand) {
    if (this.closed) return

    try {
      if (this.handleDataChannelCommand(cmd)) return

      if (cmd.type === 'createPC') {
        const pcId = 'pc-' + Math.random().toString(36).slice(2, 10)
        const safeConfig: RTCConfiguration = {
          bundlePolicy: cmd.config?.bundlePolicy,
          iceTransportPolicy: cmd.config?.iceTransportPolicy,
        }
        const pc = new RTCPeerConnection(safeConfig)
        this.pcs.set(pcId, pc)
        this.wireEvents(pc, pcId)
        this.safePostMessage({
          type: 'createPC',
          cmdId: cmd.cmdId!,
          pcId,
          snapshot: this.getSnapshot(pc),
        } satisfies BridgeResponse)
        return
      }

      const pc = cmd.pcId ? this.pcs.get(cmd.pcId) : undefined
      if (!pc && cmd.type !== 'close') {
        this.safePostMessage({
          type: cmd.type,
          cmdId: cmd.cmdId!,
          error: 'unknown pcId: ' + cmd.pcId,
        } satisfies BridgeResponse)
        return
      }

      let response: BridgeResponse

      switch (cmd.type) {
        case 'createOffer': {
          const offer = await pc!.createOffer(
            cmd.options as RTCOfferOptions | undefined,
          )
          response = {
            type: 'createOffer',
            cmdId: cmd.cmdId!,
            pcId: cmd.pcId,
            sdp: { type: offer.type, sdp: offer.sdp },
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'createAnswer': {
          const answer = await pc!.createAnswer(
            cmd.options as RTCAnswerOptions | undefined,
          )
          response = {
            type: 'createAnswer',
            cmdId: cmd.cmdId!,
            pcId: cmd.pcId,
            sdp: { type: answer.type, sdp: answer.sdp },
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'setLocalDescription': {
          await pc!.setLocalDescription(cmd.sdp)
          response = {
            type: 'setLocalDescription',
            cmdId: cmd.cmdId!,
            pcId: cmd.pcId,
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'setRemoteDescription': {
          await pc!.setRemoteDescription(cmd.sdp as RTCSessionDescriptionInit)
          response = {
            type: 'setRemoteDescription',
            cmdId: cmd.cmdId!,
            pcId: cmd.pcId,
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'addIceCandidate': {
          await pc!.addIceCandidate(cmd.candidate)
          response = {
            type: 'addIceCandidate',
            cmdId: cmd.cmdId!,
            pcId: cmd.pcId,
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'createDataChannel': {
          const dc = pc!.createDataChannel(
            cmd.label!,
            cmd.options as RTCDataChannelInit | undefined,
          )
          const dcId = this.registerDataChannel(cmd.pcId!, dc)
          response = {
            type: 'createDataChannel',
            cmdId: cmd.cmdId!,
            pcId: cmd.pcId,
            dcId,
            channel: this.getDataChannelSnapshot(dc),
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'close': {
          const statsP = this.pendingStats.get(cmd.pcId!)
          if (statsP) await statsP
          if (pc) {
            this.releaseDataChannelsForPc(cmd.pcId!)
            pc.close()
            this.pcs.delete(cmd.pcId!)
          }
          response = {
            type: 'close',
            cmdId: cmd.cmdId!,
            pcId: cmd.pcId,
          }
          break
        }
        default:
          response = {
            type: cmd.type,
            cmdId: cmd.cmdId!,
            error: 'unknown command: ' + cmd.type,
          }
      }

      this.safePostMessage(response)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      if (cmd.cmdId != null) {
        this.safePostMessage({
          type: cmd.type,
          cmdId: cmd.cmdId,
          error: message,
        } satisfies BridgeResponse)
        return
      }
      this.safePostMessage({
        type: 'event:dcerror',
        pcId: cmd.pcId,
        dcId: cmd.dcId,
        error: message,
      } satisfies BridgeEvent)
    }
  }

  // close tears down all PCs/channels and closes the bridge port.
  close() {
    if (this.closed) return

    for (const dcId of Array.from(this.dataChannels.keys())) {
      this.releaseDataChannel(dcId)
    }
    for (const [, pc] of this.pcs) {
      pc.close()
    }
    this.pcs.clear()
    this.pendingStats.clear()
    this.safePostMessage({
      type: 'event:bridgeclose',
      error: 'WebRTC bridge endpoint closed',
    } satisfies BridgeEvent)
    this.closed = true
    this.port.close()
  }
}
