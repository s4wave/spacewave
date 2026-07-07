import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebRTCBridgeEndpoint } from './webrtc-bridge-endpoint.js'

type PostedMessage = {
  message: unknown
  transfer: unknown[]
}

type EndpointMessage = {
  type: string
  cmdId?: number
  pcId?: string
  dcId?: string
  channel?: ChannelMetadata
  data?: unknown
  error?: string | { message: string }
  dc?: unknown
}

type ChannelMetadata = {
  label: string
  ordered: boolean
  protocol: string
  negotiated: boolean
  id: number | null
  maxPacketLifeTime: number | null
  maxRetransmits: number | null
  readyState?: RTCDataChannelState
  bufferedAmount?: number
  bufferedAmountLowThreshold?: number
}

class FakeMessagePort {
  onmessage: ((event: MessageEvent) => void) | null = null
  onmessageerror: ((event: MessageEvent) => void) | null = null
  posted: PostedMessage[] = []
  started = false
  closed = false

  postMessage(message: unknown, transfer: unknown[] = []): void {
    this.posted.push({ message, transfer })
  }

  start(): void {
    this.started = true
  }

  close(): void {
    this.closed = true
  }

  deliver(message: unknown): void {
    const event = { data: message } as MessageEvent
    this.onmessage?.(event)
  }
}

class FakeRTCDataChannel {
  readonly label: string
  readonly ordered: boolean
  readonly protocol: string
  readonly negotiated: boolean
  readonly id: number | null
  readonly maxPacketLifeTime: number | null
  readonly maxRetransmits: number | null
  binaryType: BinaryType = 'blob'
  readyState: RTCDataChannelState = 'connecting'
  bufferedAmount = 0
  bufferedAmountLowThreshold = 0
  sent: unknown[] = []
  closeCalls = 0
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: ((event: Event) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclosing: ((event: Event) => void) | null = null
  onbufferedamountlow: ((event: Event) => void) | null = null

  constructor(label: string, options: RTCDataChannelInit = {}) {
    this.label = label
    this.ordered = options.ordered ?? true
    this.protocol = options.protocol ?? ''
    this.negotiated = options.negotiated ?? false
    this.id = options.id ?? null
    this.maxPacketLifeTime = options.maxPacketLifeTime ?? null
    this.maxRetransmits = options.maxRetransmits ?? null
  }

  send(data: unknown): void {
    this.sent.push(data)
  }

  close(): void {
    this.closeCalls += 1
    this.readyState = 'closed'
  }

  emitOpen(): void {
    this.readyState = 'open'
    this.onopen?.(new Event('open'))
  }

  emitMessage(data: unknown): void {
    const event = { data } as MessageEvent
    this.onmessage?.(event)
  }

  emitClose(): void {
    this.readyState = 'closed'
    this.onclose?.(new Event('close'))
  }

  emitError(message: string): void {
    const error = new Error(message)
    const event = { type: 'error', error, message } as unknown as Event
    this.onerror?.(event)
  }

  emitClosing(): void {
    this.readyState = 'closing'
    this.onclosing?.(new Event('closing'))
  }

  emitBufferedAmountLow(): void {
    this.onbufferedamountlow?.(new Event('bufferedamountlow'))
  }
}

class FakeRTCPeerConnection {
  static instances: FakeRTCPeerConnection[] = []

  readonly config: RTCConfiguration | undefined
  connectionState: RTCPeerConnectionState = 'new'
  signalingState: RTCSignalingState = 'stable'
  iceConnectionState: RTCIceConnectionState = 'new'
  iceGatheringState: RTCIceGatheringState = 'new'
  localDescription: RTCSessionDescriptionInit | null = null
  remoteDescription: RTCSessionDescriptionInit | null = null
  dataChannels: FakeRTCDataChannel[] = []
  closeCalls = 0
  onicecandidate: ((event: RTCPeerConnectionIceEvent) => void) | null = null
  onconnectionstatechange: ((event: Event) => void) | null = null
  onsignalingstatechange: ((event: Event) => void) | null = null
  oniceconnectionstatechange: ((event: Event) => void) | null = null
  onicegatheringstatechange: ((event: Event) => void) | null = null
  onicecandidateerror:
    | ((event: RTCPeerConnectionIceErrorEvent) => void)
    | null = null
  onnegotiationneeded: ((event: Event) => void) | null = null
  ondatachannel: ((event: RTCDataChannelEvent) => void) | null = null

  constructor(config?: RTCConfiguration) {
    this.config = config
    FakeRTCPeerConnection.instances.push(this)
  }

  createDataChannel(
    label: string,
    options?: RTCDataChannelInit,
  ): RTCDataChannel {
    const channel = new FakeRTCDataChannel(label, options)
    this.dataChannels.push(channel)
    return channel as unknown as RTCDataChannel
  }

  close(): void {
    this.closeCalls += 1
    this.connectionState = 'closed'
  }

  getStats(): Promise<RTCStatsReport> {
    return Promise.resolve(new Map() as unknown as RTCStatsReport)
  }

  emitDataChannel(channel: FakeRTCDataChannel): void {
    const event = { channel } as unknown as RTCDataChannelEvent
    this.ondatachannel?.(event)
  }
}

function assertRecord(
  value: unknown,
): asserts value is Record<PropertyKey, unknown> {
  if (!value || typeof value !== 'object') {
    throw new Error('expected an object message')
  }
}

function messageAt(port: FakeMessagePort, index: number): EndpointMessage {
  const posted = port.posted[index]
  expect(posted).toBeDefined()
  assertRecord(posted.message)
  const message = posted.message as unknown as EndpointMessage
  expect(typeof message.type).toBe('string')
  return message
}

function lastMessage(port: FakeMessagePort): EndpointMessage {
  return messageAt(port, port.posted.length - 1)
}

function latestPc(): FakeRTCPeerConnection {
  const pc = FakeRTCPeerConnection.instances.at(-1)
  if (!pc) throw new Error('expected fake RTCPeerConnection instance')
  return pc
}

function bytesOf(value: unknown): number[] {
  if (value instanceof ArrayBuffer) {
    return Array.from(new Uint8Array(value))
  }
  if (ArrayBuffer.isView(value)) {
    return Array.from(
      new Uint8Array(value.buffer, value.byteOffset, value.byteLength),
    )
  }
  throw new Error('expected binary payload')
}

async function flushEndpoint(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
}

async function createEndpointPc(port: FakeMessagePort): Promise<string> {
  port.deliver({
    type: 'createPC',
    cmdId: 1,
    config: {
      iceServers: [{ urls: 'stun:malicious.example' }],
      bundlePolicy: 'max-bundle',
      iceTransportPolicy: 'relay',
    },
  })
  await flushEndpoint()

  const response = messageAt(port, 0)
  expect(response).toMatchObject({
    type: 'createPC',
    cmdId: 1,
    snapshot: {
      connectionState: 'new',
      signalingState: 'stable',
      iceConnectionState: 'new',
      iceGatheringState: 'new',
      localDescription: null,
      remoteDescription: null,
    },
  })
  expect(typeof response.pcId).toBe('string')
  return response.pcId ?? ''
}

async function createEndpointDataChannel(
  port: FakeMessagePort,
  pcId: string,
): Promise<{
  dc: FakeRTCDataChannel
  dcId: string
  response: EndpointMessage
}> {
  port.deliver({
    type: 'createDataChannel',
    cmdId: 2,
    pcId,
    label: 'bifrost-quic',
    options: {
      negotiated: true,
      id: 1,
      ordered: false,
      protocol: 'bifrost-quic',
    },
  })
  await flushEndpoint()

  const pc = latestPc()
  const dc = pc.dataChannels[0]
  expect(dc).toBeDefined()
  expect(dc.binaryType).toBe('arraybuffer')

  const response = lastMessage(port)
  expect(response).toMatchObject({
    type: 'createDataChannel',
    cmdId: 2,
    pcId,
    channel: {
      label: 'bifrost-quic',
      ordered: false,
      protocol: 'bifrost-quic',
      negotiated: true,
      id: 1,
      maxPacketLifeTime: null,
      maxRetransmits: null,
    },
  })
  expect(typeof response.dcId).toBe('string')
  expect('dc' in response).toBe(false)
  expect(port.posted.at(-1)?.transfer).not.toContain(dc)

  return { dc, dcId: response.dcId ?? '', response }
}

function expectNoDataChannelTransfer(
  port: FakeMessagePort,
  index: number,
  channel: FakeRTCDataChannel,
): EndpointMessage {
  const posted = port.posted[index]
  expect(posted).toBeDefined()
  expect(posted.transfer).not.toContain(channel)
  const message = messageAt(port, index)
  expect('dc' in message).toBe(false)
  return message
}

afterEach(() => {
  FakeRTCPeerConnection.instances = []
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('WebRTCBridgeEndpoint data channel ownership', () => {
  it('strips unsafe ICE servers and returns DataChannel bridge metadata without transferring the RTCDataChannel', async () => {
    vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection)
    const port = new FakeMessagePort()
    new WebRTCBridgeEndpoint(port as unknown as MessagePort)

    const pcId = await createEndpointPc(port)
    expect(latestPc().config).toEqual({
      bundlePolicy: 'max-bundle',
      iceTransportPolicy: 'relay',
    })

    const { dc, response } = await createEndpointDataChannel(port, pcId)
    expect(response.dcId).not.toBe('')
    expect(response.channel).toMatchObject({
      label: 'bifrost-quic',
      ordered: false,
      protocol: 'bifrost-quic',
      negotiated: true,
      id: 1,
    })
    expect(port.posted.at(-1)?.transfer).not.toContain(dc)
  })

  it('forwards dc:send string and binary payloads exactly, then dc:close closes and releases the channel', async () => {
    vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection)
    const port = new FakeMessagePort()
    new WebRTCBridgeEndpoint(port as unknown as MessagePort)
    const pcId = await createEndpointPc(port)
    const { dc, dcId } = await createEndpointDataChannel(port, pcId)

    const buffer = new Uint8Array([3, 1, 4, 1]).buffer
    port.deliver({
      type: 'dc:send',
      cmdId: 3,
      pcId,
      dcId,
      data: 'hello-over-bridge',
    })
    port.deliver({
      type: 'dc:send',
      cmdId: 4,
      pcId,
      dcId,
      data: buffer,
    })
    await flushEndpoint()

    expect(dc.sent[0]).toBe('hello-over-bridge')
    expect(dc.sent[1]).toBe(buffer)

    port.deliver({ type: 'dc:close', cmdId: 5, pcId, dcId })
    await flushEndpoint()
    expect(dc.closeCalls).toBe(1)

    port.deliver({
      type: 'dc:send',
      cmdId: 6,
      pcId,
      dcId,
      data: 'after-close',
    })
    await flushEndpoint()
    expect(dc.sent).toHaveLength(2)
  })

  it('posts real data channel events with dcId metadata and never transfers RTCDataChannel objects', async () => {
    vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection)
    const port = new FakeMessagePort()
    new WebRTCBridgeEndpoint(port as unknown as MessagePort)
    const pcId = await createEndpointPc(port)
    const { dc, dcId } = await createEndpointDataChannel(port, pcId)

    port.posted.length = 0
    dc.emitOpen()
    dc.emitMessage('server-control')
    dc.emitMessage(new Uint8Array([9, 2, 6]).buffer)
    await flushEndpoint()
    dc.emitBufferedAmountLow()
    dc.emitClosing()
    dc.emitError('channel broke')
    dc.emitClose()
    await flushEndpoint()

    const eventTypes = port.posted.map((entry) => messageType(entry))
    expect(eventTypes).toEqual([
      'event:dcopen',
      'event:dcmessage',
      'event:dcmessage',
      'event:dcbufferedamountlow',
      'event:dcclosing',
      'event:dcerror',
      'event:dcclose',
    ])

    for (let index = 0; index < port.posted.length; index += 1) {
      const event = expectNoDataChannelTransfer(port, index, dc)
      expect(event.pcId).toBe(pcId)
      expect(event.dcId).toBe(dcId)
    }

    expect(messageAt(port, 1).data).toBe('server-control')
    expect(bytesOf(messageAt(port, 2).data)).toEqual([9, 2, 6])
    expect(messageAt(port, 5).error).toBe('channel broke')

    const inbound = new FakeRTCDataChannel('remote-quic', {
      negotiated: true,
      id: 1,
      ordered: false,
      protocol: 'bifrost-quic',
    })
    latestPc().emitDataChannel(inbound)
    await flushEndpoint()

    const inboundEvent = expectNoDataChannelTransfer(
      port,
      port.posted.length - 1,
      inbound,
    )
    expect(inboundEvent.type).toBe('event:datachannel')
    expect(inboundEvent.pcId).toBe(pcId)
    expect(inboundEvent.dcId).not.toBe(dcId)
    expect(inboundEvent.channel).toMatchObject({
      label: 'remote-quic',
      ordered: false,
      protocol: 'bifrost-quic',
      negotiated: true,
      id: 1,
    })
  })
})

function messageType(entry: PostedMessage): string {
  assertRecord(entry.message)
  const message = entry.message as unknown as EndpointMessage
  return message.type
}
