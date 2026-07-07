const BASE58_ALPHABET =
  '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'
const BASE58_BASE = BASE58_ALPHABET.length
const BASE58_LEADER = BASE58_ALPHABET.charAt(0)
const BASE58_FACTOR = Math.log(BASE58_BASE) / Math.log(256)
const BASE58_IFACTOR = Math.log(256) / Math.log(BASE58_BASE)

const BASE58_MAP = new Uint8Array(256)
for (let i = 0; i < BASE58_MAP.length; i++) {
  BASE58_MAP[i] = 255
}
for (let i = 0; i < BASE58_ALPHABET.length; i++) {
  BASE58_MAP[BASE58_ALPHABET.charCodeAt(i)] = i
}

// base58Encode encodes bytes using the Bitcoin base58 alphabet.
export function base58Encode(source: Uint8Array): string {
  if (source.length === 0) {
    return ''
  }

  let zeroes = 0
  let pbegin = 0
  const pend = source.length
  while (pbegin !== pend && source[pbegin] === 0) {
    pbegin++
    zeroes++
  }

  const size = ((pend - pbegin) * BASE58_IFACTOR + 1) >>> 0
  const b58 = new Uint8Array(size)
  let length = 0
  while (pbegin !== pend) {
    let carry = source[pbegin]
    let i = 0
    for (
      let it = size - 1;
      (carry !== 0 || i < length) && it !== -1;
      it--, i++
    ) {
      carry += 256 * b58[it]
      b58[it] = carry % BASE58_BASE
      carry = (carry / BASE58_BASE) | 0
    }
    length = i
    pbegin++
  }

  let it = size - length
  while (it !== size && b58[it] === 0) {
    it++
  }

  let str = BASE58_LEADER.repeat(zeroes)
  for (; it < size; it++) {
    str += BASE58_ALPHABET.charAt(b58[it])
  }
  return str
}

// base58Decode decodes a base58-encoded string to bytes.
export function base58Decode(source: string): Uint8Array | null {
  if (source.length === 0) {
    return new Uint8Array()
  }

  let psz = 0
  while (psz < source.length && source[psz] === BASE58_LEADER) {
    psz++
  }
  const zeroes = psz

  const size = ((source.length - psz) * BASE58_FACTOR + 1) >>> 0
  const b256 = new Uint8Array(size)
  let length = 0
  while (psz < source.length) {
    const charCode = source.charCodeAt(psz)
    if (charCode > 255) {
      return null
    }
    let carry = BASE58_MAP[charCode]
    if (carry === 255) {
      return null
    }

    let i = 0
    for (
      let it = size - 1;
      (carry !== 0 || i < length) && it !== -1;
      it--, i++
    ) {
      carry += BASE58_BASE * b256[it]
      b256[it] = carry % 256
      carry = (carry / 256) | 0
    }
    length = i
    psz++
  }

  let it = size - length
  while (it !== size && b256[it] === 0) {
    it++
  }

  const result = new Uint8Array(zeroes + (size - it))
  let offset = zeroes
  for (; it < size; it++) {
    result[offset++] = b256[it]
  }
  return result
}
