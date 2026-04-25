import type { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'

// dnsLabelRegex validates DNS label format for usernames.
export const dnsLabelRegex = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

const BASE58_ALPHABET =
  '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'

// base58Encode encodes bytes to a base58 string.
export function base58Encode(input: Uint8Array): string {
  if (input.length === 0) return ''

  // Count leading zeros.
  let zeros = 0
  while (zeros < input.length && input[zeros] === 0) {
    zeros++
  }

  // Allocate enough space in big-endian base58 representation.
  const size = Math.ceil((input.length * 138) / 100) + 1
  const buf = new Uint8Array(size)

  for (let i = 0; i < input.length; i++) {
    let carry = input[i]
    for (let j = size - 1; j >= 0; j--) {
      carry += 256 * buf[j]
      buf[j] = carry % 58
      carry = Math.floor(carry / 58)
    }
  }

  // Skip leading zeros in base58 result.
  let start = 0
  while (start < buf.length && buf[start] === 0) {
    start++
  }

  // Build result string.
  let result = '1'.repeat(zeros)
  for (let i = start; i < buf.length; i++) {
    result += BASE58_ALPHABET[buf[i]]
  }
  return result
}

// base58Decode decodes a base58 string to bytes.
export function base58Decode(input: string): Uint8Array {
  if (input.length === 0) return new Uint8Array(0)

  // Count leading '1's (zeros in base58).
  let zeros = 0
  while (zeros < input.length && input[zeros] === '1') {
    zeros++
  }

  const size = Math.ceil((input.length * 733) / 1000) + 1
  const buf = new Uint8Array(size)

  for (let i = 0; i < input.length; i++) {
    const idx = BASE58_ALPHABET.indexOf(input[i])
    if (idx < 0) throw new Error(`invalid base58 character: ${input[i]}`)
    let carry = idx
    for (let j = buf.length - 1; j >= 0; j--) {
      carry += 58 * buf[j]
      buf[j] = carry % 256
      carry = Math.floor(carry / 256)
    }
  }

  // Skip leading zeros in byte result.
  let start = 0
  while (start < buf.length && buf[start] === 0) {
    start++
  }

  const result = new Uint8Array(zeros + (buf.length - start))
  // Leading zeros are already 0 in a new Uint8Array.
  result.set(buf.subarray(start), zeros)
  return result
}

// EntityKeypair holds generated entity key material.
export interface EntityKeypair {
  pem: string
  peerId: string
  // custodiedPemBase64 is the base64-encoded entity PEM for cloud custody.
  // When no PIN is set, this is plaintext (TLS-protected in transit, not
  // encrypted at rest). With a PIN, the caller wraps it before upload.
  custodiedPemBase64: string
}

// SessionKeypair holds generated session key material.
export interface SessionKeypair {
  peerId: string
}

// AuthKeypairs holds generated account and session auth key material.
export interface AuthKeypairs {
  entity: EntityKeypair
  session: SessionKeypair
}

// generateAuthKeypairs creates account and session auth key material in Go.
export async function generateAuthKeypairs(
  spacewave: SpacewaveProvider,
  abortSignal?: AbortSignal,
): Promise<AuthKeypairs> {
  const resp = await spacewave.generateAuthKeypairs(abortSignal)
  const entity = resp.entity
  const session = resp.session
  if (
    !entity?.pemPrivateKey ||
    !entity.peerId ||
    !entity.custodiedPemBase64 ||
    !session?.peerId
  ) {
    throw new Error('Generated keypair response is incomplete')
  }
  return {
    entity: {
      pem: entity.pemPrivateKey,
      peerId: entity.peerId,
      custodiedPemBase64: entity.custodiedPemBase64,
    },
    session: {
      peerId: session.peerId,
    },
  }
}

// wrapPemWithPin applies the optional Layer 1 PIN wrapping used by custodied keys.
export async function wrapPemWithPin(
  spacewave: SpacewaveProvider,
  pem: string,
  pin: string,
  abortSignal?: AbortSignal,
): Promise<string> {
  const resp = await spacewave.wrapPemWithPin(pem, pin, abortSignal)
  const wrapped = resp.wrappedPemBase64 ?? ''
  if (!wrapped) {
    throw new Error('Wrapped PEM response is empty')
  }
  return wrapped
}

// unwrapPemWithPin removes the optional Layer 1 PIN wrapping used by custodied keys.
export async function unwrapPemWithPin(
  spacewave: SpacewaveProvider,
  encryptedBase64: string,
  pin: string,
  abortSignal?: AbortSignal,
): Promise<Uint8Array> {
  const resp = await spacewave.unwrapPemWithPin(
    encryptedBase64,
    pin,
    abortSignal,
  )
  const pem = resp.pemPrivateKey
  if (!pem?.length) {
    throw new Error('Unwrapped PEM response is empty')
  }
  return pem
}

// base64ToBytes decodes a base64 string to bytes.
export function base64ToBytes(dat: string): Uint8Array {
  return Uint8Array.from(atob(dat), (c) => c.charCodeAt(0))
}

export function bytesToBase64(dat: Uint8Array): string {
  let out = ''
  for (let i = 0; i < dat.length; i++) {
    out += String.fromCharCode(dat[i])
  }
  return btoa(out)
}
