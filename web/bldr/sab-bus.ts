// SabBus implements a shared bus ring buffer for intra-tab plugin IPC.
//
// Multiple endpoints (plugins) share a single SharedArrayBuffer. Messages
// carry uint16 source/target plugin IDs. Each endpoint registers as a reader
// and maintains its own read position. Writers use CAS to claim slots safely
// under concurrent access.
//
// SharedArrayBuffer layout:
//   Control region (Int32Array):
//     [0]       writeIdx      Monotonic slot counter (CAS by writers)
//     [1]       state         0=open, 1=closed
//     [2]       readerCount   Number of registered readers
//     [3]       reserved
//     [4..19]   readerIdxs    Per-reader read positions (max 16)
//   Data region (after CTRL_BYTES):
//     numSlots * slotSize bytes
//     Each slot: [uint16 targetId][uint16 sourceId][uint32 payloadLen][payload]

import type { Sink, Source, Duplex } from 'it-stream-types'
import { pushable } from 'it-pushable'
import type { Pushable } from 'it-pushable'

// Control layout.
const CTRL_WRITE_IDX = 0
const CTRL_STATE = 1
const CTRL_READER_COUNT = 2
const READER_IDX_START = 4
const MAX_READERS = 16
const CTRL_INT32S = READER_IDX_START + MAX_READERS
const CTRL_BYTES = CTRL_INT32S * 4

// Message header: [2B target][2B source][4B length] = 8 bytes.
const MSG_HEADER = 8

const STATE_OPEN = 0
const STATE_CLOSED = 1

const DEFAULT_SLOT_SIZE = 8192
const DEFAULT_NUM_SLOTS = 64

// BROADCAST_ID addresses a message to all endpoints on the bus.
export const BROADCAST_ID = 0xffff

// SabBusOpts configures a SabBus.
export interface SabBusOpts {
  slotSize?: number
  numSlots?: number
}

// SabBusMessage is a decoded message from the bus.
export interface SabBusMessage {
  targetId: number
  sourceId: number
  data: Uint8Array
}

// createBusSab allocates a SharedArrayBuffer for the bus.
export function createBusSab(opts?: SabBusOpts): SharedArrayBuffer {
  const slotSize = opts?.slotSize ?? DEFAULT_SLOT_SIZE
  const numSlots = opts?.numSlots ?? DEFAULT_NUM_SLOTS
  return new SharedArrayBuffer(CTRL_BYTES + numSlots * slotSize)
}

// SabBusEndpoint is one participant on the shared bus.
// Each endpoint has a pluginId and can write messages to and read
// messages from the bus. Messages not addressed to this endpoint
// (or to BROADCAST_ID) are skipped transparently.
export class SabBusEndpoint {
  private readonly ctrl: Int32Array
  private readonly sab: SharedArrayBuffer
  private readonly slotSize: number
  private readonly numSlots: number
  private readerSlot = -1
  private readonly pluginId: number
  private closed = false

  constructor(sab: SharedArrayBuffer, pluginId: number, opts?: SabBusOpts) {
    this.sab = sab
    this.pluginId = pluginId
    this.slotSize = opts?.slotSize ?? DEFAULT_SLOT_SIZE
    this.numSlots = opts?.numSlots ?? DEFAULT_NUM_SLOTS
    this.ctrl = new Int32Array(sab, 0, CTRL_INT32S)
  }

  // register claims a reader slot on the bus. Must be called before read().
  register(): void {
    const slot = Atomics.add(this.ctrl, CTRL_READER_COUNT, 1)
    if (slot >= MAX_READERS) {
      throw new Error(`SabBus: max readers (${MAX_READERS}) exceeded`)
    }
    this.readerSlot = slot
    // Start reading from the current write position.
    Atomics.store(
      this.ctrl,
      READER_IDX_START + slot,
      Atomics.load(this.ctrl, CTRL_WRITE_IDX),
    )
  }

  // write sends a message to the bus addressed to targetId.
  // Uses compare-and-swap to safely claim a slot under concurrent writers.
  async write(targetId: number, data: Uint8Array): Promise<void> {
    const maxPayload = this.slotSize - MSG_HEADER
    if (data.byteLength > maxPayload) {
      throw new Error(
        `SabBus: message ${data.byteLength} bytes exceeds max ${maxPayload}`,
      )
    }

    // Claim a slot via CAS loop.
    let claimedIdx: number
    while (!this.closed) {
      const writeIdx = Atomics.load(this.ctrl, CTRL_WRITE_IDX)

      // Check that no reader is more than numSlots behind.
      const readerCount = Atomics.load(this.ctrl, CTRL_READER_COUNT)
      let minRead = writeIdx
      for (let i = 0; i < readerCount; i++) {
        const r = Atomics.load(this.ctrl, READER_IDX_START + i)
        if (r < minRead) {
          minRead = r
        }
      }
      if (writeIdx - minRead >= this.numSlots) {
        await new Promise<void>((r) => setTimeout(r, 1))
        continue
      }

      // Try to claim this slot.
      const actual = Atomics.compareExchange(
        this.ctrl,
        CTRL_WRITE_IDX,
        writeIdx,
        writeIdx + 1,
      )
      if (actual === writeIdx) {
        claimedIdx = writeIdx
        break
      }
      // Another writer claimed it; retry.
    }
    if (this.closed) {
      return
    }

    const slotOff = CTRL_BYTES + (claimedIdx! % this.numSlots) * this.slotSize
    const hdr = new DataView(this.sab, slotOff, MSG_HEADER)
    hdr.setUint16(0, targetId, true)
    hdr.setUint16(2, this.pluginId, true)
    hdr.setUint32(4, data.byteLength, true)

    new Uint8Array(this.sab, slotOff + MSG_HEADER, data.byteLength).set(data)

    // Wake all readers.
    Atomics.notify(this.ctrl, CTRL_WRITE_IDX)
  }

  // read returns the next message addressed to this endpoint (or broadcast).
  // Messages for other endpoints are advanced past silently.
  // Returns null when the bus is closed.
  async read(): Promise<SabBusMessage | null> {
    if (this.readerSlot < 0) {
      throw new Error('SabBus: not registered, call register() first')
    }
    const readerIdx = READER_IDX_START + this.readerSlot

    while (!this.closed) {
      const readPos = Atomics.load(this.ctrl, readerIdx)
      const writePos = Atomics.load(this.ctrl, CTRL_WRITE_IDX)

      if (readPos < writePos) {
        const slotOff =
          CTRL_BYTES + (readPos % this.numSlots) * this.slotSize
        const hdr = new DataView(this.sab, slotOff, MSG_HEADER)
        const targetId = hdr.getUint16(0, true)
        const sourceId = hdr.getUint16(2, true)
        const length = hdr.getUint32(4, true)

        // Advance past this slot regardless of target.
        Atomics.add(this.ctrl, readerIdx, 1)
        Atomics.notify(this.ctrl, readerIdx)

        // Only deliver if addressed to us or broadcast.
        if (targetId === this.pluginId || targetId === BROADCAST_ID) {
          const data = new Uint8Array(length)
          data.set(new Uint8Array(this.sab, slotOff + MSG_HEADER, length))
          return { targetId, sourceId, data }
        }
        continue
      }

      // No data. Check bus state.
      if (Atomics.load(this.ctrl, CTRL_STATE) !== STATE_OPEN) {
        return null
      }

      // Wait for new writes.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const atomics = Atomics as any
      if (typeof atomics.waitAsync === 'function') {
        const result = atomics.waitAsync(this.ctrl, CTRL_WRITE_IDX, writePos)
        if (result.async) {
          await result.value
        }
      } else {
        await new Promise<void>((r) => setTimeout(r, 1))
      }
    }

    return null
  }

  // close stops this endpoint from reading or writing.
  close(): void {
    this.closed = true
  }

  // closeAll signals the bus as closed, waking all readers.
  closeAll(): void {
    Atomics.store(this.ctrl, CTRL_STATE, STATE_CLOSED)
    Atomics.notify(this.ctrl, CTRL_WRITE_IDX)
    this.closed = true
  }
}

// SabBusStream adapts a SabBusEndpoint for a specific remote plugin ID
// into a PacketStream (Duplex) compatible with StarPC. Messages written
// to the sink are sent to the target plugin. Messages read from the
// source come from the target plugin (filtered by the bus).
export class SabBusStream
  implements
    Duplex<AsyncGenerator<Uint8Array>, Source<Uint8Array>, Promise<void>>
{
  public source: AsyncGenerator<Uint8Array>
  public sink: Sink<Source<Uint8Array>, Promise<void>>

  private readonly _source: Pushable<Uint8Array>
  private readonly endpoint: SabBusEndpoint
  private readonly targetId: number
  private closed = false

  constructor(endpoint: SabBusEndpoint, targetId: number) {
    this.endpoint = endpoint
    this.targetId = targetId

    const source = pushable<Uint8Array>({ objectMode: true })
    this._source = source
    this.source = source
    this.sink = this._createSink()

    this._readLoop().catch((err) => {
      if (!this.closed) {
        this._source.end(err instanceof Error ? err : new Error(String(err)))
      }
    })
  }

  private async _readLoop(): Promise<void> {
    while (!this.closed) {
      const msg = await this.endpoint.read()
      if (!msg) {
        break
      }
      // Deliver only messages from the target plugin for this stream.
      if (msg.sourceId === this.targetId) {
        this._source.push(msg.data)
      }
    }
    if (!this.closed) {
      this._source.end()
    }
  }

  private _createSink(): Sink<Source<Uint8Array>, Promise<void>> {
    return async (source: Source<Uint8Array>) => {
      try {
        for await (const msg of source) {
          await this.endpoint.write(this.targetId, msg)
        }
      } catch (err) {
        this.close(err instanceof Error ? err : new Error(String(err)))
      }
    }
  }

  // close tears down this stream.
  public close(error?: Error): void {
    if (this.closed) {
      return
    }
    this.closed = true
    this._source.end(error)
  }
}
