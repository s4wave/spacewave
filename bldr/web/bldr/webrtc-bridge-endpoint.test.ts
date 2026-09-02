import { afterEach, describe, expect, it, vi } from 'vitest'
import { WebRTCBridgeEndpoint } from './webrtc-bridge-endpoint.js'

class FakePeerConnection {
  static instances: FakePeerConnection[] = []
  readonly config: RTCConfiguration
  remoteDescription: RTCSessionDescription | null = null
  localDescription: RTCSessionDescription | null = null
  connectionState: RTCPeerConnectionState = 'new'
  signalingState: RTCSignalingState = 'stable'
  iceConnectionState: RTCIceConnectionState = 'new'
  iceGatheringState: RTCIceGatheringState = 'new'
  candidates: (RTCIceCandidateInit | null)[] = []

  setConfigurationCalls: RTCConfiguration[] = []

  constructor(config: RTCConfiguration) {
    this.config = config
    FakePeerConnection.instances.push(this)
  }

  setConfiguration(config: RTCConfiguration) {
    this.setConfigurationCalls.push(config)
    ;(this.config as { iceServers?: RTCIceServer[] }).iceServers =
      config.iceServers
  }

  async setRemoteDescription(sdp: RTCSessionDescriptionInit) {
    this.remoteDescription = sdp as RTCSessionDescription
  }
  async addIceCandidate(candidate?: RTCIceCandidateInit | null) {
    if (!this.remoteDescription) throw new Error('remote description is null')
    this.candidates.push(candidate ?? null)
  }
  close() {}
}

type TestBridgeResponse = { cmdId: string; pcId?: string }

function receive(port: MessagePort): Promise<TestBridgeResponse> {
  return new Promise((resolve) => {
    port.onmessage = (event) => resolve(event.data)
    port.start()
  })
}

// receiveN resolves after n messages arrive on the port.
function receiveN(port: MessagePort, n: number): Promise<TestBridgeResponse[]> {
  const responses: TestBridgeResponse[] = []
  return new Promise((resolve) => {
    port.onmessage = (event) => {
      responses.push(event.data)
      if (responses.length >= n) resolve(responses)
    }
    port.start()
  })
}

describe('WebRTCBridgeEndpoint', () => {
  const original = globalThis.RTCPeerConnection
  afterEach(() => {
    globalThis.RTCPeerConnection = original
    FakePeerConnection.instances = []
  })

  it('uses trusted ICE servers and flushes candidates after the remote description', async () => {
    globalThis.RTCPeerConnection =
      FakePeerConnection as unknown as typeof RTCPeerConnection
    const { port1, port2 } = new MessageChannel()
    const endpoint = new WebRTCBridgeEndpoint(port1, [
      { urls: ['stun:trusted.example:3478'] },
    ])

    let next = receive(port2)
    port2.postMessage({
      type: 'createPC',
      cmdId: 'create',
      config: { iceServers: [{ urls: 'turn:worker.example' }] },
    })
    const created = await next
    const pc = FakePeerConnection.instances[0]
    expect(pc.config.iceServers).toEqual([
      { urls: ['stun:trusted.example:3478'] },
    ])

    port2.postMessage({
      type: 'addIceCandidate',
      cmdId: 'ice',
      pcId: created.pcId,
      candidate: { candidate: 'candidate:1' },
    })
    await Promise.resolve()
    expect(pc.candidates).toEqual([])

    const responses: TestBridgeResponse[] = []
    port2.onmessage = (event) => responses.push(event.data)
    port2.postMessage({
      type: 'setRemoteDescription',
      cmdId: 'remote',
      pcId: created.pcId,
      sdp: { type: 'offer', sdp: 'v=0\r\n' },
    })
    await vi.waitFor(() => expect(responses).toHaveLength(2))
    expect(responses.map((response) => response.cmdId)).toEqual([
      'ice',
      'remote',
    ])
    expect(pc.candidates).toEqual([{ candidate: 'candidate:1' }])
    endpoint.close()
  })

  it('rejects queued candidates when the peer connection closes', async () => {
    globalThis.RTCPeerConnection =
      FakePeerConnection as unknown as typeof RTCPeerConnection
    const { port1, port2 } = new MessageChannel()
    const endpoint = new WebRTCBridgeEndpoint(port1)

    let next = receive(port2)
    port2.postMessage({ type: 'createPC', cmdId: 'create', config: {} })
    const created = await next

    const responses: TestBridgeResponse[] = []
    port2.onmessage = (event) => responses.push(event.data)
    port2.postMessage({
      type: 'addIceCandidate',
      cmdId: 'ice',
      pcId: created.pcId,
      candidate: { candidate: 'candidate:1' },
    })
    port2.postMessage({ type: 'close', cmdId: 'close', pcId: created.pcId })

    await vi.waitFor(() => expect(responses).toHaveLength(2))
    expect(responses.map((response) => response.cmdId)).toEqual([
      'ice',
      'close',
    ])
    expect(
      (responses[0] as TestBridgeResponse & { error?: string }).error,
    ).toContain('closed before remote description')
    endpoint.close()
  })

  it('shares one credential fetch across concurrent createPC commands', async () => {
    globalThis.RTCPeerConnection =
      FakePeerConnection as unknown as typeof RTCPeerConnection
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        iceServers: [{ urls: ['turn:turn.cloudflare.com:443?transport=tcp'] }],
        expiresAt: Date.now() + 6 * 60 * 60 * 1000,
      }),
    }))
    const originalFetch = globalThis.fetch
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const { port1, port2 } = new MessageChannel()
    const endpoint = new WebRTCBridgeEndpoint(
      port1,
      [],
      '/api/turn-credentials',
    )

    const responsesPromise = receiveN(port2, 2)
    port2.postMessage({ type: 'createPC', cmdId: 'a', config: {} })
    port2.postMessage({ type: 'createPC', cmdId: 'b', config: {} })
    await responsesPromise

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(FakePeerConnection.instances).toHaveLength(2)
    expect(FakePeerConnection.instances[0].config.iceServers).toEqual([
      { urls: ['turn:turn.cloudflare.com:443?transport=tcp'] },
    ])
    endpoint.close()
    globalThis.fetch = originalFetch
  })

  it('refreshes credentials before expiry and updates live peer connections', async () => {
    vi.useFakeTimers()
    globalThis.RTCPeerConnection =
      FakePeerConnection as unknown as typeof RTCPeerConnection
    const originalFetch = globalThis.fetch
    let calls = 0
    globalThis.fetch = (async () => {
      calls++
      return {
        ok: true,
        json: async () => ({
          iceServers: [
            {
              urls: [`turns:turn${calls}.example:443`],
              username: 'u1',
              credential: 'c1',
            },
          ],
          expiresAt: Date.now() + 6 * 60 * 60 * 1000,
        }),
      }
    }) as unknown as typeof fetch

    const { port1, port2 } = new MessageChannel()
    const endpoint = new WebRTCBridgeEndpoint(
      port1,
      [],
      '/api/turn-credentials',
    )

    let next = receive(port2)
    port2.postMessage({ type: 'createPC', cmdId: 'create', config: {} })
    await next
    const pc = FakePeerConnection.instances[0]
    expect(pc.config.iceServers).toEqual([
      { urls: ['turns:turn1.example:443'], username: 'u1', credential: 'c1' },
    ])

    // Advance to just past the refresh point (expiry - 1h margin).
    await vi.advanceTimersByTimeAsync(5 * 60 * 60 * 1000 + 1)
    expect(calls).toBe(2)
    expect(pc.setConfigurationCalls).toHaveLength(1)
    expect(pc.setConfigurationCalls[0].iceServers).toEqual([
      { urls: ['turns:turn2.example:443'], username: 'u1', credential: 'c1' },
    ])

    endpoint.close()
    vi.useRealTimers()
    globalThis.fetch = originalFetch
  })

  it('rejects invalid and cross-origin credential responses and falls back to the static list', async () => {
    globalThis.RTCPeerConnection =
      FakePeerConnection as unknown as typeof RTCPeerConnection
    const originalFetch = globalThis.fetch

    // A cross-origin absolute endpoint must be rejected outright.
    globalThis.fetch = vi.fn() as unknown as typeof fetch
    const { port1, port2 } = new MessageChannel()
    const crossOrigin = new WebRTCBridgeEndpoint(
      port1,
      [{ urls: ['stun:trusted.example:3478'] }],
      'https://evil.example/api/turn',
    )
    // Cross-origin endpoint is discarded: no fetch ever happens.
    expect(globalThis.fetch).not.toHaveBeenCalled()
    let next = receive(port2)
    port2.postMessage({ type: 'createPC', cmdId: 'x', config: {} })
    await next
    expect(FakePeerConnection.instances[0].config.iceServers).toEqual([
      { urls: ['stun:trusted.example:3478'] },
    ])
    crossOrigin.close()

    // An invalid response body falls back to the static list.
    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({
        iceServers: [{ urls: ['ftp://bad.example:21'] }],
        expiresAt: Date.now() + 3600_000,
      }),
    })) as unknown as typeof fetch
    const { port1: p3, port2: p4 } = new MessageChannel()
    const invalid = new WebRTCBridgeEndpoint(
      p3,
      [{ urls: ['stun:trusted.example:3478'] }],
      '/api/turn-credentials',
    )
    next = receive(p4)
    p4.postMessage({ type: 'createPC', cmdId: 'y', config: {} })
    await next
    expect(FakePeerConnection.instances[1].config.iceServers).toEqual([
      { urls: ['stun:trusted.example:3478'] },
    ])
    invalid.close()

    globalThis.fetch = originalFetch
  })

  it('aborts an in-flight credential request when closed', async () => {
    globalThis.RTCPeerConnection =
      FakePeerConnection as unknown as typeof RTCPeerConnection
    const originalFetch = globalThis.fetch
    let requestSignal: AbortSignal | undefined
    globalThis.fetch = vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>(() => {
          requestSignal = init?.signal ?? undefined
        }),
    ) as unknown as typeof fetch

    const { port1, port2 } = new MessageChannel()
    const endpoint = new WebRTCBridgeEndpoint(
      port1,
      [],
      '/api/turn-credentials',
    )
    port2.postMessage({ type: 'createPC', cmdId: 'create', config: {} })
    await vi.waitFor(() => expect(requestSignal).toBeDefined())

    endpoint.close()

    expect(requestSignal?.aborted).toBe(true)
    globalThis.fetch = originalFetch
  })

  it('cancels the refresh timer and in-flight effects on close', async () => {
    vi.useFakeTimers()
    globalThis.RTCPeerConnection =
      FakePeerConnection as unknown as typeof RTCPeerConnection
    const originalFetch = globalThis.fetch
    let calls = 0
    globalThis.fetch = (async () => {
      calls++
      return {
        ok: true,
        json: async () => ({
          iceServers: [{ urls: ['turns:turn.example:443'] }],
          expiresAt: Date.now() + 6 * 60 * 60 * 1000,
        }),
      }
    }) as unknown as typeof fetch

    const { port1, port2 } = new MessageChannel()
    const endpoint = new WebRTCBridgeEndpoint(
      port1,
      [],
      '/api/turn-credentials',
    )

    let next = receive(port2)
    port2.postMessage({ type: 'createPC', cmdId: 'create', config: {} })
    await next
    expect(calls).toBe(1)

    endpoint.close()
    // Advancing well past the refresh point must not trigger another fetch.
    await vi.advanceTimersByTimeAsync(6 * 60 * 60 * 1000)
    expect(calls).toBe(1)

    vi.useRealTimers()
    globalThis.fetch = originalFetch
  })
})
