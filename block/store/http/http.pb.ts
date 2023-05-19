/* eslint-disable */
import {
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef, PutOpts } from '../../block.pb.js'

export const protobufPackage = 'block.store.http'

/** Config configures the block store http controller. */
export interface Config {
  /** BlockStoreId is the block store id to use on the bus. */
  blockStoreId: string
  /** Url is the base url to access the api. */
  url: string
  /** ReadOnly disables writing to the http store. */
  readOnly: boolean
  /**
   * ForceHashType forces writing the given hash type to the store.
   * If unset, accepts any hash type.
   */
  forceHashType: HashType
  /** BucketIds is a list of bucket ids to serve LookupBlockFromNetwork directives. */
  bucketIds: string[]
  /** SkipNotFound skips returning a value if the block was not found. */
  skipNotFound: boolean
  /** Verbose enables verbose logging of the block store. */
  verbose: boolean
}

/** PutRequest is the request body for a Put request. */
export interface PutRequest {
  /** Data is the data to put at the key. */
  data: Uint8Array
  /** PutOpts sets the put options. */
  putOpts: PutOpts | undefined
}

/** PutResponse is the response to a Put request. */
export interface PutResponse {
  /** Ref contains the put block ref. */
  ref: BlockRef | undefined
  /**
   * Exists indicates that the block already existed in the store.
   * Some stores may always return false for this.
   */
  exists: boolean
  /**
   * Err contains any error putting the ref.
   * If empty, the ref must not be nil, op succeeded.
   */
  err: string
}

/** GetResponse is the response to a Get request. */
export interface GetResponse {
  /** NotFound indicates that the block did not exist in the store. */
  notFound: boolean
  /** Data contains the block data, if not_found and err are empty. */
  data: Uint8Array
  /**
   * Err contains any error getting the ref.
   * If empty, not_found or data must be set.
   */
  err: string
}

/** ExistsResponse is the response to a Exists request. */
export interface ExistsResponse {
  /** Exists indicates that the block existed in the store. */
  exists: boolean
  /**
   * NotFound indicates that the block did not exist in the store.
   * If false, exists=true.
   */
  notFound: boolean
  /**
   * Err contains any error checking if the ref exists.
   * If empty, not_found or data must be set.
   */
  err: string
}

/** RmResponse is the response to a Rm request. */
export interface RmResponse {
  /**
   * Removed indicates the request was processed successfully.
   * Must be set if err is empty.
   */
  removed: boolean
  /** Err contains any error deleting the block. */
  err: string
}

function createBaseConfig(): Config {
  return {
    blockStoreId: '',
    url: '',
    readOnly: false,
    forceHashType: 0,
    bucketIds: [],
    skipNotFound: false,
    verbose: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.blockStoreId !== '') {
      writer.uint32(10).string(message.blockStoreId)
    }
    if (message.url !== '') {
      writer.uint32(18).string(message.url)
    }
    if (message.readOnly === true) {
      writer.uint32(24).bool(message.readOnly)
    }
    if (message.forceHashType !== 0) {
      writer.uint32(32).int32(message.forceHashType)
    }
    for (const v of message.bucketIds) {
      writer.uint32(42).string(v!)
    }
    if (message.skipNotFound === true) {
      writer.uint32(48).bool(message.skipNotFound)
    }
    if (message.verbose === true) {
      writer.uint32(56).bool(message.verbose)
    }
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
        case 1:
          if (tag !== 10) {
            break
          }

          message.blockStoreId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.url = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.readOnly = reader.bool()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.forceHashType = reader.int32() as any
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.bucketIds.push(reader.string())
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.skipNotFound = reader.bool()
          continue
        case 7:
          if (tag !== 56) {
            break
          }

          message.verbose = reader.bool()
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

  fromJSON(object: any): Config {
    return {
      blockStoreId: isSet(object.blockStoreId)
        ? String(object.blockStoreId)
        : '',
      url: isSet(object.url) ? String(object.url) : '',
      readOnly: isSet(object.readOnly) ? Boolean(object.readOnly) : false,
      forceHashType: isSet(object.forceHashType)
        ? hashTypeFromJSON(object.forceHashType)
        : 0,
      bucketIds: Array.isArray(object?.bucketIds)
        ? object.bucketIds.map((e: any) => String(e))
        : [],
      skipNotFound: isSet(object.skipNotFound)
        ? Boolean(object.skipNotFound)
        : false,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.blockStoreId !== undefined &&
      (obj.blockStoreId = message.blockStoreId)
    message.url !== undefined && (obj.url = message.url)
    message.readOnly !== undefined && (obj.readOnly = message.readOnly)
    message.forceHashType !== undefined &&
      (obj.forceHashType = hashTypeToJSON(message.forceHashType))
    if (message.bucketIds) {
      obj.bucketIds = message.bucketIds.map((e) => e)
    } else {
      obj.bucketIds = []
    }
    message.skipNotFound !== undefined &&
      (obj.skipNotFound = message.skipNotFound)
    message.verbose !== undefined && (obj.verbose = message.verbose)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.blockStoreId = object.blockStoreId ?? ''
    message.url = object.url ?? ''
    message.readOnly = object.readOnly ?? false
    message.forceHashType = object.forceHashType ?? 0
    message.bucketIds = object.bucketIds?.map((e) => e) || []
    message.skipNotFound = object.skipNotFound ?? false
    message.verbose = object.verbose ?? false
    return message
  },
}

function createBasePutRequest(): PutRequest {
  return { data: new Uint8Array(), putOpts: undefined }
}

export const PutRequest = {
  encode(
    message: PutRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.data.length !== 0) {
      writer.uint32(10).bytes(message.data)
    }
    if (message.putOpts !== undefined) {
      PutOpts.encode(message.putOpts, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PutRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePutRequest()
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
          if (tag !== 18) {
            break
          }

          message.putOpts = PutOpts.decode(reader, reader.uint32())
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
  // Transform<PutRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PutRequest | PutRequest[]>
      | Iterable<PutRequest | PutRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PutRequest.encode(p).finish()]
        }
      } else {
        yield* [PutRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PutRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<PutRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PutRequest.decode(p)]
        }
      } else {
        yield* [PutRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): PutRequest {
    return {
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(),
      putOpts: isSet(object.putOpts)
        ? PutOpts.fromJSON(object.putOpts)
        : undefined,
    }
  },

  toJSON(message: PutRequest): unknown {
    const obj: any = {}
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    message.putOpts !== undefined &&
      (obj.putOpts = message.putOpts
        ? PutOpts.toJSON(message.putOpts)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<PutRequest>, I>>(base?: I): PutRequest {
    return PutRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<PutRequest>, I>>(
    object: I
  ): PutRequest {
    const message = createBasePutRequest()
    message.data = object.data ?? new Uint8Array()
    message.putOpts =
      object.putOpts !== undefined && object.putOpts !== null
        ? PutOpts.fromPartial(object.putOpts)
        : undefined
    return message
  },
}

function createBasePutResponse(): PutResponse {
  return { ref: undefined, exists: false, err: '' }
}

export const PutResponse = {
  encode(
    message: PutResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(10).fork()).ldelim()
    }
    if (message.exists === true) {
      writer.uint32(16).bool(message.exists)
    }
    if (message.err !== '') {
      writer.uint32(26).string(message.err)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PutResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePutResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.ref = BlockRef.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.exists = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.err = reader.string()
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
  // Transform<PutResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PutResponse | PutResponse[]>
      | Iterable<PutResponse | PutResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PutResponse.encode(p).finish()]
        }
      } else {
        yield* [PutResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PutResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<PutResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PutResponse.decode(p)]
        }
      } else {
        yield* [PutResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): PutResponse {
    return {
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
      exists: isSet(object.exists) ? Boolean(object.exists) : false,
      err: isSet(object.err) ? String(object.err) : '',
    }
  },

  toJSON(message: PutResponse): unknown {
    const obj: any = {}
    message.ref !== undefined &&
      (obj.ref = message.ref ? BlockRef.toJSON(message.ref) : undefined)
    message.exists !== undefined && (obj.exists = message.exists)
    message.err !== undefined && (obj.err = message.err)
    return obj
  },

  create<I extends Exact<DeepPartial<PutResponse>, I>>(base?: I): PutResponse {
    return PutResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<PutResponse>, I>>(
    object: I
  ): PutResponse {
    const message = createBasePutResponse()
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
        : undefined
    message.exists = object.exists ?? false
    message.err = object.err ?? ''
    return message
  },
}

function createBaseGetResponse(): GetResponse {
  return { notFound: false, data: new Uint8Array(), err: '' }
}

export const GetResponse = {
  encode(
    message: GetResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.notFound === true) {
      writer.uint32(8).bool(message.notFound)
    }
    if (message.data.length !== 0) {
      writer.uint32(18).bytes(message.data)
    }
    if (message.err !== '') {
      writer.uint32(26).string(message.err)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetResponse()
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

          message.data = reader.bytes()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.err = reader.string()
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
  // Transform<GetResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetResponse | GetResponse[]>
      | Iterable<GetResponse | GetResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetResponse.encode(p).finish()]
        }
      } else {
        yield* [GetResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<GetResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetResponse.decode(p)]
        }
      } else {
        yield* [GetResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): GetResponse {
    return {
      notFound: isSet(object.notFound) ? Boolean(object.notFound) : false,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(),
      err: isSet(object.err) ? String(object.err) : '',
    }
  },

  toJSON(message: GetResponse): unknown {
    const obj: any = {}
    message.notFound !== undefined && (obj.notFound = message.notFound)
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    message.err !== undefined && (obj.err = message.err)
    return obj
  },

  create<I extends Exact<DeepPartial<GetResponse>, I>>(base?: I): GetResponse {
    return GetResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<GetResponse>, I>>(
    object: I
  ): GetResponse {
    const message = createBaseGetResponse()
    message.notFound = object.notFound ?? false
    message.data = object.data ?? new Uint8Array()
    message.err = object.err ?? ''
    return message
  },
}

function createBaseExistsResponse(): ExistsResponse {
  return { exists: false, notFound: false, err: '' }
}

export const ExistsResponse = {
  encode(
    message: ExistsResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.exists === true) {
      writer.uint32(8).bool(message.exists)
    }
    if (message.notFound === true) {
      writer.uint32(16).bool(message.notFound)
    }
    if (message.err !== '') {
      writer.uint32(26).string(message.err)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ExistsResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseExistsResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.exists = reader.bool()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.notFound = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.err = reader.string()
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
  // Transform<ExistsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ExistsResponse | ExistsResponse[]>
      | Iterable<ExistsResponse | ExistsResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExistsResponse.encode(p).finish()]
        }
      } else {
        yield* [ExistsResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ExistsResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ExistsResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExistsResponse.decode(p)]
        }
      } else {
        yield* [ExistsResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ExistsResponse {
    return {
      exists: isSet(object.exists) ? Boolean(object.exists) : false,
      notFound: isSet(object.notFound) ? Boolean(object.notFound) : false,
      err: isSet(object.err) ? String(object.err) : '',
    }
  },

  toJSON(message: ExistsResponse): unknown {
    const obj: any = {}
    message.exists !== undefined && (obj.exists = message.exists)
    message.notFound !== undefined && (obj.notFound = message.notFound)
    message.err !== undefined && (obj.err = message.err)
    return obj
  },

  create<I extends Exact<DeepPartial<ExistsResponse>, I>>(
    base?: I
  ): ExistsResponse {
    return ExistsResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ExistsResponse>, I>>(
    object: I
  ): ExistsResponse {
    const message = createBaseExistsResponse()
    message.exists = object.exists ?? false
    message.notFound = object.notFound ?? false
    message.err = object.err ?? ''
    return message
  },
}

function createBaseRmResponse(): RmResponse {
  return { removed: false, err: '' }
}

export const RmResponse = {
  encode(
    message: RmResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.removed === true) {
      writer.uint32(8).bool(message.removed)
    }
    if (message.err !== '') {
      writer.uint32(18).string(message.err)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RmResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRmResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.removed = reader.bool()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.err = reader.string()
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
  // Transform<RmResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RmResponse | RmResponse[]>
      | Iterable<RmResponse | RmResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RmResponse.encode(p).finish()]
        }
      } else {
        yield* [RmResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RmResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<RmResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RmResponse.decode(p)]
        }
      } else {
        yield* [RmResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RmResponse {
    return {
      removed: isSet(object.removed) ? Boolean(object.removed) : false,
      err: isSet(object.err) ? String(object.err) : '',
    }
  },

  toJSON(message: RmResponse): unknown {
    const obj: any = {}
    message.removed !== undefined && (obj.removed = message.removed)
    message.err !== undefined && (obj.err = message.err)
    return obj
  },

  create<I extends Exact<DeepPartial<RmResponse>, I>>(base?: I): RmResponse {
    return RmResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<RmResponse>, I>>(
    object: I
  ): RmResponse {
    const message = createBaseRmResponse()
    message.removed = object.removed ?? false
    message.err = object.err ?? ''
    return message
  },
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
