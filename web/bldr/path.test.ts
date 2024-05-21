import { describe, it, expect } from 'vitest'
import { cleanPath, joinPath, splitPath } from './path.js'

describe('cleanPath', () => {
  interface PathTest {
    path: string
    result: string
  }

  const cleanTests: PathTest[] = [
    // Already clean
    { path: '', result: '.' },
    { path: 'abc', result: 'abc' },
    { path: 'abc/def', result: 'abc/def' },
    { path: 'a/b/c', result: 'a/b/c' },
    { path: '.', result: '.' },
    { path: '..', result: '..' },
    { path: '../..', result: '../..' },
    { path: '../../abc', result: '../../abc' },
    { path: '/abc', result: '/abc' },
    { path: '/', result: '/' },

    // Remove trailing slash
    { path: 'abc/', result: 'abc' },
    { path: 'abc/def/', result: 'abc/def' },
    { path: 'a/b/c/', result: 'a/b/c' },
    { path: './', result: '.' },
    { path: '../', result: '..' },
    { path: '../../', result: '../..' },
    { path: '/abc/', result: '/abc' },

    // Remove doubled slash
    { path: 'abc//def//ghi', result: 'abc/def/ghi' },
    { path: '//abc', result: '/abc' },
    { path: '///abc', result: '/abc' },
    { path: '//abc//', result: '/abc' },
    { path: 'abc//', result: 'abc' },

    // Remove . elements
    { path: 'abc/./def', result: 'abc/def' },
    { path: '/./abc/def', result: '/abc/def' },
    { path: 'abc/.', result: 'abc' },

    // Remove .. elements
    { path: 'abc/def/ghi/../jkl', result: 'abc/def/jkl' },
    { path: 'abc/def/../ghi/../jkl', result: 'abc/jkl' },
    { path: 'abc/def/..', result: 'abc' },
    { path: 'abc/def/../..', result: '.' },
    { path: '/abc/def/../..', result: '/' },
    { path: 'abc/def/../../..', result: '..' },
    { path: '/abc/def/../../..', result: '/' },
    { path: 'abc/def/../../../ghi/jkl/../../../mno', result: '../../mno' },

    // Combinations
    { path: 'abc/./../def', result: 'def' },
    { path: 'abc//./../def', result: 'def' },
    { path: 'abc/../../././../def', result: '../../def' },
  ]
  cleanTests.forEach(({ path, result }) => {
    it(`should clean ${path} to ${result}`, () => {
      expect(cleanPath(path)).toBe(result)
      expect(cleanPath(result)).toBe(result)
    })
  })
})

// Test Cases
describe('joinPath and splitPath', () => {
  const testCases = [
    { path: '/a/b/c/../..', expected: '/a' },
    { path: '/../../..', expected: '/' },
    { path: 'a/b/c', expected: 'a/b/c' },
    { path: './a', expected: 'a' },
    { path: 'abc/.', expected: 'abc' },
    { path: 'abc/def/../..', expected: '.' },
    { path: '', expected: '.' },
    { path: '..', expected: '..' },
  ]

  it('should correctly clean paths', () => {
    testCases.forEach(({ path, expected }) => {
      expect(cleanPath(path)).toBe(expected)
    })
  })

  // Testing splitPath and joinPath
  const splitJoinTestCases = [
    { path: '/a/b/c', isAbsolute: true },
    { path: 'a/b/c', isAbsolute: false },
    { path: '/.././a/b/..', isAbsolute: true },
    { path: './a/../b/c', isAbsolute: false },
  ]

  it('should correctly split and join paths', () => {
    splitJoinTestCases.forEach(({ path, isAbsolute }) => {
      const { pathParts, isAbsolute: splitIsAbsolute } = splitPath(path)
      const joinedPath = joinPath(pathParts, splitIsAbsolute)
      expect(joinedPath).toBe(cleanPath(path))
      expect(splitIsAbsolute).toBe(isAbsolute)
    })
  })
})
