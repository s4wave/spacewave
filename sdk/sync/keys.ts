// Object-key codec for the sync library.
// This module owns the sync/v1 key layout: identity components map to
// canonical key text and back. Fingerprints and storage consume these keys
// verbatim. No normalization or hashing: keys are bytes rendered as text.

/** metadataKey identifies the version and scope catalog for one application instance. */
export function metadataKey(instance: string): string {
  return `${keyPrefix}${encodeComponent('application', instance)}/metadata`
}

// keyPrefix begins every generated sync key.
const keyPrefix = 'sync/v1/'

// maxComponentBytes bounds one identity component in UTF-8 bytes.
const maxComponentBytes = 256

// maxRecordKeyBytes bounds a record key in UTF-8 bytes.
const maxRecordKeyBytes = 1024

// Shared codec instances; ignoreBOM keeps a leading U+FEFF in decoded text.
const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })

/**
 * CollectionKey returns the canonical collection key for one identity.
 * Components are encoded as lowercase hex of their UTF-8 bytes, so any
 * well-formed component produces a structurally unambiguous key.
 */
export function collectionKey(
  application: string,
  scope: string,
  collection: string,
): string {
  return (
    `${keyPrefix}${encodeComponent('application', application)}` +
    `/${encodeComponent('scope', scope)}` +
    `/collections/${encodeComponent('collection', collection)}`
  )
}

/**
 * ReceiptKey returns the canonical receipt key for one identity.
 * Components are encoded like collectionKey.
 */
export function receiptKey(application: string, scope: string): string {
  return (
    `${keyPrefix}${encodeComponent('application', application)}` +
    `/${encodeComponent('scope', scope)}` +
    '/receipts'
  )
}

/**
 * ParseCollectionKey parses a canonical collection key back to its identity.
 * Accepts exactly the shape collectionKey generates, including lowercase hex
 * and valid UTF-8. Returns undefined for malformed, noncanonical, or
 * reserved keys, including metadata and receipt locations.
 */
export function parseCollectionKey(
  key: string,
): { application: string; scope: string; collection: string } | undefined {
  if (!key.startsWith(keyPrefix)) return undefined
  const parts = key.slice(keyPrefix.length).split('/')
  if (parts.length !== 4 || parts[2] !== 'collections') return undefined
  const application = decodeComponent(parts[0])
  if (application === undefined) return undefined
  const scope = decodeComponent(parts[1])
  if (scope === undefined) return undefined
  const collection = decodeComponent(parts[3])
  if (collection === undefined) return undefined
  return { application, scope, collection }
}

/**
 * EncodeRecordKey returns the raw UTF-8 bytes of a valid record key.
 * Record keys are nonempty well-formed Unicode of at most 1024 UTF-8 bytes;
 * invalid keys throw TypeError naming the field, never the value.
 */
export function encodeRecordKey(key: string): Uint8Array {
  return checkComponent('record key', key, maxRecordKeyBytes)
}

// encodeComponent validates one identity component and returns its lowercase
// hex form. Throws TypeError naming the field on any invalid component.
function encodeComponent(field: string, value: string): string {
  return toHex(checkComponent(field, value, maxComponentBytes))
}

// checkComponent enforces the shared component contract: nonempty,
// well-formed Unicode, and at most maxBytes UTF-8 bytes. Returns the UTF-8
// bytes; throws TypeError naming the field, never the value.
function checkComponent(
  field: string,
  value: string,
  maxBytes: number,
): Uint8Array {
  if (value.length === 0) {
    throw new TypeError(`invalid ${field}: must be nonempty`)
  }
  if (!value.isWellFormed()) {
    throw new TypeError(`invalid ${field}: must be well-formed Unicode`)
  }
  const bytes = encoder.encode(value)
  if (bytes.length > maxBytes) {
    throw new TypeError(`invalid ${field}: exceeds ${maxBytes} UTF-8 bytes`)
  }
  return bytes
}

// decodeComponent parses lowercase hex back to a validated component string.
// Returns undefined unless the text is canonical: paired lowercase hex of
// nonempty valid UTF-8 within the component byte limit.
function decodeComponent(text: string): string | undefined {
  // Reject oversize hex before allocating decoded bytes.
  if (text.length > maxComponentBytes * 2) return undefined
  const bytes = fromHex(text)
  if (bytes === undefined || bytes.length === 0) {
    return undefined
  }
  try {
    return decoder.decode(bytes)
  } catch {
    return undefined
  }
}

// toHex renders bytes as lowercase two-digit hex.
function toHex(bytes: Uint8Array): string {
  let out = ''
  for (const b of bytes) {
    out += (b >> 4).toString(16) + (b & 15).toString(16)
  }
  return out
}

// fromHex parses lowercase two-digit hex back to bytes, or undefined when
// the text is not canonical lowercase hex.
function fromHex(text: string): Uint8Array | undefined {
  if (text.length === 0 || text.length % 2 !== 0) return undefined
  const out = new Uint8Array(text.length / 2)
  for (let i = 0; i < out.length; i++) {
    const hi = hexValue(text.charCodeAt(2 * i))
    const lo = hexValue(text.charCodeAt(2 * i + 1))
    if (hi === undefined || lo === undefined) return undefined
    out[i] = hi * 16 + lo
  }
  return out
}

// hexValue maps one ASCII hex character code to its value, lowercase only.
function hexValue(code: number): number | undefined {
  if (code >= 0x30 && code <= 0x39) return code - 0x30
  if (code >= 0x61 && code <= 0x66) return code - 0x61 + 10
  return undefined
}
