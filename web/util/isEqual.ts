type SeenPairs = WeakMap<object, WeakMap<object, true>>

const objectToString = Object.prototype.toString

function sameValueZero(value: unknown, other: unknown): boolean {
  return value === other || (value !== value && other !== other)
}

function isObjectLike(value: unknown): value is object {
  return (
    value !== null && (typeof value === 'object' || typeof value === 'function')
  )
}

function getTag(value: unknown): string {
  return objectToString.call(value)
}

function hasSeenPair(seen: SeenPairs, value: object, other: object): boolean {
  return seen.get(value)?.has(other) ?? false
}

function markSeenPair(seen: SeenPairs, value: object, other: object): void {
  let matches = seen.get(value)
  if (!matches) {
    matches = new WeakMap()
    seen.set(value, matches)
  }
  matches.set(other, true)
}

function compareArrayBuffers(
  value: ArrayBufferLike,
  other: ArrayBufferLike,
): boolean {
  if (value.byteLength !== other.byteLength) {
    return false
  }
  const valueBytes = new Uint8Array(value)
  const otherBytes = new Uint8Array(other)
  for (let index = 0; index < valueBytes.length; index += 1) {
    if (valueBytes[index] !== otherBytes[index]) {
      return false
    }
  }
  return true
}

function compareDataViews(value: DataView, other: DataView): boolean {
  if (
    value.byteLength !== other.byteLength ||
    value.byteOffset !== other.byteOffset
  ) {
    return false
  }
  return compareArrayBuffers(value.buffer, other.buffer)
}

function compareArrayBufferViews(
  value: ArrayBufferView,
  other: ArrayBufferView,
): boolean {
  if (
    value.constructor !== other.constructor ||
    value.byteLength !== other.byteLength
  ) {
    return false
  }

  const valueBytes = new Uint8Array(
    value.buffer,
    value.byteOffset,
    value.byteLength,
  )
  const otherBytes = new Uint8Array(
    other.buffer,
    other.byteOffset,
    other.byteLength,
  )
  for (let index = 0; index < valueBytes.length; index += 1) {
    if (valueBytes[index] !== otherBytes[index]) {
      return false
    }
  }
  return true
}

function compareArrays(
  value: readonly unknown[],
  other: readonly unknown[],
  seen: SeenPairs,
): boolean {
  if (value.length !== other.length) {
    return false
  }
  for (let index = 0; index < value.length; index += 1) {
    if (!deepEqual(value[index], other[index], seen)) {
      return false
    }
  }
  return true
}

function compareMaps(
  value: ReadonlyMap<unknown, unknown>,
  other: ReadonlyMap<unknown, unknown>,
  seen: SeenPairs,
): boolean {
  if (value.size !== other.size) {
    return false
  }

  const matched = new Set<number>()
  const otherEntries = Array.from(other.entries())
  for (const valueEntry of value.entries()) {
    let found = false
    for (let index = 0; index < otherEntries.length; index += 1) {
      if (matched.has(index)) {
        continue
      }
      const otherEntry = otherEntries[index]
      if (deepEqual(valueEntry, otherEntry, seen)) {
        matched.add(index)
        found = true
        break
      }
    }
    if (!found) {
      return false
    }
  }
  return true
}

function compareSets(
  value: ReadonlySet<unknown>,
  other: ReadonlySet<unknown>,
  seen: SeenPairs,
): boolean {
  if (value.size !== other.size) {
    return false
  }

  const matched = new Set<number>()
  const otherValues = Array.from(other.values())
  for (const valueItem of value.values()) {
    let found = false
    for (let index = 0; index < otherValues.length; index += 1) {
      if (matched.has(index)) {
        continue
      }
      if (deepEqual(valueItem, otherValues[index], seen)) {
        matched.add(index)
        found = true
        break
      }
    }
    if (!found) {
      return false
    }
  }
  return true
}

function compareObjects(
  value: Record<string, unknown>,
  other: Record<string, unknown>,
  seen: SeenPairs,
): boolean {
  const valueKeys = Object.keys(value)
  const otherKeys = Object.keys(other)
  if (valueKeys.length !== otherKeys.length) {
    return false
  }

  for (const key of valueKeys) {
    if (!Object.prototype.hasOwnProperty.call(other, key)) {
      return false
    }
    if (!deepEqual(value[key], other[key], seen)) {
      return false
    }
  }

  const valueCtor = value.constructor
  const otherCtor = other.constructor
  if (
    valueCtor !== otherCtor &&
    'constructor' in value &&
    'constructor' in other &&
    !(
      typeof valueCtor === 'function' &&
      valueCtor instanceof valueCtor &&
      typeof otherCtor === 'function' &&
      otherCtor instanceof otherCtor
    )
  ) {
    return false
  }

  return true
}

function deepEqual(value: unknown, other: unknown, seen: SeenPairs): boolean {
  if (sameValueZero(value, other)) {
    return true
  }
  if (!isObjectLike(value) || !isObjectLike(other)) {
    return false
  }
  if (hasSeenPair(seen, value, other)) {
    return true
  }

  const valueTag = getTag(value)
  const otherTag = getTag(other)
  if (valueTag !== otherTag) {
    return false
  }

  markSeenPair(seen, value, other)
  markSeenPair(seen, other, value)

  if (Array.isArray(value) && Array.isArray(other)) {
    return compareArrays(value, other, seen)
  }

  if (ArrayBuffer.isView(value) && ArrayBuffer.isView(other)) {
    if (value instanceof DataView && other instanceof DataView) {
      return compareDataViews(value, other)
    }
    if (value instanceof DataView || other instanceof DataView) {
      return false
    }
    return compareArrayBufferViews(value, other)
  }

  switch (valueTag) {
    case '[object ArrayBuffer]':
      return compareArrayBuffers(value as ArrayBuffer, other as ArrayBuffer)
    case '[object Boolean]':
    case '[object Date]':
    case '[object Number]':
      return sameValueZero(Number(value), Number(other))
    case '[object Error]':
      return (
        (value as Error).name === (other as Error).name &&
        (value as Error).message === (other as Error).message
      )
    case '[object Map]':
      return compareMaps(
        value as ReadonlyMap<unknown, unknown>,
        other as ReadonlyMap<unknown, unknown>,
        seen,
      )
    case '[object RegExp]':
      return (
        RegExp.prototype.toString.call(value) ===
        RegExp.prototype.toString.call(other)
      )
    case '[object String]':
      return (
        String.prototype.valueOf.call(value) ===
        String.prototype.valueOf.call(other)
      )
    case '[object Set]':
      return compareSets(
        value as ReadonlySet<unknown>,
        other as ReadonlySet<unknown>,
        seen,
      )
    case '[object Symbol]':
      return (
        Symbol.prototype.valueOf.call(value) ===
        Symbol.prototype.valueOf.call(other)
      )
    case '[object Arguments]':
    case '[object Object]':
      return compareObjects(
        value as Record<string, unknown>,
        other as Record<string, unknown>,
        seen,
      )
    default:
      return false
  }
}

export default function isEqual(value: unknown, other: unknown): boolean {
  return deepEqual(value, other, new WeakMap())
}
