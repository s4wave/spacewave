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
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt)]
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
    return Config.fromPartial(base ?? {})
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
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListVolumesRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListVolumesRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ListVolumesRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListVolumesRequest | ListVolumesRequest[]>
      | Iterable<ListVolumesRequest | ListVolumesRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListVolumesRequest.encode(p).finish()]
        }
      } else {
        yield* [ListVolumesRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListVolumesRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ListVolumesRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListVolumesRequest.decode(p)]
        }
      } else {
        yield* [ListVolumesRequest.decode(pkt)]
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
    base?: I
  ): ListVolumesRequest {
    return ListVolumesRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ListVolumesRequest>, I>>(
    _: I
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
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.volumes) {
      VolumeInfo.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListVolumesResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListVolumesResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.volumes.push(VolumeInfo.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ListVolumesResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListVolumesResponse | ListVolumesResponse[]>
      | Iterable<ListVolumesResponse | ListVolumesResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListVolumesResponse.encode(p).finish()]
        }
      } else {
        yield* [ListVolumesResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListVolumesResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ListVolumesResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListVolumesResponse.decode(p)]
        }
      } else {
        yield* [ListVolumesResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ListVolumesResponse {
    return {
      volumes: Array.isArray(object?.volumes)
        ? object.volumes.map((e: any) => VolumeInfo.fromJSON(e))
        : [],
    }
  },

  toJSON(message: ListVolumesResponse): unknown {
    const obj: any = {}
    if (message.volumes) {
      obj.volumes = message.volumes.map((e) =>
        e ? VolumeInfo.toJSON(e) : undefined
      )
    } else {
      obj.volumes = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ListVolumesResponse>, I>>(
    base?: I
  ): ListVolumesResponse {
    return ListVolumesResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ListVolumesResponse>, I>>(
    object: I
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
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.buckets) {
      VolumeBucketInfo.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListBucketsResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListBucketsResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.buckets.push(VolumeBucketInfo.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ListBucketsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListBucketsResponse | ListBucketsResponse[]>
      | Iterable<ListBucketsResponse | ListBucketsResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketsResponse.encode(p).finish()]
        }
      } else {
        yield* [ListBucketsResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListBucketsResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ListBucketsResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketsResponse.decode(p)]
        }
      } else {
        yield* [ListBucketsResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ListBucketsResponse {
    return {
      buckets: Array.isArray(object?.buckets)
        ? object.buckets.map((e: any) => VolumeBucketInfo.fromJSON(e))
        : [],
    }
  },

  toJSON(message: ListBucketsResponse): unknown {
    const obj: any = {}
    if (message.buckets) {
      obj.buckets = message.buckets.map((e) =>
        e ? VolumeBucketInfo.toJSON(e) : undefined
      )
    } else {
      obj.buckets = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ListBucketsResponse>, I>>(
    base?: I
  ): ListBucketsResponse {
    return ListBucketsResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ListBucketsResponse>, I>>(
    object: I
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
    writer: _m0.Writer = _m0.Writer.create()
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
    length?: number
  ): ApplyBucketConfigRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseApplyBucketConfigRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.config = Config1.decode(reader, reader.uint32())
          break
        case 2:
          message.volumeIdRe = reader.string()
          break
        case 3:
          message.volumeIdList.push(reader.string())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ApplyBucketConfigRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ApplyBucketConfigRequest | ApplyBucketConfigRequest[]>
      | Iterable<ApplyBucketConfigRequest | ApplyBucketConfigRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigRequest.encode(p).finish()]
        }
      } else {
        yield* [ApplyBucketConfigRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ApplyBucketConfigRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ApplyBucketConfigRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigRequest.decode(p)]
        }
      } else {
        yield* [ApplyBucketConfigRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ApplyBucketConfigRequest {
    return {
      config: isSet(object.config)
        ? Config1.fromJSON(object.config)
        : undefined,
      volumeIdRe: isSet(object.volumeIdRe) ? String(object.volumeIdRe) : '',
      volumeIdList: Array.isArray(object?.volumeIdList)
        ? object.volumeIdList.map((e: any) => String(e))
        : [],
    }
  },

  toJSON(message: ApplyBucketConfigRequest): unknown {
    const obj: any = {}
    message.config !== undefined &&
      (obj.config = message.config ? Config1.toJSON(message.config) : undefined)
    message.volumeIdRe !== undefined && (obj.volumeIdRe = message.volumeIdRe)
    if (message.volumeIdList) {
      obj.volumeIdList = message.volumeIdList.map((e) => e)
    } else {
      obj.volumeIdList = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ApplyBucketConfigRequest>, I>>(
    base?: I
  ): ApplyBucketConfigRequest {
    return ApplyBucketConfigRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ApplyBucketConfigRequest>, I>>(
    object: I
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
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.applyConfResult !== undefined) {
      ApplyBucketConfigResult.encode(
        message.applyConfResult,
        writer.uint32(10).fork()
      ).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): ApplyBucketConfigResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseApplyBucketConfigResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.applyConfResult = ApplyBucketConfigResult.decode(
            reader,
            reader.uint32()
          )
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ApplyBucketConfigResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ApplyBucketConfigResponse | ApplyBucketConfigResponse[]>
      | Iterable<ApplyBucketConfigResponse | ApplyBucketConfigResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigResponse.encode(p).finish()]
        }
      } else {
        yield* [ApplyBucketConfigResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ApplyBucketConfigResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ApplyBucketConfigResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigResponse.decode(p)]
        }
      } else {
        yield* [ApplyBucketConfigResponse.decode(pkt)]
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
    message.applyConfResult !== undefined &&
      (obj.applyConfResult = message.applyConfResult
        ? ApplyBucketConfigResult.toJSON(message.applyConfResult)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<ApplyBucketConfigResponse>, I>>(
    base?: I
  ): ApplyBucketConfigResponse {
    return ApplyBucketConfigResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ApplyBucketConfigResponse>, I>>(
    object: I
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
    data: new Uint8Array(),
  }
}

export const BucketOpRequest = {
  encode(
    message: BucketOpRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.op !== 0) {
      writer.uint32(8).int32(message.op)
    }
    if (message.bucketOpArgs !== undefined) {
      BucketOpArgs.encode(
        message.bucketOpArgs,
        writer.uint32(18).fork()
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
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBucketOpRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.op = reader.int32() as any
          break
        case 2:
          message.bucketOpArgs = BucketOpArgs.decode(reader, reader.uint32())
          break
        case 3:
          message.blockRef = BlockRef.decode(reader, reader.uint32())
          break
        case 4:
          message.putOpts = PutOpts.decode(reader, reader.uint32())
          break
        case 5:
          message.data = reader.bytes()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<BucketOpRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<BucketOpRequest | BucketOpRequest[]>
      | Iterable<BucketOpRequest | BucketOpRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketOpRequest.encode(p).finish()]
        }
      } else {
        yield* [BucketOpRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BucketOpRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<BucketOpRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketOpRequest.decode(p)]
        }
      } else {
        yield* [BucketOpRequest.decode(pkt)]
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
        : new Uint8Array(),
    }
  },

  toJSON(message: BucketOpRequest): unknown {
    const obj: any = {}
    message.op !== undefined && (obj.op = bucketOpToJSON(message.op))
    message.bucketOpArgs !== undefined &&
      (obj.bucketOpArgs = message.bucketOpArgs
        ? BucketOpArgs.toJSON(message.bucketOpArgs)
        : undefined)
    message.blockRef !== undefined &&
      (obj.blockRef = message.blockRef
        ? BlockRef.toJSON(message.blockRef)
        : undefined)
    message.putOpts !== undefined &&
      (obj.putOpts = message.putOpts
        ? PutOpts.toJSON(message.putOpts)
        : undefined)
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<BucketOpRequest>, I>>(
    base?: I
  ): BucketOpRequest {
    return BucketOpRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<BucketOpRequest>, I>>(
    object: I
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
    message.data = object.data ?? new Uint8Array()
    return message
  },
}

function createBaseBucketOpResponse(): BucketOpResponse {
  return { event: undefined, data: new Uint8Array(), found: false }
}

export const BucketOpResponse = {
  encode(
    message: BucketOpResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.event !== undefined) {
      Event.encode(message.event, writer.uint32(10).fork()).ldelim()
    }
    if (message.data.length !== 0) {
      writer.uint32(18).bytes(message.data)
    }
    if (message.found === true) {
      writer.uint32(24).bool(message.found)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BucketOpResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBucketOpResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.event = Event.decode(reader, reader.uint32())
          break
        case 2:
          message.data = reader.bytes()
          break
        case 3:
          message.found = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<BucketOpResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<BucketOpResponse | BucketOpResponse[]>
      | Iterable<BucketOpResponse | BucketOpResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketOpResponse.encode(p).finish()]
        }
      } else {
        yield* [BucketOpResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BucketOpResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<BucketOpResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketOpResponse.decode(p)]
        }
      } else {
        yield* [BucketOpResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): BucketOpResponse {
    return {
      event: isSet(object.event) ? Event.fromJSON(object.event) : undefined,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(),
      found: isSet(object.found) ? Boolean(object.found) : false,
    }
  },

  toJSON(message: BucketOpResponse): unknown {
    const obj: any = {}
    message.event !== undefined &&
      (obj.event = message.event ? Event.toJSON(message.event) : undefined)
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    message.found !== undefined && (obj.found = message.found)
    return obj
  },

  create<I extends Exact<DeepPartial<BucketOpResponse>, I>>(
    base?: I
  ): BucketOpResponse {
    return BucketOpResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<BucketOpResponse>, I>>(
    object: I
  ): BucketOpResponse {
    const message = createBaseBucketOpResponse()
    message.event =
      object.event !== undefined && object.event !== null
        ? Event.fromPartial(object.event)
        : undefined
    message.data = object.data ?? new Uint8Array()
    message.found = object.found ?? false
    return message
  },
}

function createBaseObjectStoreOpRequest(): ObjectStoreOpRequest {
  return { op: 0, volumeId: '', storeName: '', key: '', data: new Uint8Array() }
}

export const ObjectStoreOpRequest = {
  encode(
    message: ObjectStoreOpRequest,
    writer: _m0.Writer = _m0.Writer.create()
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
    length?: number
  ): ObjectStoreOpRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseObjectStoreOpRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.op = reader.int32() as any
          break
        case 2:
          message.volumeId = reader.string()
          break
        case 3:
          message.storeName = reader.string()
          break
        case 4:
          message.key = reader.string()
          break
        case 5:
          message.data = reader.bytes()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ObjectStoreOpRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ObjectStoreOpRequest | ObjectStoreOpRequest[]>
      | Iterable<ObjectStoreOpRequest | ObjectStoreOpRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ObjectStoreOpRequest.encode(p).finish()]
        }
      } else {
        yield* [ObjectStoreOpRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ObjectStoreOpRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ObjectStoreOpRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ObjectStoreOpRequest.decode(p)]
        }
      } else {
        yield* [ObjectStoreOpRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ObjectStoreOpRequest {
    return {
      op: isSet(object.op) ? objectStoreOpFromJSON(object.op) : 0,
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : '',
      storeName: isSet(object.storeName) ? String(object.storeName) : '',
      key: isSet(object.key) ? String(object.key) : '',
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(),
    }
  },

  toJSON(message: ObjectStoreOpRequest): unknown {
    const obj: any = {}
    message.op !== undefined && (obj.op = objectStoreOpToJSON(message.op))
    message.volumeId !== undefined && (obj.volumeId = message.volumeId)
    message.storeName !== undefined && (obj.storeName = message.storeName)
    message.key !== undefined && (obj.key = message.key)
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<ObjectStoreOpRequest>, I>>(
    base?: I
  ): ObjectStoreOpRequest {
    return ObjectStoreOpRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ObjectStoreOpRequest>, I>>(
    object: I
  ): ObjectStoreOpRequest {
    const message = createBaseObjectStoreOpRequest()
    message.op = object.op ?? 0
    message.volumeId = object.volumeId ?? ''
    message.storeName = object.storeName ?? ''
    message.key = object.key ?? ''
    message.data = object.data ?? new Uint8Array()
    return message
  },
}

function createBaseObjectStoreOpResponse(): ObjectStoreOpResponse {
  return { data: new Uint8Array(), found: false, keys: [] }
}

export const ObjectStoreOpResponse = {
  encode(
    message: ObjectStoreOpResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.data.length !== 0) {
      writer.uint32(10).bytes(message.data)
    }
    if (message.found === true) {
      writer.uint32(16).bool(message.found)
    }
    for (const v of message.keys) {
      writer.uint32(26).string(v!)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): ObjectStoreOpResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseObjectStoreOpResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.data = reader.bytes()
          break
        case 2:
          message.found = reader.bool()
          break
        case 3:
          message.keys.push(reader.string())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ObjectStoreOpResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ObjectStoreOpResponse | ObjectStoreOpResponse[]>
      | Iterable<ObjectStoreOpResponse | ObjectStoreOpResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ObjectStoreOpResponse.encode(p).finish()]
        }
      } else {
        yield* [ObjectStoreOpResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ObjectStoreOpResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ObjectStoreOpResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ObjectStoreOpResponse.decode(p)]
        }
      } else {
        yield* [ObjectStoreOpResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ObjectStoreOpResponse {
    return {
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(),
      found: isSet(object.found) ? Boolean(object.found) : false,
      keys: Array.isArray(object?.keys)
        ? object.keys.map((e: any) => String(e))
        : [],
    }
  },

  toJSON(message: ObjectStoreOpResponse): unknown {
    const obj: any = {}
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    message.found !== undefined && (obj.found = message.found)
    if (message.keys) {
      obj.keys = message.keys.map((e) => e)
    } else {
      obj.keys = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ObjectStoreOpResponse>, I>>(
    base?: I
  ): ObjectStoreOpResponse {
    return ObjectStoreOpResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ObjectStoreOpResponse>, I>>(
    object: I
  ): ObjectStoreOpResponse {
    const message = createBaseObjectStoreOpResponse()
    message.data = object.data ?? new Uint8Array()
    message.found = object.found ?? false
    message.keys = object.keys?.map((e) => e) || []
    return message
  },
}

/** HydraDaemonService is the control service for a daemon, contacted by the CLI. */
export interface HydraDaemonService {
  /** ListVolumes lists volumes by the daemon. */
  ListVolumes(request: ListVolumesRequest): Promise<ListVolumesResponse>
  /** ListBuckets lists buckets by the daemon. */
  ListBuckets(request: ListBucketsRequest): Promise<ListBucketsResponse>
  /** ApplyBucketConfig applies a bucket config to volumes. */
  ApplyBucketConfig(
    request: ApplyBucketConfigRequest
  ): AsyncIterable<ApplyBucketConfigResponse>
  /** BucketOp performs a bucket operation. */
  BucketOp(request: BucketOpRequest): Promise<BucketOpResponse>
  /** ObjectStoreOp performs an object store operation. */
  ObjectStoreOp(request: ObjectStoreOpRequest): Promise<ObjectStoreOpResponse>
}

export class HydraDaemonServiceClientImpl implements HydraDaemonService {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || 'hydra.api.HydraDaemonService'
    this.rpc = rpc
    this.ListVolumes = this.ListVolumes.bind(this)
    this.ListBuckets = this.ListBuckets.bind(this)
    this.ApplyBucketConfig = this.ApplyBucketConfig.bind(this)
    this.BucketOp = this.BucketOp.bind(this)
    this.ObjectStoreOp = this.ObjectStoreOp.bind(this)
  }
  ListVolumes(request: ListVolumesRequest): Promise<ListVolumesResponse> {
    const data = ListVolumesRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'ListVolumes', data)
    return promise.then((data) =>
      ListVolumesResponse.decode(new _m0.Reader(data))
    )
  }

  ListBuckets(request: ListBucketsRequest): Promise<ListBucketsResponse> {
    const data = ListBucketsRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'ListBuckets', data)
    return promise.then((data) =>
      ListBucketsResponse.decode(new _m0.Reader(data))
    )
  }

  ApplyBucketConfig(
    request: ApplyBucketConfigRequest
  ): AsyncIterable<ApplyBucketConfigResponse> {
    const data = ApplyBucketConfigRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'ApplyBucketConfig',
      data
    )
    return ApplyBucketConfigResponse.decodeTransform(result)
  }

  BucketOp(request: BucketOpRequest): Promise<BucketOpResponse> {
    const data = BucketOpRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'BucketOp', data)
    return promise.then((data) => BucketOpResponse.decode(new _m0.Reader(data)))
  }

  ObjectStoreOp(request: ObjectStoreOpRequest): Promise<ObjectStoreOpResponse> {
    const data = ObjectStoreOpRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'ObjectStoreOp', data)
    return promise.then((data) =>
      ObjectStoreOpResponse.decode(new _m0.Reader(data))
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
    data: Uint8Array
  ): Promise<Uint8Array>
  clientStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>
  ): Promise<Uint8Array>
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array
  ): AsyncIterable<Uint8Array>
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>
  ): AsyncIterable<Uint8Array>
}

declare var self: any | undefined
declare var window: any | undefined
declare var global: any | undefined
var tsProtoGlobalThis: any = (() => {
  if (typeof globalThis !== 'undefined') {
    return globalThis
  }
  if (typeof self !== 'undefined') {
    return self
  }
  if (typeof window !== 'undefined') {
    return window
  }
  if (typeof global !== 'undefined') {
    return global
  }
  throw 'Unable to locate global object'
})()

function bytesFromBase64(b64: string): Uint8Array {
  if (tsProtoGlobalThis.Buffer) {
    return Uint8Array.from(tsProtoGlobalThis.Buffer.from(b64, 'base64'))
  } else {
    const bin = tsProtoGlobalThis.atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i)
    }
    return arr
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (tsProtoGlobalThis.Buffer) {
    return tsProtoGlobalThis.Buffer.from(arr).toString('base64')
  } else {
    const bin: string[] = []
    arr.forEach((byte) => {
      bin.push(String.fromCharCode(byte))
    })
    return tsProtoGlobalThis.btoa(bin.join(''))
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
