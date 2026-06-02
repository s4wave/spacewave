import path from 'node:path'

import {
  DesktopCLIInstallActionKind,
  DesktopCLIInstallStatus,
  type DesktopCLIEntrypointIdentity,
  type DesktopCLIInstallActionItem,
  type DesktopCLIInstallState,
  type DesktopCLIInstallTarget,
} from '../desktop-runtime/desktop-runtime.pb.js'
import { buildDesktopCLIInstallTargets } from './desktop-cli-install-target-policy.js'

export interface DesktopCLIInstallProbe {
  fileExists(path: string): Promise<boolean>
  targetWritable(path: string): Promise<boolean>
  readEntrypointIdentity(
    path: string,
  ): Promise<DesktopCLIEntrypointIdentity | undefined>
}

export interface DesktopCLIInstallDetectionOpts {
  homeDir: string
  pathEntries: string[]
  platformId: string
  selectedTargetId?: string
  available?: DesktopCLIEntrypointIdentity
  probe: DesktopCLIInstallProbe
}

export interface CLIInstallStateGenerationOpts {
  installActionsEnabled?: boolean
}

export async function detectDesktopCLIInstallState(
  opts: DesktopCLIInstallDetectionOpts,
): Promise<DesktopCLIInstallState> {
  try {
    const targets = selectTargetCandidate(
      await buildTargetCandidates(opts),
      opts.selectedTargetId,
    )
    const selectedTarget = targets.find((target) => target.selected)
    const installed = selectedTarget?.path
      ? await readManagedEntrypoint(opts.probe, selectedTarget.path)
      : undefined
    const firstPathCommand = await findFirstPathCommand(opts)
    const conflictPath =
      firstPathCommand &&
      firstPathCommand !== selectedTarget?.path &&
      !(await readManagedEntrypoint(opts.probe, firstPathCommand))
        ? firstPathCommand
        : ''

    if (conflictPath) {
      return buildCLIInstallState({
        status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_CONFLICT,
        label: 'Command line tool conflict',
        detail: 'Another spacewave command appears before the managed target.',
        installed,
        available: opts.available,
        targets,
        conflictPath,
      })
    }

    if (!installed) {
      return buildCLIInstallState({
        status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING,
        label: 'Command line tool not installed',
        detail: 'The managed command line tool was not detected.',
        available: opts.available,
        targets,
      })
    }

    const updateAvailable = isUpdateAvailable(installed, opts.available)
    return buildCLIInstallState({
      status: updateAvailable
        ? DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATE_AVAILABLE
        : DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED,
      label: updateAvailable
        ? 'Command line tool update available'
        : 'Command line tool installed',
      detail: '',
      installed,
      available: opts.available,
      targets,
    })
  } catch (err) {
    return buildCLIInstallState({
      status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_ERROR,
      label: 'Command line tool check failed',
      detail: '',
      errorMessage: err instanceof Error ? err.message : String(err),
      available: opts.available,
      targets: await safeBuildTargetCandidates(opts),
    })
  }
}

export function buildCLIInstallState(opts: {
  status: DesktopCLIInstallStatus
  label: string
  detail?: string
  installed?: DesktopCLIEntrypointIdentity
  available?: DesktopCLIEntrypointIdentity
  targets?: DesktopCLIInstallTarget[]
  conflictPath?: string
  errorMessage?: string
}): DesktopCLIInstallState {
  const targets = cloneInstallTargets(opts.targets)
  const selectedTargetId = targets.find((target) => target.selected)?.id ?? ''
  return {
    status: opts.status,
    label: opts.label,
    detail: opts.detail ?? '',
    installed: cloneEntrypointIdentity(opts.installed),
    available: cloneEntrypointIdentity(opts.available),
    targets,
    conflictPath: opts.conflictPath ?? '',
    errorMessage: opts.errorMessage ?? '',
    selectedTargetId,
    actions: buildDetectionActions(
      { ...opts, targets, selectedTargetId },
      0n,
      {},
    ),
  }
}

export function bindCLIInstallStateGeneration(
  state: DesktopCLIInstallState,
  generation: bigint,
  opts: CLIInstallStateGenerationOpts = {},
): DesktopCLIInstallState {
  const next = cloneCLIInstallState(state)
  next.generation = generation
  next.targets = next.targets?.map((target) => ({ ...target, generation }))
  next.selectedTargetId =
    next.selectedTargetId ||
    next.targets?.find((target) => target.selected)?.id ||
    ''
  next.actions = buildDetectionActions(next, generation, opts)
  return next
}

export function cloneCLIInstallState(
  state: DesktopCLIInstallState,
): DesktopCLIInstallState {
  return {
    ...state,
    installed: cloneEntrypointIdentity(state.installed),
    available: cloneEntrypointIdentity(state.available),
    targets: cloneInstallTargets(state.targets),
    actions: cloneInstallActions(state.actions),
  }
}

function buildDetectionActions(
  state: DesktopCLIInstallState,
  generation: bigint,
  opts: CLIInstallStateGenerationOpts,
): DesktopCLIInstallActionItem[] {
  const actions: DesktopCLIInstallActionItem[] = [
    {
      id: 'recheck',
      kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_RECHECK,
      label: 'Check again',
      enabled: true,
      generation,
    },
    {
      id: 'open-settings',
      kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_OPEN_SETTINGS,
      label: 'Settings',
      enabled: true,
      generation,
    },
  ]
  const selectedTargetId = state.selectedTargetId || ''
  const selectedTarget = state.targets?.find(
    (target) => target.id === selectedTargetId,
  )
  for (const target of state.targets ?? []) {
    if (!target.id || target.id === selectedTargetId) continue
    actions.push({
      id: `select-target:${target.id}`,
      kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_SELECT_TARGET,
      label: `Use ${target.label || target.path || target.id}`,
      enabled: true,
      targetId: target.id,
      generation,
      detail: target.detail,
    })
  }
  if (!opts.installActionsEnabled) return actions
  const canWrite = selectedTarget?.writable ?? false
  const hasTrustedRelease =
    state.available?.entrypointRole === 'cli' &&
    state.available?.manifestId === 'spacewave-cli' &&
    state.available?.projectId === 'spacewave' &&
    !!state.available?.channelKey &&
    !!state.available?.platformId &&
    !!state.available?.manifestRev
  if (canWrite && hasTrustedRelease) {
    if (
      state.status ===
      DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING
    ) {
      actions.push({
        id: 'install',
        kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_INSTALL,
        label: 'Install',
        enabled: true,
        targetId: selectedTargetId,
        generation,
      })
    }
    if (
      state.status ===
      DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATE_AVAILABLE
    ) {
      actions.push({
        id: 'update',
        kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_UPDATE,
        label: 'Update',
        enabled: true,
        targetId: selectedTargetId,
        generation,
      })
    }
  }
  return actions
}

async function buildTargetCandidates(
  opts: DesktopCLIInstallDetectionOpts,
): Promise<DesktopCLIInstallTarget[]> {
  return buildDesktopCLIInstallTargets({
    homeDir: opts.homeDir,
    platformId: opts.platformId,
    pathEntries: opts.pathEntries,
    canWrite: opts.probe.targetWritable,
  })
}

async function safeBuildTargetCandidates(
  opts: DesktopCLIInstallDetectionOpts,
): Promise<DesktopCLIInstallTarget[]> {
  try {
    return selectTargetCandidate(
      await buildTargetCandidates(opts),
      opts.selectedTargetId,
    )
  } catch {
    return []
  }
}

function selectTargetCandidate(
  targets: DesktopCLIInstallTarget[],
  selectedTargetId: string | undefined,
): DesktopCLIInstallTarget[] {
  if (
    !selectedTargetId ||
    !targets.some((target) => target.id === selectedTargetId)
  ) {
    return targets
  }
  return targets.map((target) => ({
    ...target,
    selected: target.id === selectedTargetId,
  }))
}

async function findFirstPathCommand(
  opts: DesktopCLIInstallDetectionOpts,
): Promise<string> {
  for (const entry of opts.pathEntries) {
    if (!entry) continue
    const candidate = path.join(entry, 'spacewave')
    if (await opts.probe.fileExists(candidate)) return candidate
  }
  return ''
}

async function readManagedEntrypoint(
  probe: DesktopCLIInstallProbe,
  commandPath: string,
): Promise<DesktopCLIEntrypointIdentity | undefined> {
  const identity = await probe.readEntrypointIdentity(commandPath)
  if (identity?.entrypointRole !== 'cli') return undefined
  if (identity.manifestId !== 'spacewave-cli') return undefined
  if (identity.projectId !== 'spacewave') return undefined
  return {
    ...identity,
    path: identity.path || commandPath,
  }
}

function isUpdateAvailable(
  installed: DesktopCLIEntrypointIdentity,
  available: DesktopCLIEntrypointIdentity | undefined,
): boolean {
  if (!available?.manifestRev) return false
  if (installed.projectId !== available.projectId) return true
  if (installed.channelKey !== available.channelKey) return true
  if (installed.platformId !== available.platformId) return true
  if (installed.manifestId !== available.manifestId) return true
  return (installed.manifestRev ?? 0n) < available.manifestRev
}

function cloneEntrypointIdentity(
  identity: DesktopCLIEntrypointIdentity | undefined,
): DesktopCLIEntrypointIdentity {
  if (!identity) return {}
  return { ...identity }
}

function cloneInstallTargets(
  targets: DesktopCLIInstallTarget[] | undefined,
): DesktopCLIInstallTarget[] {
  return targets?.map((target) => ({ ...target })) ?? []
}

function cloneInstallActions(
  actions: DesktopCLIInstallActionItem[] | undefined,
): DesktopCLIInstallActionItem[] {
  return actions?.map((action) => ({ ...action })) ?? []
}
