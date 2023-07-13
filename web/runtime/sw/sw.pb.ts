/* eslint-disable */
import { FetchRequest, FetchResponse } from '../../fetch/fetch.pb.js'

export const protobufPackage = 'web.runtime.sw'

/**
 * ServiceWorkerHost is exposed by the Go Runtime for the Worker to call.
 *
 * Implements FetchService.
 */
export interface ServiceWorkerHost {
  /** Fetch proxies a Fetch request with a streaming response. */
  Fetch(
    request: AsyncIterable<FetchRequest>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<FetchResponse>
}

export const ServiceWorkerHostServiceName = 'web.runtime.sw.ServiceWorkerHost'
export class ServiceWorkerHostClientImpl implements ServiceWorkerHost {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || ServiceWorkerHostServiceName
    this.rpc = rpc
    this.Fetch = this.Fetch.bind(this)
  }
  Fetch(
    request: AsyncIterable<FetchRequest>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<FetchResponse> {
    const data = FetchRequest.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'Fetch',
      data,
      abortSignal || undefined,
    )
    return FetchResponse.decodeTransform(result)
  }
}

/**
 * ServiceWorkerHost is exposed by the Go Runtime for the Worker to call.
 *
 * Implements FetchService.
 */
export type ServiceWorkerHostDefinition = typeof ServiceWorkerHostDefinition
export const ServiceWorkerHostDefinition = {
  name: 'ServiceWorkerHost',
  fullName: 'web.runtime.sw.ServiceWorkerHost',
  methods: {
    /** Fetch proxies a Fetch request with a streaming response. */
    fetch: {
      name: 'Fetch',
      requestType: FetchRequest,
      requestStream: true,
      responseType: FetchResponse,
      responseStream: true,
      options: {},
    },
  },
} as const

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>
  clientStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array>
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array>
}
