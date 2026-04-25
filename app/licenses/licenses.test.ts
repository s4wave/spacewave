import { describe, expect, it } from 'vitest'
import {
  licenseEntries,
  licenseStats,
  groupByCategory,
  reconstructText,
} from './data.js'

// Approved SPDX license identifiers for this project.
const APPROVED_LICENSES = new Set([
  'MIT',
  'Apache-2.0',
  'Apache-2.0 OR MIT',
  'BSD-2-Clause',
  'BSD-3-Clause',
  'ISC',
  'MPL-2.0',
  'LGPL-2.1',
  'LGPL-2.1-only',
  'LGPL-3.0',
  'LGPL-3.0-only',
  'CC0-1.0',
  '0BSD',
  'Unlicense',
  'BlueOak-1.0.0',
])

describe('license compliance', () => {
  it('all dependencies use approved licenses', () => {
    const violations = licenseEntries.filter(
      (e) => e.spdx !== 'Unknown' && !APPROVED_LICENSES.has(e.spdx),
    )
    if (violations.length > 0) {
      const msg = violations.map((e) => `  ${e.name} (${e.spdx})`).join('\n')
      expect.fail(
        `${violations.length} dependencies have unapproved licenses:\n${msg}`,
      )
    }
  })

  it('no Unknown licenses in production dependencies', () => {
    const unknown = licenseEntries.filter(
      (e) => e.spdx === 'Unknown' && e.source !== 'go',
    )
    if (unknown.length > 0) {
      const msg = unknown.map((e) => `  ${e.name}`).join('\n')
      expect.fail(
        `${unknown.length} JS dependencies have Unknown licenses:\n${msg}`,
      )
    }
  })
})

describe('license data integrity', () => {
  it('has entries from both ecosystems', () => {
    const stats = licenseStats()
    expect(stats.goCount).toBeGreaterThan(0)
    expect(stats.jsCount).toBeGreaterThan(0)
    expect(stats.total).toBe(licenseEntries.length)
  })

  it('all entries have name, version, and spdx', () => {
    for (const entry of licenseEntries) {
      expect(entry.name).toBeTruthy()
      expect(entry.version).toBeTruthy()
      expect(entry.spdx).toBeTruthy()
      expect(['go', 'js', 'both']).toContain(entry.source)
    }
  })

  it('all entries exposed on community page have a package URL', () => {
    for (const entry of licenseEntries) {
      expect(entry.repo).toBeTruthy()
    }
  })

  it('aperture-managed forks point at Aperture-owned repos', () => {
    const expectedRepos = new Map([
      ['@aptre/it-ws', 'https://github.com/aperturerobotics/it-ws'],
      [
        '@aptre/protobuf-es-lite',
        'https://github.com/aperturerobotics/protobuf-es-lite',
      ],
      [
        'github.com/aperturerobotics/go-quickjs-wasi-reactor',
        'https://github.com/aperturerobotics/go-quickjs-wasi-reactor',
      ],
      [
        'github.com/aperturerobotics/bldr-saucer',
        'https://github.com/aperturerobotics/bldr-saucer',
      ],
    ])

    for (const [name, repo] of expectedRepos) {
      expect(licenseEntries.find((e) => e.name === name)?.repo).toBe(repo)
    }
  })

  it('can reconstruct text for deduped entries', () => {
    const deduped = licenseEntries.filter(
      (e) => e.copyrightNotice && !e.fullText,
    )
    expect(deduped.length).toBeGreaterThan(0)
    for (const entry of deduped) {
      const text = reconstructText(entry)
      expect(text).toBeTruthy()
      expect(text).toContain(entry.copyrightNotice!)
    }
  })

  it('groups by category produce non-empty groups', () => {
    const groups = groupByCategory(licenseEntries)
    expect(groups.length).toBeGreaterThan(0)
    for (const group of groups) {
      expect(group.entries.length).toBeGreaterThan(0)
      expect(group.category.id).toBeTruthy()
      expect(group.category.label).toBeTruthy()
    }
  })
})
