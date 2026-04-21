// WebRTC bridge endpoint for the main thread (WebDocument side).
//
// Receives signaling commands from the worker's ProxyRTCPeerConnection via
// a bridge MessagePort, drives real RTCPeerConnection instances, and
// transfers RTCDataChannels back to the worker.

import type {
  BridgeCommand,
  BridgeResponse,
  BridgeEvent,
  PeerConnectionSnapshot,
} from '../runtime/wasm/webrtc-bridge.js'

type IceCandidateStats = RTCStats & {
  address?: string
  port?: number
  protocol?: string
  candidateType?: string
}

function isIceCandidateStats(
  stat: RTCStats,
): stat is IceCandidateStats {
  return stat.type === 'local-candidate' || stat.type === 'remote-candidate'
}

function toTransferable(dc: RTCDataChannel): Transferable {
  return dc as unknown as Transferable
}

// WebRTCBridgeEndpoint handles a single bridge MessagePort connection from
// a worker. It manages real RTCPeerConnection instances on the main thread
// and forwards events and DC transfers back to the worker.
export class WebRTCBridgeEndpoint {
  private port: MessagePort
  private pcs = new Map<string, RTCPeerConnection>()
  private closed = false
  // Pending stats promises keyed by pcId, awaited before close to avoid
  // collecting stats on an already-closed PC.
  private pendingStats = new Map<string, Promise<void>>()

  constructor(port: MessagePort) {
    this.port = port
    this.port.onmessage = (e: MessageEvent<BridgeCommand>) =>
      this.handleCommand(e.data)
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

  private async logIceFailureStats(pc: RTCPeerConnection, pcId: string) {
    try {
      const report = await pc.getStats()
      const local = new Map<string, IceCandidateStats>()
      const remote = new Map<string, IceCandidateStats>()
      const pairs: string[] = []

      report.forEach((stat) => {
        if (isIceCandidateStats(stat)) {
          if (stat.type === 'local-candidate') {
            local.set(stat.id, stat)
          }
          if (stat.type === 'remote-candidate') {
            remote.set(stat.id, stat)
          }
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

  // logIceStats logs local/remote candidates and candidate pairs for
  // diagnostic purposes at any connection state.
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
      // ignore
    }
  }

  private wireEvents(pc: RTCPeerConnection, pcId: string) {
    pc.onicecandidate = (e) => {
      if (this.closed) return
      // Include full RTCIceCandidate properties (not just RTCIceCandidateInit)
      // so that pion/webrtc's valueToICECandidate takes the standard path
      // instead of the "Firefox/missing-fields" fallback that drops sdpMid.
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
      this.port.postMessage(event)
    }

    pc.onconnectionstatechange = () => {
      if (this.closed) return
      if (pc.connectionState === 'failed') {
        const p = this.logIceFailureStats(pc, pcId)
        this.pendingStats.set(pcId, p)
        p.finally(() => this.pendingStats.delete(pcId))
      }
      this.port.postMessage({
        type: 'event:connectionstatechange',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.onsignalingstatechange = () => {
      if (this.closed) return
      this.port.postMessage({
        type: 'event:signalingstatechange',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.oniceconnectionstatechange = () => {
      if (this.closed) return
      // Log candidate pair stats when entering checking to diagnose ICE
      if (pc.iceConnectionState === 'checking') {
        void this.logIceStats(pc, pcId, 'checking')
      }
      this.port.postMessage({
        type: 'event:iceconnectionstatechange',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.onicegatheringstatechange = () => {
      if (this.closed) return
      this.port.postMessage({
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
      this.port.postMessage({
        type: 'event:negotiationneeded',
        pcId,
        snapshot: this.getSnapshot(pc),
      } satisfies BridgeEvent)
    }

    pc.ondatachannel = (e) => {
      if (this.closed) return
      const dc = e.channel
      const event: BridgeEvent = {
        type: 'event:datachannel',
        pcId,
        dc,
        label: dc.label,
        snapshot: this.getSnapshot(pc),
      }
      this.port.postMessage(event, [toTransferable(dc)])
    }
  }

  private async handleCommand(cmd: BridgeCommand) {
    if (this.closed) return

    try {
      if (cmd.type === 'createPC') {
        const pcId = 'pc-' + Math.random().toString(36).slice(2, 10)
        // Sanitize config: only allow safe fields, strip iceServers to
        // prevent a compromised worker from injecting malicious TURN servers.
        const safeConfig: RTCConfiguration = {
          bundlePolicy: cmd.config?.bundlePolicy,
          iceTransportPolicy: cmd.config?.iceTransportPolicy,
        }
        const pc = new RTCPeerConnection(safeConfig)
        this.pcs.set(pcId, pc)
        this.wireEvents(pc, pcId)
        const response: BridgeResponse = {
          type: 'createPC',
          cmdId: cmd.cmdId,
          pcId,
          snapshot: this.getSnapshot(pc),
        }
        this.port.postMessage(response)
        return
      }

      const pc = cmd.pcId ? this.pcs.get(cmd.pcId) : undefined
      if (!pc && cmd.type !== 'close') {
        this.port.postMessage({
          type: cmd.type,
          cmdId: cmd.cmdId,
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
            cmdId: cmd.cmdId,
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
            cmdId: cmd.cmdId,
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
            cmdId: cmd.cmdId,
            pcId: cmd.pcId,
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'setRemoteDescription': {
          await pc!.setRemoteDescription(cmd.sdp as RTCSessionDescriptionInit)
          response = {
            type: 'setRemoteDescription',
            cmdId: cmd.cmdId,
            pcId: cmd.pcId,
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'addIceCandidate': {
          await pc!.addIceCandidate(cmd.candidate)
          response = {
            type: 'addIceCandidate',
            cmdId: cmd.cmdId,
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
          response = {
            type: 'createDataChannel',
            cmdId: cmd.cmdId,
            pcId: cmd.pcId,
            dc,
            snapshot: this.getSnapshot(pc!),
          }
          // Transfer the DC to the worker before signaling/open
          this.port.postMessage(response, [toTransferable(dc)])
          return // skip normal postMessage below
        }
        case 'close': {
          // Await any in-flight stats collection before closing so that
          // getStats() runs on a live PC rather than a closed one.
          const statsP = this.pendingStats.get(cmd.pcId!)
          if (statsP) await statsP
          if (pc) {
            pc.close()
            this.pcs.delete(cmd.pcId!)
          }
          response = {
            type: 'close',
            cmdId: cmd.cmdId,
            pcId: cmd.pcId,
          }
          break
        }
        default:
          response = {
            type: cmd.type,
            cmdId: cmd.cmdId,
            error: 'unknown command: ' + cmd.type,
          }
      }

      this.port.postMessage(response)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      this.port.postMessage({
        type: cmd.type,
        cmdId: cmd.cmdId,
        error: message,
      } satisfies BridgeResponse)
    }
  }

  // close tears down all PCs and closes the bridge port.
  close() {
    if (this.closed) return
    this.closed = true
    for (const [, pc] of this.pcs) {
      pc.close()
    }
    this.pcs.clear()
    this.port.close()
  }
}
