import { describe, it, expect } from 'vitest'
import {
  splitPath,
  joinPath,
  joinPathPts,
  cleanSplitValidateRelativePath,
} from './path.js'

describe('splitPath', () => {
  it('splits a relative path into parts', () => {
    const result = splitPath('a/b/c')
    expect(result.parts).toEqual(['a', 'b', 'c'])
    expect(result.isAbsolute).toBe(false)
  })

  it('splits an absolute path and marks isAbsolute', () => {
    const result = splitPath('/a/b')
    expect(result.parts).toEqual(['a', 'b'])
    expect(result.isAbsolute).toBe(true)
  })

  it('returns empty parts for empty string', () => {
    const result = splitPath('')
    expect(result.parts).toEqual([])
    expect(result.isAbsolute).toBe(false)
  })

  it('returns empty parts for root path', () => {
    const result = splitPath('/')
    expect(result.parts).toEqual([])
    expect(result.isAbsolute).toBe(true)
  })

  it('returns empty parts for dot path', () => {
    const result = splitPath('.')
    expect(result.parts).toEqual([])
    expect(result.isAbsolute).toBe(false)
  })

  it('cleans multiple slashes', () => {
    const result = splitPath('a///b//c')
    expect(result.parts).toEqual(['a', 'b', 'c'])
    expect(result.isAbsolute).toBe(false)
  })

  it('resolves parent references', () => {
    const result = splitPath('a/b/../c')
    expect(result.parts).toEqual(['a', 'c'])
    expect(result.isAbsolute).toBe(false)
  })

  it('resolves dot segments', () => {
    const result = splitPath('a/./b/./c')
    expect(result.parts).toEqual(['a', 'b', 'c'])
    expect(result.isAbsolute).toBe(false)
  })

  it('handles single component', () => {
    const result = splitPath('foo')
    expect(result.parts).toEqual(['foo'])
    expect(result.isAbsolute).toBe(false)
  })

  it('handles absolute single component', () => {
    const result = splitPath('/foo')
    expect(result.parts).toEqual(['foo'])
    expect(result.isAbsolute).toBe(true)
  })
})

describe('joinPath', () => {
  it('joins relative parts', () => {
    expect(joinPath(['a', 'b'], false)).toBe('a/b')
  })

  it('joins absolute parts', () => {
    expect(joinPath(['a', 'b'], true)).toBe('/a/b')
  })

  it('returns dot for empty relative path', () => {
    expect(joinPath([], false)).toBe('.')
  })

  it('returns slash for empty absolute path', () => {
    expect(joinPath([], true)).toBe('/')
  })

  it('handles single component', () => {
    expect(joinPath(['foo'], false)).toBe('foo')
  })
})

describe('joinPathPts', () => {
  it('concatenates multiple part arrays', () => {
    expect(joinPathPts(['a', 'b'], ['c'], ['d', 'e'])).toEqual([
      'a',
      'b',
      'c',
      'd',
      'e',
    ])
  })

  it('returns empty array for no arguments', () => {
    expect(joinPathPts()).toEqual([])
  })

  it('returns the single array when one argument', () => {
    expect(joinPathPts(['a', 'b'])).toEqual(['a', 'b'])
  })
})

describe('cleanSplitValidateRelativePath', () => {
  it('cleans and splits a relative path with parent refs', () => {
    expect(cleanSplitValidateRelativePath('a/b/../c')).toEqual(['a', 'c'])
  })

  it('returns empty array for root path (coerced to relative)', () => {
    expect(cleanSplitValidateRelativePath('/')).toEqual([])
  })

  it('returns empty array for dot path', () => {
    expect(cleanSplitValidateRelativePath('.')).toEqual([])
  })

  it('returns empty array for empty path', () => {
    expect(cleanSplitValidateRelativePath('')).toEqual([])
  })

  it('strips leading slash from absolute path', () => {
    expect(cleanSplitValidateRelativePath('/a/b')).toEqual(['a', 'b'])
  })

  it('handles simple relative path', () => {
    expect(cleanSplitValidateRelativePath('a/b/c')).toEqual(['a', 'b', 'c'])
  })
})
