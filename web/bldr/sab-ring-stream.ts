import type { Sink, Source, Duplex } from 'it-stream-types'
import { pushable } from 'it-pushable'
import type { Pushable } from 'it-pushable'

// Control region layout (Int32Array indices).
const CTRL_WRITE_IDX = 0
const CTRL_READ_IDX = 1
const CTRL_STATE = 2
const CTRL_INT32S = 4
const CTRL_BYTES = CTRL_INT32S * 4

// Stream state values.
const STATE_OPEN = 0
const STATE_CLOSED = 1

// Default ring buffer parameters.
const DEFAULT_SLOT_SIZE = 8192
const DEFAULT_NUM_SLOTS = 32

// SabRingStreamOpts configures a SabRingStream.
export interface SabRingStreamOpts {
  // slotSize is the byte size of each ring buffer slot.
  // Max message payload is slotSize - 4 (4 bytes for length prefix).
  slotSize?: number
  // numSlots is the number of slots in the ring buffer.
  numSlots?: number
}

// createSabPair allocates two SharedArrayBuffers for bidirectional communication.
// Side A constructs SabRingStream(aSab, bSab).
// Side B constructs SabRingStream(bSab, aSab).
export function createSabPair(opts?: SabRingStreamOpts): {
  aSab: SharedArrayBuffer
  bSab: SharedArrayBuffer
} {
  const slotSize = opts?.slotSize ?? DEFAULT_SLOT_SIZE
  const numSlots = opts?.numSlots ?? DEFAULT_NUM_SLOTS
  const size = CTRL_BYTES + numSlots * slotSize
  return {
    aSab: new SharedArrayBuffer(size),
    bSab: new SharedArrayBuffer(size),
  }
}

// sabBufferSize returns the SharedArrayBuffer byte size for given opts.
export function sabBufferSize(opts?: SabRingStreamOpts): number {
  const slotSize = opts?.slotSize ?? DEFAULT_SLOT_SIZE
  const numSlots = opts?.numSlots ?? DEFAULT_NUM_SLOTS
  return CTRL_BYTES + numSlots * slotSize
}

// waitForChange waits until arr[index] differs from expected.
// Uses Atomics.waitAsync when available, otherwise polls.
async function waitForChange(
  arr: Int32Array,
  index: number,
  expected: number,
): Promise<void> {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const atomics = Atomics as any
  if (typeof atomics.waitAsync === 'function') {
    const result = atomics.waitAsync(arr, index, expected)
    if (result.async) {
      await result.value
    }
    return
  }
  // Polling fallback.
  while (Atomics.load(arr, index) === expected) {
    await new Promise<void>((r) => setTimeout(r, 1))
  }
}

// SabRingStream implements a bidirectional packet stream over SharedArrayBuffers.
//
// Each direction uses one SharedArrayBuffer laid out as:
//   [0..15]  Control region: writeIdx(i32), readIdx(i32), state(i32), reserved(i32)
//   [16..]   Data region: numSlots * slotSize bytes
//
// Each slot is [4-byte LE length][payload]. Writer advances writeIdx with
// Atomics.add and Atomics.notify. Reader waits on writeIdx with
// Atomics.waitAsync (or polling fallback) and advances readIdx.
//
// Satisfies the same Duplex<AsyncGenerator<Uint8Array>, Source<Uint8Array>,
// Promise<void>> interface as ChannelStream from starpc.
export class SabRingStream
  implements
    Duplex<AsyncGenerator<Uint8Array>, Source<Uint8Array>, Promise<void>>
{
  public source: AsyncGenerator<Uint8Array>
  public sink: Sink<Source<Uint8Array>, Promise<void>>

  private readonly _source: Pushable<Uint8Array>
  private readonly txCtrl: Int32Array
  private readonly rxCtrl: Int32Array
  private readonly txSab: SharedArrayBuffer
  private readonly rxSab: SharedArrayBuffer
  private readonly slotSize: number
  private readonly numSlots: number
  private closed = false

  constructor(
    txSab: SharedArrayBuffer,
    rxSab: SharedArrayBuffer,
    opts?: SabRingStreamOpts,
  ) {
    this.txSab = txSab
    this.rxSab = rxSab
    this.slotSize = opts?.slotSize ?? DEFAULT_SLOT_SIZE
    this.numSlots = opts?.numSlots ?? DEFAULT_NUM_SLOTS
    this.txCtrl = new Int32Array(txSab, 0, CTRL_INT32S)
    this.rxCtrl = new Int32Array(rxSab, 0, CTRL_INT32S)

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

  // _readLoop delivers messages from the rx ring buffer to the source.
  // Drains all available messages before checking close state so that
  // messages written before close are always delivered.
  private async _readLoop(): Promise<void> {
    while (true) {
      const readIdx = Atomics.load(this.rxCtrl, CTRL_READ_IDX)
      const writeIdx = Atomics.load(this.rxCtrl, CTRL_WRITE_IDX)

      if (readIdx < writeIdx) {
        const slotOff = CTRL_BYTES + (readIdx % this.numSlots) * this.slotSize
        const len = new DataView(this.rxSab, slotOff, 4).getUint32(0, true)
        const data = new Uint8Array(len)
        data.set(new Uint8Array(this.rxSab, slotOff + 4, len))

        Atomics.add(this.rxCtrl, CTRL_READ_IDX, 1)
        Atomics.notify(this.rxCtrl, CTRL_READ_IDX)
        this._source.push(data)
        continue
      }

      // No messages pending. Check termination conditions.
      if (this.closed) {
        break
      }
      if (Atomics.load(this.rxCtrl, CTRL_STATE) !== STATE_OPEN) {
        break
      }

      // Wait for the writer to advance writeIdx or for a close signal.
      await waitForChange(this.rxCtrl, CTRL_WRITE_IDX, writeIdx)
    }

    if (!this.closed) {
      this._source.end()
    }
  }

  // _write writes a single packet to the tx ring buffer.
  private async _write(data: Uint8Array): Promise<void> {
    const maxPayload = this.slotSize - 4
    if (data.byteLength > maxPayload) {
      throw new Error(
        `SabRingStream: message ${data.byteLength} bytes exceeds slot max ${maxPayload}`,
      )
    }

    // Wait for a free slot.
    while (!this.closed) {
      const writeIdx = Atomics.load(this.txCtrl, CTRL_WRITE_IDX)
      const readIdx = Atomics.load(this.txCtrl, CTRL_READ_IDX)
      if (writeIdx - readIdx < this.numSlots) {
        break
      }
      await waitForChange(this.txCtrl, CTRL_READ_IDX, readIdx)
    }
    if (this.closed) {
      return
    }

    const writeIdx = Atomics.load(this.txCtrl, CTRL_WRITE_IDX)
    const slotOff = CTRL_BYTES + (writeIdx % this.numSlots) * this.slotSize

    // Write length prefix + payload.
    new DataView(this.txSab, slotOff, 4).setUint32(0, data.byteLength, true)
    new Uint8Array(this.txSab, slotOff + 4, data.byteLength).set(data)

    Atomics.add(this.txCtrl, CTRL_WRITE_IDX, 1)
    Atomics.notify(this.txCtrl, CTRL_WRITE_IDX)
  }

  // _closeTx signals that this side is done writing.
  private _closeTx(): void {
    Atomics.store(this.txCtrl, CTRL_STATE, STATE_CLOSED)
    // Wake the remote reader so it sees the closed state.
    Atomics.notify(this.txCtrl, CTRL_WRITE_IDX)
  }

  private _createSink(): Sink<Source<Uint8Array>, Promise<void>> {
    return async (source: Source<Uint8Array>) => {
      try {
        for await (const msg of source) {
          await this._write(msg)
        }
      } catch (err) {
        this.close(err instanceof Error ? err : new Error(String(err)))
        return
      }
      this._closeTx()
    }
  }

  // close tears down the stream in both directions.
  public close(error?: Error): void {
    if (this.closed) {
      return
    }
    this.closed = true
    this._closeTx()
    this._source.end(error)
  }
}
