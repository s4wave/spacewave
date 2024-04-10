/* eslint-disable */
import { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { VolumeInfo } from '../volume.pb.js'

export const protobufPackage = 'volume.rpc'

/** WatchVolumeInfoRequest is a request to watch volume information. */
export interface WatchVolumeInfoRequest {
  /** VolumeId is the volume id to watch. */
  volumeId: string
}

/** WatchVolumeInfoResponse is a state snapshot of the volume info. */
export interface WatchVolumeInfoResponse {
  /** NotFound is set of the volume info is empty (not found). */
  notFound: boolean
  /** VolumeInfo contains the located volume information. */
  volumeInfo: VolumeInfo | undefined
}

/** GetVolumeInfoRequest is a request to get volume information. */
export interface GetVolumeInfoRequest {}

/** GetVolumeInfoResponse is the response to the request for volume info. */
export interface GetVolumeInfoResponse {
  /** VolumeInfo is the volume information object. */
  volumeInfo: VolumeInfo | undefined
}

/** GetPeerPrivRequest is a request to get the volume peer privkey. */
export interface GetPeerPrivRequest {}

/** GetPeerPrivResponse is the response to looking up the volume peer privkey. */
export interface GetPeerPrivResponse {
  /** PrivKey is the private key. */
  privKey: string
}

function createBaseWatchVolumeInfoRequest(): WatchVolumeInfoRequest {
  return { volumeId: '' }
}

export const WatchVolumeInfoRequest = {
  encode(
    message: WatchVolumeInfoRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.volumeId !== '') {
      writer.uint32(10).string(message.volumeId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): WatchVolumeInfoRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWatchVolumeInfoRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.volumeId = reader.string()
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
  // Transform<WatchVolumeInfoRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WatchVolumeInfoRequest | WatchVolumeInfoRequest[]>
      | Iterable<WatchVolumeInfoRequest | WatchVolumeInfoRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WatchVolumeInfoRequest.encode(p).finish()]
        }
      } else {
        yield* [WatchVolumeInfoRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WatchVolumeInfoRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WatchVolumeInfoRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WatchVolumeInfoRequest.decode(p)]
        }
      } else {
        yield* [WatchVolumeInfoRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WatchVolumeInfoRequest {
    return {
      volumeId: isSet(object.volumeId)
        ? globalThis.String(object.volumeId)
        : '',
    }
  },

  toJSON(message: WatchVolumeInfoRequest): unknown {
    const obj: any = {}
    if (message.volumeId !== '') {
      obj.volumeId = message.volumeId
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WatchVolumeInfoRequest>, I>>(
    base?: I,
  ): WatchVolumeInfoRequest {
    return WatchVolumeInfoRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WatchVolumeInfoRequest>, I>>(
    object: I,
  ): WatchVolumeInfoRequest {
    const message = createBaseWatchVolumeInfoRequest()
    message.volumeId = object.volumeId ?? ''
    return message
  },
}

function createBaseWatchVolumeInfoResponse(): WatchVolumeInfoResponse {
  return { notFound: false, volumeInfo: undefined }
}

export const WatchVolumeInfoResponse = {
  encode(
    message: WatchVolumeInfoResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.notFound !== false) {
      writer.uint32(8).bool(message.notFound)
    }
    if (message.volumeInfo !== undefined) {
      VolumeInfo.encode(message.volumeInfo, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): WatchVolumeInfoResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWatchVolumeInfoResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.notFound = reader.bool()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.volumeInfo = VolumeInfo.decode(reader, reader.uint32())
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
  // Transform<WatchVolumeInfoResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WatchVolumeInfoResponse | WatchVolumeInfoResponse[]>
      | Iterable<WatchVolumeInfoResponse | WatchVolumeInfoResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WatchVolumeInfoResponse.encode(p).finish()]
        }
      } else {
        yield* [WatchVolumeInfoResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WatchVolumeInfoResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WatchVolumeInfoResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WatchVolumeInfoResponse.decode(p)]
        }
      } else {
        yield* [WatchVolumeInfoResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WatchVolumeInfoResponse {
    return {
      notFound: isSet(object.notFound)
        ? globalThis.Boolean(object.notFound)
        : false,
      volumeInfo: isSet(object.volumeInfo)
        ? VolumeInfo.fromJSON(object.volumeInfo)
        : undefined,
    }
  },

  toJSON(message: WatchVolumeInfoResponse): unknown {
    const obj: any = {}
    if (message.notFound !== false) {
      obj.notFound = message.notFound
    }
    if (message.volumeInfo !== undefined) {
      obj.volumeInfo = VolumeInfo.toJSON(message.volumeInfo)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WatchVolumeInfoResponse>, I>>(
    base?: I,
  ): WatchVolumeInfoResponse {
    return WatchVolumeInfoResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WatchVolumeInfoResponse>, I>>(
    object: I,
  ): WatchVolumeInfoResponse {
    const message = createBaseWatchVolumeInfoResponse()
    message.notFound = object.notFound ?? false
    message.volumeInfo =
      object.volumeInfo !== undefined && object.volumeInfo !== null
        ? VolumeInfo.fromPartial(object.volumeInfo)
        : undefined
    return message
  },
}

function createBaseGetVolumeInfoRequest(): GetVolumeInfoRequest {
  return {}
}

export const GetVolumeInfoRequest = {
  encode(
    _: GetVolumeInfoRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GetVolumeInfoRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetVolumeInfoRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetVolumeInfoRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetVolumeInfoRequest | GetVolumeInfoRequest[]>
      | Iterable<GetVolumeInfoRequest | GetVolumeInfoRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetVolumeInfoRequest.encode(p).finish()]
        }
      } else {
        yield* [GetVolumeInfoRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetVolumeInfoRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetVolumeInfoRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetVolumeInfoRequest.decode(p)]
        }
      } else {
        yield* [GetVolumeInfoRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): GetVolumeInfoRequest {
    return {}
  },

  toJSON(_: GetVolumeInfoRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<GetVolumeInfoRequest>, I>>(
    base?: I,
  ): GetVolumeInfoRequest {
    return GetVolumeInfoRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetVolumeInfoRequest>, I>>(
    _: I,
  ): GetVolumeInfoRequest {
    const message = createBaseGetVolumeInfoRequest()
    return message
  },
}

function createBaseGetVolumeInfoResponse(): GetVolumeInfoResponse {
  return { volumeInfo: undefined }
}

export const GetVolumeInfoResponse = {
  encode(
    message: GetVolumeInfoResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.volumeInfo !== undefined) {
      VolumeInfo.encode(message.volumeInfo, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GetVolumeInfoResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetVolumeInfoResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.volumeInfo = VolumeInfo.decode(reader, reader.uint32())
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
  // Transform<GetVolumeInfoResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetVolumeInfoResponse | GetVolumeInfoResponse[]>
      | Iterable<GetVolumeInfoResponse | GetVolumeInfoResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetVolumeInfoResponse.encode(p).finish()]
        }
      } else {
        yield* [GetVolumeInfoResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetVolumeInfoResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetVolumeInfoResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetVolumeInfoResponse.decode(p)]
        }
      } else {
        yield* [GetVolumeInfoResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetVolumeInfoResponse {
    return {
      volumeInfo: isSet(object.volumeInfo)
        ? VolumeInfo.fromJSON(object.volumeInfo)
        : undefined,
    }
  },

  toJSON(message: GetVolumeInfoResponse): unknown {
    const obj: any = {}
    if (message.volumeInfo !== undefined) {
      obj.volumeInfo = VolumeInfo.toJSON(message.volumeInfo)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetVolumeInfoResponse>, I>>(
    base?: I,
  ): GetVolumeInfoResponse {
    return GetVolumeInfoResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetVolumeInfoResponse>, I>>(
    object: I,
  ): GetVolumeInfoResponse {
    const message = createBaseGetVolumeInfoResponse()
    message.volumeInfo =
      object.volumeInfo !== undefined && object.volumeInfo !== null
        ? VolumeInfo.fromPartial(object.volumeInfo)
        : undefined
    return message
  },
}

function createBaseGetPeerPrivRequest(): GetPeerPrivRequest {
  return {}
}

export const GetPeerPrivRequest = {
  encode(
    _: GetPeerPrivRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetPeerPrivRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetPeerPrivRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetPeerPrivRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetPeerPrivRequest | GetPeerPrivRequest[]>
      | Iterable<GetPeerPrivRequest | GetPeerPrivRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetPeerPrivRequest.encode(p).finish()]
        }
      } else {
        yield* [GetPeerPrivRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetPeerPrivRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetPeerPrivRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetPeerPrivRequest.decode(p)]
        }
      } else {
        yield* [GetPeerPrivRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): GetPeerPrivRequest {
    return {}
  },

  toJSON(_: GetPeerPrivRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<GetPeerPrivRequest>, I>>(
    base?: I,
  ): GetPeerPrivRequest {
    return GetPeerPrivRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetPeerPrivRequest>, I>>(
    _: I,
  ): GetPeerPrivRequest {
    const message = createBaseGetPeerPrivRequest()
    return message
  },
}

function createBaseGetPeerPrivResponse(): GetPeerPrivResponse {
  return { privKey: '' }
}

export const GetPeerPrivResponse = {
  encode(
    message: GetPeerPrivResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.privKey !== '') {
      writer.uint32(10).string(message.privKey)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetPeerPrivResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetPeerPrivResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.privKey = reader.string()
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
  // Transform<GetPeerPrivResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetPeerPrivResponse | GetPeerPrivResponse[]>
      | Iterable<GetPeerPrivResponse | GetPeerPrivResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetPeerPrivResponse.encode(p).finish()]
        }
      } else {
        yield* [GetPeerPrivResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetPeerPrivResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetPeerPrivResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetPeerPrivResponse.decode(p)]
        }
      } else {
        yield* [GetPeerPrivResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetPeerPrivResponse {
    return {
      privKey: isSet(object.privKey) ? globalThis.String(object.privKey) : '',
    }
  },

  toJSON(message: GetPeerPrivResponse): unknown {
    const obj: any = {}
    if (message.privKey !== '') {
      obj.privKey = message.privKey
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetPeerPrivResponse>, I>>(
    base?: I,
  ): GetPeerPrivResponse {
    return GetPeerPrivResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetPeerPrivResponse>, I>>(
    object: I,
  ): GetPeerPrivResponse {
    const message = createBaseGetPeerPrivResponse()
    message.privKey = object.privKey ?? ''
    return message
  },
}

/** AccessVolumes is a service to access available volumes over RPC. */
export interface AccessVolumes {
  /**
   * WatchVolumeInfo watches information about a volume.
   * The most recent message contains the most recently known state.
   * If the volume was not found (directive is idle) returns empty.
   */
  WatchVolumeInfo(
    request: WatchVolumeInfoRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchVolumeInfoResponse>
  /**
   * VolumeRpc uses the LookupVolume directive access a Volume handle.
   * Exposes the ProxyVolume service.
   * Id: volume id
   */
  VolumeRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
}

export const AccessVolumesServiceName = 'volume.rpc.AccessVolumes'
export class AccessVolumesClientImpl implements AccessVolumes {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || AccessVolumesServiceName
    this.rpc = rpc
    this.WatchVolumeInfo = this.WatchVolumeInfo.bind(this)
    this.VolumeRpc = this.VolumeRpc.bind(this)
  }
  WatchVolumeInfo(
    request: WatchVolumeInfoRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchVolumeInfoResponse> {
    const data = WatchVolumeInfoRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'WatchVolumeInfo',
      data,
      abortSignal || undefined,
    )
    return WatchVolumeInfoResponse.decodeTransform(result)
  }

  VolumeRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'VolumeRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }
}

/** AccessVolumes is a service to access available volumes over RPC. */
export type AccessVolumesDefinition = typeof AccessVolumesDefinition
export const AccessVolumesDefinition = {
  name: 'AccessVolumes',
  fullName: 'volume.rpc.AccessVolumes',
  methods: {
    /**
     * WatchVolumeInfo watches information about a volume.
     * The most recent message contains the most recently known state.
     * If the volume was not found (directive is idle) returns empty.
     */
    watchVolumeInfo: {
      name: 'WatchVolumeInfo',
      requestType: WatchVolumeInfoRequest,
      requestStream: false,
      responseType: WatchVolumeInfoResponse,
      responseStream: true,
      options: {},
    },
    /**
     * VolumeRpc uses the LookupVolume directive access a Volume handle.
     * Exposes the ProxyVolume service.
     * Id: volume id
     */
    volumeRpc: {
      name: 'VolumeRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
  },
} as const

/**
 * ProxyVolume is a service exposing a Volume handle via RPC.
 *
 * Other available services:
 *  - rpc.block.BlockStore
 *  - rpc.bucket.BucketStore
 *  - rpc.mqueue.MqueueStore
 *  - rpc.object.ObjectStore
 */
export interface ProxyVolume {
  /** GetVolumeInfo returns the basic volume information. */
  GetVolumeInfo(
    request: GetVolumeInfoRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetVolumeInfoResponse>
  /**
   * GetPeerPriv returns the volume peer private key.
   * Returns ErrPrivKeyUnavailable if the private key is unavailable.
   */
  GetPeerPriv(
    request: GetPeerPrivRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetPeerPrivResponse>
}

export const ProxyVolumeServiceName = 'volume.rpc.ProxyVolume'
export class ProxyVolumeClientImpl implements ProxyVolume {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || ProxyVolumeServiceName
    this.rpc = rpc
    this.GetVolumeInfo = this.GetVolumeInfo.bind(this)
    this.GetPeerPriv = this.GetPeerPriv.bind(this)
  }
  GetVolumeInfo(
    request: GetVolumeInfoRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetVolumeInfoResponse> {
    const data = GetVolumeInfoRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'GetVolumeInfo',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      GetVolumeInfoResponse.decode(_m0.Reader.create(data)),
    )
  }

  GetPeerPriv(
    request: GetPeerPrivRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetPeerPrivResponse> {
    const data = GetPeerPrivRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'GetPeerPriv',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      GetPeerPrivResponse.decode(_m0.Reader.create(data)),
    )
  }
}

/**
 * ProxyVolume is a service exposing a Volume handle via RPC.
 *
 * Other available services:
 *  - rpc.block.BlockStore
 *  - rpc.bucket.BucketStore
 *  - rpc.mqueue.MqueueStore
 *  - rpc.object.ObjectStore
 */
export type ProxyVolumeDefinition = typeof ProxyVolumeDefinition
export const ProxyVolumeDefinition = {
  name: 'ProxyVolume',
  fullName: 'volume.rpc.ProxyVolume',
  methods: {
    /** GetVolumeInfo returns the basic volume information. */
    getVolumeInfo: {
      name: 'GetVolumeInfo',
      requestType: GetVolumeInfoRequest,
      requestStream: false,
      responseType: GetVolumeInfoResponse,
      responseStream: false,
      options: {},
    },
    /**
     * GetPeerPriv returns the volume peer private key.
     * Returns ErrPrivKeyUnavailable if the private key is unavailable.
     */
    getPeerPriv: {
      name: 'GetPeerPriv',
      requestType: GetPeerPrivRequest,
      requestStream: false,
      responseType: GetPeerPrivResponse,
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
    : T extends globalThis.Array<infer U>
      ? globalThis.Array<DeepPartial<U>>
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
