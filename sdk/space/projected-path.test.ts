import { describe, expect, it } from 'vitest'
import {
  buildProjectedObjectPath,
  buildProjectedSpaceRootPath,
  joinProjectedSubpath,
  normalizeProjectedSubpath,
  parseProjectedSpacePath,
} from './projected-path.js'

describe('projected Space paths', () => {
  it('builds projected space roots and UnixFS object roots', () => {
    expect(
      buildProjectedSpaceRootPath({
        sessionIndex: 7,
        sharedObjectId: 'space-1',
      }),
    ).toBe('u/7/so/space-1')
    expect(
      buildProjectedObjectPath({
        sessionIndex: 7,
        sharedObjectId: 'space-1',
        objectKey: 'docs/demo',
      }),
    ).toBe('u/7/so/space-1/-/docs/demo')
  })

  it('builds descendant paths with current encoded segment output', () => {
    expect(
      buildProjectedObjectPath({
        sessionIndex: 7,
        sharedObjectId: 'space-1',
        objectKey: 'docs/demo',
        path: '/nested/hello.txt',
      }),
    ).toBe('u/7/so/space-1/-/docs/demo/-/nested/hello.txt')
    expect(
      buildProjectedObjectPath({
        sessionIndex: 2,
        sharedObjectId: 'space id',
        objectKey: 'key/with spaces',
        path: '/child node/#1',
      }),
    ).toBe('u/2/so/space%20id/-/key/with%20spaces/-/child%20node/%231')
  })

  it('normalizes empty and duplicate-separator subpaths distinctly from object URI helpers', () => {
    expect(normalizeProjectedSubpath('')).toBe('')
    expect(normalizeProjectedSubpath('/')).toBe('')
    expect(normalizeProjectedSubpath('//nested//hello.txt')).toBe(
      'nested/hello.txt',
    )
    expect(joinProjectedSubpath(['/nested/', '/hello.txt'])).toBe(
      'nested/hello.txt',
    )
  })

  it('parses backend projected path cases', () => {
    expect(parseProjectedSpacePath('/u/0/so/my-space')).toMatchObject({
      sessionIndex: 0,
      sharedObjectId: 'my-space',
      path: 'u/0/so/my-space',
    })
    expect(
      parseProjectedSpacePath('/u/1/so/abc123/-/my-object/-/nested'),
    ).toMatchObject({
      sessionIndex: 1,
      sharedObjectId: 'abc123',
      path: 'u/1/so/abc123/-/my-object/-/nested',
      objectKey: 'my-object',
      objectPath: 'nested',
    })
    expect(
      parseProjectedSpacePath(
        '/u/2/so/space-id/-/key/with%20spaces/-/child%20node',
      ),
    ).toMatchObject({
      sessionIndex: 2,
      sharedObjectId: 'space-id',
      path: 'u/2/so/space-id/-/key/with spaces/-/child node',
      objectKey: 'key/with spaces',
      objectPath: 'child node',
    })
    expect(
      parseProjectedSpacePath('/u/3/so/space-id/-/foo/-/bar/-/child'),
    ).toMatchObject({
      sessionIndex: 3,
      sharedObjectId: 'space-id',
      path: 'u/3/so/space-id/-/foo/-/bar/-/child',
      objectKey: 'foo/-/bar',
      objectPath: 'child',
    })
    expect(parseProjectedSpacePath('/u/42/so/test-so')).toMatchObject({
      sessionIndex: 42,
      sharedObjectId: 'test-so',
      path: 'u/42/so/test-so',
    })
  })

  it('rejects malformed paths the backend projected parser rejects', () => {
    expect(() => parseProjectedSpacePath('')).toThrow('empty projected path')
    expect(() => parseProjectedSpacePath('/other/u/0/so/id')).toThrow(
      'invalid projected path format',
    )
    expect(() => parseProjectedSpacePath('/u/abc/so/id')).toThrow(
      'parse session index',
    )
    expect(() => parseProjectedSpacePath('/u/0/xx/id')).toThrow(
      'invalid projected path format',
    )
    expect(() => parseProjectedSpacePath('/u/0')).toThrow(
      'invalid projected path format',
    )
    expect(() => parseProjectedSpacePath('/u/0/so/%E0%A4%A')).toThrow()
  })
})
