import * as $ from '@goscript/builtin/index.js'
import * as context from '@goscript/context/index.js'
import * as srpc from '@goscript/github.com/aperturerobotics/starpc/srpc/index.js'
import * as resource from '@goscript/github.com/s4wave/spacewave/bldr/resource/index.js'
import * as resource_server from '@goscript/github.com/s4wave/spacewave/bldr/resource/server/index.js'

export interface ResourceRef {
  GetResourceID(): number
  GetClient(): [srpc.Client | null, $.GoError]
  Release(): void
}

export class Client implements resource_server.ResourceClientContext {
  private released = false

  constructor(
    private ctx: context.Context,
    private server: resource_server.ResourceServer,
  ) {}

  public AccessRootResource(): ResourceRef {
    return this.CreateResourceReference(1)
  }

  public CreateResourceReference(resourceID: number): ResourceRef {
    return new resourceRef(this, resourceID)
  }

  public Release(): void {
    this.released = true
  }

  public Context(): context.Context {
    return this.ctx
  }

  public AddResource(
    mux: srpc.Invoker | null,
    releaseFn: (() => void) | null,
  ): [number, $.GoError] {
    if (this.released) {
      return [0, resource.ErrClientReleased]
    }
    return this.server.AddResource(mux, releaseFn)
  }

  public AddResourceValue(
    mux: srpc.Invoker | null,
    _value: unknown,
    releaseFn: (() => void) | null,
  ): [number, $.GoError] {
    return this.AddResource(mux, releaseFn)
  }

  public AttachRawInvoker(
    _ctx: context.Context | null,
    _label: string,
    mux: srpc.Invoker | null,
  ): [number, $.GoError] {
    return this.AttachResource(_ctx, _label, mux)
  }

  public AttachResourceTree(
    _ctx: context.Context | null,
    _label: string,
    mux: srpc.Invoker | null,
  ): [number, $.GoError] {
    return this.AttachResource(_ctx, _label, mux)
  }

  public AttachResource(
    _ctx: context.Context | null,
    _label: string,
    mux: srpc.Invoker | null,
  ): [number, $.GoError] {
    return this.AddResource(mux, null)
  }

  public DetachResource(
    _ctx: context.Context | null,
    resourceID: number,
  ): $.GoError {
    return this.ReleaseResource(resourceID) ? null : resource.ErrResourceNotFound
  }

  public ReleaseResource(resourceID: number): boolean {
    return this.server.ReleaseResource(resourceID)
  }

  public GetResourceValue(resourceID: number): [unknown, $.GoError] {
    return this.server.GetResourceValue(resourceID)
  }

  public GetAttachedResource(_resourceID: number): [srpc.Client | null, $.GoError] {
    return [null, resource.ErrResourceNotFound]
  }

  public getResourceClient(resourceID: number): [srpc.Client | null, $.GoError] {
    const mux = this.server.GetResourceMux(resourceID)
    if (mux == null) {
      return [null, resource.ErrResourceNotFound]
    }
    return [
      srpc.NewClientWithInvoker(mux, (ctx) =>
        resource_server.WithResourceClientContext(ctx, this),
      ),
      null,
    ]
  }
}

class resourceRef implements ResourceRef {
  private released = false

  constructor(
    private client: Client,
    private resourceID: number,
  ) {}

  public GetResourceID(): number {
    return this.resourceID
  }

  public GetClient(): [srpc.Client | null, $.GoError] {
    if (this.released) {
      return [null, resource.ErrResourceOrClientReleased]
    }
    return this.client.getResourceClient(this.resourceID)
  }

  public Release(): void {
    if (this.released) {
      return
    }
    this.released = true
    this.client.ReleaseResource(this.resourceID)
  }
}

export function NewClient(
  ctx: context.Context,
  service: resource.SRPCResourceServiceClient | null,
): [Client | null, $.GoError] {
  const srpcClient = service?.SRPCClient() ?? null
  const invoker = (srpcClient as { invoker?: srpc.Invoker | null } | null)
    ?.invoker ?? null
  const server =
    resource_server.GetResourceServerForInvoker(invoker) ??
    resource_server.GetFallbackResourceServer()
  if (server == null) {
    return [null, resource.ErrResourceNotFound]
  }
  return [new Client(ctx ?? context.Background(), server), null]
}
