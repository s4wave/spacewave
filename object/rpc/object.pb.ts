/* eslint-disable */
import { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'object.rpc'

/** RmObjectStoreRequest requests to remove an object store and all contents. */
export interface RmObjectStoreRequest {
  /** ObjectStoreId is the object store identifier to remove. */
  objectStoreId: string
}

/** RmObjectStoreResponse is the response to removing an object store. */
export interface RmObjectStoreResponse {
  /**
   * Error is any error removing the object store.
   * Will be empty if the store did not exist.
   */
  error: string
}

function createBaseRmObjectStoreRequest(): RmObjectStoreRequest {
  return { objectStoreId: '' }
}

export const RmObjectStoreRequest = {
  encode(
    message: RmObjectStoreRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.objectStoreId !== '') {
      writer.uint32(10).string(message.objectStoreId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): RmObjectStoreRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRmObjectStoreRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.objectStoreId = reader.string()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RmObjectStoreRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RmObjectStoreRequest | RmObjectStoreRequest[]>
      | Iterable<RmObjectStoreRequest | RmObjectStoreRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RmObjectStoreRequest.encode(p).finish()]
        }
      } else {
        yield* [RmObjectStoreRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RmObjectStoreRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RmObjectStoreRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RmObjectStoreRequest.decode(p)]
        }
      } else {
        yield* [RmObjectStoreRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RmObjectStoreRequest {
    return {
      objectStoreId: isSet(object.objectStoreId)
        ? String(object.objectStoreId)
        : '',
    }
  },

  toJSON(message: RmObjectStoreRequest): unknown {
    const obj: any = {}
    if (message.objectStoreId !== '') {
      obj.objectStoreId = message.objectStoreId
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RmObjectStoreRequest>, I>>(
    base?: I,
  ): RmObjectStoreRequest {
    return RmObjectStoreRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RmObjectStoreRequest>, I>>(
    object: I,
  ): RmObjectStoreRequest {
    const message = createBaseRmObjectStoreRequest()
    message.objectStoreId = object.objectStoreId ?? ''
    return message
  },
}

function createBaseRmObjectStoreResponse(): RmObjectStoreResponse {
  return { error: '' }
}

export const RmObjectStoreResponse = {
  encode(
    message: RmObjectStoreResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): RmObjectStoreResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRmObjectStoreResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.error = reader.string()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RmObjectStoreResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RmObjectStoreResponse | RmObjectStoreResponse[]>
      | Iterable<RmObjectStoreResponse | RmObjectStoreResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RmObjectStoreResponse.encode(p).finish()]
        }
      } else {
        yield* [RmObjectStoreResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RmObjectStoreResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RmObjectStoreResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RmObjectStoreResponse.decode(p)]
        }
      } else {
        yield* [RmObjectStoreResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RmObjectStoreResponse {
    return { error: isSet(object.error) ? String(object.error) : '' }
  },

  toJSON(message: RmObjectStoreResponse): unknown {
    const obj: any = {}
    if (message.error !== '') {
      obj.error = message.error
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RmObjectStoreResponse>, I>>(
    base?: I,
  ): RmObjectStoreResponse {
    return RmObjectStoreResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RmObjectStoreResponse>, I>>(
    object: I,
  ): RmObjectStoreResponse {
    const message = createBaseRmObjectStoreResponse()
    message.error = object.error ?? ''
    return message
  },
}

/** ObjectStore implements ObjectStore wrapping a object_store.Store. */
export interface ObjectStore {
  /**
   * ObjectStoreRpc is a rpc request for an ObjectStore Kvtx service by ID.
   * Corresponds to a call to BuildObjectStoreAPI.
   * If the ObjectStore doesn't exist, it will be created.
   * Exposes service: rpc.kvtx.Kvtx
   * Component ID: object store ID.
   */
  ObjectStoreRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
  /** RmObjectStore deletes the object store and all contents by ID. */
  RmObjectStore(
    request: RmObjectStoreRequest,
    abortSignal?: AbortSignal,
  ): Promise<RmObjectStoreResponse>
}

export const ObjectStoreServiceName = 'object.rpc.ObjectStore'
export class ObjectStoreClientImpl implements ObjectStore {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || ObjectStoreServiceName
    this.rpc = rpc
    this.ObjectStoreRpc = this.ObjectStoreRpc.bind(this)
    this.RmObjectStore = this.RmObjectStore.bind(this)
  }
  ObjectStoreRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'ObjectStoreRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }

  RmObjectStore(
    request: RmObjectStoreRequest,
    abortSignal?: AbortSignal,
  ): Promise<RmObjectStoreResponse> {
    const data = RmObjectStoreRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'RmObjectStore',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      RmObjectStoreResponse.decode(_m0.Reader.create(data)),
    )
  }
}

/** ObjectStore implements ObjectStore wrapping a object_store.Store. */
export type ObjectStoreDefinition = typeof ObjectStoreDefinition
export const ObjectStoreDefinition = {
  name: 'ObjectStore',
  fullName: 'object.rpc.ObjectStore',
  methods: {
    /**
     * ObjectStoreRpc is a rpc request for an ObjectStore Kvtx service by ID.
     * Corresponds to a call to BuildObjectStoreAPI.
     * If the ObjectStore doesn't exist, it will be created.
     * Exposes service: rpc.kvtx.Kvtx
     * Component ID: object store ID.
     */
    objectStoreRpc: {
      name: 'ObjectStoreRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
    /** RmObjectStore deletes the object store and all contents by ID. */
    rmObjectStore: {
      name: 'RmObjectStore',
      requestType: RmObjectStoreRequest,
      requestStream: false,
      responseType: RmObjectStoreResponse,
      responseStream: false,
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

type Builtin =
  | Date
  | Function
  | Uint8Array
  | string
  | number
  | boolean
  | undefined

export type DeepPartial<T> = T extends Builtin
  ? T
  : T extends Long
  ? string | number | Long
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string }
  ? { [K in keyof Omit<T, '$case'>]?: DeepPartial<T[K]> } & {
      $case: T['$case']
    }
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

type KeysOfUnion<T> = T extends T ? keyof T : never
export type Exact<P, I extends P> = P extends Builtin
  ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & {
      [K in Exclude<keyof I, KeysOfUnion<P>>]: never
    }

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
