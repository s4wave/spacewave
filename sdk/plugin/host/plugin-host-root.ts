import { Resource } from '../../resource/resource.js'
import type { ClientResourceRef } from '../../resource/client.js'
import { StateAtom } from '../../state/state.js'
import { PluginHostResourceServiceClient } from './host_srpc.pb.js'

// PluginHostRoot provides typed access to the plugin host resource tree.
export class PluginHostRoot extends Resource {
  private svc: PluginHostResourceServiceClient

  constructor(ref: ClientResourceRef) {
    super(ref)
    this.svc = new PluginHostResourceServiceClient(ref.client)
  }

  // accessAssetsFS returns a reference to the plugin's assets filesystem resource.
  async accessAssetsFS(signal?: AbortSignal): Promise<ClientResourceRef> {
    const resp = await this.svc.AccessAssetsFS({}, signal)
    return this.resourceRef.createRef(resp.resourceId ?? 0)
  }

  // accessDistFS returns a reference to the plugin's dist filesystem resource.
  async accessDistFS(signal?: AbortSignal): Promise<ClientResourceRef> {
    const resp = await this.svc.AccessDistFS({}, signal)
    return this.resourceRef.createRef(resp.resourceId ?? 0)
  }

  // accessVolume returns a reference to the plugin's host volume resource.
  async accessVolume(signal?: AbortSignal): Promise<ClientResourceRef> {
    const resp = await this.svc.AccessVolume({}, signal)
    return this.resourceRef.createRef(resp.resourceId ?? 0)
  }

  // accessStateAtom returns a StateAtom resource for the given store ID.
  async accessStateAtom(
    storeId?: string,
    signal?: AbortSignal,
  ): Promise<StateAtom> {
    const resp = await this.svc.AccessStateAtom(
      { storeId: storeId ?? '' },
      signal,
    )
    return this.resourceRef.createResource(resp.resourceId ?? 0, StateAtom)
  }

  // getPluginInfo returns information about the running plugin.
  async getPluginInfo(signal?: AbortSignal) {
    return this.svc.GetPluginInfo({}, signal)
  }
}
