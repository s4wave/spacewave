import { describe, expect, test } from 'vitest'
import {
  collectionKey,
  encodeRecordKey,
  metadataKey,
  parseCollectionKey,
  receiptKey,
} from './keys.js'

describe('collectionKey', () => {
  test('round-trips plain components', () => {
    const key = collectionKey('notes', 'user-1', 'items')
    expect(key).toBe('sync/v1/6e6f746573/757365722d31/collections/6974656d73')
    expect(parseCollectionKey(key)).toEqual({
      application: 'notes',
      scope: 'user-1',
      collection: 'items',
    })
  })

  test('round-trips slash, control, and Unicode components', () => {
    const cases = [
      ['a/b', 'sc\nope', 'colle\tction'],
      ['héllo', '日本語', '🙂'],
      ['\u0000ctl', '\u001f', 'a\u007fb'],
    ]
    for (const [app, scope, coll] of cases) {
      const key = collectionKey(app, scope, coll)
      expect(parseCollectionKey(key)).toEqual({
        application: app,
        scope: scope,
        collection: coll,
      })
    }
  })

  test('distinct identities never collide', () => {
    expect(collectionKey('a/b', 'c', 'd')).not.toBe(
      collectionKey('a', 'b/c', 'd'),
    )
    expect(collectionKey('a', 'b', 'c')).not.toBe(
      collectionKey('a', 'b', 'c/d'),
    )
    // Different component counts produce different byte streams.
    expect(collectionKey('ab', 'c', 'd')).not.toBe(
      collectionKey('a', 'bc', 'd'),
    )
  })

  test('rejects invalid components with TypeError naming the field', () => {
    for (const fn of [
      () => collectionKey('', 's', 'c'),
      () => collectionKey('a', '', 'c'),
      () => collectionKey('a', 's', ''),
      () => collectionKey('a', 's', '\ud800'),
      () => receiptKey('', 's'),
      () => receiptKey('a', '\udfff'),
    ]) {
      expect(fn).toThrow(TypeError)
      try {
        fn()
      } catch (e) {
        expect((e as Error).message).toMatch(/application|scope|collection/)
        expect((e as Error).message).not.toContain('d800')
      }
    }
  })

  test('rejects components over 256 UTF-8 bytes', () => {
    expect(() => collectionKey('a'.repeat(257), 's', 'c')).toThrow(TypeError)
    // 128 two-byte characters is exactly at the limit.
    expect(() => collectionKey('é'.repeat(128), 's', 'c')).not.toThrow()
    expect(() => collectionKey('é'.repeat(129), 's', 'c')).toThrow(TypeError)
  })
})

describe('receiptKey', () => {
  test('produces the canonical receipts shape', () => {
    expect(receiptKey('notes', 'user-1')).toBe(
      'sync/v1/6e6f746573/757365722d31/receipts',
    )
  })
})

describe('parseCollectionKey', () => {
  test('rejects malformed keys', () => {
    for (const key of [
      '',
      'sync/v1',
      'sync/v1/',
      'sync/v1/6e6f746573',
      'sync/v1/6e6f746573/757365722d31',
      'sync/v1/6e6f746573/757365722d31/collections',
      'sync/v1/6e6f746573/757365722d31/collections/',
      'sync/v1/6e6f746573/757365722d31/receipts',
      'sync/v1/6e6f746573/757365722d31/other/6974656d73',
      'sync/v1/6E6F746573/757365722d31/collections/6974656d73',
      'sync/v1/6e6f746573/757365722d31/collections/zz',
      'sync/v1/6e6f746573/757365722d31/collections/6e6f74657',
      'sync/v1//757365722d31/collections/6974656d73',
      'sync/v1/6e6f746573/757365722d31/collections/fffe',
      `sync/v1/${'6e'.repeat(257)}/757365722d31/collections/6974656d73`,
      'other/v1/6e6f746573/757365722d31/collections/6974656d73',
    ]) {
      expect(parseCollectionKey(key)).toBeUndefined()
    }
  })

  test('rejects the reserved metadata key', () => {
    expect(parseCollectionKey(metadataKey('notes'))).toBeUndefined()
  })

  test('parses every valid hex application, including short names', () => {
    // 6e6f7465 is 'note'.
    expect(
      parseCollectionKey(
        'sync/v1/6e6f7465/757365722d31/collections/6974656d73',
      ),
    ).toEqual({ application: 'note', scope: 'user-1', collection: 'items' })
  })

  test('round-trips a leading U+FEFF byte order mark', () => {
    const app = '\uFEFFapp'
    const key = collectionKey(app, 's', 'c')
    expect(parseCollectionKey(key)).toEqual({
      application: app,
      scope: 's',
      collection: 'c',
    })
  })

  test('accepts keys with odd but paired component boundaries', () => {
    // 'ab' as application, 'c' as scope, 'd' as collection is canonical.
    const key = collectionKey('ab', 'c', 'd')
    expect(parseCollectionKey(key)).toEqual({
      application: 'ab',
      scope: 'c',
      collection: 'd',
    })
  })
})

describe('encodeRecordKey', () => {
  test('returns raw UTF-8 bytes', () => {
    const bytes = encodeRecordKey('héllo 🙂')
    expect(bytes).toEqual(new TextEncoder().encode('héllo 🙂'))
  })

  test('accepts slash and control characters', () => {
    expect(encodeRecordKey('a/b\nc')).toEqual(
      new TextEncoder().encode('a/b\nc'),
    )
  })

  test('rejects empty, lone-surrogate, and oversize keys', () => {
    expect(() => encodeRecordKey('')).toThrow(TypeError)
    expect(() => encodeRecordKey('\ud800')).toThrow(TypeError)
    expect(() => encodeRecordKey('a'.repeat(1025))).toThrow(TypeError)
    expect(() => encodeRecordKey('é'.repeat(512))).not.toThrow()
    expect(() => encodeRecordKey('é'.repeat(513))).toThrow(TypeError)
  })
})
