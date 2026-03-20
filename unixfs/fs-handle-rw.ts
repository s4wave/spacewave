import type { FSHandle } from './fs-handle.js'

// SeekStart is the constant for seeking from the start.
const SeekStart = 0
// SeekCurrent is the constant for seeking from the current position.
const SeekCurrent = 1
// SeekEnd is the constant for seeking from the end.
const SeekEnd = 2

// FSHandleReadWriter wraps a FSHandle to implement sequential read/write/seek.
export class FSHandleReadWriter {
  private readonly signal: AbortSignal
  private readonly h: FSHandle
  private readonly ts: () => Date

  private idx = 0n
  private size = 0n

  // constructor constructs a new ReadWriter from a FSHandle.
  // If ts is null, uses Date.now.
  constructor(
    signal: AbortSignal,
    h: FSHandle,
    ts?: () => Date,
  ) {
    this.signal = signal
    this.h = h
    this.ts = ts ?? (() => new Date())
  }

  // read reads data from the file at the current index.
  // Returns the number of bytes read.
  async read(p: Uint8Array): Promise<bigint> {
    const nr = await this.h.readAtTo(this.signal, this.idx, p)
    if (nr > 0n) {
      this.idx += nr
    }
    return nr
  }

  // write writes data to the file at the current index.
  // Returns the number of bytes written.
  async write(p: Uint8Array): Promise<bigint> {
    const wts = this.ts()
    await this.h.writeAt(this.signal, this.idx, p, wts)
    const written = BigInt(p.length)
    this.idx += written
    return written
  }

  // seek moves the read/writer to a location in the file.
  // whence: 0=SeekStart, 1=SeekCurrent, 2=SeekEnd
  async seek(offset: bigint, whence: number): Promise<bigint> {
    switch (whence) {
      case SeekCurrent:
        this.idx += offset
        break
      case SeekStart:
        this.idx = offset
        break
      case SeekEnd: {
        const size = await this.getSize()
        this.idx = size + offset
        break
      }
    }
    return this.idx
  }

  // getSize determines the size from the cached data or by calling getSize.
  private async getSize(): Promise<bigint> {
    if (this.size !== 0n) {
      return this.size
    }
    this.size = await this.h.getSize(this.signal)
    return this.size
  }
}
