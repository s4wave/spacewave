// Copyright 2019 Google LLC.
// SPDX-License-Identifier: Apache-2.0
// https://gist.github.com/inexorabletash/5462871
// https://github.com/w3c/IndexedDB/issues/47

// IDBKeyRangeWithPrefix returns an IDBKeyRange from the given prefix.
//
// Basic p(r)olyfill for proposed feature
//
// This defines no successor of empty arrays, so the range for prefix
// [] or [[],[]] has no upper bound.
// An alternate definition would preclude adding additional nesting,
// so the range for prefix [] would have upper bound [[]] and the
// range for prefix [[], []] would have upper bound [[], [[]]].
export function IDBKeyRangeWithPrefix(prefix: any): IDBKeyRange {
  // Ensure prefix is a valid key itself:
  if (indexedDB.cmp(prefix, prefix) !== 0) throw new TypeError()

  var MAX_DATE_VALUE = 8640000000000000
  var UPPER_BOUND = {
    NUMBER: new Date(-MAX_DATE_VALUE),
    DATE: '',
    STRING: [],
    ARRAY: undefined,
  }

  var upperKey = successor(prefix)
  if (upperKey === undefined) return IDBKeyRange.lowerBound(prefix)
  return IDBKeyRange.bound(prefix, upperKey, false, true)

  function successor(key: any) {
    if (typeof key === 'number') {
      if (key === Infinity) return UPPER_BOUND.NUMBER
      if (key === -Infinity) return -Number.MAX_VALUE
      if (key === 0) return Number.MIN_VALUE
      var epsilon = Math.abs(key)
      while (key + epsilon / 2 !== key) epsilon = epsilon / 2
      return key + epsilon
    }

    if (key instanceof Date) {
      if (key.valueOf() + 1 > MAX_DATE_VALUE) return UPPER_BOUND.DATE
      return new Date(key.valueOf() + 1)
    }

    if (typeof key === 'string') {
      var len = key.length
      while (len > 0) {
        var head = key.substring(0, len - 1),
          tail = key.charCodeAt(len - 1)
        if (tail !== 0xffff) return head + String.fromCharCode(tail + 1)
        key = head
        --len
      }
      return UPPER_BOUND.STRING
    }

    if (Array.isArray(key)) {
      key = key.slice() // Operate on a copy.
      len = key.length
      while (len > 0) {
        tail = successor(key.pop())
        if (tail !== undefined) {
          key.push(tail)
          return key
        }
        --len
      }
      return UPPER_BOUND.ARRAY
    }

    throw new TypeError()
  }
}
