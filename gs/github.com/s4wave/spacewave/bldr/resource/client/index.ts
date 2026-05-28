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

type attachedResource = {
  mux: srpc.Invoker | null
  releaseFn: (() => void) | null
}

export class Client implements resource_server.ResourceClientContext {
  private released = false
  private attachedResources = new Map<number, attachedResource>()

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
    for (const res of this.attachedResources.values()) {
      res.releaseFn?.()
    }
    this.attachedResources.clear()
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
    value: unknown,
    releaseFn: (() => void) | null,
  ): [number, $.GoError] {
    if (this.released) {
      return [0, resource.ErrClientReleased]
    }
    return this.server.AddResourceValue(mux, value, releaseFn)
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
    if (this.released) {
      return [0, resource.ErrClientReleased]
    }
    const resourceID = this.server.AllocateResourceID()
    this.attachedResources.set(resourceID, { mux, releaseFn: null })
    return [resourceID, null]
  }

  public SetAttachedRelease(
    resourceID: number,
    releaseFn: (() => void) | null,
  ): void {
    const attached = this.attachedResources.get(resourceID)
    if (attached != null) {
      attached.releaseFn = releaseFn
    }
  }

  public DetachResource(
    _ctx: context.Context | null,
    resourceID: number,
  ): $.GoError {
    return this.ReleaseResource(resourceID) ? null : resource.ErrResourceNotFound
  }

  public ReleaseResource(resourceID: number): boolean {
    const attached = this.attachedResources.get(resourceID)
    if (attached != null) {
      this.attachedResources.delete(resourceID)
      attached.releaseFn?.()
      return true
    }
    return this.server.ReleaseResource(resourceID)
  }

  public GetResourceValue(resourceID: number): [unknown, $.GoError] {
    return this.server.GetResourceValue(resourceID)
  }

  public GetAttachedResource(resourceID: number): [srpc.Client | null, $.GoError] {
    const attached = this.attachedResources.get(resourceID)
    if (attached?.mux == null) {
      return [null, resource.ErrResourceNotFound]
    }
    const owner = new attachedResourceOwner(this)
    return [
      srpc.NewClientWithInvoker(attached.mux, (ctx) =>
        resource_server.WithResourceClientContext(ctx, owner),
      ),
      null,
    ]
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

class attachedResourceOwner implements resource_server.ResourceClientContext {
  constructor(private client: Client) {}

  public Context(): context.Context {
    return this.client.Context()
  }

  public AddResource(
    mux: srpc.Invoker | null,
    releaseFn: (() => void) | null,
  ): [number, $.GoError] {
    return this.AddResourceValue(mux, null, releaseFn)
  }

  public AddResourceValue(
    mux: srpc.Invoker | null,
    _value: unknown,
    releaseFn: (() => void) | null,
  ): [number, $.GoError] {
    const [resourceID, err] = this.client.AttachResource(
      this.client.Context(),
      'attached-child',
      mux,
    )
    if (err != null) {
      return [0, err]
    }
    this.client.SetAttachedRelease(resourceID, releaseFn)
    return [resourceID, null]
  }

  public ReleaseResource(resourceID: number): boolean {
    return this.client.ReleaseResource(resourceID)
  }

  public GetResourceValue(_resourceID: number): [unknown, $.GoError] {
    return [null, resource.ErrResourceNotFound]
  }

  public GetAttachedResource(_resourceID: number): [srpc.Client | null, $.GoError] {
    return [null, resource.ErrResourceNotFound]
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
