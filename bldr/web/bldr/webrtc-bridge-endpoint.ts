// WebRTC bridge endpoint for the main thread (WebDocument side).
//
// Receives signaling commands from the worker's ProxyRTCPeerConnection via
// a bridge MessagePort, drives real RTCPeerConnection instances, and
// transfers RTCDataChannels back to the worker.
//
// Only the trusted document shell supplies ICE servers. When configured with
// a same-origin credential endpoint, the endpoint fetches short-lived
// credentials from it, refreshes them before expiry, and applies refreshed
// servers to live peer connections with setConfiguration. Worker-provided
// servers remain ignored.

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

function isIceCandidateStats(stat: RTCStats): stat is IceCandidateStats {
  return stat.type === 'local-candidate' || stat.type === 'remote-candidate'
}

function toTransferable(dc: RTCDataChannel): Transferable {
  return dc
}

// Credential response returned by the trusted same-origin ICE credential
// endpoint. iceServers carries short-lived TURN/STUN servers and expiresAt is
// the epoch-ms expiry of the credential set.
interface IceCredentialResponse {
  iceServers: RTCIceServer[]
  expiresAt: number
}

// refreshMarginMs refreshes the credential this long before its expiry, so a
// temporary API failure still leaves time to retry before the current
// credential expires.
const refreshMarginMs = 60 * 60 * 1000
// minRetryDelayMs / maxRetryDelayMs bound the exponential backoff between
// failed credential fetches while a usable credential is still current.
const minRetryDelayMs = 5_000
const maxRetryDelayMs = 60_000
// allowedIceUrlSchemes are the only URL schemes accepted from the credential
// endpoint and the static trusted list.
const allowedIceUrlSchemes = new Set(['stun', 'stuns', 'turn', 'turns'])

// validateIceServers filters a candidate ICE server list down to servers whose
// URLs use an allowed scheme. Returns undefined when nothing valid remains.
function validateIceServers(servers: unknown): RTCIceServer[] | undefined {
  if (!Array.isArray(servers)) return undefined
  const valid: RTCIceServer[] = []
  for (const server of servers) {
    if (typeof server !== 'object' || server === null) continue
    const candidate = server as Record<string, unknown>
    const urls = candidate.urls
    if (typeof urls !== 'string' && !Array.isArray(urls)) continue
    const urlList = typeof urls === 'string' ? [urls] : urls
    if (!urlList.every(isAllowedIceURL)) continue
    if (
      candidate.username !== undefined &&
      typeof candidate.username !== 'string'
    ) {
      continue
    }
    if (
      candidate.credential !== undefined &&
      typeof candidate.credential !== 'string'
    ) {
      continue
    }
    if (
      candidate.credentialType !== undefined &&
      candidate.credentialType !== 'password'
    ) {
      continue
    }
    valid.push({
      urls: [...urlList],
      ...(candidate.username === undefined
        ? {}
        : { username: candidate.username }),
      ...(candidate.credential === undefined
        ? {}
        : { credential: candidate.credential }),
      ...(candidate.credentialType === undefined
        ? {}
        : { credentialType: candidate.credentialType }),
    })
  }
  return valid.length > 0 ? valid : undefined
}

function isAllowedIceURL(url: unknown): url is string {
  if (typeof url !== 'string') return false
  try {
    return allowedIceUrlSchemes.has(new URL(url).protocol.replace(':', ''))
  } catch {
    return false
  }
}

// parseCredentialResponse validates a same-origin credential endpoint
// response body. Returns undefined when the shape or schemes are invalid.
function parseCredentialResponse(
  body: unknown,
): IceCredentialResponse | undefined {
  if (typeof body !== 'object' || body === null) return undefined
  const { iceServers, expiresAt } = body as Record<string, unknown>
  if (
    typeof expiresAt !== 'number' ||
    !Number.isFinite(expiresAt) ||
    !(expiresAt > Date.now())
  ) {
    return undefined
  }
  const validServers = validateIceServers(iceServers)
  if (!validServers) return undefined
  return { iceServers: validServers, expiresAt }
}

// WebRTCBridgeEndpoint handles a single bridge MessagePort connection from
// a worker. It manages real RTCPeerConnection instances on the main thread
// and forwards events and DC transfers back to the worker.
export class WebRTCBridgeEndpoint {
  private port: MessagePort
  private pcs = new Map<string, RTCPeerConnection>()
  // iceServers is the current trusted list. It starts from the static dist
  // list and is replaced by credentials fetched from the credential endpoint.
  private iceServers: RTCIceServer[]
  // credentialEndpoint is the optional trusted same-origin endpoint that
  // returns { iceServers, expiresAt }. Empty when unset.
  private readonly credentialEndpoint: string
  // credentialFetch shares one in-flight credential request across concurrent
  // peer creations. Resolves to the current list when no fetch is needed.
  private credentialFetch?: Promise<RTCIceServer[]>
  // refreshTimer schedules the next credential refresh before expiry.
  private refreshTimer?: ReturnType<typeof setTimeout>
  // credentialAbort cancels a request when the bridge closes.
  private credentialAbort?: AbortController
  // refreshFailureDelayMs is the current backoff delay after a failed fetch.
  private refreshFailureDelayMs = minRetryDelayMs
  private pendingIceCandidates = new Map<
    string,
    Array<{ cmd: BridgeCommand }>
  >()
  private closed = false
  // Pending stats promises keyed by pcId, awaited before close to avoid
  // collecting stats on an already-closed PC.
  private pendingStats = new Map<string, Promise<void>>()

  constructor(
    port: MessagePort,
    iceServers: RTCIceServer[] = [],
    credentialEndpoint = '',
  ) {
    this.port = port
    this.iceServers = iceServers.map((server) => ({
      ...server,
      urls: Array.isArray(server.urls) ? [...server.urls] : server.urls,
    }))
    this.credentialEndpoint = this.resolveCredentialEndpoint(credentialEndpoint)
    this.port.onmessage = (e: MessageEvent<BridgeCommand>) =>
      this.handleCommand(e.data)
    this.port.start()
  }

  // resolveCredentialEndpoint returns the endpoint URL only when it is a
  // same-origin HTTP(S) path. Anything else falls back to the static list.
  private resolveCredentialEndpoint(endpoint: string): string {
    if (!endpoint) return ''
    try {
      const url = new URL(endpoint, window.location.origin)
      if (url.origin !== window.location.origin) return ''
      if (url.protocol !== 'http:' && url.protocol !== 'https:') return ''
      return url.toString()
    } catch {
      return ''
    }
  }

  // currentIceServers returns the trusted ICE servers to construct peer
  // connections with. When a credential endpoint is configured, it awaits the
  // current credential fetch; concurrent callers share one request. The static
  // list remains the fallback when the fetch fails or the endpoint is unset.
  private async currentIceServers(): Promise<RTCIceServer[]> {
    if (!this.credentialEndpoint) return this.iceServers
    if (!this.credentialFetch) {
      this.credentialFetch = this.fetchCredentials()
    }
    return this.credentialFetch
  }

  // fetchCredentials fetches and validates one credential response, applies
  // it, schedules the next refresh, and resets the failure backoff. On
  // failure it keeps the current list, logs, and schedules a bounded retry
  // before the current credential expires.
  private async fetchCredentials(): Promise<RTCIceServer[]> {
    const abort = new AbortController()
    this.credentialAbort = abort
    try {
      const response = await fetch(this.credentialEndpoint, {
        cache: 'no-store',
        credentials: 'same-origin',
        signal: abort.signal,
      })
      if (!response.ok) throw new Error(`status ${response.status}`)
      const parsed = parseCredentialResponse(await response.json())
      if (!parsed) throw new Error('invalid credential response')
      if (this.closed) return this.iceServers
      this.iceServers = parsed.iceServers
      this.refreshFailureDelayMs = minRetryDelayMs
      const lifetimeMs = parsed.expiresAt - Date.now()
      const refreshDelayMs =
        lifetimeMs > refreshMarginMs
          ? lifetimeMs - refreshMarginMs
          : Math.max(minRetryDelayMs, lifetimeMs / 2)
      // Never schedule a refresh after the current credentials expire; an
      // expired response is rejected, so retrying sooner is the only recovery.
      this.scheduleRefresh(Math.min(refreshDelayMs, Math.max(0, lifetimeMs)))
      this.applyIceServersToPCs()
      return parsed.iceServers
    } catch (err) {
      if (!this.closed) {
        const message = err instanceof Error ? err.message : String(err)
        console.warn(
          `WebRTCBridgeEndpoint: credential fetch failed endpoint=${this.credentialEndpoint} error=${message}`,
        )
        this.refreshFailureDelayMs = Math.min(
          this.refreshFailureDelayMs * 2,
          maxRetryDelayMs,
        )
        this.scheduleRefresh(this.refreshFailureDelayMs)
      }
      // The static or last-known-good list remains usable.
      return this.iceServers
    } finally {
      if (this.credentialAbort === abort) this.credentialAbort = undefined
    }
  }

  // scheduleRefresh schedules the next credential fetch in delayMs. Only one
  // timer is ever pending; the newest decision wins.
  private scheduleRefresh(delayMs: number) {
    if (this.closed) return
    if (this.refreshTimer) clearTimeout(this.refreshTimer)
    this.refreshTimer = setTimeout(
      () => {
        this.refreshTimer = undefined
        if (this.closed) return
        this.credentialFetch = undefined
        this.credentialFetch = this.fetchCredentials()
      },
      Math.max(0, delayMs),
    )
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
        // The worker may select safe transport policies, but only the trusted
        // document shell can supply STUN or TURN servers.
        const safeConfig: RTCConfiguration = {
          bundlePolicy: cmd.config?.bundlePolicy,
          iceTransportPolicy: cmd.config?.iceTransportPolicy,
          iceServers: await this.currentIceServers(),
        }
        // The endpoint may have closed while the credential fetch was in
        // flight; don't create PCs (or post) afterwards.
        if (this.closed) return
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
          const answer = await pc!.createAnswer(cmd.options)
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
          const pending = this.pendingIceCandidates.get(cmd.pcId!) ?? []
          this.pendingIceCandidates.delete(cmd.pcId!)
          for (const queued of pending) {
            try {
              await pc!.addIceCandidate(queued.cmd.candidate)
              this.port.postMessage({
                type: 'addIceCandidate',
                cmdId: queued.cmd.cmdId,
                pcId: queued.cmd.pcId,
                snapshot: this.getSnapshot(pc!),
              } satisfies BridgeResponse)
            } catch (err) {
              this.port.postMessage({
                type: 'addIceCandidate',
                cmdId: queued.cmd.cmdId,
                error: err instanceof Error ? err.message : String(err),
              } satisfies BridgeResponse)
            }
          }
          response = {
            type: 'setRemoteDescription',
            cmdId: cmd.cmdId,
            pcId: cmd.pcId,
            snapshot: this.getSnapshot(pc!),
          }
          break
        }
        case 'addIceCandidate': {
          if (!pc!.remoteDescription) {
            const pending = this.pendingIceCandidates.get(cmd.pcId!) ?? []
            pending.push({ cmd })
            this.pendingIceCandidates.set(cmd.pcId!, pending)
            return
          }
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
          const pending = this.pendingIceCandidates.get(cmd.pcId!) ?? []
          this.pendingIceCandidates.delete(cmd.pcId!)
          for (const queued of pending) {
            this.port.postMessage({
              type: 'addIceCandidate',
              cmdId: queued.cmd.cmdId,
              error: 'peer connection closed before remote description',
            } satisfies BridgeResponse)
          }
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

  // applyIceServersToPCs updates live peer connections with refreshed
  // trusted ICE servers before their credentials expire. setConfiguration
  // merges, so unspecified fields keep their current values.
  private applyIceServersToPCs() {
    for (const pc of this.pcs.values()) {
      try {
        pc.setConfiguration({ iceServers: this.iceServers })
      } catch (err) {
        console.warn(
          `WebRTCBridgeEndpoint: setConfiguration failed error=${err instanceof Error ? err.message : String(err)}`,
        )
      }
    }
  }

  // close tears down all PCs, cancels the credential refresh, and closes the
  // bridge port.
  close() {
    if (this.closed) return
    this.closed = true
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer)
      this.refreshTimer = undefined
    }
    this.credentialAbort?.abort()
    this.credentialAbort = undefined
    this.credentialFetch = undefined
    this.pendingIceCandidates.clear()
    try {
      this.port.postMessage({
        type: 'event:bridgeclose',
        error: 'WebRTC bridge endpoint closed',
      } satisfies BridgeEvent)
    } catch {
      // The worker side may already be gone.
    }
    for (const [, pc] of this.pcs) {
      pc.close()
    }
    this.pcs.clear()
    this.port.close()
  }
}
