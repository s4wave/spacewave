import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'

import {
  SystemStatusService,
  SystemStatusServiceClient,
} from './status_srpc.pb.js'
import type {
  WatchControllersResponse,
  WatchDirectivesResponse,
} from './status.pb.js'

// SystemStatus wraps the SystemStatusService on a session resource.
export class SystemStatus {
  private service: SystemStatusService

  constructor(resourceRef: ClientResourceRef) {
    this.service = new SystemStatusServiceClient(resourceRef.client)
  }

  // watchControllers streams the list of active controllers on change.
  public watchControllers(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchControllersResponse> {
    return this.service.WatchControllers({}, abortSignal)
  }

  // watchDirectives streams the list of active directives on change.
  public watchDirectives(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchDirectivesResponse> {
    return this.service.WatchDirectives({}, abortSignal)
  }
}
