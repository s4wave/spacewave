import nodePath from 'node:path'
import { randomUUID } from 'node:crypto'
import {
  chmod,
  lstat,
  mkdir,
  open,
  readFile,
  rename,
  rm,
} from 'node:fs/promises'

import type {
  DesktopCLIEntrypointIdentity,
  DesktopCLIInstallTarget,
} from '../desktop-runtime/desktop-runtime.pb.js'
import type { DesktopCLIInstallProbe } from './desktop-cli-install-detector.js'
import { blockedTargetReason } from './desktop-cli-install-target-policy.js'

export interface DesktopCLIInstallFilesystem {
  readFile(path: string): Promise<Uint8Array>
  writeFileExclusive(path: string, data: Uint8Array): Promise<void>
  rename(oldPath: string, newPath: string): Promise<void>
  mkdir(path: string): Promise<void>
  chmod(path: string, mode: number): Promise<void>
  remove(path: string): Promise<void>
  pathKind(path: string): Promise<'missing' | 'file' | 'symlink' | 'other'>
}

export interface DesktopCLIInstallExecutorOpts {
  target: DesktopCLIInstallTarget
  installed?: DesktopCLIEntrypointIdentity
  available?: DesktopCLIEntrypointIdentity
  operation: 'install' | 'update'
  readReleaseBinary: (
    expected?: DesktopCLIEntrypointIdentity,
  ) => Promise<Uint8Array>
  probe: Pick<DesktopCLIInstallProbe, 'fileExists' | 'readEntrypointIdentity'>
  filesystem?: DesktopCLIInstallFilesystem
  now?: () => number
}

export async function executeDesktopCLIInstall(
  opts: DesktopCLIInstallExecutorOpts,
): Promise<void> {
  validateInstallRequest(opts)
  const fs = opts.filesystem ?? nodeFilesystem
  const targetPath = opts.target.path ?? ''
  const targetDir = nodePath.dirname(targetPath)
  const stamp = `${opts.now?.() ?? Date.now()}-${randomUUID()}`
  const tempPath = `${targetPath}.spacewave-tmp-${stamp}`
  const backupPath = `${targetPath}.spacewave-backup-${stamp}`
  const existing = await opts.probe.fileExists(targetPath)
  let backupCreated = false

  try {
    if (existing) {
      await assertRegularExistingTarget(fs, targetPath)
      await assertPathMissing(fs, backupPath)
      await assertExistingBinaryReplaceable(opts)
      await fs.rename(targetPath, backupPath)
      backupCreated = true
    }
    const bytes = await opts.readReleaseBinary(opts.available)
    if (bytes.byteLength === 0) {
      throw new Error('release CLI binary is empty')
    }
    await fs.mkdir(targetDir)
    await assertPathMissing(fs, tempPath)
    await fs.writeFileExclusive(tempPath, bytes)
    await fs.chmod(tempPath, 0o755)
    await fs.rename(tempPath, targetPath)
  } catch (err) {
    await restoreBackup(fs, targetPath, tempPath, backupPath, backupCreated)
    throw err
  }
}

function validateInstallRequest(opts: DesktopCLIInstallExecutorOpts): void {
  const targetPath = opts.target.path ?? ''
  if (!targetPath)
    throw new Error('desktop CLI install target path is required')
  if (!isAbsoluteTarget(targetPath, opts.available?.platformId ?? '')) {
    throw new Error('desktop CLI install target path must be absolute')
  }
  if (
    targetBasename(targetPath, opts.available?.platformId ?? '') !==
    commandNameForPlatform(opts.available?.platformId ?? '')
  ) {
    throw new Error('desktop CLI install target command name is invalid')
  }
  if (!(opts.target.writable ?? false)) {
    throw new Error('desktop CLI install target is not writable')
  }
  if (!isUserLevelTarget(targetPath)) {
    throw new Error('desktop CLI install target must be user-level')
  }
  if (opts.available?.entrypointRole !== 'cli') {
    throw new Error('release CLI entrypoint role is not trusted')
  }
  if (opts.available?.manifestId !== 'spacewave-cli') {
    throw new Error('release CLI Manifest identity is not trusted')
  }
  if (opts.available?.projectId !== 'spacewave') {
    throw new Error('release CLI project identity is not trusted')
  }
  if (!opts.available?.channelKey) {
    throw new Error('release CLI channel identity is required')
  }
  if (!opts.available?.platformId) {
    throw new Error('release CLI platform identity is required')
  }
  if (!opts.available?.manifestRev) {
    throw new Error('release CLI Manifest revision is required')
  }
  const blockedReason =
    opts.target.blockedReason ||
    blockedTargetReason(targetPath, opts.available?.platformId ?? '')
  if (blockedReason) {
    throw new Error(`desktop CLI install target is blocked: ${blockedReason}`)
  }
}

async function assertExistingBinaryReplaceable(
  opts: DesktopCLIInstallExecutorOpts,
): Promise<void> {
  const targetPath = opts.target.path ?? ''
  const identity =
    opts.installed?.manifestId && opts.installed?.entrypointRole
      ? opts.installed
      : await opts.probe.readEntrypointIdentity(targetPath)
  if (identity?.entrypointRole !== 'cli') {
    throw new Error('existing CLI binary is unmanaged')
  }
  if (identity.manifestId !== 'spacewave-cli') {
    throw new Error('existing CLI binary is unmanaged')
  }
  if (identity.projectId !== opts.available?.projectId) {
    throw new Error('existing CLI binary is unmanaged')
  }
  if (identity.channelKey !== opts.available?.channelKey) {
    throw new Error('existing CLI binary is unmanaged')
  }
  if (identity.platformId !== opts.available?.platformId) {
    throw new Error('existing CLI binary is unmanaged')
  }
}

async function restoreBackup(
  fs: DesktopCLIInstallFilesystem,
  targetPath: string,
  tempPath: string,
  backupPath: string,
  backupCreated: boolean,
): Promise<void> {
  await fs.remove(tempPath).catch(() => {})
  if (!backupCreated) return
  await fs.remove(targetPath).catch(() => {})
  await fs.rename(backupPath, targetPath)
}

function isUserLevelTarget(targetPath: string): boolean {
  const normalized = targetPath.replaceAll('\\', '/')
  if (normalized.includes('/../')) return false
  if (normalized.startsWith('/Applications/')) return false
  if (normalized.startsWith('/usr/')) return false
  if (normalized.startsWith('/opt/')) return false
  if (normalized.startsWith('/var/')) return false
  return true
}

async function assertRegularExistingTarget(
  fs: DesktopCLIInstallFilesystem,
  targetPath: string,
): Promise<void> {
  const kind = await fs.pathKind(targetPath)
  if (kind === 'file') return
  if (kind === 'missing') return
  throw new Error('desktop CLI install target is not a regular file')
}

async function assertPathMissing(
  fs: DesktopCLIInstallFilesystem,
  path: string,
): Promise<void> {
  if ((await fs.pathKind(path)) === 'missing') return
  throw new Error('desktop CLI install temporary path already exists')
}

function commandNameForPlatform(platformId: string): string {
  return platformId.includes('/windows/') ? 'spacewave.exe' : 'spacewave'
}

function isAbsoluteTarget(targetPath: string, platformId: string): boolean {
  return platformId.includes('/windows/')
    ? nodePath.win32.isAbsolute(targetPath)
    : nodePath.posix.isAbsolute(targetPath)
}

function targetBasename(targetPath: string, platformId: string): string {
  return platformId.includes('/windows/')
    ? nodePath.win32.basename(targetPath)
    : nodePath.posix.basename(targetPath)
}

const nodeFilesystem: DesktopCLIInstallFilesystem = {
  readFile,
  writeFileExclusive: async (path, data) => {
    const file = await open(path, 'wx', 0o755)
    try {
      await file.writeFile(data)
    } finally {
      await file.close()
    }
  },
  rename,
  mkdir: (path) => mkdir(path, { recursive: true }).then(() => undefined),
  chmod,
  remove: (path) => rm(path, { force: true }).then(() => undefined),
  pathKind: async (path) => {
    try {
      const stat = await lstat(path)
      if (stat.isSymbolicLink()) return 'symlink'
      if (stat.isFile()) return 'file'
      return 'other'
    } catch (err) {
      if (
        typeof err === 'object' &&
        err &&
        'code' in err &&
        err.code === 'ENOENT'
      ) {
        return 'missing'
      }
      throw err
    }
  },
}
