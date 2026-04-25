import { describe, it, expect } from 'vitest'
import { parseNote, reassembleNote, stripWikiLinks } from './frontmatter.js'

describe('parseNote', () => {
  it('parses frontmatter and body', () => {
    const content = '---\ntags: [alpha, beta]\nstatus: in-progress\n---\n\n# My Note\n\nBody text.'
    const result = parseNote(content)

    expect(result.frontmatter.tags).toEqual(['alpha', 'beta'])
    expect(result.frontmatter.status).toBe('in-progress')
    expect(result.body).toContain('# My Note')
    expect(result.body).toContain('Body text.')
    expect(result.rawFrontmatter).toContain('---')
    expect(result.rawFrontmatter).toContain('tags:')
  })

  it('handles notes without frontmatter', () => {
    const content = '# Just a heading\n\nSome text.'
    const result = parseNote(content)

    expect(result.frontmatter).toEqual({})
    expect(result.rawFrontmatter).toBe('')
    expect(result.body).toBe('# Just a heading\n\nSome text.')
  })

  it('handles empty content', () => {
    const result = parseNote('')
    expect(result.frontmatter).toEqual({})
    expect(result.rawFrontmatter).toBe('')
    expect(result.body).toBe('')
  })

  it('parses complex frontmatter with arrays and nested values', () => {
    const content = [
      '---',
      'categories:',
      '  - "[[Clippings]]"',
      'author:',
      '  - "[[Kevin Kelly]]"',
      'url: https://example.com',
      'created: 2023-09-12',
      '---',
      '',
      '# Article Title',
    ].join('\n')
    const result = parseNote(content)

    expect(result.frontmatter.categories).toEqual(['[[Clippings]]'])
    expect(result.frontmatter.author).toEqual(['[[Kevin Kelly]]'])
    expect(result.frontmatter.url).toBe('https://example.com')
    expect(result.body).toContain('# Article Title')
  })
})

describe('reassembleNote', () => {
  it('prepends frontmatter to body', () => {
    const raw = '---\ntags: [alpha]\n---\n'
    const body = '# Title\n\nBody.'
    const result = reassembleNote(raw, body)
    expect(result).toBe('---\ntags: [alpha]\n---\n\n# Title\n\nBody.')
  })

  it('returns body alone when no frontmatter', () => {
    const result = reassembleNote('', '# Title\n\nBody.')
    expect(result).toBe('# Title\n\nBody.')
  })

  it('strips leading newlines from body before reassembly', () => {
    const raw = '---\ntags: [alpha]\n---\n'
    const body = '\n\n# Title'
    const result = reassembleNote(raw, body)
    expect(result).toBe('---\ntags: [alpha]\n---\n\n# Title')
  })
})

describe('stripWikiLinks', () => {
  it('removes [[brackets]] from wiki links', () => {
    expect(stripWikiLinks('[[Clippings]]')).toBe('Clippings')
  })

  it('handles multiple wiki links', () => {
    expect(stripWikiLinks('[[A]] and [[B]]')).toBe('A and B')
  })

  it('returns plain text unchanged', () => {
    expect(stripWikiLinks('plain text')).toBe('plain text')
  })
})
