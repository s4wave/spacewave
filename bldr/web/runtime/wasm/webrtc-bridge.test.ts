import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ProxyRTCPeerConnection,
  setBridgePort,
  type BridgeCommand,
  type BridgeEvent,
  type BridgeResponse,
} from './webrtc-bridge.js'

type BridgeGlobals = typeof globalThis & {
  __bldrWebRtcBridgePort?: MessagePort | null
  __bldrWebRtcBridgeDispatcher?: unknown
}

type PostedMessage = {
  message: unknown
  transfer: Transferable[]
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

type BridgeCommandWithChannel = BridgeCommand & {
  dcId?: string
  channel?: ChannelMetadata
  data?: unknown
}

type BridgeCommandWithCmdId = BridgeCommandWithChannel & { cmdId: number }

type BridgeEventWithChannel = BridgeEvent & {
  dcId?: string
  channel?: ChannelMetadata
  data?: unknown
  error?: { message: string } | string
}

class FakeMessagePort {
  onmessage: ((event: MessageEvent) => void) | null = null
  onmessageerror: ((event: MessageEvent) => void) | null = null
  posted: PostedMessage[] = []
  started = false
  closed = false

  postMessage(message: unknown, transfer: Transferable[] = []): void {
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

const connectedSnapshot = {
  connectionState: 'connected',
  signalingState: 'stable',
  iceConnectionState: 'connected',
  iceGatheringState: 'complete',
  localDescription: null,
  remoteDescription: null,
} satisfies BridgeEvent['snapshot']

const quicChannel = {
  label: 'bifrost-quic',
  ordered: false,
  protocol: 'bifrost-quic',
  negotiated: true,
  id: 1,
  maxPacketLifeTime: null,
  maxRetransmits: null,
  readyState: 'connecting',
  bufferedAmount: 0,
  bufferedAmountLowThreshold: 0,
} satisfies ChannelMetadata

function bridgeGlobals(): BridgeGlobals {
  // The bridge module stores its dispatcher on globalThis by design.
  return globalThis as BridgeGlobals
}

function assertRecord(
  value: unknown,
): asserts value is Record<PropertyKey, unknown> {
  if (!value || typeof value !== 'object') {
    throw new Error('expected an object message')
  }
}

function commandAt(
  port: FakeMessagePort,
  index: number,
): BridgeCommandWithChannel {
  const posted = port.posted[index]
  expect(posted).toBeDefined()
  assertRecord(posted.message)
  const command = posted.message as unknown as BridgeCommandWithChannel
  expect(typeof command.type).toBe('string')
  return command
}

function commandWithCmdIdAt(
  port: FakeMessagePort,
  index: number,
): BridgeCommandWithCmdId {
  const command = commandAt(port, index)
  expect(typeof command.cmdId).toBe('number')
  return command as BridgeCommandWithCmdId
}

function lastCommand(port: FakeMessagePort): BridgeCommandWithChannel {
  return commandAt(port, port.posted.length - 1)
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

async function flushBridge(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function setupPeerConnection(): {
  port: FakeMessagePort
  pc: ProxyRTCPeerConnection
  createPc: BridgeCommandWithCmdId
} {
  const port = new FakeMessagePort()
  setBridgePort(port as unknown as MessagePort)
  const pc = new ProxyRTCPeerConnection({
    bundlePolicy: 'max-bundle',
    iceServers: [{ urls: 'stun:ignored.example' }],
  })
  const createPc = commandWithCmdIdAt(port, 0)
  expect(createPc.type).toBe('createPC')
  return { port, pc, createPc }
}

async function acceptPeerConnection(
  port: FakeMessagePort,
  createPc: BridgeCommandWithCmdId,
  pcId = 'pc-worker-1',
): Promise<void> {
  port.deliver({
    type: 'createPC',
    cmdId: createPc.cmdId,
    pcId,
    snapshot: connectedSnapshot,
  })
  await flushBridge()
}

async function createQuicDataChannel(
  port: FakeMessagePort,
  pc: ProxyRTCPeerConnection,
  pcId = 'pc-worker-1',
): Promise<{
  dc: RTCDataChannel
  createDc: BridgeCommandWithCmdId
  dcId: string
}> {
  const dc = pc.createDataChannel('bifrost-quic', {
    negotiated: true,
    id: 1,
    ordered: false,
    protocol: 'bifrost-quic',
  }) as unknown as RTCDataChannel
  await flushBridge()

  const createDc = commandWithCmdIdAt(port, port.posted.length - 1)
  expect(createDc).toMatchObject({
    type: 'createDataChannel',
    pcId,
    label: 'bifrost-quic',
    options: {
      negotiated: true,
      id: 1,
      ordered: false,
      protocol: 'bifrost-quic',
    },
  })

  const dcId = 'dc-quic-1'
  port.deliver({
    type: 'createDataChannel',
    cmdId: createDc.cmdId,
    pcId,
    dcId,
    channel: quicChannel,
    snapshot: connectedSnapshot,
  } satisfies BridgeResponse)
  await flushBridge()

  return { dc, createDc, dcId }
}

function errorMessageFromEvent(value: unknown): string | undefined {
  if (value instanceof Error) return value.message
  if (!value || typeof value !== 'object') return undefined

  if ('error' in value) {
    const error = value.error
    if (error instanceof Error) return error.message
    if (typeof error === 'string') return error
    if (error && typeof error === 'object' && 'message' in error) {
      const message = error.message
      if (typeof message === 'string') return message
    }
  }

  if ('message' in value) {
    const message = value.message
    if (typeof message === 'string') return message
  }

  return undefined
}

afterEach(() => {
  const globals = bridgeGlobals()
  delete globals.__bldrWebRtcBridgePort
  delete globals.__bldrWebRtcBridgeDispatcher
  vi.restoreAllMocks()
})

describe('ProxyRTCPeerConnection data channel bridge', () => {
  it('creates the negotiated QUIC channel metadata and flushes copied bytes only after dcopen', async () => {
    const { port, pc, createPc } = setupPeerConnection()
    await acceptPeerConnection(port, createPc)

    const { dc, dcId } = await createQuicDataChannel(port, pc)
    expect(dc.label).toBe('bifrost-quic')
    expect(dc.ordered).toBe(false)
    expect(dc.negotiated).toBe(true)
    expect(dc.id).toBe(1)
    expect(dc.protocol).toBe('bifrost-quic')

    const backing = new Uint8Array([99, 1, 2, 3, 4, 88])
    const view = new Uint8Array(backing.buffer, 2, 3)
    dc.send(view)
    backing.set([7, 7, 7], 2)

    expect(
      port.posted.map((entry) => commandAtPayloadType(entry)),
    ).not.toContain('dc:send')

    port.deliver({
      type: 'event:dcopen',
      pcId: 'pc-worker-1',
      dcId,
    } satisfies BridgeEventWithChannel)
    await flushBridge()

    const send = lastCommand(port)
    expect(send.type).toBe('dc:send')
    expect(send.pcId).toBe('pc-worker-1')
    expect(send.dcId).toBe(dcId)
    expect(send.data).toBeInstanceOf(ArrayBuffer)
    expect(bytesOf(send.data)).toEqual([2, 3, 4])
  })

  it('delivers bridged string and binary dcmessage payloads through onmessage', async () => {
    const { port, pc, createPc } = setupPeerConnection()
    await acceptPeerConnection(port, createPc)
    const { dc, dcId } = await createQuicDataChannel(port, pc)

    const received: unknown[] = []
    dc.onmessage = (event) => {
      received.push(event.data)
    }

    const binary = new Uint8Array([5, 8, 13]).buffer
    port.deliver({
      type: 'event:dcmessage',
      pcId: 'pc-worker-1',
      dcId,
      data: 'quic-control',
    } satisfies BridgeEventWithChannel)
    port.deliver({
      type: 'event:dcmessage',
      pcId: 'pc-worker-1',
      dcId,
      data: binary,
    } satisfies BridgeEventWithChannel)

    expect(received[0]).toBe('quic-control')
    expect(bytesOf(received[1])).toEqual([5, 8, 13])
  })

  it('surfaces bridge setup failure to pending data channels as error.message and close', async () => {
    const { port, pc, createPc } = setupPeerConnection()
    const dc = pc.createDataChannel('bifrost-quic', {
      negotiated: true,
      id: 1,
      ordered: false,
      protocol: 'bifrost-quic',
    }) as unknown as RTCDataChannel
    const suppressedRejection = pc.createOffer().catch(() => undefined)

    let errorMessage: string | undefined
    let closeCount = 0
    dc.onerror = (event) => {
      errorMessage = errorMessageFromEvent(event)
    }
    dc.onclose = () => {
      closeCount += 1
    }

    port.deliver({
      type: 'createPC',
      cmdId: createPc.cmdId,
      error: 'bridge disconnected before data channel opened',
    })
    await suppressedRejection
    await flushBridge()

    expect(errorMessage).toBe('bridge disconnected before data channel opened')
    expect(closeCount).toBe(1)
    expect(dc.readyState).toBe('closed')
  })

  it('returns stable SCTP and transceiver support objects used by pion syscall/js', async () => {
    const { port, pc, createPc } = setupPeerConnection()
    await acceptPeerConnection(port, createPc)

    const sctp = pc.sctp
    expect(sctp).not.toBeNull()
    expect(pc.sctp).toBe(sctp)
    expect(sctp?.maxMessageSize).toBe(65536)
    expect(sctp?.state).toBe('connected')

    const transport = sctp?.transport
    expect(transport).toBeDefined()
    expect(sctp?.transport).toBe(transport)
    expect(transport?.state).toBe('connected')
    expect(transport?.getRemoteCertificates()).toEqual([])

    const iceTransport = transport?.iceTransport
    expect(iceTransport).toBeDefined()
    expect(transport?.iceTransport).toBe(iceTransport)
    expect(iceTransport?.state).toBe('connected')
    expect(iceTransport?.getSelectedCandidatePair()).toBeNull()

    const transceiver = pc.addTransceiver('audio')
    expect(transceiver.direction).toBe('sendrecv')
    expect(transceiver.currentDirection).toBe('sendrecv')
    expect(transceiver.sender.track).toBeNull()
    expect(transceiver.sender.dtmf).toBeNull()
    expect(transceiver.receiver.track).toBeNull()
    expect(transceiver.mid).toBeNull()
    expect(pc.getTransceivers()).toEqual([transceiver])
  })
})

function commandAtPayloadType(entry: PostedMessage): string | undefined {
  assertRecord(entry.message)
  const command = entry.message as unknown as BridgeCommandWithChannel
  return command.type
}
