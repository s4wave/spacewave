import { describe, expect, test } from 'vitest'
import {
  escapeObjectKey,
  joinObjectUriPath,
  parseObjectUri,
} from './object-uri.js'

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

// The cases must match TestParseSpacewaveURIEscapes in
// sdk/space/object-uri_test.go, which reads these keys back from these
// addresses, so the writer and the reader stay inverse.
describe('escapeObjectKey', () => {
  const cases: Array<[string, string]> = [
    ['My Repo', 'My%20Repo'],
    ['100%', '100%25'],
    ['repos/café', 'repos/caf%C3%A9'],
    ['/repos/x', '%2Frepos/x'],
    ['repos/x/', 'repos/x%2F'],
    ['a//b', 'a%2F%2Fb'],
    ['a/./b', 'a/%2E/b'],
    ['a/../b', 'a/%2E%2E/b'],
    ['a/-/b', 'a/%2D/b'],
    ['-', '%2D'],
  ]

  for (const [key, escaped] of cases) {
    test(`escapes ${JSON.stringify(key)}`, () => {
      expect(escapeObjectKey(key)).toBe(escaped)
    })
  }
})
