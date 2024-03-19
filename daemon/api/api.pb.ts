/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef, PutOpts } from '../../block/block.pb.js'
import {
  ApplyBucketConfigResult,
  BucketOpArgs,
  Config as Config1,
} from '../../bucket/bucket.pb.js'
import { Event } from '../../bucket/event/event.pb.js'
import {
  ListBucketsRequest,
  VolumeBucketInfo,
  VolumeInfo,
} from '../../volume/volume.pb.js'

export const protobufPackage = 'hydra.api'

/** BucketOp is a bucket operation. */
export enum BucketOp {
  BucketOp_UNKNOWN = 0,
  BucketOp_BLOCK_GET = 1,
  BucketOp_BLOCK_PUT = 2,
  BucketOp_BLOCK_RM = 3,
  UNRECOGNIZED = -1,
}

export function bucketOpFromJSON(object: any): BucketOp {
  switch (object) {
    case 0:
    case 'BucketOp_UNKNOWN':
      return BucketOp.BucketOp_UNKNOWN
    case 1:
    case 'BucketOp_BLOCK_GET':
      return BucketOp.BucketOp_BLOCK_GET
    case 2:
    case 'BucketOp_BLOCK_PUT':
      return BucketOp.BucketOp_BLOCK_PUT
    case 3:
    case 'BucketOp_BLOCK_RM':
      return BucketOp.BucketOp_BLOCK_RM
    case -1:
    case 'UNRECOGNIZED':
    default:
      return BucketOp.UNRECOGNIZED
  }
}

export function bucketOpToJSON(object: BucketOp): string {
  switch (object) {
    case BucketOp.BucketOp_UNKNOWN:
      return 'BucketOp_UNKNOWN'
    case BucketOp.BucketOp_BLOCK_GET:
      return 'BucketOp_BLOCK_GET'
    case BucketOp.BucketOp_BLOCK_PUT:
      return 'BucketOp_BLOCK_PUT'
    case BucketOp.BucketOp_BLOCK_RM:
      return 'BucketOp_BLOCK_RM'
    case BucketOp.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** ObjectStoreOp is a object store operation. */
export enum ObjectStoreOp {
  ObjectStoreOp_UNKNOWN = 0,
  /** ObjectStoreOp_GET_KEY - ObjectStoreOp_GET_KEY gets a key. */
  ObjectStoreOp_GET_KEY = 1,
  /** ObjectStoreOp_PUT_KEY - ObjectStoreOp_PUT_KEY sets a key. */
  ObjectStoreOp_PUT_KEY = 2,
  /** ObjectStoreOp_LIST_KEYS - ObjectStoreOp_LIST_KEYS lists keys by a prefix. */
  ObjectStoreOp_LIST_KEYS = 3,
  /** ObjectStoreOp_DELETE_KEY - ObjectStoreOp_DELETE_KEY deletes a key. */
  ObjectStoreOp_DELETE_KEY = 4,
  UNRECOGNIZED = -1,
}

export function objectStoreOpFromJSON(object: any): ObjectStoreOp {
  switch (object) {
    case 0:
    case 'ObjectStoreOp_UNKNOWN':
      return ObjectStoreOp.ObjectStoreOp_UNKNOWN
    case 1:
    case 'ObjectStoreOp_GET_KEY':
      return ObjectStoreOp.ObjectStoreOp_GET_KEY
    case 2:
    case 'ObjectStoreOp_PUT_KEY':
      return ObjectStoreOp.ObjectStoreOp_PUT_KEY
    case 3:
    case 'ObjectStoreOp_LIST_KEYS':
      return ObjectStoreOp.ObjectStoreOp_LIST_KEYS
    case 4:
    case 'ObjectStoreOp_DELETE_KEY':
      return ObjectStoreOp.ObjectStoreOp_DELETE_KEY
    case -1:
    case 'UNRECOGNIZED':
    default:
      return ObjectStoreOp.UNRECOGNIZED
  }
}

export function objectStoreOpToJSON(object: ObjectStoreOp): string {
  switch (object) {
    case ObjectStoreOp.ObjectStoreOp_UNKNOWN:
      return 'ObjectStoreOp_UNKNOWN'
    case ObjectStoreOp.ObjectStoreOp_GET_KEY:
      return 'ObjectStoreOp_GET_KEY'
    case ObjectStoreOp.ObjectStoreOp_PUT_KEY:
      return 'ObjectStoreOp_PUT_KEY'
    case ObjectStoreOp.ObjectStoreOp_LIST_KEYS:
      return 'ObjectStoreOp_LIST_KEYS'
    case ObjectStoreOp.ObjectStoreOp_DELETE_KEY:
      return 'ObjectStoreOp_DELETE_KEY'
    case ObjectStoreOp.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Config is hydra api configuration. */
export interface Config {}

/** ListVolumesRequest looks up volumes. */
export interface ListVolumesRequest {}

/** ListVolumesResponse returns volumes. */
export interface ListVolumesResponse {
  /** Volumes is the list of volumes returned from the request. */
  volumes: VolumeInfo[]
}

/** ListBucketsResponse returns buckets. */
export interface ListBucketsResponse {
  /** Buckets is the list of buckets returned from the request. */
  buckets: VolumeBucketInfo[]
}

/** ApplyBucketConfigRequest requests to apply a bucket config to volumes. */
export interface ApplyBucketConfigRequest {
  /** Config is the bucket config. */
  config: Config1 | undefined
  /**
   * VolumeIdRe is a regex string to match volume IDs.
   * Set to '.*' to match all volumes.
   * If empty, will update volumes that already have the config only.
   * If VolumeIDList is set, it will override this field.
   * Cannot be specified if VolumeIDList is set.
   */
  volumeIdRe: string
  /**
   * VolumeIdList is a list of volume IDs to match.
   * Cannot be specified if VolumeIDRe is set.
   */
  volumeIdList: string[]
}

/** ApplyBucketConfigResponse returns results of the request. */
export interface ApplyBucketConfigResponse {
  /** ApplyConfResult is a result value for the application. */
  applyConfResult: ApplyBucketConfigResult | undefined
}

export interface BucketOpRequest {
  /** Op is the operation to perform against the bucket. */
  op: BucketOp
  /** BucketOpArgs are common bucket operation arguments. */
  bucketOpArgs: BucketOpArgs | undefined
  /**
   * BlockRef is the block ref to lookup.
   * Used when op == BLOCK_GET || op == BLOCK_RM
   */
  blockRef: BlockRef | undefined
  /**
   * PutOpts are overriding put options.
   * Defaults are specified by the bucket.
   * Used when op == BLOCK_PUT
   */
  putOpts: PutOpts | undefined
  /**
   * Data is the data to put in the block.
   * May be constrained by the bucket block size limit.
   * Used when op == BLOCK_PUT
   */
  data: Uint8Array
}

/** BucketOpResponse is the response type for BucketOp. */
export interface BucketOpResponse {
  /**
   * Event is the bucket event, if any.
   * Used when op == BLOCK_PUT
   */
  event: Event | undefined
  /**
   * Data is the returned data, if any.
   * Used when op == BLOCK_GET
   */
  data: Uint8Array
  /**
   * Found indicates if the data field is filled.
   * Used when op == BLOCK_GET
   */
  found: boolean
}

/** ObjectStoreOpRequest is the object store operation request. */
export interface ObjectStoreOpRequest {
  /** Op is the operation to perform against the bucket. */
  op: ObjectStoreOp
  /** VolumeId is the volume id. */
  volumeId: string
  /** StoreName is the object store name. */
  storeName: string
  /**
   * Key is the key to get, put, or delete.
   * Field is the prefix if a list request.
   */
  key: string
  /**
   * Data is the data to put.
   * May be constrained by a size limit.
   * Used when op == PUT_KEY
   */
  data: Uint8Array
}

/** ObjectStoreOpResponse is the response type for ObjectStoreOp. */
export interface ObjectStoreOpResponse {
  /**
   * Data is the returned data, if any.
   * Used when op == BLOCK_GET
   */
  data: Uint8Array
  /**
   * Found indicates if the data field is filled.
   * Used when op == BLOCK_GET
   */
  found: boolean
  /** Keys are the output keys from the list call. */
  keys: string[]
}

function createBaseConfig(): Config {
  return {}
}

export const Config = {
  encode(_: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
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
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): Config {
    return {}
  },

  toJSON(_: Config): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(_: I): Config {
    const message = createBaseConfig()
    return message
  },
}

function createBaseListVolumesRequest(): ListVolumesRequest {
  return {}
}

export const ListVolumesRequest = {
  encode(
    _: ListVolumesRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListVolumesRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListVolumesRequest()
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
  // Transform<ListVolumesRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListVolumesRequest | ListVolumesRequest[]>
      | Iterable<ListVolumesRequest | ListVolumesRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListVolumesRequest.encode(p).finish()]
        }
      } else {
        yield* [ListVolumesRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListVolumesRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ListVolumesRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListVolumesRequest.decode(p)]
        }
      } else {
        yield* [ListVolumesRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): ListVolumesRequest {
    return {}
  },

  toJSON(_: ListVolumesRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<ListVolumesRequest>, I>>(
    base?: I,
  ): ListVolumesRequest {
    return ListVolumesRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ListVolumesRequest>, I>>(
    _: I,
  ): ListVolumesRequest {
    const message = createBaseListVolumesRequest()
    return message
  },
}

function createBaseListVolumesResponse(): ListVolumesResponse {
  return { volumes: [] }
}

export const ListVolumesResponse = {
  encode(
    message: ListVolumesResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.volumes) {
      VolumeInfo.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListVolumesResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListVolumesResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.volumes.push(VolumeInfo.decode(reader, reader.uint32()))
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
  // Transform<ListVolumesResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListVolumesResponse | ListVolumesResponse[]>
      | Iterable<ListVolumesResponse | ListVolumesResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListVolumesResponse.encode(p).finish()]
        }
      } else {
        yield* [ListVolumesResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListVolumesResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ListVolumesResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListVolumesResponse.decode(p)]
        }
      } else {
        yield* [ListVolumesResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ListVolumesResponse {
    return {
      volumes: globalThis.Array.isArray(object?.volumes)
        ? object.volumes.map((e: any) => VolumeInfo.fromJSON(e))
        : [],
    }
  },

  toJSON(message: ListVolumesResponse): unknown {
    const obj: any = {}
    if (message.volumes?.length) {
      obj.volumes = message.volumes.map((e) => VolumeInfo.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ListVolumesResponse>, I>>(
    base?: I,
  ): ListVolumesResponse {
    return ListVolumesResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ListVolumesResponse>, I>>(
    object: I,
  ): ListVolumesResponse {
    const message = createBaseListVolumesResponse()
    message.volumes =
      object.volumes?.map((e) => VolumeInfo.fromPartial(e)) || []
    return message
  },
}

function createBaseListBucketsResponse(): ListBucketsResponse {
  return { buckets: [] }
}

export const ListBucketsResponse = {
  encode(
    message: ListBucketsResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.buckets) {
      VolumeBucketInfo.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListBucketsResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListBucketsResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.buckets.push(VolumeBucketInfo.decode(reader, reader.uint32()))
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
  // Transform<ListBucketsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListBucketsResponse | ListBucketsResponse[]>
      | Iterable<ListBucketsResponse | ListBucketsResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListBucketsResponse.encode(p).finish()]
        }
      } else {
        yield* [ListBucketsResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListBucketsResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ListBucketsResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListBucketsResponse.decode(p)]
        }
      } else {
        yield* [ListBucketsResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ListBucketsResponse {
    return {
      buckets: globalThis.Array.isArray(object?.buckets)
        ? object.buckets.map((e: any) => VolumeBucketInfo.fromJSON(e))
        : [],
    }
  },

  toJSON(message: ListBucketsResponse): unknown {
    const obj: any = {}
    if (message.buckets?.length) {
      obj.buckets = message.buckets.map((e) => VolumeBucketInfo.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ListBucketsResponse>, I>>(
    base?: I,
  ): ListBucketsResponse {
    return ListBucketsResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ListBucketsResponse>, I>>(
    object: I,
  ): ListBucketsResponse {
    const message = createBaseListBucketsResponse()
    message.buckets =
      object.buckets?.map((e) => VolumeBucketInfo.fromPartial(e)) || []
    return message
  },
}

function createBaseApplyBucketConfigRequest(): ApplyBucketConfigRequest {
  return { config: undefined, volumeIdRe: '', volumeIdList: [] }
}

export const ApplyBucketConfigRequest = {
  encode(
    message: ApplyBucketConfigRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.config !== undefined) {
      Config1.encode(message.config, writer.uint32(10).fork()).ldelim()
    }
    if (message.volumeIdRe !== '') {
      writer.uint32(18).string(message.volumeIdRe)
    }
    for (const v of message.volumeIdList) {
      writer.uint32(26).string(v!)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): ApplyBucketConfigRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseApplyBucketConfigRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.config = Config1.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.volumeIdRe = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.volumeIdList.push(reader.string())
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
  // Transform<ApplyBucketConfigRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ApplyBucketConfigRequest | ApplyBucketConfigRequest[]>
      | Iterable<ApplyBucketConfigRequest | ApplyBucketConfigRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ApplyBucketConfigRequest.encode(p).finish()]
        }
      } else {
        yield* [ApplyBucketConfigRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ApplyBucketConfigRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ApplyBucketConfigRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ApplyBucketConfigRequest.decode(p)]
        }
      } else {
        yield* [ApplyBucketConfigRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ApplyBucketConfigRequest {
    return {
      config: isSet(object.config)
        ? Config1.fromJSON(object.config)
        : undefined,
      volumeIdRe: isSet(object.volumeIdRe)
        ? globalThis.String(object.volumeIdRe)
        : '',
      volumeIdList: globalThis.Array.isArray(object?.volumeIdList)
        ? object.volumeIdList.map((e: any) => globalThis.String(e))
        : [],
    }
  },

  toJSON(message: ApplyBucketConfigRequest): unknown {
    const obj: any = {}
    if (message.config !== undefined) {
      obj.config = Config1.toJSON(message.config)
    }
    if (message.volumeIdRe !== '') {
      obj.volumeIdRe = message.volumeIdRe
    }
    if (message.volumeIdList?.length) {
      obj.volumeIdList = message.volumeIdList
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ApplyBucketConfigRequest>, I>>(
    base?: I,
  ): ApplyBucketConfigRequest {
    return ApplyBucketConfigRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ApplyBucketConfigRequest>, I>>(
    object: I,
  ): ApplyBucketConfigRequest {
    const message = createBaseApplyBucketConfigRequest()
    message.config =
      object.config !== undefined && object.config !== null
        ? Config1.fromPartial(object.config)
        : undefined
    message.volumeIdRe = object.volumeIdRe ?? ''
    message.volumeIdList = object.volumeIdList?.map((e) => e) || []
    return message
  },
}

function createBaseApplyBucketConfigResponse(): ApplyBucketConfigResponse {
  return { applyConfResult: undefined }
}

export const ApplyBucketConfigResponse = {
  encode(
    message: ApplyBucketConfigResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.applyConfResult !== undefined) {
      ApplyBucketConfigResult.encode(
        message.applyConfResult,
        writer.uint32(10).fork(),
      ).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): ApplyBucketConfigResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseApplyBucketConfigResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.applyConfResult = ApplyBucketConfigResult.decode(
            reader,
            reader.uint32(),
          )
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
  // Transform<ApplyBucketConfigResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ApplyBucketConfigResponse | ApplyBucketConfigResponse[]>
      | Iterable<ApplyBucketConfigResponse | ApplyBucketConfigResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ApplyBucketConfigResponse.encode(p).finish()]
        }
      } else {
        yield* [ApplyBucketConfigResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ApplyBucketConfigResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ApplyBucketConfigResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ApplyBucketConfigResponse.decode(p)]
        }
      } else {
        yield* [ApplyBucketConfigResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ApplyBucketConfigResponse {
    return {
      applyConfResult: isSet(object.applyConfResult)
        ? ApplyBucketConfigResult.fromJSON(object.applyConfResult)
        : undefined,
    }
  },

  toJSON(message: ApplyBucketConfigResponse): unknown {
    const obj: any = {}
    if (message.applyConfResult !== undefined) {
      obj.applyConfResult = ApplyBucketConfigResult.toJSON(
        message.applyConfResult,
      )
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ApplyBucketConfigResponse>, I>>(
    base?: I,
  ): ApplyBucketConfigResponse {
    return ApplyBucketConfigResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ApplyBucketConfigResponse>, I>>(
    object: I,
  ): ApplyBucketConfigResponse {
    const message = createBaseApplyBucketConfigResponse()
    message.applyConfResult =
      object.applyConfResult !== undefined && object.applyConfResult !== null
        ? ApplyBucketConfigResult.fromPartial(object.applyConfResult)
        : undefined
    return message
  },
}

function createBaseBucketOpRequest(): BucketOpRequest {
  return {
    op: 0,
    bucketOpArgs: undefined,
    blockRef: undefined,
    putOpts: undefined,
    data: new Uint8Array(0),
  }
}

export const BucketOpRequest = {
  encode(
    message: BucketOpRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.op !== 0) {
      writer.uint32(8).int32(message.op)
    }
    if (message.bucketOpArgs !== undefined) {
      BucketOpArgs.encode(
        message.bucketOpArgs,
        writer.uint32(18).fork(),
      ).ldelim()
    }
    if (message.blockRef !== undefined) {
      BlockRef.encode(message.blockRef, writer.uint32(26).fork()).ldelim()
    }
    if (message.putOpts !== undefined) {
      PutOpts.encode(message.putOpts, writer.uint32(34).fork()).ldelim()
    }
    if (message.data.length !== 0) {
      writer.uint32(42).bytes(message.data)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BucketOpRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBucketOpRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.op = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.bucketOpArgs = BucketOpArgs.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.blockRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.putOpts = PutOpts.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.data = reader.bytes()
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
  // Transform<BucketOpRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<BucketOpRequest | BucketOpRequest[]>
      | Iterable<BucketOpRequest | BucketOpRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [BucketOpRequest.encode(p).finish()]
        }
      } else {
        yield* [BucketOpRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BucketOpRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BucketOpRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [BucketOpRequest.decode(p)]
        }
      } else {
        yield* [BucketOpRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): BucketOpRequest {
    return {
      op: isSet(object.op) ? bucketOpFromJSON(object.op) : 0,
      bucketOpArgs: isSet(object.bucketOpArgs)
        ? BucketOpArgs.fromJSON(object.bucketOpArgs)
        : undefined,
      blockRef: isSet(object.blockRef)
        ? BlockRef.fromJSON(object.blockRef)
        : undefined,
      putOpts: isSet(object.putOpts)
        ? PutOpts.fromJSON(object.putOpts)
        : undefined,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
    }
  },

  toJSON(message: BucketOpRequest): unknown {
    const obj: any = {}
    if (message.op !== 0) {
      obj.op = bucketOpToJSON(message.op)
    }
    if (message.bucketOpArgs !== undefined) {
      obj.bucketOpArgs = BucketOpArgs.toJSON(message.bucketOpArgs)
    }
    if (message.blockRef !== undefined) {
      obj.blockRef = BlockRef.toJSON(message.blockRef)
    }
    if (message.putOpts !== undefined) {
      obj.putOpts = PutOpts.toJSON(message.putOpts)
    }
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<BucketOpRequest>, I>>(
    base?: I,
  ): BucketOpRequest {
    return BucketOpRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<BucketOpRequest>, I>>(
    object: I,
  ): BucketOpRequest {
    const message = createBaseBucketOpRequest()
    message.op = object.op ?? 0
    message.bucketOpArgs =
      object.bucketOpArgs !== undefined && object.bucketOpArgs !== null
        ? BucketOpArgs.fromPartial(object.bucketOpArgs)
        : undefined
    message.blockRef =
      object.blockRef !== undefined && object.blockRef !== null
        ? BlockRef.fromPartial(object.blockRef)
        : undefined
    message.putOpts =
      object.putOpts !== undefined && object.putOpts !== null
        ? PutOpts.fromPartial(object.putOpts)
        : undefined
    message.data = object.data ?? new Uint8Array(0)
    return message
  },
}

function createBaseBucketOpResponse(): BucketOpResponse {
  return { event: undefined, data: new Uint8Array(0), found: false }
}

export const BucketOpResponse = {
  encode(
    message: BucketOpResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.event !== undefined) {
      Event.encode(message.event, writer.uint32(10).fork()).ldelim()
    }
    if (message.data.length !== 0) {
      writer.uint32(18).bytes(message.data)
    }
    if (message.found !== false) {
      writer.uint32(24).bool(message.found)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BucketOpResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBucketOpResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.event = Event.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.data = reader.bytes()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.found = reader.bool()
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
  // Transform<BucketOpResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<BucketOpResponse | BucketOpResponse[]>
      | Iterable<BucketOpResponse | BucketOpResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [BucketOpResponse.encode(p).finish()]
        }
      } else {
        yield* [BucketOpResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BucketOpResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BucketOpResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [BucketOpResponse.decode(p)]
        }
      } else {
        yield* [BucketOpResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): BucketOpResponse {
    return {
      event: isSet(object.event) ? Event.fromJSON(object.event) : undefined,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
      found: isSet(object.found) ? globalThis.Boolean(object.found) : false,
    }
  },

  toJSON(message: BucketOpResponse): unknown {
    const obj: any = {}
    if (message.event !== undefined) {
      obj.event = Event.toJSON(message.event)
    }
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    if (message.found !== false) {
      obj.found = message.found
    }
    return obj
  },

  create<I extends Exact<DeepPartial<BucketOpResponse>, I>>(
    base?: I,
  ): BucketOpResponse {
    return BucketOpResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<BucketOpResponse>, I>>(
    object: I,
  ): BucketOpResponse {
    const message = createBaseBucketOpResponse()
    message.event =
      object.event !== undefined && object.event !== null
        ? Event.fromPartial(object.event)
        : undefined
    message.data = object.data ?? new Uint8Array(0)
    message.found = object.found ?? false
    return message
  },
}

function createBaseObjectStoreOpRequest(): ObjectStoreOpRequest {
  return {
    op: 0,
    volumeId: '',
    storeName: '',
    key: '',
    data: new Uint8Array(0),
  }
}

export const ObjectStoreOpRequest = {
  encode(
    message: ObjectStoreOpRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.op !== 0) {
      writer.uint32(8).int32(message.op)
    }
    if (message.volumeId !== '') {
      writer.uint32(18).string(message.volumeId)
    }
    if (message.storeName !== '') {
      writer.uint32(26).string(message.storeName)
    }
    if (message.key !== '') {
      writer.uint32(34).string(message.key)
    }
    if (message.data.length !== 0) {
      writer.uint32(42).bytes(message.data)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): ObjectStoreOpRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseObjectStoreOpRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.op = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.volumeId = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.storeName = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.key = reader.string()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.data = reader.bytes()
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
  // Transform<ObjectStoreOpRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ObjectStoreOpRequest | ObjectStoreOpRequest[]>
      | Iterable<ObjectStoreOpRequest | ObjectStoreOpRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ObjectStoreOpRequest.encode(p).finish()]
        }
      } else {
        yield* [ObjectStoreOpRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ObjectStoreOpRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ObjectStoreOpRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ObjectStoreOpRequest.decode(p)]
        }
      } else {
        yield* [ObjectStoreOpRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ObjectStoreOpRequest {
    return {
      op: isSet(object.op) ? objectStoreOpFromJSON(object.op) : 0,
      volumeId: isSet(object.volumeId)
        ? globalThis.String(object.volumeId)
        : '',
      storeName: isSet(object.storeName)
        ? globalThis.String(object.storeName)
        : '',
      key: isSet(object.key) ? globalThis.String(object.key) : '',
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
    }
  },

  toJSON(message: ObjectStoreOpRequest): unknown {
    const obj: any = {}
    if (message.op !== 0) {
      obj.op = objectStoreOpToJSON(message.op)
    }
    if (message.volumeId !== '') {
      obj.volumeId = message.volumeId
    }
    if (message.storeName !== '') {
      obj.storeName = message.storeName
    }
    if (message.key !== '') {
      obj.key = message.key
    }
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ObjectStoreOpRequest>, I>>(
    base?: I,
  ): ObjectStoreOpRequest {
    return ObjectStoreOpRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ObjectStoreOpRequest>, I>>(
    object: I,
  ): ObjectStoreOpRequest {
    const message = createBaseObjectStoreOpRequest()
    message.op = object.op ?? 0
    message.volumeId = object.volumeId ?? ''
    message.storeName = object.storeName ?? ''
    message.key = object.key ?? ''
    message.data = object.data ?? new Uint8Array(0)
    return message
  },
}

function createBaseObjectStoreOpResponse(): ObjectStoreOpResponse {
  return { data: new Uint8Array(0), found: false, keys: [] }
}

export const ObjectStoreOpResponse = {
  encode(
    message: ObjectStoreOpResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.data.length !== 0) {
      writer.uint32(10).bytes(message.data)
    }
    if (message.found !== false) {
      writer.uint32(16).bool(message.found)
    }
    for (const v of message.keys) {
      writer.uint32(26).string(v!)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): ObjectStoreOpResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseObjectStoreOpResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.data = reader.bytes()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.found = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.keys.push(reader.string())
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
  // Transform<ObjectStoreOpResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ObjectStoreOpResponse | ObjectStoreOpResponse[]>
      | Iterable<ObjectStoreOpResponse | ObjectStoreOpResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ObjectStoreOpResponse.encode(p).finish()]
        }
      } else {
        yield* [ObjectStoreOpResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ObjectStoreOpResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ObjectStoreOpResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ObjectStoreOpResponse.decode(p)]
        }
      } else {
        yield* [ObjectStoreOpResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ObjectStoreOpResponse {
    return {
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
      found: isSet(object.found) ? globalThis.Boolean(object.found) : false,
      keys: globalThis.Array.isArray(object?.keys)
        ? object.keys.map((e: any) => globalThis.String(e))
        : [],
    }
  },

  toJSON(message: ObjectStoreOpResponse): unknown {
    const obj: any = {}
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    if (message.found !== false) {
      obj.found = message.found
    }
    if (message.keys?.length) {
      obj.keys = message.keys
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ObjectStoreOpResponse>, I>>(
    base?: I,
  ): ObjectStoreOpResponse {
    return ObjectStoreOpResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ObjectStoreOpResponse>, I>>(
    object: I,
  ): ObjectStoreOpResponse {
    const message = createBaseObjectStoreOpResponse()
    message.data = object.data ?? new Uint8Array(0)
    message.found = object.found ?? false
    message.keys = object.keys?.map((e) => e) || []
    return message
  },
}

/** HydraDaemonService is the control service for a daemon, contacted by the CLI. */
export interface HydraDaemonService {
  /** ListVolumes lists volumes by the daemon. */
  ListVolumes(
    request: ListVolumesRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListVolumesResponse>
  /** ListBuckets lists buckets by the daemon. */
  ListBuckets(
    request: ListBucketsRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListBucketsResponse>
  /** ApplyBucketConfig applies a bucket config to volumes. */
  ApplyBucketConfig(
    request: ApplyBucketConfigRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<ApplyBucketConfigResponse>
  /** BucketOp performs a bucket operation. */
  BucketOp(
    request: BucketOpRequest,
    abortSignal?: AbortSignal,
  ): Promise<BucketOpResponse>
  /** ObjectStoreOp performs an object store operation. */
  ObjectStoreOp(
    request: ObjectStoreOpRequest,
    abortSignal?: AbortSignal,
  ): Promise<ObjectStoreOpResponse>
}

export const HydraDaemonServiceServiceName = 'hydra.api.HydraDaemonService'
export class HydraDaemonServiceClientImpl implements HydraDaemonService {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || HydraDaemonServiceServiceName
    this.rpc = rpc
    this.ListVolumes = this.ListVolumes.bind(this)
    this.ListBuckets = this.ListBuckets.bind(this)
    this.ApplyBucketConfig = this.ApplyBucketConfig.bind(this)
    this.BucketOp = this.BucketOp.bind(this)
    this.ObjectStoreOp = this.ObjectStoreOp.bind(this)
  }
  ListVolumes(
    request: ListVolumesRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListVolumesResponse> {
    const data = ListVolumesRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'ListVolumes',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      ListVolumesResponse.decode(_m0.Reader.create(data)),
    )
  }

  ListBuckets(
    request: ListBucketsRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListBucketsResponse> {
    const data = ListBucketsRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'ListBuckets',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      ListBucketsResponse.decode(_m0.Reader.create(data)),
    )
  }

  ApplyBucketConfig(
    request: ApplyBucketConfigRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<ApplyBucketConfigResponse> {
    const data = ApplyBucketConfigRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'ApplyBucketConfig',
      data,
      abortSignal || undefined,
    )
    return ApplyBucketConfigResponse.decodeTransform(result)
  }

  BucketOp(
    request: BucketOpRequest,
    abortSignal?: AbortSignal,
  ): Promise<BucketOpResponse> {
    const data = BucketOpRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'BucketOp',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      BucketOpResponse.decode(_m0.Reader.create(data)),
    )
  }

  ObjectStoreOp(
    request: ObjectStoreOpRequest,
    abortSignal?: AbortSignal,
  ): Promise<ObjectStoreOpResponse> {
    const data = ObjectStoreOpRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'ObjectStoreOp',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      ObjectStoreOpResponse.decode(_m0.Reader.create(data)),
    )
  }
}

/** HydraDaemonService is the control service for a daemon, contacted by the CLI. */
export type HydraDaemonServiceDefinition = typeof HydraDaemonServiceDefinition
export const HydraDaemonServiceDefinition = {
  name: 'HydraDaemonService',
  fullName: 'hydra.api.HydraDaemonService',
  methods: {
    /** ListVolumes lists volumes by the daemon. */
    listVolumes: {
      name: 'ListVolumes',
      requestType: ListVolumesRequest,
      requestStream: false,
      responseType: ListVolumesResponse,
      responseStream: false,
      options: {},
    },
    /** ListBuckets lists buckets by the daemon. */
    listBuckets: {
      name: 'ListBuckets',
      requestType: ListBucketsRequest,
      requestStream: false,
      responseType: ListBucketsResponse,
      responseStream: false,
      options: {},
    },
    /** ApplyBucketConfig applies a bucket config to volumes. */
    applyBucketConfig: {
      name: 'ApplyBucketConfig',
      requestType: ApplyBucketConfigRequest,
      requestStream: false,
      responseType: ApplyBucketConfigResponse,
      responseStream: true,
      options: {},
    },
    /** BucketOp performs a bucket operation. */
    bucketOp: {
      name: 'BucketOp',
      requestType: BucketOpRequest,
      requestStream: false,
      responseType: BucketOpResponse,
      responseStream: false,
      options: {},
    },
    /** ObjectStoreOp performs an object store operation. */
    objectStoreOp: {
      name: 'ObjectStoreOp',
      requestType: ObjectStoreOpRequest,
      requestStream: false,
      responseType: ObjectStoreOpResponse,
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

function bytesFromBase64(b64: string): Uint8Array {
  if ((globalThis as any).Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, 'base64'))
  } else {
    const bin = globalThis.atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i)
    }
    return arr
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if ((globalThis as any).Buffer) {
    return globalThis.Buffer.from(arr).toString('base64')
  } else {
    const bin: string[] = []
    arr.forEach((byte) => {
      bin.push(globalThis.String.fromCharCode(byte))
    })
    return globalThis.btoa(bin.join(''))
  }
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
