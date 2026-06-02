import type { MessageStream } from 'starpc'

import { ItState } from '../../bldr/it-state.js'
import {
  DesktopCLIInstallActionKind,
  DesktopCLIInstallStatus,
  DesktopCLIInstallState,
  type DesktopCLIEntrypointIdentity,
  type DesktopCLIInstallActionItem,
  type InvokeCLIInstallActionRequest,
  type InvokeCLIInstallActionResponse,
  type WatchCLIInstallStateRequest,
  type WatchCLIInstallStateResponse,
} from '../desktop-runtime/desktop-runtime.pb.js'
import type { DesktopCLIInstallResourceService } from '../desktop-runtime/desktop-runtime_srpc.pb.js'
import {
  bindCLIInstallStateGeneration,
  cloneCLIInstallState,
  type DesktopCLIInstallDetectionOpts,
  type DesktopCLIInstallProbe,
} from './desktop-cli-install-detector.js'
import {
  executeDesktopCLIInstall,
  type DesktopCLIInstallFilesystem,
} from './desktop-cli-install-executor.js'

interface DesktopCLIInstallResourceOpts {
  detectCLIInstallState?: (
    selectedTargetId?: string,
  ) => Promise<DesktopCLIInstallState>
  openCLISettings?: () => Promise<void> | void
  readReleaseBinary?: (
    expected?: DesktopCLIEntrypointIdentity,
  ) => Promise<Uint8Array>
  probe?: Pick<DesktopCLIInstallProbe, 'fileExists' | 'readEntrypointIdentity'>
  filesystem?: DesktopCLIInstallFilesystem
  now?: () => number
}

// DesktopCLIInstallResource owns desktop-managed CLI install detection state.
export class DesktopCLIInstallResource implements DesktopCLIInstallResourceService {
  private generation = 1n
  private state: DesktopCLIInstallState
  private readonly stateStream = new ItState<WatchCLIInstallStateResponse>(
    () => Promise.resolve(this.buildStateResponse()),
    { mostRecentOnly: true },
  )

  constructor(private readonly opts: DesktopCLIInstallResourceOpts = {}) {
    this.state = bindCLIInstallStateGeneration(
      buildInitialCLIInstallState(),
      this.generation,
      this.buildGenerationOpts(),
    )
  }

  public WatchCLIInstallState(
    _request: WatchCLIInstallStateRequest,
    _abortSignal?: AbortSignal,
  ): MessageStream<WatchCLIInstallStateResponse> {
    return this.stateStream.getIterable()
  }

  public async InvokeCLIInstallAction(
    request: InvokeCLIInstallActionRequest,
    _abortSignal?: AbortSignal,
  ): Promise<InvokeCLIInstallActionResponse> {
    const action = this.findCurrentAction(request)
    if (
      action.kind ===
      DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_SELECT_TARGET
    ) {
      await this.selectTarget(action)
      return {}
    }
    switch (action.id) {
      case 'recheck':
        await this.recheck()
        break
      case 'open-settings':
        await this.opts.openCLISettings?.()
        break
      case 'install':
        await this.installOrUpdate(action, 'install')
        break
      case 'update':
        await this.installOrUpdate(action, 'update')
        break
      default:
        throw new Error(`unsupported desktop CLI install action: ${action.id}`)
    }
    return {}
  }

  public setDetectedState(state: DesktopCLIInstallState): void {
    const currentGeneration = this.state.generation ?? this.generation
    const currentShape = bindCLIInstallStateGeneration(
      state,
      currentGeneration,
      this.buildGenerationOpts(),
    )
    if (DesktopCLIInstallState.equals(this.state, currentShape)) return
    const next = bindCLIInstallStateGeneration(
      state,
      currentGeneration + 1n,
      this.buildGenerationOpts(),
    )
    if (DesktopCLIInstallState.equals(this.state, next)) return
    this.state = next
    this.pushState()
  }

  public async recheck(): Promise<void> {
    const detector = this.opts.detectCLIInstallState
    if (!detector) return
    this.setDetectedState(
      await detector(this.state.selectedTargetId || undefined),
    )
  }

  private async installOrUpdate(
    action: DesktopCLIInstallActionItem,
    operation: 'install' | 'update',
  ): Promise<void> {
    const readReleaseBinary = this.opts.readReleaseBinary
    const probe = this.opts.probe
    if (!readReleaseBinary || !probe) {
      throw new Error('desktop CLI install executor is not configured')
    }
    const target = this.state.targets?.find(
      (item) => item.id === action.targetId,
    )
    if (!target) throw new Error('desktop CLI install target not found')
    this.setOperationState(operation)
    try {
      await executeDesktopCLIInstall({
        operation,
        target,
        installed: this.state.installed,
        available: this.state.available,
        readReleaseBinary,
        probe,
        filesystem: this.opts.filesystem,
        now: this.opts.now,
      })
      await this.recheck()
    } catch (err) {
      this.setDetectedState({
        ...this.state,
        status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_ERROR,
        label: 'Command line tool update failed',
        detail: 'The previous binary was restored when a backup was present.',
        errorMessage: err instanceof Error ? err.message : String(err),
      })
      throw err
    }
  }

  private setOperationState(operation: 'install' | 'update'): void {
    this.setDetectedState({
      ...this.state,
      status:
        operation === 'install'
          ? DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLING
          : DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATING,
      label:
        operation === 'install'
          ? 'Installing command line tool'
          : 'Updating command line tool',
      detail: 'Writing the managed CLI into the selected user target.',
      errorMessage: '',
    })
  }

  private async selectTarget(
    action: DesktopCLIInstallActionItem,
  ): Promise<void> {
    const targetId = action.targetId || ''
    if (!targetId) throw new Error('desktop CLI install target is required')
    if (!this.state.targets?.some((target) => target.id === targetId)) {
      throw new Error('desktop CLI install target not found')
    }
    if (this.opts.detectCLIInstallState) {
      this.setDetectedState(await this.opts.detectCLIInstallState(targetId))
      return
    }
    const currentGeneration = this.state.generation ?? this.generation
    const next = cloneCLIInstallState(this.state)
    next.selectedTargetId = targetId
    next.targets = next.targets?.map((target) => ({
      ...target,
      selected: target.id === targetId,
    }))
    this.state = bindCLIInstallStateGeneration(
      next,
      currentGeneration + 1n,
      this.buildGenerationOpts(),
    )
    this.pushState()
  }

  public getState(): DesktopCLIInstallState {
    return cloneCLIInstallState(this.state)
  }

  private findCurrentAction(
    request: InvokeCLIInstallActionRequest,
  ): DesktopCLIInstallActionItem {
    if ((request.generation ?? 0n) !== (this.state.generation ?? 0n)) {
      throw new Error('desktop CLI install action generation is stale')
    }
    const actionId = request.actionId || ''
    if (!actionId) throw new Error('desktop CLI install action id is required')
    const action = this.state.actions?.find((item) => item.id === actionId)
    if (!action) throw new Error('desktop CLI install action not found')
    if (!(action.enabled ?? false)) {
      throw new Error('desktop CLI install action is disabled')
    }
    if ((action.generation ?? 0n) !== (this.state.generation ?? 0n)) {
      throw new Error('desktop CLI install action generation is stale')
    }
    return action
  }

  private buildGenerationOpts() {
    return {
      installActionsEnabled: !!(this.opts.readReleaseBinary && this.opts.probe),
    }
  }

  private buildStateResponse(): WatchCLIInstallStateResponse {
    return { state: this.getState() }
  }

  private pushState(): void {
    this.generation = this.state.generation ?? this.generation
    this.stateStream.pushChangeEvent(this.buildStateResponse())
  }
}

function buildInitialCLIInstallState(): DesktopCLIInstallState {
  return {
    status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UNKNOWN,
    label: 'Checking command line tool',
    detail: '',
    installed: {},
    available: {},
    targets: [],
    conflictPath: '',
    errorMessage: '',
    selectedTargetId: '',
    actions: [],
  }
}

export type { DesktopCLIInstallDetectionOpts, DesktopCLIInstallResourceOpts }
