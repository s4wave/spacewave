import { describe, expect, it, vi } from 'vitest'

import {
  executeDesktopCLIInstall,
  type DesktopCLIInstallFilesystem,
} from './desktop-cli-install-executor.js'

describe('executeDesktopCLIInstall', () => {
  it('installs a missing managed CLI with temp write, chmod, and atomic rename', async () => {
    const fs = new MemoryInstallFilesystem()
    const targetPath = '/Users/test/bin/spacewave'

    await executeDesktopCLIInstall({
      operation: 'install',
      target: {
        id: 'home-bin',
        path: targetPath,
        writable: true,
        selected: true,
      },
      available: releaseIdentity(),
      readReleaseBinary: async () => bytes('managed-cli'),
      probe: {
        fileExists: async (path) => fs.exists(path),
        readEntrypointIdentity: vi.fn(),
      },
      filesystem: fs,
      now: () => 7,
    })

    expect(text(fs.files.get(targetPath))).toBe('managed-cli')
    expect(fs.modes.get(targetPath)).toBe(0o755)
    expect(
      fs.createdPaths.some((path) => path.includes('.spacewave-tmp-7-')),
    ).toBe(true)
    expect(
      Array.from(fs.files.keys()).some((path) =>
        path.includes('.spacewave-tmp-'),
      ),
    ).toBe(false)
  })

  it('restores the previous managed binary when update write fails', async () => {
    const fs = new MemoryInstallFilesystem()
    const targetPath = '/Users/test/bin/spacewave'
    fs.files.set(targetPath, bytes('previous-cli'))
    fs.failWriteIncludes = '.spacewave-tmp-8-'

    await expect(
      executeDesktopCLIInstall({
        operation: 'update',
        target: {
          id: 'home-bin',
          path: targetPath,
          writable: true,
          selected: true,
        },
        installed: releaseIdentity({ manifestRev: 8n, path: targetPath }),
        available: releaseIdentity({ manifestRev: 9n }),
        readReleaseBinary: async () => bytes('next-cli'),
        probe: {
          fileExists: async (path) => fs.exists(path),
          readEntrypointIdentity: vi.fn(),
        },
        filesystem: fs,
        now: () => 8,
      }),
    ).rejects.toThrow('write failed')

    expect(text(fs.files.get(targetPath))).toBe('previous-cli')
    expect(
      Array.from(fs.files.keys()).some((path) =>
        path.includes('.spacewave-backup-'),
      ),
    ).toBe(false)
  })

  it('refuses to replace unmanaged binaries', async () => {
    const fs = new MemoryInstallFilesystem()
    const targetPath = '/Users/test/bin/spacewave'
    fs.files.set(targetPath, bytes('custom-cli'))

    await expect(
      executeDesktopCLIInstall({
        operation: 'update',
        target: {
          id: 'home-bin',
          path: targetPath,
          writable: true,
          selected: true,
        },
        available: releaseIdentity({ manifestRev: 9n }),
        readReleaseBinary: async () => bytes('next-cli'),
        probe: {
          fileExists: async (path) => fs.exists(path),
          readEntrypointIdentity: async () => ({
            entrypointRole: 'standalone',
            manifestId: '',
          }),
        },
        filesystem: fs,
        now: () => 9,
      }),
    ).rejects.toThrow('existing CLI binary is unmanaged')

    expect(text(fs.files.get(targetPath))).toBe('custom-cli')
  })

  it('rejects symlink targets before replacing an existing binary', async () => {
    const fs = new MemoryInstallFilesystem()
    const targetPath = '/Users/test/bin/spacewave'
    fs.files.set(targetPath, bytes('previous-cli'))
    fs.symlinks.add(targetPath)

    await expect(
      executeDesktopCLIInstall({
        operation: 'update',
        target: {
          id: 'home-bin',
          path: targetPath,
          writable: true,
          selected: true,
        },
        installed: releaseIdentity({ manifestRev: 8n, path: targetPath }),
        available: releaseIdentity({ manifestRev: 9n }),
        readReleaseBinary: async () => bytes('next-cli'),
        probe: {
          fileExists: async (path) => fs.exists(path),
          readEntrypointIdentity: vi.fn(),
        },
        filesystem: fs,
        now: () => 10,
      }),
    ).rejects.toThrow('desktop CLI install target is not a regular file')

    expect(text(fs.files.get(targetPath))).toBe('previous-cli')
  })
})

class MemoryInstallFilesystem implements DesktopCLIInstallFilesystem {
  public readonly files = new Map<string, Uint8Array>()
  public readonly modes = new Map<string, number>()
  public readonly symlinks = new Set<string>()
  public readonly createdPaths: string[] = []
  public failWriteIncludes = ''

  public exists(path: string): boolean {
    return this.files.has(path)
  }

  public async readFile(path: string): Promise<Uint8Array> {
    const data = this.files.get(path)
    if (!data) throw new Error('not found')
    return data
  }

  public async writeFileExclusive(
    path: string,
    data: Uint8Array,
  ): Promise<void> {
    if (this.exists(path)) throw new Error('exists')
    if (this.failWriteIncludes && path.includes(this.failWriteIncludes)) {
      throw new Error('write failed')
    }
    this.createdPaths.push(path)
    this.files.set(path, new Uint8Array(data))
  }

  public async rename(oldPath: string, newPath: string): Promise<void> {
    const data = this.files.get(oldPath)
    if (!data) throw new Error('not found')
    this.files.set(newPath, data)
    this.files.delete(oldPath)
    const mode = this.modes.get(oldPath)
    if (mode !== undefined) {
      this.modes.set(newPath, mode)
      this.modes.delete(oldPath)
    }
    if (this.symlinks.delete(oldPath)) this.symlinks.add(newPath)
  }

  public async mkdir(_path: string): Promise<void> {}

  public async chmod(path: string, mode: number): Promise<void> {
    this.modes.set(path, mode)
  }

  public async remove(path: string): Promise<void> {
    this.files.delete(path)
    this.modes.delete(path)
    this.symlinks.delete(path)
  }

  public async pathKind(
    path: string,
  ): Promise<'missing' | 'file' | 'symlink' | 'other'> {
    if (this.symlinks.has(path)) return 'symlink'
    if (this.files.has(path)) return 'file'
    return 'missing'
  }
}

function releaseIdentity(opts: { manifestRev?: bigint; path?: string } = {}) {
  return {
    path: opts.path ?? '',
    projectId: 'spacewave',
    entrypointRole: 'cli',
    channelKey: 'stable',
    manifestId: 'spacewave-cli',
    manifestRev: opts.manifestRev ?? 9n,
    platformId: 'desktop/darwin/arm64',
  }
}

function bytes(value: string): Uint8Array {
  return new TextEncoder().encode(value)
}

function text(value: Uint8Array | undefined): string {
  if (!value) return ''
  return new TextDecoder().decode(value)
}
