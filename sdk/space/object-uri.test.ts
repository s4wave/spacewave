import { describe, expect, test } from 'vitest'
import { joinObjectUriPath, parseObjectUri } from './object-uri.js'

// The cases must match sdk/space/object-uri_test.go exactly so both runtimes
// parse identically.
describe('parseObjectUri', () => {
  const cases: Array<[string, string, string]> = [
    ['some/object/key', 'some/object/key', ''],
    ['some/object/key/-/foo/bar', 'some/object/key', 'foo/bar'],
    ['some/object/key/-', 'some/object/key', ''],
    ['key/-/', 'key', ''],
    ['key/-/foo', 'key', 'foo'],
    ['-/foo', 'foo', ''],
    ['-/-/foo', '-/foo', ''],
    ['-', '', ''],
    ['-/', '', ''],
  ]

  for (const [uri, objectKey, path] of cases) {
    test(`parses ${JSON.stringify(uri)}`, () => {
      expect(parseObjectUri(uri)).toEqual({ objectKey, path })
    })
  }
})

describe('joinObjectUriPath', () => {
  test('filters empty segments and drops a trailing dash marker', () => {
    expect(joinObjectUriPath(['a', '', 'b', '-'], false)).toBe('a/b')
  })

  test('prefixes the separator when absolute', () => {
    expect(joinObjectUriPath(['a', 'b'], true)).toBe('/a/b')
  })
})
