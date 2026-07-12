import { describe, expect, it } from 'vitest'

import {
  DOWNLOAD_MANIFEST,
  releaseAssetUrl,
  resolveInstallerEntries,
  resolvePrimaryEntry,
} from './manifest.js'

describe('resolvePrimaryEntry', () => {
  it('returns null when detection missed', () => {
    expect(resolvePrimaryEntry(null, DOWNLOAD_MANIFEST)).toBeNull()
  })

  it('returns the first matching entry (installer) for a supported tuple', () => {
    const entry = resolvePrimaryEntry(
      { os: 'macos', arch: 'arm64' },
      DOWNLOAD_MANIFEST,
    )
    expect(entry?.kind).toBe('installer')
    expect(entry?.filename).toBe('spacewave-macos-arm64.dmg')
  })

  it('returns the cli entry when kind=cli is specified', () => {
    const entry = resolvePrimaryEntry(
      { os: 'macos', arch: 'arm64' },
      DOWNLOAD_MANIFEST,
      'cli',
    )
    expect(entry?.kind).toBe('cli')
    expect(entry?.filename).toBe('spacewave-cli-macos-arm64.zip')
  })

  it('returns the installer entry when kind=installer is specified', () => {
    const entry = resolvePrimaryEntry(
      { os: 'linux', arch: 'arm64' },
      DOWNLOAD_MANIFEST,
      'installer',
    )
    expect(entry?.kind).toBe('installer')
    expect(entry?.filename).toBe('spacewave-linux-arm64.AppImage')
  })

  it('returns null for a tuple absent from the manifest', () => {
    const entry = resolvePrimaryEntry(
      { os: 'macos', arch: 'amd64' },
      // Manifest containing only arm64 macOS.
      [DOWNLOAD_MANIFEST[0]],
    )
    expect(entry).toBeNull()
  })

  it('marks windows installer entries as unsigned during the interim period', () => {
    const entry = resolvePrimaryEntry(
      { os: 'windows', arch: 'amd64' },
      DOWNLOAD_MANIFEST,
      'installer',
    )
    expect(entry?.unsigned).toBe(true)
    expect(entry?.ext).toBe('zip')
  })

  it('does not mark macos entries as unsigned', () => {
    const entry = resolvePrimaryEntry(
      { os: 'macos', arch: 'arm64' },
      DOWNLOAD_MANIFEST,
    )
    expect(entry?.unsigned).toBeUndefined()
  })

  it('covers the same host matrix for cli as for installers', () => {
    const installerHosts = DOWNLOAD_MANIFEST.flatMap((e) =>
      e.kind === 'installer' ? [`${e.os}-${e.arch}`] : [],
    )
    const cliHosts = DOWNLOAD_MANIFEST.flatMap((e) =>
      e.kind === 'cli' ? [`${e.os}-${e.arch}`] : [],
    )
    expect(cliHosts.toSorted()).toEqual(installerHosts.toSorted())
  })

  it('uses cli filename naming convention spacewave-cli-{os}-{arch}.{ext}', () => {
    for (const e of DOWNLOAD_MANIFEST.filter((x) => x.kind === 'cli')) {
      expect(e.filename.startsWith('spacewave-cli-')).toBe(true)
      const expectedExt = e.os === 'linux' ? 'tar.gz' : 'zip'
      expect(e.ext).toBe(expectedExt)
    }
  })
})

describe('releaseAssetUrl', () => {
  it('points at the exact release tag download path', () => {
    expect(releaseAssetUrl('spacewave-macos-arm64.dmg', '0.53.1')).toBe(
      'https://github.com/s4wave/spacewave/releases/download/v0.53.1/spacewave-macos-arm64.dmg',
    )
  })
})

describe('resolveInstallerEntries', () => {
  it('returns only installer entries with latest urls when version is null', () => {
    const entries = resolveInstallerEntries(null)
    expect(entries.length).toBeGreaterThan(0)
    expect(entries.every((e) => e.kind === 'installer')).toBe(true)
    for (const e of entries) {
      expect(e.url).toContain('/releases/latest/download/')
    }
  })

  it('repoints every installer url at the exact tag for a concrete version', () => {
    const entries = resolveInstallerEntries('0.53.1')
    expect(entries.every((e) => e.kind === 'installer')).toBe(true)
    for (const e of entries) {
      expect(e.url).toBe(
        `https://github.com/s4wave/spacewave/releases/download/v0.53.1/${e.filename}`,
      )
    }
  })

  it('preserves the manifest arch/os matrix regardless of version', () => {
    const latest = resolveInstallerEntries(null).map((e) => e.filename)
    const pinned = resolveInstallerEntries('0.53.1').map((e) => e.filename)
    expect(pinned.toSorted()).toEqual(latest.toSorted())
  })
})
