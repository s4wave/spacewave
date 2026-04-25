import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { LayoutHostClient, type LayoutHost } from './layout_srpc.pb.js'
import type { MessageStream } from 'starpc'
import type {
  WatchLayoutModelRequest,
  LayoutModel,
  NavigateTabRequest,
  NavigateTabResponse,
  AddTabRequest,
  AddTabResponse,
} from './layout.pb.js'

// LayoutHostHandle represents a handle to a layout host resource.
export class LayoutHostHandle extends Resource implements LayoutHost {
  private service: LayoutHostClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new LayoutHostClient(resourceRef.client)
  }

  // WatchLayoutModel watches the LayoutModel for the layout.
  public WatchLayoutModel(
    request: MessageStream<WatchLayoutModelRequest>,
    abortSignal?: AbortSignal,
  ): MessageStream<LayoutModel> {
    return this.service.WatchLayoutModel(request, abortSignal)
  }

  // NavigateTab navigates to a new location within a layout tab.
  public NavigateTab(
    request: NavigateTabRequest,
    abortSignal?: AbortSignal,
  ): Promise<NavigateTabResponse> {
    return this.service.NavigateTab(request, abortSignal)
  }

  // AddTab adds a new tab to the layout.
  public AddTab(
    request: AddTabRequest,
    abortSignal?: AbortSignal,
  ): Promise<AddTabResponse> {
    return this.service.AddTab(request, abortSignal)
  }
}
