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

  constructor(config: RTCConfiguration) {
    this.config = config
    FakePeerConnection.instances.push(this)
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

describe('WebRTCBridgeEndpoint', () => {
  const original = globalThis.RTCPeerConnection
  afterEach(() => {
    globalThis.RTCPeerConnection = original
    FakePeerConnection.instances = []
  })

  it('uses trusted ICE servers and flushes candidates after the remote description', async () => {
    globalThis.RTCPeerConnection = FakePeerConnection as unknown as typeof RTCPeerConnection
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
    expect(responses.map((response) => response.cmdId)).toEqual(['ice', 'remote'])
    expect(pc.candidates).toEqual([{ candidate: 'candidate:1' }])
    endpoint.close()
  })

  it('rejects queued candidates when the peer connection closes', async () => {
    globalThis.RTCPeerConnection = FakePeerConnection as unknown as typeof RTCPeerConnection
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
    expect(responses.map((response) => response.cmdId)).toEqual(['ice', 'close'])
    expect((responses[0] as TestBridgeResponse & { error?: string }).error).toContain(
      'closed before remote description',
    )
    endpoint.close()
  })
})
