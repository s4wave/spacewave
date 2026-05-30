import { createHandler } from 'starpc'
import type { MessageStream, Mux } from 'starpc'

import { ItState } from '../../bldr/it-state.js'
import { constructChildResource } from '../../../sdk/resource/server/construct.js'
import { getCurrentResourceClient } from '../../../sdk/resource/server/server.js'
import { newResourceMux } from '../../../sdk/resource/server/mux.js'
import type { RemoteResourceClient } from '../../../sdk/resource/server/tracked-client.js'
import {
  DesktopTrayEntryResourceServiceDefinition,
  DesktopTrayActionHandlerServiceClient,
  DesktopTrayResourceServiceDefinition,
  type DesktopTrayEntryResourceService,
  type DesktopTrayResourceService,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray_srpc.pb.js'
import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  type DesktopTrayEntry,
  type DesktopTrayState,
  type InvokeDesktopTrayEntryRequest,
  type InvokeDesktopTrayEntryResponse,
  type RegisterDesktopTrayEntryRequest,
  type RegisterDesktopTrayEntryResponse,
  type SetDesktopTrayEntryActiveRequest,
  type SetDesktopTrayEntryActiveResponse,
  type SetDesktopTrayEntryEnabledRequest,
  type SetDesktopTrayEntryEnabledResponse,
  type SetDesktopTrayEntryRequest,
  type SetDesktopTrayEntryResponse,
  type WatchDesktopTrayRequest,
  type WatchDesktopTrayResponse,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
interface DesktopTrayRegistration {
  resourceId: number
  entry: DesktopTrayEntry
  attachedActionResourceId: number
  client: RemoteResourceClient
}

// DesktopTrayResource owns an Electron-scoped desktop tray entry registry.
export class DesktopTrayResource implements DesktopTrayResourceService {
  private readonly registrations = new Map<number, DesktopTrayRegistration>()
  private readonly stateStream = new ItState<WatchDesktopTrayResponse>(
    () => Promise.resolve(this.buildStateResponse()),
    { mostRecentOnly: true },
  )
  private e2eState?: DesktopTrayState

  public RegisterDesktopTrayEntry(
    request: RegisterDesktopTrayEntryRequest,
    _abortSignal?: AbortSignal,
  ): Promise<RegisterDesktopTrayEntryResponse> {
    const entry = request.entry
    if (!entry) throw new Error('desktop tray entry is required')
    if (!entry.id) throw new Error('desktop tray entry id is required')
    if (this.hasEntryId(entry.id, 0)) {
      throw new Error('desktop tray entry already registered')
    }
    const client = getCurrentResourceClient()

    const resource = new DesktopTrayEntryResource(this)
    let resourceId = 0
    const child = constructChildResource(() => ({
      mux: resource.getMux(),
      result: resource,
      releaseFn: () => this.unregister(resourceId),
    }))
    resourceId = child.resourceId
    resource.setResourceId(resourceId)

    if (this.hasEntryId(entry.id, 0)) {
      this.unregister(resourceId)
      throw new Error('desktop tray entry already registered')
    }

    this.registrations.set(resourceId, {
      resourceId,
      entry: cloneEntry(entry),
      attachedActionResourceId: request.attachedActionResourceId ?? 0,
      client,
    })
    this.pushState()
    return Promise.resolve({ resourceId })
  }

  public WatchDesktopTray(
    _request: WatchDesktopTrayRequest,
    _abortSignal?: AbortSignal,
  ): MessageStream<WatchDesktopTrayResponse> {
    return this.stateStream.getIterable()
  }

  public InvokeDesktopTrayEntry(
    request: InvokeDesktopTrayEntryRequest,
    abortSignal?: AbortSignal,
  ): Promise<InvokeDesktopTrayEntryResponse> {
    const entryId = request.entryId || ''
    if (!entryId) throw new Error('desktop tray entry id is required')

    const reg = this.findEntry(entryId)
    if (!reg) throw new Error('desktop tray entry not found')

    const entry = reg.entry
    if (
      entry.kind !== DesktopTrayEntryKind.ACTION ||
      !(entry.enabled ?? false)
    ) {
      throw new Error('desktop tray entry is not invokable')
    }

    const action = entry.action
    if (action?.kind !== DesktopTrayActionKind.ATTACHED_HANDLER) {
      throw new Error('desktop tray entry is not invokable')
    }
    if (!reg.attachedActionResourceId) {
      throw new Error('desktop tray action handler is required')
    }

    const client = reg.client.getRawAttachedClient(reg.attachedActionResourceId)
    const handler = new DesktopTrayActionHandlerServiceClient(client)
    return handler
      .HandleDesktopTrayAction(
        {
          entryId: entry.id,
          action,
        },
        abortSignal,
      )
      .then(() => ({}))
  }

  public setEntry(resourceId: number, entry: DesktopTrayEntry): void {
    if (!entry.id) throw new Error('desktop tray entry id is required')
    const reg = this.registrations.get(resourceId)
    if (!reg) throw new Error('desktop tray entry not found')
    if (this.hasEntryId(entry.id, resourceId)) {
      throw new Error('desktop tray entry already registered')
    }
    reg.entry = cloneEntry(entry)
    this.pushState()
  }

  public setActive(resourceId: number, active: boolean): void {
    const reg = this.registrations.get(resourceId)
    if (!reg) throw new Error('desktop tray entry not found')
    if ((reg.entry.active ?? false) === active) return
    reg.entry = { ...reg.entry, active }
    this.pushState()
  }

  public setEnabled(resourceId: number, enabled: boolean): void {
    const reg = this.registrations.get(resourceId)
    if (!reg) throw new Error('desktop tray entry not found')
    if ((reg.entry.enabled ?? false) === enabled) return
    reg.entry = { ...reg.entry, enabled }
    this.pushState()
  }

  public getState(): DesktopTrayState {
    if (this.e2eState) {
      return cloneState(this.e2eState)
    }
    const entries = Array.from(this.registrations.values())
      .toSorted(compareRegistrations)
      .map((reg) => cloneEntry(reg.entry))
    return {
      entries,
      iconState: maxIconState(entries, DesktopTrayIconState.NORMAL),
      statusText: titleStatusText(entries),
    }
  }

  public replaceStateForE2E(state: DesktopTrayState | undefined): void {
    this.e2eState = state ? cloneState(state) : undefined
    this.pushState()
  }

  private unregister(resourceId: number): void {
    if (!this.registrations.delete(resourceId)) return
    this.pushState()
  }

  private hasEntryId(entryId: string, exceptResourceId: number): boolean {
    for (const reg of this.registrations.values()) {
      if (reg.resourceId === exceptResourceId) continue
      if (reg.entry.id === entryId) return true
    }
    return false
  }

  private findEntry(entryId: string): DesktopTrayRegistration | undefined {
    for (const reg of this.registrations.values()) {
      if (reg.entry.id === entryId) return reg
    }
    return undefined
  }

  private buildStateResponse(): WatchDesktopTrayResponse {
    return { state: this.getState() }
  }

  private pushState(): void {
    this.stateStream.pushChangeEvent(this.buildStateResponse())
  }
}

class DesktopTrayEntryResource implements DesktopTrayEntryResourceService {
  private readonly mux: Mux
  private resourceId = 0

  constructor(private readonly tray: DesktopTrayResource) {
    this.mux = newResourceMux(
      createHandler(DesktopTrayEntryResourceServiceDefinition, this),
    )
  }

  public getMux(): Mux {
    return this.mux
  }

  public setResourceId(resourceId: number): void {
    this.resourceId = resourceId
  }

  public SetDesktopTrayEntry(
    request: SetDesktopTrayEntryRequest,
    _abortSignal?: AbortSignal,
  ): Promise<SetDesktopTrayEntryResponse> {
    const entry = request.entry
    if (!entry) throw new Error('desktop tray entry is required')
    this.tray.setEntry(this.resourceId, entry)
    return Promise.resolve({})
  }

  public SetDesktopTrayEntryActive(
    request: SetDesktopTrayEntryActiveRequest,
    _abortSignal?: AbortSignal,
  ): Promise<SetDesktopTrayEntryActiveResponse> {
    this.tray.setActive(this.resourceId, request.active ?? false)
    return Promise.resolve({})
  }

  public SetDesktopTrayEntryEnabled(
    request: SetDesktopTrayEntryEnabledRequest,
    _abortSignal?: AbortSignal,
  ): Promise<SetDesktopTrayEntryEnabledResponse> {
    this.tray.setEnabled(this.resourceId, request.enabled ?? false)
    return Promise.resolve({})
  }
}

function compareRegistrations(
  left: DesktopTrayRegistration,
  right: DesktopTrayRegistration,
): number {
  return (
    comparePath(left.entry.path, right.entry.path) ||
    compareText(left.entry.group, right.entry.group) ||
    (left.entry.order ?? 0) - (right.entry.order ?? 0) ||
    compareText(left.entry.id, right.entry.id) ||
    left.resourceId - right.resourceId
  )
}

function comparePath(
  left: string[] | undefined,
  right: string[] | undefined,
): number {
  return compareText((left ?? []).join('\0'), (right ?? []).join('\0'))
}

function compareText(
  left: string | undefined,
  right: string | undefined,
): number {
  return (left ?? '').localeCompare(right ?? '')
}

function maxIconState(
  entries: DesktopTrayEntry[],
  initial: DesktopTrayIconState,
): DesktopTrayIconState {
  let state = initial
  for (const entry of entries) {
    if ((entry.iconState ?? DesktopTrayIconState.UNSPECIFIED) > state) {
      state = entry.iconState ?? DesktopTrayIconState.UNSPECIFIED
    }
  }
  return state
}

function titleStatusText(entries: DesktopTrayEntry[]): string {
  const title = entries.find((entry) => entry.id === 'title')?.label ?? ''
  return title.replace(/^Spacewave: /, '')
}

function cloneEntry(entry: DesktopTrayEntry): DesktopTrayEntry {
  return {
    ...entry,
    kind: entry.kind ?? DesktopTrayEntryKind.UNSPECIFIED,
    path: [...(entry.path ?? [])],
    action: entry.action ? { ...entry.action } : undefined,
  }
}

function cloneState(state: DesktopTrayState): DesktopTrayState {
  return {
    ...state,
    entries: state.entries?.map(cloneEntry) ?? [],
  }
}

export const DesktopTrayResourceHandler = DesktopTrayResourceServiceDefinition
