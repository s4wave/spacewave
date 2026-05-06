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
import type {
  DesktopRuntimeState,
  OpenOrFocusMainWindowRequest,
  OpenOrFocusMainWindowResponse,
  QuitDesktopRuntimeRequest,
  QuitDesktopRuntimeResponse,
  WatchDesktopStateRequest,
  WatchDesktopStateResponse,
} from '../desktop-runtime/desktop-runtime.pb.js'

interface DesktopRuntimeResourceOpts {
  openOrFocusMainWindow: () => Promise<void> | void
  quitDesktopRuntime: () => Promise<void> | void
}

// DesktopRuntimeResource owns Electron main desktop-shell lifecycle state.
export class DesktopRuntimeResource implements DesktopRuntimeResourceService {
  private state: DesktopRuntimeState = {
    mainWindowOpen: false,
    quitting: false,
    statusText: 'Running',
  }
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
    _request: OpenOrFocusMainWindowRequest,
    _abortSignal?: AbortSignal,
  ): Promise<OpenOrFocusMainWindowResponse> {
    await this.opts.openOrFocusMainWindow()
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

  public setQuitting(quitting: boolean): void {
    if (this.state.quitting === quitting) return
    this.state = { ...this.state, quitting }
    this.pushState()
  }

  public getState(): DesktopRuntimeState {
    return { ...this.state }
  }

  private buildStateResponse(): WatchDesktopStateResponse {
    return { state: this.getState() }
  }

  private pushState(): void {
    this.stateStream.pushChangeEvent(this.buildStateResponse())
  }
}

export type { DesktopRuntimeResourceOpts }
