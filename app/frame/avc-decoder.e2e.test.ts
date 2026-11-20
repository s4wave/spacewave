import { expect, test } from 'vitest'

import { Codec } from '@s4wave/core/frametrack/frametrack.pb.js'

import { AvcDecoder } from './avc-decoder.js'

/** readClipParts extracts the AVC description and the first two access units. */
async function readClipParts() {
  const clip = new URL('../../core/testpattern/bars.flv', import.meta.url)
  const data = new Uint8Array(await (await fetch(clip)).arrayBuffer())
  let offset = 13
  const readTag = () => {
    const header = new DataView(data.buffer, offset, 11)
    const size = header.getUint32(1) >>> 8
    const payload = data.subarray(offset + 16, offset + 11 + size)
    offset += 15 + size
    return payload
  }
  return { description: readTag(), key: readTag(), delta: readTag() }
}

test('an AVC gap holds the last picture until the next keyframe decodes', async () => {
  // Use the same description and access units as the Device's source.
  const { description, key, delta } = await readClipParts()
  const header = {
    codec: Codec.H264_AVC,
    width: 640,
    height: 360,
    decoderDescription: description,
    groupSequence: 1n,
  }
  const pictures: number[] = []
  let nextPicture = Promise.withResolvers<number>()
  const decoder = new AvcDecoder(
    (picture) => {
      pictures.push(picture.timestamp)
      picture.close()
      nextPicture.resolve(pictures.length)
    },
    (error) => nextPicture.reject(error),
  )

  try {
    // Decode a real IDR before simulating the missing start of the next group.
    decoder.push(header, { keyframe: true, data: key, capturedAt: new Date(1) })
    expect(await nextPicture.promise).toBe(1)

    // Both a leading delta and a late frame from the old group must be dropped.
    nextPicture = Promise.withResolvers<number>()
    decoder.push(
      { ...header, groupSequence: 2n },
      { data: delta, capturedAt: new Date(2) },
    )
    decoder.push(header, { data: delta, capturedAt: new Date(3) })
    decoder.push(
      { ...header, groupSequence: 3n },
      {
        keyframe: true,
        data: key,
        capturedAt: new Date(4),
      },
    )
    expect(await nextPicture.promise).toBe(2)
    expect(pictures).toEqual([1000, 4000])
  } finally {
    decoder.close()
  }
}, 10000)
