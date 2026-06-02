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
      try {
        await fs.access(path.dirname(targetPath), fsConstants.W_OK)
        return true
      } catch {
        return false
      }
    },
    readEntrypointIdentity: readEntrypointIdentity,
  }
}

export function buildDesktopCLIInstallDetector(
  release: ManagedCLIRelease | undefined,
  probe: DesktopCLIInstallProbe,
) {
  const available = managedCLIReleaseIdentity(release)
  return () =>
    detectDesktopCLIInstallState({
      homeDir: os.homedir(),
      pathEntries: processPathEntries(),
      platformId: nativePlatformId(),
      available,
      probe,
    })
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
  release: ManagedCLIRelease | undefined,
): (() => Promise<Uint8Array>) | undefined {
  if (!release?.binaryPath) return undefined
  const binaryPath = release.binaryPath
  return () => fs.readFile(binaryPath)
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
