/* eslint-disable */
export const protobufPackage = 'web.runtime.sw'

/** ServiceWorkerHost is exposed by the Go Runtime for the Worker to call. */
export interface ServiceWorkerHost {}

export class ServiceWorkerHostClientImpl implements ServiceWorkerHost {
  private readonly rpc: Rpc
  constructor(rpc: Rpc) {
    this.rpc = rpc
  }
}

/** ServiceWorkerHost is exposed by the Go Runtime for the Worker to call. */
export type ServiceWorkerHostDefinition = typeof ServiceWorkerHostDefinition
export const ServiceWorkerHostDefinition = {
  name: 'ServiceWorkerHost',
  fullName: 'web.runtime.sw.ServiceWorkerHost',
  methods: {},
} as const

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array
  ): Promise<Uint8Array>
}
