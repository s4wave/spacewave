import type { Mux } from 'starpc'
import { getCurrentResourceClient } from './server.js'

// ConstructResult is the return type of the buildFn callback.
interface ConstructResult<T> {
  mux: Mux
  result: T
  releaseFn?: () => void
}

// constructChildResource creates a child resource on the current
// client. Must be called synchronously within an RPC handler
// dispatched through ResourceRpc (before any await).
//
// The child resource's signal derives from the client session
// (not the RPC call). Child resources outlive individual RPCs.
function constructChildResource<T>(
  buildFn: (signal: AbortSignal) => ConstructResult<T>,
): { result: T; resourceId: number } {
  const client = getCurrentResourceClient()

  const childController = new AbortController()
  const onClientAbort = () => childController.abort()
  client.signal.addEventListener('abort', onClientAbort, { once: true })

  let built: ConstructResult<T>
  try {
    built = buildFn(childController.signal)
  } catch (err) {
    client.signal.removeEventListener('abort', onClientAbort)
    childController.abort()
    throw err
  }

  const { mux, result, releaseFn } = built

  const wrappedReleaseFn = () => {
    client.signal.removeEventListener('abort', onClientAbort)
    if (releaseFn) releaseFn()
    childController.abort()
  }

  let resourceId: number
  try {
    resourceId = client.addResource(mux, wrappedReleaseFn)
  } catch (err) {
    if (releaseFn) releaseFn()
    childController.abort()
    client.signal.removeEventListener('abort', onClientAbort)
    throw err
  }

  return { result, resourceId }
}

export { constructChildResource }
export type { ConstructResult }
