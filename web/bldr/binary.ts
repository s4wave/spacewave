// encodeUint32Le encodes the number as a uint32 with little endian.
export function encodeUint32Le(value: number): Uint8Array {
  // output is a 4 byte array
  const output = new Uint8Array(4)
  for (let index = 0; index < output.length; index++) {
    const b = value & 0xff
    output[index] = b
    value = (value - b) / 256
  }
  return output
}

// decodeUint32Le decodes a uint32 from a 4 byte Uint8Array.
// returns 0 if decoding failed.
// callers should check that len(data) == 4
export function decodeUint32Le(data: Uint8Array): number {
  let value = 0
  let nbytes = 4
  if (data.length < nbytes) {
    nbytes = data.length
  }
  for (let i = nbytes - 1; i >= 0; i--) {
    value = value * 256 + data[i]
  }
  return value
}

// prependPacketLen adds the message length prefix to a packet.
export function prependPacketLen(msgData: Uint8Array): Uint8Array {
  const msgLen = msgData.length
  const msgLenData = encodeUint32Le(msgLen)
  const merged = new Uint8Array(msgLen + msgLenData.length)
  merged.set(msgLenData)
  merged.set(msgData, msgLenData.length)
  return merged
}

// compareUint8Arrays compares two Uint8Array for equality.
// this is (unfortunately) the fastest known method for such a comparison.
export function compareUint8Arrays(a: Uint8Array, b: Uint8Array): boolean {
  if (a === b) return true
  if (!a || !b) return false
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}
