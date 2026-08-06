import {
  createContextKey,
  serverContextValue,
  withServerContextValue,
  type Mux,
  type ServerContext,
} from 'starpc'
import type { ClientResourceRef } from '../client.js'
import type { RemoteResourceClient } from './tracked-client.js'

export interface ConstructResult<T> {
  mux: Mux
  result: T
  releaseFn?: () => void
}

export interface ResourceCall {
  constructChildResource<T>(
    build: (signal: AbortSignal) => ConstructResult<T>,
  ): { result: T; resourceId: number }
  getAttachedRef(resourceId: number): ClientResourceRef
}

interface ResourceCallInit {
  readonly client: RemoteResourceClient
  readonly parentResourceId: number
  readonly serviceId: string
  readonly methodId: string
}

class ActiveResourceCall implements ResourceCall {
  constructor(private readonly init: ResourceCallInit) {}

  constructChildResource<T>(
    build: (signal: AbortSignal) => ConstructResult<T>,
  ): { result: T; resourceId: number } {
    const { client } = this.init
    const childController = new AbortController()
    const onClientAbort = () => childController.abort()
    client.signal.addEventListener('abort', onClientAbort, { once: true })

    let built: ConstructResult<T>
    try {
      built = build(childController.signal)
    } catch (error) {
      client.signal.removeEventListener('abort', onClientAbort)
      childController.abort()
      throw error
    }

    const release = () => {
      client.signal.removeEventListener('abort', onClientAbort)
      try {
        built.releaseFn?.()
      } finally {
        childController.abort()
      }
    }
    try {
      const resourceId = client.addResource(
        built.mux,
        release,
        this.init.parentResourceId,
        this.init.serviceId,
        this.init.methodId,
      )
      return { result: built.result, resourceId }
    } catch (error) {
      release()
      throw error
    }
  }

  getAttachedRef(resourceId: number): ClientResourceRef {
    return this.init.client.getAttachedRef(resourceId)
  }
}

const resourceCallKey = createContextKey<ResourceCall>()

export function withResourceCall(
  context: ServerContext,
  init: ResourceCallInit,
): ServerContext {
  return withServerContextValue(
    context,
    resourceCallKey,
    new ActiveResourceCall(init),
  )
}

export function getResourceCall(context: ServerContext): ResourceCall {
  const call = serverContextValue(context, resourceCallKey)
  if (!call) throw new Error('no resource call in server context')
  return call
}
