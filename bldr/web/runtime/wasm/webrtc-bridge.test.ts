import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  DataChannelWrapper,
  ProxyRTCPeerConnection,
  setBridgePort,
  type BridgeMessage,
} from './webrtc-bridge.js'

type BridgeTestGlobals = typeof globalThis & {
  __bldrWebRtcBridgePort?: MessagePort | null
  __bldrWebRtcBridgeDispatcher?: unknown
}

type SentPayload = string | Uint8Array

class FakeRTCDataChannel {
  readonly label = 'fake-data-channel'
  readonly ordered = true
  readonly protocol = ''
  readonly negotiated = false
  readonly id = null
  readonly maxPacketLifeTime = null
  readonly maxRetransmits = null

  readyState: RTCDataChannelState
  bufferedAmount = 0
  bufferedAmountLowThreshold = 0
  binaryType: BinaryType = 'arraybuffer'
  onopen: ((this: RTCDataChannel, ev: Event) => unknown) | null = null
  onmessage: ((this: RTCDataChannel, ev: MessageEvent) => unknown) | null =
    null
  onclose: ((this: RTCDataChannel, ev: Event) => unknown) | null = null
  onerror: ((this: RTCDataChannel, ev: Event) => unknown) | null = null
  onbufferedamountlow:
    | ((this: RTCDataChannel, ev: Event) => unknown)
    | null = null
  readonly sent: SentPayload[] = []
  closed = false

  constructor(readyState: RTCDataChannelState = 'connecting') {
    this.readyState = readyState
  }

  send(data: string | ArrayBuffer | ArrayBufferView): void {
    if (typeof data === 'string') {
      this.sent.push(data)
      return
    }
    if (data instanceof ArrayBuffer) {
      this.sent.push(new Uint8Array(data.slice(0)))
      return
    }
    this.sent.push(
      new Uint8Array(data.buffer, data.byteOffset, data.byteLength).slice(),
    )
  }

  close(): void {
    this.closed = true
    this.readyState = 'closed'
  }
}

class FakeMessagePort {
  onmessage: ((this: MessagePort, ev: MessageEvent<BridgeMessage>) => void) |
    null = null
  readonly postMessage = vi.fn<(message: unknown) => void>()
  readonly start = vi.fn<() => void>()
  readonly close = vi.fn<() => void>()

  dispatch(data: BridgeMessage): void {
    this.onmessage?.call(this as unknown as MessagePort, {
      data,
    } as MessageEvent<BridgeMessage>)
  }
}

function asRTCDataChannel(channel: FakeRTCDataChannel): RTCDataChannel {
  return channel as unknown as RTCDataChannel
}

function asMessagePort(port: FakeMessagePort): MessagePort {
  return port as unknown as MessagePort
}

function payloadSnapshot(payload: SentPayload): string | number[] {
  return typeof payload === 'string' ? payload : Array.from(payload)
}

afterEach(() => {
  const globals = globalThis as BridgeTestGlobals
  delete globals.__bldrWebRtcBridgePort
  delete globals.__bldrWebRtcBridgeDispatcher
  vi.restoreAllMocks()
})

describe('DataChannelWrapper transferred channel bridge', () => {
  it('queues pre-attach text and binary sends, reports queued bytes, and replays them in order', () => {
    const wrapper = new DataChannelWrapper('quic')
    const binaryBuffer = new ArrayBuffer(3)
    new Uint8Array(binaryBuffer).set([1, 2, 3])
    const viewBacking = new Uint8Array([7, 8, 9, 10])

    wrapper.send('hi')
    wrapper.send(binaryBuffer)
    wrapper.send(viewBacking.subarray(1, 3))

    expect(wrapper.bufferedAmount).toBe(7)

    const realChannel = new FakeRTCDataChannel()
    wrapper.attach(asRTCDataChannel(realChannel))

    expect(realChannel.sent.map(payloadSnapshot)).toEqual([
      'hi',
      [1, 2, 3],
      [8, 9],
    ])
    expect(wrapper.bufferedAmount).toBe(realChannel.bufferedAmount)
  })

  it('delegates post-attach sends directly and binds stored handlers to the real channel shape', () => {
    const wrapper = new DataChannelWrapper('quic')
    const onopen = vi.fn<(ev: Event) => void>()
    const onmessage = vi.fn<(ev: MessageEvent) => void>()
    const onclose = vi.fn<(ev: Event) => void>()
    const onerror = vi.fn<(ev: Event) => void>()
    const onbufferedamountlow = vi.fn<(ev: Event) => void>()

    wrapper.bufferedAmountLowThreshold = 11
    wrapper.onopen = onopen
    wrapper.onmessage = onmessage
    wrapper.onclose = onclose
    wrapper.onerror = onerror
    wrapper.onbufferedamountlow = onbufferedamountlow

    const realChannel = new FakeRTCDataChannel()
    wrapper.attach(asRTCDataChannel(realChannel))

    expect(realChannel.bufferedAmountLowThreshold).toBe(11)
    expect(realChannel.onopen).toBe(onopen)
    expect(realChannel.onmessage).toBe(onmessage)
    expect(realChannel.onclose).toBe(onclose)
    expect(realChannel.onbufferedamountlow).toBe(onbufferedamountlow)
    expect(realChannel.onerror).toBeTypeOf('function')

    realChannel.onerror?.call(
      asRTCDataChannel(realChannel),
      new Event('error'),
    )
    expect(onerror).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }))

    wrapper.send(new Uint8Array([4, 5, 6]))
    expect(realChannel.sent.map(payloadSnapshot)).toEqual([[4, 5, 6]])
    expect(wrapper.bufferedAmount).toBe(realChannel.bufferedAmount)

    const nextMessageHandler = vi.fn<(ev: MessageEvent) => void>()
    wrapper.onmessage = nextMessageHandler
    expect(realChannel.onmessage).toBe(nextMessageHandler)

    const message = new MessageEvent('message', { data: new Uint8Array([1]) })
    realChannel.onmessage?.call(asRTCDataChannel(realChannel), message)
    expect(nextMessageHandler).toHaveBeenCalledWith(message)
  })

  it('closes a pending wrapper on bridge death, clears queued bytes, and fires error before close', () => {
    const wrapper = new DataChannelWrapper('quic')
    const events: string[] = []
    wrapper.onerror = (event) => events.push(event.type)
    wrapper.onclose = (event) => events.push(event.type)

    wrapper.send('queued')
    expect(wrapper.bufferedAmount).toBe(6)

    wrapper.bridgeDied()

    expect(wrapper.readyState).toBe('closed')
    expect(wrapper.bufferedAmount).toBe(0)
    expect(events).toEqual(['error', 'close'])

    wrapper.send('after-death')
    expect(wrapper.bufferedAmount).toBe(0)

    const lateChannel = new FakeRTCDataChannel()
    wrapper.attach(asRTCDataChannel(lateChannel))
    expect(lateChannel.closed).toBe(true)
    expect(lateChannel.sent).toEqual([])
  })
})

describe('ProxyRTCPeerConnection supporting WebRTC objects', () => {
  it('keeps SCTP absent until a data channel exists and closes pending channels on bridge death', async () => {
    const bridgePort = new FakeMessagePort()
    setBridgePort(asMessagePort(bridgePort))

    const pc = new ProxyRTCPeerConnection()
    expect(bridgePort.start).toHaveBeenCalledTimes(1)
    expect(bridgePort.postMessage).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'createPC' }),
    )
    expect(pc.sctp).toBeNull()

    bridgePort.dispatch({ type: 'createPC', cmdId: 1, pcId: 'pc-1' })
    await Promise.resolve()
    await Promise.resolve()

    const channel = pc.createDataChannel('quic')
    const events: string[] = []
    channel.onerror = (event) => events.push(event.type)
    channel.onclose = (event) => events.push(event.type)
    channel.send('queued')
    expect(channel.bufferedAmount).toBe(6)

    const sctp = pc.sctp as {
      maxMessageSize: number
      state: RTCSctpTransportState
      transport: {
        state: RTCDtlsTransportState
        iceTransport: {
          state: RTCIceTransportState
          getSelectedCandidatePair: () => RTCIceCandidatePair | null
        }
        getRemoteCertificates: () => ArrayBuffer[]
      }
    } | null
    expect(sctp).not.toBeNull()
    expect(sctp!.maxMessageSize).toBe(65536)
    expect(sctp!.state).toBe('connected')
    expect(sctp!.transport.state).toBe('connected')
    expect(sctp!.transport.getRemoteCertificates()).toEqual([])
    expect(sctp!.transport.iceTransport.state).toBe('connected')
    expect(sctp!.transport.iceTransport.getSelectedCandidatePair()).toBeNull()

    await Promise.resolve()
    await Promise.resolve()
    expect(bridgePort.postMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'createDataChannel',
        pcId: 'pc-1',
        label: 'quic',
      }),
    )

    bridgePort.dispatch({
      type: 'event:bridgeclose',
      error: 'bridge closed by test',
    })

    expect(pc.connectionState).toBe('closed')
    expect(pc.signalingState).toBe('closed')
    expect(pc.iceConnectionState).toBe('closed')
    expect(channel.readyState).toBe('closed')
    expect(channel.bufferedAmount).toBe(0)
    expect(events).toEqual(['error', 'close'])

    channel.send('after-death')
    expect(channel.bufferedAmount).toBe(0)
  })
})
