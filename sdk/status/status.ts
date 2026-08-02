import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'

import {
  SystemStatusService,
  SystemStatusServiceClient,
} from './status_srpc.pb.js'
import type {
  ReportRecoveryStatusRequest,
  ReportRecoveryStatusResponse,
  WatchControllersResponse,
  WatchDirectivesResponse,
  WatchPluginsResponse,
  WatchNetworkStatsResponse,
  WatchRecoveryStatusResponse,
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

  // watchPlugins streams the list of active plugin load requests on change.
  public watchPlugins(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchPluginsResponse> {
    return this.service.WatchPlugins({}, abortSignal)
  }

  // watchNetworkStats streams the session transport's live network snapshot.
  public watchNetworkStats(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchNetworkStatsResponse> {
    return this.service.WatchNetworkStats({}, abortSignal)
  }

  // reportRecoveryStatus publishes renderer-owned recovery facts to the
  // session-local status resource.
  public async reportRecoveryStatus(
    req: ReportRecoveryStatusRequest,
    abortSignal?: AbortSignal,
  ): Promise<ReportRecoveryStatusResponse> {
    return await this.service.ReportRecoveryStatus(req, abortSignal)
  }

  // watchRecoveryStatus streams composed runtime recovery status snapshots.
  public watchRecoveryStatus(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchRecoveryStatusResponse> {
    return this.service.WatchRecoveryStatus({}, abortSignal)
  }
}
