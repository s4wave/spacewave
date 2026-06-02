import path from 'node:path'

import {
  DesktopCLIInstallTargetPathState,
  type DesktopCLIInstallTarget,
} from '../desktop-runtime/desktop-runtime.pb.js'

export interface DesktopCLIInstallTargetPolicyOpts {
  homeDir: string
  platformId: string
  pathEntries: string[]
  canWrite: (targetPath: string) => Promise<boolean>
}

export async function buildDesktopCLIInstallTargets(
  opts: DesktopCLIInstallTargetPolicyOpts,
): Promise<DesktopCLIInstallTarget[]> {
  const candidates = targetCandidates(opts.homeDir, opts.platformId)
  const targets: DesktopCLIInstallTarget[] = []
  for (const candidate of candidates) {
    const blockedReason = blockedTargetReason(candidate.path, opts.platformId)
    targets.push({
      id: candidate.id,
      label: candidate.label,
      path: candidate.path,
      writable: blockedReason ? false : await opts.canWrite(candidate.path),
      selected: targets.length === 0,
      detail: targetDetail(candidate.path, opts.pathEntries, blockedReason),
      pathState: targetPathState(
        candidate.path,
        opts.pathEntries,
        blockedReason,
      ),
      blockedReason,
    })
  }
  return targets
}

export function targetCandidates(
  homeDir: string,
  platformId: string,
): Array<{ id: string; label: string; path: string }> {
  switch (platformOS(platformId)) {
    case 'windows':
      return [
        {
          id: 'local-app-data',
          label: 'Local app data',
          path: path.win32.join(
            homeDir,
            'AppData',
            'Local',
            'Programs',
            'Spacewave',
            'spacewave.exe',
          ),
        },
      ]
    case 'linux':
      return [
        {
          id: 'home-local-bin',
          label: 'Local user bin',
          path: path.posix.join(homeDir, '.local', 'bin', 'spacewave'),
        },
        {
          id: 'home-bin',
          label: 'User bin',
          path: path.posix.join(homeDir, 'bin', 'spacewave'),
        },
      ]
    default:
      return [
        {
          id: 'home-bin',
          label: 'User bin',
          path: path.posix.join(homeDir, 'bin', 'spacewave'),
        },
        {
          id: 'home-local-bin',
          label: 'Local user bin',
          path: path.posix.join(homeDir, '.local', 'bin', 'spacewave'),
        },
      ]
  }
}

export function blockedTargetReason(
  targetPath: string,
  platformId: string,
): string {
  const normalized = targetPath.replaceAll('\\', '/')
  if (platformOS(platformId) === 'windows' && !normalized.endsWith('.exe')) {
    return 'Windows CLI target must end in .exe'
  }
  if (normalized.startsWith('/Applications/')) return 'System app path'
  if (normalized.startsWith('/usr/')) return 'System prefix'
  if (normalized.startsWith('/opt/')) return 'Package-manager prefix'
  if (normalized.startsWith('/var/')) return 'System state prefix'
  if (normalized.includes('/.spacewave/'))
    return 'State directory is not an install target'
  return ''
}

export function targetPathState(
  targetPath: string,
  pathEntries: string[],
  blockedReason = '',
): DesktopCLIInstallTargetPathState {
  if (blockedReason) {
    return DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_BLOCKED
  }
  if (pathEntries.length === 0) {
    return DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_UNKNOWN
  }
  const dir = normalizePathDir(targetPath)
  return pathEntries.map(normalizePathEntry).includes(dir)
    ? DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_ON_PATH
    : DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_OFF_PATH
}

function targetDetail(
  targetPath: string,
  pathEntries: string[],
  blockedReason: string,
): string {
  const state = targetPathState(targetPath, pathEntries, blockedReason)
  switch (state) {
    case DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_ON_PATH:
      return 'Detected on PATH'
    case DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_OFF_PATH:
      return 'Manual PATH update needed'
    case DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_BLOCKED:
      return blockedReason
    default:
      return 'PATH evidence unavailable'
  }
}

function normalizePathDir(targetPath: string): string {
  const normalized = targetPath.replaceAll('\\', '/')
  const slash = normalized.lastIndexOf('/')
  return slash >= 0 ? normalized.slice(0, slash) : normalized
}

function normalizePathEntry(entry: string): string {
  return entry.replaceAll('\\', '/').replace(/\/+$/, '')
}

function platformOS(platformId: string): 'darwin' | 'linux' | 'windows' {
  if (platformId.includes('/windows/')) return 'windows'
  if (platformId.includes('/linux/')) return 'linux'
  return 'darwin'
}
