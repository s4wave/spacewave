import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  DebugDbResourceService,
  DebugDbResourceServiceClient,
} from './debugdb_srpc.pb.js'
import type { BenchmarkConfig, StorageInfo } from './debugdb.pb.js'
import { DebugDbBenchmark } from './benchmark.js'

// DebugDb wraps the DebugDbResourceService as a Resource.
export class DebugDb extends Resource {
  private service: DebugDbResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new DebugDbResourceServiceClient(resourceRef.client)
  }

  // getStorageInfo returns information about the current storage backend.
  public async getStorageInfo(abortSignal?: AbortSignal): Promise<StorageInfo> {
    const resp = await this.service.GetStorageInfo({}, abortSignal)
    return resp.info ?? {}
  }

  // startBenchmark starts a new benchmark run and returns a DebugDbBenchmark resource.
  public async startBenchmark(
    config?: BenchmarkConfig,
    abortSignal?: AbortSignal,
  ): Promise<DebugDbBenchmark> {
    const resp = await this.service.StartBenchmark(
      { config: config ?? {} },
      abortSignal,
    )
    return this.resourceRef.createResource(
      resp.resourceId ?? 0,
      DebugDbBenchmark,
    )
  }
}
