// KvDisplayMode is the value display/encoding mode for the KV viewer.
// 'text' renders bytes as UTF-8 text, 'hex' as a lowercase hex dump, 'json' as
// pretty-printed JSON parsed from the UTF-8 bytes.
export type KvDisplayMode = 'text' | 'hex' | 'json'

// KV_DISPLAY_MODES is the ordered display mode set shown in the toggle.
export const KV_DISPLAY_MODES: KvDisplayMode[] = ['text', 'hex', 'json']

// KV_DISPLAY_MODE_LABELS maps each mode to its toggle label.
export const KV_DISPLAY_MODE_LABELS: Record<KvDisplayMode, string> = {
  text: 'Text',
  hex: 'Hex',
  json: 'JSON',
}

const textDecoder = new TextDecoder('utf-8', { fatal: false })
const strictTextDecoder = new TextDecoder('utf-8', { fatal: true })
const textEncoder = new TextEncoder()

// bytesToText decodes bytes as UTF-8 text, substituting replacement characters
// for invalid sequences.
export function bytesToText(bytes: Uint8Array): string {
  return textDecoder.decode(bytes)
}

// bytesToHex renders bytes as a lowercase, space-separated hex string.
export function bytesToHex(bytes: Uint8Array): string {
  const parts: string[] = []
  for (const byte of bytes) {
    parts.push(byte.toString(16).padStart(2, '0'))
  }
  return parts.join(' ')
}

// bytesToJson pretty-prints bytes parsed as UTF-8 JSON, or throws if invalid.
export function bytesToJson(bytes: Uint8Array): string {
  return JSON.stringify(JSON.parse(strictTextDecoder.decode(bytes)), null, 2)
}

// renderValue formats bytes in the requested display mode.
export function renderValue(bytes: Uint8Array, mode: KvDisplayMode): string {
  if (mode === 'hex') return bytesToHex(bytes)
  if (mode === 'json') return bytesToJson(bytes)
  return bytesToText(bytes)
}

// parseValue encodes a display string back into bytes for the given mode. It
// throws when the input is not valid for that mode (bad hex or malformed JSON).
export function parseValue(input: string, mode: KvDisplayMode): Uint8Array {
  if (mode === 'hex') return hexToBytes(input)
  if (mode === 'json') {
    return textEncoder.encode(JSON.stringify(JSON.parse(input)))
  }
  return textEncoder.encode(input)
}

// hexToBytes parses a hex string allowing spaces and an optional 0x prefix.
export function hexToBytes(input: string): Uint8Array {
  const cleaned = input.replace(/0x/gi, '').replace(/\s+/g, '')
  if (cleaned.length % 2 !== 0) {
    throw new Error('hex input must have an even number of digits')
  }
  const bytes = new Uint8Array(cleaned.length / 2)
  for (let i = 0; i < bytes.length; i++) {
    const byte = Number.parseInt(cleaned.slice(i * 2, i * 2 + 2), 16)
    if (Number.isNaN(byte)) {
      throw new Error('hex input contains a non-hex digit')
    }
    bytes[i] = byte
  }
  return bytes
}

// detectMode picks the best display mode for bytes: 'json' when they parse as
// JSON, 'text' when they are valid printable UTF-8, otherwise 'hex'.
export function detectMode(bytes: Uint8Array): KvDisplayMode {
  if (bytes.length === 0) return 'text'
  let text: string
  try {
    text = strictTextDecoder.decode(bytes)
  } catch {
    return 'hex'
  }
  // Control bytes other than tab/newline/carriage-return indicate binary data.
  for (const char of text) {
    const code = char.codePointAt(0) ?? 0
    if (code < 0x20 && code !== 0x09 && code !== 0x0a && code !== 0x0d) {
      return 'hex'
    }
  }
  const trimmed = text.trim()
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    try {
      JSON.parse(trimmed)
      return 'json'
    } catch {
      return 'text'
    }
  }
  return 'text'
}
