import type {
  Frame,
  GroupHeader,
} from '@s4wave/core/frametrack/frametrack.pb.js'

// decodeWindow caps the frames waiting for the browser's decoder.
const decodeWindow = 4

/** AvcDecoder decodes a contiguous prefix of each AVC group. */
export class AvcDecoder {
  /** group is the latest group seen, including a group awaiting a keyframe. */
  private group = 0n

  /** decoder holds the active group's codec state. */
  private decoder: VideoDecoder | undefined

  constructor(
    private readonly show: (picture: VideoFrame) => void,
    private readonly fail: (error: DOMException) => void,
  ) {}

  /** push accepts the next access unit, waiting for a keyframe after a gap. */
  public push(header: GroupHeader, frame: Frame): void {
    // A group change ends the old prefix and its queued output.
    const sequence = header.groupSequence ?? 0n
    if (sequence < this.group) return
    if (sequence > this.group) {
      this.close()
      this.group = sequence
    }

    // A decoder can start only at a keyframe with its parameter sets.
    const decoder =
      this.decoder ?? (frame.keyframe ? this.open(header) : undefined)
    if (!decoder) return

    // A full queue cuts this group. Deltas after the cut need the lost frame.
    const data = frame.data
    if (!data?.byteLength || decoder.decodeQueueSize >= decodeWindow) {
      this.close()
      return
    }

    // Decode this picture with its source timestamp in microseconds.
    try {
      decoder.decode(
        new EncodedVideoChunk({
          type: frame.keyframe ? 'key' : 'delta',
          timestamp: (frame.capturedAt?.getTime() ?? 0) * 1000,
          data: data as BufferSource,
        }),
      )
    } catch (error) {
      this.close()
      if (!(error instanceof DOMException) || error.name !== 'DataError') {
        throw error
      }
    }
  }

  /** close releases the active decoder and retires any pending output. */
  public close(): void {
    const decoder = this.decoder
    this.decoder = undefined
    if (decoder && decoder.state !== 'closed') decoder.close()
  }

  /** open configures WebCodecs for the access units in one group. */
  private open(header: GroupHeader): VideoDecoder {
    // The configuration record supplies both the codec ID and parameter sets.
    if (typeof VideoDecoder === 'undefined') {
      throw new Error('This browser cannot decode H.264 pictures.')
    }
    const description = header.decoderDescription
    if (!description || description.byteLength < 7 || description[0] !== 1) {
      throw new Error('The Device sent an invalid H.264 decoder description.')
    }
    const profile = Array.from(description.subarray(1, 4), (byte) =>
      byte.toString(16).padStart(2, '0'),
    ).join('')

    // Output from a retired group must never replace the last picture.
    const decoder = new VideoDecoder({
      output: (picture) => {
        if (this.decoder === decoder) this.show(picture)
        else picture.close()
      },
      error: (error) => {
        if (this.decoder !== decoder) return
        this.close()
        if (error.name === 'NotSupportedError') this.fail(error)
      },
    })
    this.decoder = decoder

    // AVC access units use length-prefixed NAL units and a separate description.
    decoder.configure({
      codec: `avc1.${profile}`,
      codedWidth: header.width,
      codedHeight: header.height,
      description: description as BufferSource,
      optimizeForLatency: true,
    })
    return decoder
  }
}
