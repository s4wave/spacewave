import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import type {
  FrameEvent,
  Input,
} from '@s4wave/core/frametrack/frametrack.pb.js'

import type { WatchDisplayResponse } from './display.pb.js'
import { DisplayResourceServiceClient } from './display_srpc.pb.js'

export { CreateDisplayOpTypeID, DisplayTypeID } from './display_typeid.pb.js'

/** DisplayHandle opens the watch and the live connection of one Display. */
export class DisplayHandle extends Resource {
  private readonly service: DisplayResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new DisplayResourceServiceClient(resourceRef.client)
  }

  /** watchDisplay reports the Display and its Device on each change. */
  public watchDisplay(
    signal?: AbortSignal,
  ): AsyncIterable<WatchDisplayResponse> {
    return this.service.WatchDisplay({}, signal)
  }

  /** connectDisplay streams pictures and sends input on their subscription. */
  public connectDisplay(
    input: AsyncIterable<Input>,
    signal?: AbortSignal,
  ): AsyncIterable<FrameEvent> {
    return this.service.ConnectDisplay(input, signal)
  }
}
