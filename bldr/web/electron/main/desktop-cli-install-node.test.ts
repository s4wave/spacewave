import os from 'node:os'
import path from 'node:path'
import { mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises'

import { describe, expect, it } from 'vitest'

import {
  buildDesktopCLIInstallDetector,
  buildManagedCLIReleaseResolver,
  isDesktopCLIInstallTargetWritable,
  managedCLIReleaseIdentity,
  parseManagedCLIReleaseSidecar,
  readManagedCLIReleaseBinary,
  type ManagedCLIReleaseResolver,
} from './desktop-cli-install-node.js'

describe('desktop CLI install node release resolver', () => {
  it('parses launcher-managed CLI sidecar identity', () => {
    const release = parseManagedCLIReleaseSidecar(`{
      "binary_path": "/spacewave-test/updates/0.1.0/cli-dist/spacewave",
      "project_id": "spacewave",
      "entrypoint_role": "cli",
      "channel_key": "stable",
      "manifest_id": "spacewave-cli",
      "manifest_rev": 9,
      "platform_id": "desktop/darwin/arm64",
      "manifest_ref": "ignored-by-electron"
    }`)

    expect(release).toMatchObject({
      binaryPath: '/spacewave-test/updates/0.1.0/cli-dist/spacewave',
      projectId: 'spacewave',
      entrypointRole: 'cli',
      channelKey: 'stable',
      manifestId: 'spacewave-cli',
      manifestRev: 9n,
      platformId: 'desktop/darwin/arm64',
    })
    expect(managedCLIReleaseIdentity(release)).toMatchObject({
      path: release?.binaryPath,
      manifestId: 'spacewave-cli',
      manifestRev: 9n,
    })
  })

  it('detects the latest release identity from the resolver', async () => {
    const state = { rev: 9n }
    const resolver: ManagedCLIReleaseResolver = async () => ({
      binaryPath: '/spacewave-test/updates/0.1.0/cli-dist/spacewave',
      projectId: 'spacewave',
      entrypointRole: 'cli',
      channelKey: 'stable',
      manifestId: 'spacewave-cli',
      manifestRev: state.rev,
      platformId: 'desktop/darwin/arm64',
    })
    const detect = buildDesktopCLIInstallDetector(resolver, {
      fileExists: async () => false,
      targetWritable: async () => true,
      readEntrypointIdentity: async () => undefined,
    })

    expect((await detect()).available?.manifestRev).toBe(9n)
    state.rev = 10n
    expect((await detect()).available?.manifestRev).toBe(10n)
  })

  it('rejects stale release identity before reading bytes', async () => {
    const dir = await mkdtemp(path.join(os.tmpdir(), 'spacewave-cli-release-'))
    try {
      const binaryPath = path.join(dir, 'spacewave')
      await writeFile(binaryPath, new Uint8Array([1, 2, 3]))
      const readReleaseBinary = readManagedCLIReleaseBinary(async () => ({
        binaryPath,
        projectId: 'spacewave',
        entrypointRole: 'cli',
        channelKey: 'stable',
        manifestId: 'spacewave-cli',
        manifestRev: 10n,
        platformId: 'desktop/darwin/arm64',
      }))

      await expect(
        readReleaseBinary({
          path: binaryPath,
          projectId: 'spacewave',
          entrypointRole: 'cli',
          channelKey: 'stable',
          manifestId: 'spacewave-cli',
          manifestRev: 9n,
          platformId: 'desktop/darwin/arm64',
        }),
      ).rejects.toThrow('release CLI binary changed')
      const bytes = await readReleaseBinary({
        path: binaryPath,
        projectId: 'spacewave',
        entrypointRole: 'cli',
        channelKey: 'stable',
        manifestId: 'spacewave-cli',
        manifestRev: 10n,
        platformId: 'desktop/darwin/arm64',
      })
      expect(Array.from(bytes)).toEqual([1, 2, 3])
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('rejects symlink release binary reads', async () => {
    if (process.platform === 'win32') return
    const dir = await mkdtemp(path.join(os.tmpdir(), 'spacewave-cli-release-'))
    try {
      const binaryPath = path.join(dir, 'spacewave')
      const outsidePath = path.join(dir, 'outside')
      await writeFile(outsidePath, new Uint8Array([1, 2, 3]))
      await symlink(outsidePath, binaryPath)
      const readReleaseBinary = readManagedCLIReleaseBinary(async () => ({
        binaryPath,
        projectId: 'spacewave',
        entrypointRole: 'cli',
        channelKey: 'stable',
        manifestId: 'spacewave-cli',
        manifestRev: 10n,
        platformId: 'desktop/darwin/arm64',
      }))

      await expect(
        readReleaseBinary({
          path: binaryPath,
          projectId: 'spacewave',
          entrypointRole: 'cli',
          channelKey: 'stable',
          manifestId: 'spacewave-cli',
          manifestRev: 10n,
          platformId: 'desktop/darwin/arm64',
        }),
      ).rejects.toThrow('release CLI binary must be a regular file')
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('rejects sidecar release paths under symlink parents', async () => {
    if (process.platform === 'win32') return
    const homeDir = await mkdtemp(
      path.join(os.tmpdir(), 'spacewave-cli-sidecar-home-'),
    )
    const previousEnv = {
      HOME: process.env.HOME,
      XDG_DATA_HOME: process.env.XDG_DATA_HOME,
    }
    try {
      process.env.HOME = homeDir
      process.env.XDG_DATA_HOME = path.join(homeDir, '.local', 'share')
      const stagingDir = managedTestStagingDir(homeDir)
      const outsideDir = path.join(homeDir, 'outside-cli-dist')
      const linkDir = path.join(stagingDir, '0.1.0', 'cli-dist')
      await mkdir(path.dirname(linkDir), { recursive: true })
      await mkdir(outsideDir, { recursive: true })
      await writeFile(path.join(outsideDir, 'spacewave'), new Uint8Array([1]))
      await symlink(outsideDir, linkDir)
      await writeFile(
        path.join(stagingDir, 'managed-cli-release.json'),
        JSON.stringify({
          binary_path: path.join(linkDir, 'spacewave'),
          project_id: 'spacewave',
          entrypoint_role: 'cli',
          channel_key: 'stable',
          manifest_id: 'spacewave-cli',
          manifest_rev: 10,
          platform_id: 'desktop/darwin/arm64',
        }),
      )

      await expect(buildManagedCLIReleaseResolver(undefined)()).resolves.toBe(
        undefined,
      )
    } finally {
      restoreEnv('HOME', previousEnv.HOME)
      restoreEnv('XDG_DATA_HOME', previousEnv.XDG_DATA_HOME)
      await rm(homeDir, { recursive: true, force: true })
    }
  })

  it('treats missing target directories as writable when the nearest parent is writable', async () => {
    const dir = await mkdtemp(path.join(os.tmpdir(), 'spacewave-cli-target-'))
    try {
      await expect(
        isDesktopCLIInstallTargetWritable(
          path.join(dir, '.local', 'bin', 'spacewave'),
        ),
      ).resolves.toBe(true)

      const fileParent = path.join(dir, 'not-a-directory')
      await writeFile(fileParent, new Uint8Array([1]))
      await expect(
        isDesktopCLIInstallTargetWritable(
          path.join(fileParent, 'bin', 'spacewave'),
        ),
      ).resolves.toBe(false)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })
})

function managedTestStagingDir(homeDir: string): string {
  if (process.platform === 'darwin') {
    return path.join(
      homeDir,
      'Library',
      'Application Support',
      'Spacewave',
      'updates',
    )
  }
  return path.join(homeDir, '.local', 'share', 'spacewave', 'updates')
}

function restoreEnv(name: string, value: string | undefined): void {
  if (value === undefined) {
    delete process.env[name]
    return
  }
  process.env[name] = value
}
