import * as $ from '@goscript/builtin/index.js'
import * as srpc from '@goscript/github.com/aperturerobotics/starpc/srpc/index.js'

export const ErrResourceNotFound = $.newError('resource not found')
export const ErrClientReleased = $.newError('client was released')
export const ErrInvalidResourceID = $.newError('invalid resource id')
export const ErrInvalidClientID = $.newError('invalid client id')
export const ErrInvalidComponentIDFormat = $.newError(
  'invalid component id format',
)
export const ErrResourceOrClientReleased = $.newError(
  'resource or client was released',
)
export const ErrNoResourceClientContext = $.newError(
  'no resource client context',
)

export class SRPCResourceServiceClient {
  constructor(private client: srpc.Client | null) {}

  public SRPCClient(): srpc.Client | null {
    return this.client
  }
}

export const SRPCResourceServiceServiceID = 'resource.ResourceService'

class resourceServiceHandler implements srpc.Handler {
  constructor(private invoker: srpc.Invoker | null) {}

  public GetServiceID(): string {
    return SRPCResourceServiceServiceID
  }

  public GetMethodIDs(): $.Slice<string> {
    return [
      'ResourceClient',
      'ResourceRpc',
      'ResourceRefRelease',
      'ResourceAttach',
    ]
  }

  public InvokeMethod(
    serviceID: string,
    methodID: string,
    stream: srpc.Stream | null,
  ): [boolean, $.GoError] | Promise<[boolean, $.GoError]> {
    return this.invoker?.InvokeMethod(serviceID, methodID, stream) ?? [
      false,
      null,
    ]
  }
}

export function NewSRPCResourceServiceClient(
  client: srpc.Client | null,
): SRPCResourceServiceClient {
  return new SRPCResourceServiceClient(client)
}

export function NewSRPCResourceServiceClientWithServiceID(
  client: srpc.Client | null,
  _serviceID: string,
): SRPCResourceServiceClient {
  return NewSRPCResourceServiceClient(client)
}

export function SRPCRegisterResourceService(
  mux: srpc.Mux | null,
  impl: srpc.Invoker | null,
): $.GoError {
  if (mux == null || impl == null) {
    return srpc.ErrUnimplemented
  }
  return mux.Register(new resourceServiceHandler(impl))
}
