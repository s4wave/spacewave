import { describe, expect, it } from 'vitest'
import {
  getUnixFSBaseName,
  getUnixFSParentPath,
  getUnixFSRelativePath,
  isSameOrChildUnixFSPath,
  joinUnixFSDisplayPath,
  normalizeUnixFSDisplayPath,
  normalizeUnixFSLookupPath,
  splitUnixFSPath,
} from './path.js'

describe('UnixFS path primitives', () => {
  it('normalizes lookup and display paths without changing root identity', () => {
    expect(normalizeUnixFSLookupPath('')).toBe('')
    expect(normalizeUnixFSLookupPath('/')).toBe('')
    expect(normalizeUnixFSLookupPath('.')).toBe('')
    expect(normalizeUnixFSLookupPath('/docs//report.md')).toBe('docs/report.md')
    expect(normalizeUnixFSLookupPath('/docs/./images')).toBe('docs/images')
    expect(normalizeUnixFSLookupPath('/docs/../images')).toBe('images')
    expect(normalizeUnixFSDisplayPath('/docs//report.md')).toBe(
      '/docs/report.md',
    )
  })

  it('joins display paths and path segments', () => {
    expect(joinUnixFSDisplayPath('/', 'docs')).toBe('/docs')
    expect(joinUnixFSDisplayPath('/docs', 'nested/report.md')).toBe(
      '/docs/nested/report.md',
    )
    expect(joinUnixFSDisplayPath('/docs/', '/nested/', 'report.md')).toBe(
      '/docs/nested/report.md',
    )
    expect(joinUnixFSDisplayPath('/docs', '..', 'images')).toBe('/images')
  })

  it('names parent, base, relative, and descendant relationships', () => {
    expect(splitUnixFSPath('/docs/report.md')).toEqual(['docs', 'report.md'])
    expect(getUnixFSParentPath('/docs/report.md')).toBe('/docs')
    expect(getUnixFSParentPath('/report.md')).toBe('/')
    expect(getUnixFSBaseName('/docs/report.md')).toBe('report.md')
    expect(getUnixFSRelativePath('/docs', '/docs/nested/report.md')).toBe(
      'nested/report.md',
    )
    expect(getUnixFSRelativePath('/', '/docs/report.md')).toBe('docs/report.md')
    expect(isSameOrChildUnixFSPath('/docs', '/docs/nested')).toBe(true)
    expect(isSameOrChildUnixFSPath('/docs', '/other/docs')).toBe(false)
  })
})
