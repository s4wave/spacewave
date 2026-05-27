import * as $ from '@goscript/builtin/index.js'
import * as context from '@goscript/context/index.js'
import * as srpc from '@goscript/github.com/aperturerobotics/starpc/srpc/index.js'
import * as resource from '@goscript/github.com/s4wave/spacewave/bldr/resource/index.js'

const contextKey = {}
const registeredServers = new WeakMap<object, ResourceServer>()
let lastResourceServer: ResourceServer | null = null

export type ResourceClientContext = {
  Context(): context.Context
  AddResource(mux: srpc.Invoker | null, releaseFn: (() => void) | null): [
    number,
    $.GoError,
  ]
  AddResourceValue(
    mux: srpc.Invoker | null,
    value: unknown,
    releaseFn: (() => void) | null,
  ): [number, $.GoError]
  ReleaseResource(resourceID: number): boolean
  GetResourceValue(resourceID: number): [unknown, $.GoError]
  GetAttachedResource(resourceID: number): [srpc.Client | null, $.GoError]
}

type trackedResource = {
  mux: srpc.Invoker | null
  value: unknown
  releaseFn: (() => void) | null
}

type ResourceMuxRegister =
  | ((mux: srpc.Mux | null) => $.GoError | globalThis.Promise<$.GoError>)
  | null

export class ResourceServer {
  private resources = new Map<number, trackedResource>()
  private resourceIDCtr = 1

  constructor(rootResourceMux: srpc.Invoker | null) {
    this.resources.set(1, {
      mux: rootResourceMux ?? srpc.NewMux(),
      value: null,
      releaseFn: null,
    })
    lastResourceServer = this
  }

  public Register(mux: srpc.Mux | null): $.GoError {
    if (mux != null) {
      registeredServers.set(mux, this)
    }
    return null
  }

  public AddResource(
    mux: srpc.Invoker | null,
    releaseFn: (() => void) | null,
  ): [number, $.GoError] {
    return this.AddResourceValue(mux, null, releaseFn)
  }

  public AddResourceValue(
    mux: srpc.Invoker | null,
    value: unknown,
    releaseFn: (() => void) | null,
  ): [number, $.GoError] {
    this.resourceIDCtr++
    const resourceID = this.resourceIDCtr
    this.resources.set(resourceID, { mux, value, releaseFn })
    return [resourceID, null]
  }

  public ReleaseResource(resourceID: number): boolean {
    const res = this.resources.get(resourceID)
    if (res == null || resourceID === 1) {
      return false
    }
    this.resources.delete(resourceID)
    res.releaseFn?.()
    return true
  }

  public GetResourceValue(resourceID: number): [unknown, $.GoError] {
    const res = this.resources.get(resourceID)
    if (res == null || res.value == null) {
      return [null, resource.ErrResourceNotFound]
    }
    return [res.value, null]
  }

  public GetResourceMux(resourceID: number): srpc.Invoker | null {
    return this.resources.get(resourceID)?.mux ?? null
  }
}

export function NewResourceServer(
  rootResourceMux: srpc.Invoker | null,
): ResourceServer {
  return new ResourceServer(rootResourceMux)
}

export function NewResourceMux(
  ...register: ResourceMuxRegister[]
): srpc.Mux {
  const mux = srpc.NewMux()
  for (const fn of register) {
    if (fn == null) {
      continue
    }
    const err = fn(mux)
    if (err != null && typeof (err as PromiseLike<$.GoError>).then === 'function') {
      throw new globalThis.Error(
        'resource_server.NewResourceMux does not support async registration',
      )
    }
    if (err != null) {
      throw err
    }
  }
  return mux
}

export async function ConstructChildResource<T>(
  ctx: context.Context,
  buildFn:
    | ((
        subCtx: context.Context | null,
      ) =>
        | [srpc.Invoker | null, T, (() => void) | null, $.GoError]
        | Promise<[srpc.Invoker | null, T, (() => void) | null, $.GoError]>)
    | null,
): Promise<[T, number, $.GoError]> {
  let zero = null as T
  const client = GetResourceClientContext(ctx)
  if (client == null || buildFn == null) {
    return [zero, 0, resource.ErrNoResourceClientContext]
  }
  const [mux, result, releaseFn, err] = await buildFn(client.Context())
  if (err != null) {
    return [zero, 0, err]
  }
  const [resourceID, addErr] = client.AddResourceValue(mux, result, releaseFn)
  if (addErr != null) {
    releaseFn?.()
    return [zero, 0, addErr]
  }
  return [result, resourceID, null]
}

export function GetResourceServerForInvoker(
  invoker: srpc.Invoker | null,
): ResourceServer | null {
  return invoker == null ? null : registeredServers.get(invoker) ?? null
}

export function GetFallbackResourceServer(): ResourceServer | null {
  return lastResourceServer
}

export function WithResourceClientContext(
  ctx: context.Context,
  client: ResourceClientContext | null,
): context.Context {
  return context.WithValue(ctx, contextKey, client)
}

export function GetResourceClientContext(
  ctx: context.Context,
): ResourceClientContext | null {
  return (ctx?.Value(contextKey) as ResourceClientContext | null) ?? null
}

export function MustGetResourceClientContext(
  ctx: context.Context,
): [ResourceClientContext | null, $.GoError] {
  const client = GetResourceClientContext(ctx)
  if (client == null) {
    return [null, resource.ErrNoResourceClientContext]
  }
  return [client, null]
}
