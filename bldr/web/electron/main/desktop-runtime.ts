import { createHandler } from 'starpc'
import type { MessageStream } from 'starpc'

import { ItState } from '../../bldr/it-state.js'
import {
  ResourceServer,
  newResourceMux,
} from '../../../sdk/resource/server/index.js'
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
  type WatchDesktopStateRequest,
  type WatchDesktopStateResponse,
} from '../desktop-runtime/desktop-runtime.pb.js'

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
    async () => this.buildStateResponse(),
    { mostRecentOnly: true },
  )
  public readonly resourceServer: ResourceServer

  constructor(private readonly opts: DesktopRuntimeResourceOpts) {
    const mux = newResourceMux(
      createHandler(DesktopRuntimeResourceServiceDefinition, this),
    )
    this.resourceServer = new ResourceServer(mux)
  }

  public WatchDesktopState(
    _request: WatchDesktopStateRequest,
    _abortSignal?: AbortSignal,
  ): MessageStream<WatchDesktopStateResponse> {
    return this.stateStream.getIterable()
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

function cloneDesktopRuntimeState(state: DesktopRuntimeState): DesktopRuntimeState {
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
): DesktopRuntimeNavigationItem[] | undefined {
  return items?.map((item) => ({ ...item }))
}

function cloneActivityItems(
  items: DesktopRuntimeActivityItem[] | undefined,
): DesktopRuntimeActivityItem[] | undefined {
  return items?.map((item) => ({ ...item }))
}

function cloneUpdate(
  update: DesktopRuntimeUpdateStatus | undefined,
): DesktopRuntimeUpdateStatus | undefined {
  if (!update) return undefined
  return { ...update }
}

function cloneAttentionItems(
  items: DesktopRuntimeAttentionItem[] | undefined,
): DesktopRuntimeAttentionItem[] | undefined {
  return items?.map((item) => ({ ...item }))
}

function cloneActionItems(
  items: DesktopRuntimeActionItem[] | undefined,
): DesktopRuntimeActionItem[] | undefined {
  return items?.map((item) => ({ ...item }))
}

export type { DesktopRuntimeResourceOpts }
