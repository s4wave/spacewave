import childProcess from 'node:child_process'
import { constants as fsConstants } from 'node:fs'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { promisify } from 'node:util'

import type { DesktopCLIEntrypointIdentity } from '../desktop-runtime/desktop-runtime.pb.js'
import type { ManagedCLIRelease } from '../../plugin/electron/electron.pb.js'
import {
  detectDesktopCLIInstallState,
  type DesktopCLIInstallProbe,
} from './desktop-cli-install-detector.js'

const execFile = promisify(childProcess.execFile)
const versionProbeTimeoutMs = 2_000
const managedCLIReleaseSidecarFilename = 'managed-cli-release.json'
const managedCLIReleaseRoots = new WeakMap<ManagedCLIRelease, string>()

export type ManagedCLIReleaseResolver = () => Promise<
  ManagedCLIRelease | undefined
>

export function buildDesktopCLIInstallProbe(): DesktopCLIInstallProbe {
  return {
    fileExists: async (targetPath) => {
      try {
        await fs.access(targetPath, fsConstants.F_OK)
        return true
      } catch {
        return false
      }
    },
    targetWritable: async (targetPath) => {
      return isDesktopCLIInstallTargetWritable(targetPath)
    },
    readEntrypointIdentity: readEntrypointIdentity,
  }
}

export async function isDesktopCLIInstallTargetWritable(
  targetPath: string,
): Promise<boolean> {
  return isDesktopCLIInstallDirWritable(path.dirname(targetPath))
}

async function isDesktopCLIInstallDirWritable(dir: string): Promise<boolean> {
  const parent = path.dirname(dir)
  if (!dir || dir === parent) return false
  try {
    const stat = await fs.lstat(dir)
    if (!stat.isDirectory()) return false
    await fs.access(dir, fsConstants.W_OK)
    return true
  } catch (err) {
    if (!isNodeErrorCode(err, 'ENOENT')) return false
    return isDesktopCLIInstallDirWritable(parent)
  }
}

export function buildDesktopCLIInstallDetector(
  resolveRelease: ManagedCLIReleaseResolver,
  probe: DesktopCLIInstallProbe,
) {
  return async (selectedTargetId?: string) => {
    const release = await resolveRelease()
    return detectDesktopCLIInstallState({
      homeDir: os.homedir(),
      pathEntries: processPathEntries(),
      platformId: nativePlatformId(),
      selectedTargetId,
      available: managedCLIReleaseIdentity(release),
      probe,
    })
  }
}

export function buildManagedCLIReleaseResolver(
  release: ManagedCLIRelease | undefined,
): ManagedCLIReleaseResolver {
  if (release?.binaryPath) {
    return async () => release
  }
  return readManagedCLIReleaseSidecar
}

export function managedCLIReleaseIdentity(
  release: ManagedCLIRelease | undefined,
): DesktopCLIEntrypointIdentity | undefined {
  if (!release?.binaryPath) return undefined
  return {
    path: release.binaryPath,
    projectId: release.projectId,
    entrypointRole: release.entrypointRole,
    channelKey: release.channelKey,
    manifestId: release.manifestId,
    manifestRev: release.manifestRev,
    platformId: release.platformId,
  }
}

export function readManagedCLIReleaseBinary(
  resolveRelease: ManagedCLIReleaseResolver,
): (expected?: DesktopCLIEntrypointIdentity) => Promise<Uint8Array> {
  return async (expected) => {
    const release = await resolveRelease()
    if (!release?.binaryPath)
      throw new Error('release CLI binary is unavailable')
    const available = managedCLIReleaseIdentity(release)
    if (!identityMatchesExpected(available, expected)) {
      throw new Error('release CLI binary changed; check again')
    }
    const trustedRoot = managedCLIReleaseRoots.get(release)
    if (trustedRoot) {
      await verifyNoSymlinkPath(trustedRoot, release.binaryPath)
    }
    return readRegularReleaseBinary(release.binaryPath)
  }
}

async function readRegularReleaseBinary(
  binaryPath: string,
): Promise<Uint8Array> {
  const stat = await fs.lstat(binaryPath)
  if (!stat.isFile())
    throw new Error('release CLI binary must be a regular file')
  return fs.readFile(binaryPath)
}

export function parseManagedCLIReleaseSidecar(
  data: string,
): ManagedCLIRelease | undefined {
  const parsed: unknown = JSON.parse(data)
  if (!isRecord(parsed)) return undefined
  return {
    binaryPath: stringField(parsed.binary_path ?? parsed.binaryPath),
    projectId: stringField(parsed.project_id ?? parsed.projectId),
    entrypointRole: stringField(
      parsed.entrypoint_role ?? parsed.entrypointRole,
    ),
    channelKey: stringField(parsed.channel_key ?? parsed.channelKey),
    manifestId: stringField(parsed.manifest_id ?? parsed.manifestId),
    manifestRev: bigintField(parsed.manifest_rev ?? parsed.manifestRev),
    platformId: stringField(parsed.platform_id ?? parsed.platformId),
  }
}

async function readManagedCLIReleaseSidecar(): Promise<
  ManagedCLIRelease | undefined
> {
  const stagingDir = managedCLIReleaseStagingDir()
  if (!stagingDir) return undefined
  const sidecarPath = path.join(stagingDir, managedCLIReleaseSidecarFilename)
  const data = await readOptionalTextFile(sidecarPath)
  if (!data) return undefined
  const release = parseManagedCLIReleaseSidecar(data)
  if (!release?.binaryPath) return undefined
  if (!path.isAbsolute(release.binaryPath)) return undefined
  if (!isPathInside(stagingDir, release.binaryPath)) return undefined
  try {
    await verifyNoSymlinkPath(stagingDir, release.binaryPath)
  } catch {
    return undefined
  }
  managedCLIReleaseRoots.set(release, stagingDir)
  return release
}

async function readOptionalTextFile(filePath: string): Promise<string> {
  try {
    return await fs.readFile(filePath, 'utf8')
  } catch (err) {
    if (isNodeErrorCode(err, 'ENOENT')) return ''
    throw err
  }
}

function managedCLIReleaseStagingDir(): string | undefined {
  if (process.platform === 'darwin') {
    return path.join(
      os.homedir(),
      'Library',
      'Application Support',
      'Spacewave',
      'updates',
    )
  }
  if (process.platform === 'win32') {
    const appData = process.env.APPDATA
    if (!appData) return undefined
    return path.join(appData, 'Spacewave', 'updates')
  }
  const dataHome =
    process.env.XDG_DATA_HOME || path.join(os.homedir(), '.local', 'share')
  return path.join(dataHome, 'spacewave', 'updates')
}

function isPathInside(parent: string, candidate: string): boolean {
  const rel = path.relative(parent, candidate)
  return !!rel && !rel.startsWith('..') && !path.isAbsolute(rel)
}

async function verifyNoSymlinkPath(
  rootPath: string,
  filePath: string,
): Promise<void> {
  const rel = path.relative(rootPath, filePath)
  if (!rel || rel.startsWith('..') || path.isAbsolute(rel)) {
    throw new Error('release CLI binary escapes staging root')
  }
  await verifyNoSymlinkPathParts(
    rootPath,
    rel.split(path.sep).filter(Boolean).slice(0, -1),
  )
}

async function verifyNoSymlinkPathParts(
  current: string,
  parts: string[],
): Promise<void> {
  const [head, ...tail] = parts
  if (!head) return
  const next = path.join(current, head)
  const stat = await fs.lstat(next)
  if (stat.isSymbolicLink()) {
    throw new Error('release CLI binary path must not contain symlink parents')
  }
  if (!stat.isDirectory()) {
    throw new Error('release CLI binary parent must be a directory')
  }
  await verifyNoSymlinkPathParts(next, tail)
}

function identityMatchesExpected(
  actual: DesktopCLIEntrypointIdentity | undefined,
  expected: DesktopCLIEntrypointIdentity | undefined,
): boolean {
  if (!actual) return false
  if (!expected?.manifestId) return true
  if (actual.projectId !== expected.projectId) return false
  if (actual.entrypointRole !== expected.entrypointRole) return false
  if (actual.channelKey !== expected.channelKey) return false
  if (actual.manifestId !== expected.manifestId) return false
  if ((actual.manifestRev ?? 0n) !== (expected.manifestRev ?? 0n)) return false
  if (actual.platformId !== expected.platformId) return false
  if (expected.path && actual.path !== expected.path) return false
  return true
}

async function readEntrypointIdentity(
  commandPath: string,
): Promise<DesktopCLIEntrypointIdentity | undefined> {
  try {
    const { stdout } = await execFile(commandPath, ['version', '--json'], {
      shell: false,
      timeout: versionProbeTimeoutMs,
      windowsHide: true,
      maxBuffer: 64 * 1024,
    })
    return parseEntrypointIdentity(stdout, commandPath)
  } catch {
    return undefined
  }
}

function parseEntrypointIdentity(
  data: string,
  commandPath: string,
): DesktopCLIEntrypointIdentity | undefined {
  const parsed = JSON.parse(data) as unknown
  if (!isRecord(parsed)) return undefined
  const manifest = parsed.manifest
  if (!isRecord(manifest)) return undefined
  return {
    path: commandPath,
    projectId: stringField(parsed.projectId),
    entrypointRole: stringField(parsed.entrypointRole),
    channelKey: stringField(parsed.channelKey),
    manifestId: stringField(manifest.manifestId),
    manifestRev: bigintField(manifest.rev),
    platformId: stringField(parsed.platformId),
  }
}

function nativePlatformId(): string {
  return `desktop/${platformOS()}/${process.arch}`
}

function platformOS(): string {
  if (process.platform === 'win32') return 'windows'
  return process.platform
}

function processPathEntries(): string[] {
  return (process.env.PATH ?? '')
    .split(path.delimiter)
    .map((entry) => entry.trim())
    .filter(Boolean)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function stringField(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function bigintField(value: unknown): bigint {
  if (typeof value === 'bigint') return value
  if (typeof value === 'number' && Number.isSafeInteger(value)) {
    return BigInt(value)
  }
  if (typeof value === 'string' && /^\d+$/.test(value)) {
    return BigInt(value)
  }
  return 0n
}

function isNodeErrorCode(err: unknown, code: string): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'code' in err &&
    err.code === code
  )
}
