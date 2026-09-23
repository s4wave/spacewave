import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { FrontendClient } from '@go/github.com/s4wave/spacewave/bldr/frontend/frontend_srpc.pb.js'

/** PluginFrontend owns a device compiler; releasing it cancels its Forge job. */
export class PluginFrontend extends Resource {
  /** frontend serves the retained compiler's existing Watch, Send, and Fetch API. */
  public readonly frontend: FrontendClient

  constructor(
    resourceRef: ClientResourceRef,
    public readonly executionKey: string,
  ) {
    super(resourceRef)
    this.frontend = new FrontendClient(resourceRef.client)
  }
}
