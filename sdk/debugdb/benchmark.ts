import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  DebugDbBenchmarkService,
  DebugDbBenchmarkServiceClient,
} from './debugdb_srpc.pb.js'
import type { BenchmarkResults, WatchProgressResponse } from './debugdb.pb.js'

// DebugDbBenchmark wraps the DebugDbBenchmarkService as a Resource.
export class DebugDbBenchmark extends Resource {
  private service: DebugDbBenchmarkService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new DebugDbBenchmarkServiceClient(resourceRef.client)
  }

  // watchProgress streams benchmark progress updates.
  public watchProgress(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchProgressResponse> {
    return this.service.WatchProgress({}, abortSignal)
  }

  // getResults returns the full benchmark results after completion.
  public async getResults(
    abortSignal?: AbortSignal,
  ): Promise<BenchmarkResults> {
    const resp = await this.service.GetResults({}, abortSignal)
    return resp.results ?? {}
  }
}
