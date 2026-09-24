// Canonical JSON codec for sync records and mutation fingerprints.
// This module owns the JsonValue contract: one component maps between
// in-memory values and deterministic bytes; fingerprints hash those bytes.

/**
 * JsonValue is the JSON data model accepted by the sync library.
 * Values are null, booleans, finite numbers, strings, arrays, and plain
 * objects (Object.prototype or null prototype). Null is distinct from an
 * absent record; the service handles that distinction separately.
 */
export type JsonValue =
  | null
  | boolean
  | number
  | string
  | JsonValue[]
  | { [key: string]: JsonValue }

/**
 * CanonicalJSON returns the deterministic compact JSON form of value.
 * Object keys sort lexically by code unit. Accepts only JsonValue shapes:
 * rejects undefined (including properties and array holes), bigint, symbols,
 * functions, non-finite numbers, cycles, class instances, accessor
 * properties, enumerable symbol keys, and extra enumerable array properties.
 * Repeated references without a cycle are allowed. Never calls toJSON or
 * getters. Throws TypeError with a field path and reason, never the value.
 */
export function canonicalJSON(value: unknown): string {
  const seen: unknown[] = []
  return writeValue(value, '$', seen)
}

/**
 * EncodeJSON returns canonicalJSON(value) as strict UTF-8 bytes.
 * See canonicalJSON for the value contract.
 */
export function encodeJSON(value: unknown): Uint8Array {
  return new TextEncoder().encode(canonicalJSON(value))
}

/**
 * DecodeJSON parses strict UTF-8 data as JSON and validates it as JsonValue.
 * Throws TypeError when the data is not valid UTF-8, not valid JSON, or not
 * a JsonValue (for example a non-finite number from a large exponent).
 */
export function decodeJSON(data: Uint8Array): JsonValue {
  let text: string
  try {
    text = new TextDecoder('utf-8', { fatal: true }).decode(data)
  } catch {
    throw new TypeError('invalid JSON at $: data is not valid UTF-8')
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    throw new TypeError('invalid JSON at $: data is not valid JSON')
  }
  // canonicalJSON is the validation owner; JSON.parse already created proper
  // data properties, so the parsed value is returned as-is.
  canonicalJSON(parsed)
  return parsed as JsonValue
}

// writeValue returns the canonical JSON text for value at path.
// seen holds active ancestor references; entries are removed on return, so
// repeated non-cyclic references stay allowed.
function writeValue(value: unknown, path: string, seen: unknown[]): string {
  switch (typeof value) {
    case 'boolean':
      return value ? 'true' : 'false'
    case 'number':
      if (!Number.isFinite(value)) {
        throw new TypeError(`invalid JSON at ${path}: non-finite number`)
      }
      return JSON.stringify(value)
    case 'string':
      return JSON.stringify(value)
    case 'object':
      if (value === null) return 'null'
      break
    default:
      // undefined, bigint, symbol, function.
      throw new TypeError(
        `invalid JSON at ${path}: ${typeof value} is not a JSON value`,
      )
  }
  for (const ancestor of seen) {
    if (ancestor === value) {
      throw new TypeError(`invalid JSON at ${path}: cycle`)
    }
  }
  seen.push(value)
  const out = Array.isArray(value)
    ? writeArray(value, path, seen)
    : writeObject(value as Record<string, unknown>, path, seen)
  seen.pop()
  return out
}

// writeArray returns the canonical form of an array. Every index in
// [0, length) must be an own enumerable data property; any other own key is
// rejected when enumerable.
function writeArray(
  value: readonly unknown[],
  path: string,
  seen: unknown[],
): string {
  const len = value.length
  const parts = Array.from<string>({ length: len })
  const present = Array.from({ length: len }, () => false)
  for (const key of Reflect.ownKeys(value)) {
    const desc = Object.getOwnPropertyDescriptor(value, key)!
    if (desc.get !== undefined || desc.set !== undefined) {
      throw new TypeError(
        `invalid JSON at ${path}[${String(key)}]: accessor property`,
      )
    }
    if (typeof key === 'symbol') {
      if (desc.enumerable) {
        throw new TypeError(
          `invalid JSON at ${path}: enumerable symbol key is not supported`,
        )
      }
      continue
    }
    const idx = Number(key)
    if (Number.isInteger(idx) && idx >= 0 && idx < len && String(idx) === key) {
      present[idx] = true
      parts[idx] = writeValue(desc.value, `${path}[${idx}]`, seen)
      continue
    }
    if (desc.enumerable) {
      throw new TypeError(
        `invalid JSON at ${path}: extra enumerable array property ${JSON.stringify(key)}`,
      )
    }
  }
  for (let i = 0; i < len; i++) {
    if (!present[i]) {
      throw new TypeError(
        `invalid JSON at ${path}[${i}]: array hole (missing index)`,
      )
    }
  }
  return `[${parts.join(',')}]`
}

// writeObject returns the canonical form of a plain object with keys sorted
// lexically. Only enumerable own string keys are encoded; accessors are
// rejected and enumerable symbol keys are rejected.
function writeObject(
  value: Record<string, unknown>,
  path: string,
  seen: unknown[],
): string {
  const proto = Object.getPrototypeOf(value)
  if (proto !== Object.prototype && proto !== null) {
    throw new TypeError(`invalid JSON at ${path}: not a plain object`)
  }
  const entries: { key: string; json: string }[] = []
  for (const key of Reflect.ownKeys(value)) {
    const desc = Object.getOwnPropertyDescriptor(value, key)!
    if (desc.get !== undefined || desc.set !== undefined) {
      throw new TypeError(
        `invalid JSON at ${path}.${String(key)}: accessor property`,
      )
    }
    if (typeof key === 'symbol') {
      if (desc.enumerable) {
        throw new TypeError(
          `invalid JSON at ${path}: enumerable symbol key is not supported`,
        )
      }
      continue
    }
    if (!desc.enumerable) continue
    entries.push({ key, json: writeValue(desc.value, `${path}.${key}`, seen) })
  }
  entries.sort((a, b) => (a.key < b.key ? -1 : a.key > b.key ? 1 : 0))
  const parts = entries.map((e) => `${JSON.stringify(e.key)}:${e.json}`)
  return `{${parts.join(',')}}`
}
