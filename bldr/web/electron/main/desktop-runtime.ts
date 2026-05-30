import { createHandler } from 'starpc'
import type { MessageStream } from 'starpc'

import { ItState } from '../../bldr/it-state.js'
import { ResourceServer } from '../../../sdk/resource/server/server.js'
import { newResourceMux } from '../../../sdk/resource/server/mux.js'
import { DesktopTrayResourceServiceDefinition } from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray_srpc.pb.js'
import {
  DesktopRuntimeResourceServiceDefinition,
  type DesktopRuntimeResourceService,
} from '../desktop-runtime/desktop-runtime_srpc.pb.js'
import {
  DesktopRuntimeHealth,
  DesktopRuntimeLifecycle,
  DesktopRuntimeReachability,
  DesktopRuntimeState,
  type DesktopRuntimeActionItem,
  type DesktopRuntimeActivityItem,
  type DesktopRuntimeAttentionItem,
  type DesktopRuntimeListenerStatus,
  type DesktopRuntimeNavigationItem,
  type DesktopRuntimeUpdateStatus,
  type OpenOrFocusMainWindowRequest,
  type OpenOrFocusMainWindowResponse,
  type QuitDesktopRuntimeRequest,
  type QuitDesktopRuntimeResponse,
  type SetDesktopStateRequest,
  type SetDesktopStateResponse,
  type WatchDesktopStateRequest,
  type WatchDesktopStateResponse,
} from '../desktop-runtime/desktop-runtime.pb.js'
import { DesktopTrayResource } from './desktop-tray-resource.js'

interface DesktopRuntimeResourceOpts {
  openOrFocusMainWindow: (
    request: OpenOrFocusMainWindowRequest,
  ) => Promise<void> | void
  quitDesktopRuntime: () => Promise<void> | void
}

// DesktopRuntimeResource owns Electron main desktop-shell lifecycle state.
export class DesktopRuntimeResource implements DesktopRuntimeResourceService {
  private state: DesktopRuntimeState = buildInitialDesktopRuntimeState()
  private readonly stateStream = new ItState<WatchDesktopStateResponse>(
    () => Promise.resolve(this.buildStateResponse()),
    { mostRecentOnly: true },
  )
  public readonly resourceServer: ResourceServer
  public readonly desktopTrayResource: DesktopTrayResource

  constructor(private readonly opts: DesktopRuntimeResourceOpts) {
    this.desktopTrayResource = new DesktopTrayResource()
    const mux = newResourceMux(
      createHandler(DesktopRuntimeResourceServiceDefinition, this),
      createHandler(
        DesktopTrayResourceServiceDefinition,
        this.desktopTrayResource,
      ),
    )
    this.resourceServer = new ResourceServer(mux)
  }

  public WatchDesktopState(
    _request: WatchDesktopStateRequest,
    _abortSignal?: AbortSignal,
  ): MessageStream<WatchDesktopStateResponse> {
    return this.stateStream.getIterable()
  }

  public SetDesktopState(
    request: SetDesktopStateRequest,
    _abortSignal?: AbortSignal,
  ): Promise<SetDesktopStateResponse> {
    this.setProjectedDesktopState(request.state ?? {})
    return Promise.resolve({})
  }

  public async OpenOrFocusMainWindow(
    request: OpenOrFocusMainWindowRequest,
    _abortSignal?: AbortSignal,
  ): Promise<OpenOrFocusMainWindowResponse> {
    await this.opts.openOrFocusMainWindow(request)
    return {}
  }

  public async QuitDesktopRuntime(
    _request: QuitDesktopRuntimeRequest,
    _abortSignal?: AbortSignal,
  ): Promise<QuitDesktopRuntimeResponse> {
    this.setQuitting(true)
    await this.opts.quitDesktopRuntime()
    return {}
  }

  public setMainWindowOpen(mainWindowOpen: boolean): void {
    if (this.state.mainWindowOpen === mainWindowOpen) return
    this.state = { ...this.state, mainWindowOpen }
    this.pushState()
  }

  public setDesktopState(state: DesktopRuntimeState): void {
    const next = cloneDesktopRuntimeState(state)
    if (DesktopRuntimeState.equals(this.state, next)) return
    this.state = next
    this.pushState()
  }

  public resetProjectedDesktopStateForE2E(): void {
    this.setProjectedDesktopState(buildInitialDesktopRuntimeState())
  }

  public setQuitting(quitting: boolean): void {
    if (this.state.quitting === quitting) return
    let health = DesktopRuntimeHealth.HEALTHY
    let lifecycle = DesktopRuntimeLifecycle.RUNNING
    if (quitting) {
      health = DesktopRuntimeHealth.QUITTING
      lifecycle = DesktopRuntimeLifecycle.QUITTING
    }
    this.state = {
      ...this.state,
      quitting,
      statusText: quitting ? 'Quitting' : 'Running',
      health,
      lifecycle,
    }
    this.pushState()
  }

  public getState(): DesktopRuntimeState {
    return cloneDesktopRuntimeState(this.state)
  }

  private buildStateResponse(): WatchDesktopStateResponse {
    return { state: this.getState() }
  }

  private setProjectedDesktopState(state: DesktopRuntimeState): void {
    const next = cloneDesktopRuntimeState({
      ...state,
      mainWindowOpen: this.state.mainWindowOpen,
      quitting: this.state.quitting,
    })
    if (this.state.quitting) {
      next.statusText = this.state.statusText
      next.health = this.state.health
      next.lifecycle = this.state.lifecycle
    }
    if (DesktopRuntimeState.equals(this.state, next)) return
    this.state = next
    this.pushState()
  }

  private pushState(): void {
    this.stateStream.pushChangeEvent(this.buildStateResponse())
  }
}

function buildInitialDesktopRuntimeState(): DesktopRuntimeState {
  return {
    mainWindowOpen: false,
    quitting: false,
    statusText: 'Running',
    health: DesktopRuntimeHealth.HEALTHY,
    lifecycle: DesktopRuntimeLifecycle.RUNNING,
    listener: {
      reachability: DesktopRuntimeReachability.UNSPECIFIED,
    },
    sessions: [],
    spaces: [],
    activity: [],
    update: {},
    attentionItems: [],
    actions: [],
  }
}

function cloneDesktopRuntimeState(
  state: DesktopRuntimeState,
): DesktopRuntimeState {
  return {
    ...state,
    listener: cloneListener(state.listener),
    sessions: cloneNavigationItems(state.sessions),
    spaces: cloneNavigationItems(state.spaces),
    activity: cloneActivityItems(state.activity),
    update: cloneUpdate(state.update),
    attentionItems: cloneAttentionItems(state.attentionItems),
    actions: cloneActionItems(state.actions),
  }
}

function cloneListener(
  listener: DesktopRuntimeListenerStatus | undefined,
): DesktopRuntimeListenerStatus | undefined {
  if (!listener) return undefined
  return { ...listener }
}

function cloneNavigationItems(
  items: DesktopRuntimeNavigationItem[] | undefined,
): DesktopRuntimeNavigationItem[] {
  return items?.map((item) => ({ ...item })) ?? []
}

function cloneActivityItems(
  items: DesktopRuntimeActivityItem[] | undefined,
): DesktopRuntimeActivityItem[] {
  return items?.map((item) => ({ ...item })) ?? []
}

function cloneUpdate(
  update: DesktopRuntimeUpdateStatus | undefined,
): DesktopRuntimeUpdateStatus {
  if (!update) return {}
  return { ...update }
}

function cloneAttentionItems(
  items: DesktopRuntimeAttentionItem[] | undefined,
): DesktopRuntimeAttentionItem[] {
  return items?.map((item) => ({ ...item })) ?? []
}

function cloneActionItems(
  items: DesktopRuntimeActionItem[] | undefined,
): DesktopRuntimeActionItem[] {
  return items?.map((item) => ({ ...item })) ?? []
}

export type { DesktopRuntimeResourceOpts }
