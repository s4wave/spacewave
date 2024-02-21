/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef, PutOpts } from '../block.pb.js'

export const protobufPackage = 'block.rpc'

/** PutBlockRequest requests to put a block into the store. */
export interface PutBlockRequest {
  /** Data is the data to put into the store. */
  data: Uint8Array
  /** PutOpts are any options when putting the block into the store. */
  putOpts: PutOpts | undefined
}

/** PutBlockResponse is the response to putting a block in the store. */
export interface PutBlockResponse {
  /** Ref is the reference of the added block. */
  ref: BlockRef | undefined
  /** Existed indicates the block already existed. */
  existed: boolean
  /** Error is any error adding the block to the store. */
  error: string
}

/** GetBlockRequest requests to get a block from the store. */
export interface GetBlockRequest {
  /** Ref is the reference to the block to fetch. */
  ref: BlockRef | undefined
}

/** GetBlockResponse is the response to looking up a block in the store. */
export interface GetBlockResponse {
  /** Exists indicates if the block exists or not. */
  exists: boolean
  /** Data is the data, if exists. */
  data: Uint8Array
  /** Error is any error getting the block from the store. */
  error: string
}

/** GetBlockExistsRequest requests to check if a block exists in the store. */
export interface GetBlockExistsRequest {
  /** Ref is the reference to the block to check. */
  ref: BlockRef | undefined
}

/** GetBlockExistsResponse is the response to checking if a block is in the store. */
export interface GetBlockExistsResponse {
  /** Exists indicates if the block exists or not. */
  exists: boolean
  /** Error is any error checking the block in the store. */
  error: string
}

/** RmBlockRequest requests to remove a block from the store. */
export interface RmBlockRequest {
  /** Ref is the reference to the block to remove. */
  ref: BlockRef | undefined
}

/** RmBlockResponse is the response to removing a block from the store. */
export interface RmBlockResponse {
  /**
   * Error is any error removing the block in the store.
   * Will be empty if the block did not exist.
   */
  error: string
}

function createBasePutBlockRequest(): PutBlockRequest {
  return { data: new Uint8Array(0), putOpts: undefined }
}

export const PutBlockRequest = {
  encode(
    message: PutBlockRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.data.length !== 0) {
      writer.uint32(10).bytes(message.data)
    }
    if (message.putOpts !== undefined) {
      PutOpts.encode(message.putOpts, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PutBlockRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePutBlockRequest()
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
  // Transform<PutBlockRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PutBlockRequest | PutBlockRequest[]>
      | Iterable<PutBlockRequest | PutBlockRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PutBlockRequest.encode(p).finish()]
        }
      } else {
        yield* [PutBlockRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PutBlockRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PutBlockRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PutBlockRequest.decode(p)]
        }
      } else {
        yield* [PutBlockRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): PutBlockRequest {
    return {
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
      putOpts: isSet(object.putOpts)
        ? PutOpts.fromJSON(object.putOpts)
        : undefined,
    }
  },

  toJSON(message: PutBlockRequest): unknown {
    const obj: any = {}
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    if (message.putOpts !== undefined) {
      obj.putOpts = PutOpts.toJSON(message.putOpts)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<PutBlockRequest>, I>>(
    base?: I,
  ): PutBlockRequest {
    return PutBlockRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<PutBlockRequest>, I>>(
    object: I,
  ): PutBlockRequest {
    const message = createBasePutBlockRequest()
    message.data = object.data ?? new Uint8Array(0)
    message.putOpts =
      object.putOpts !== undefined && object.putOpts !== null
        ? PutOpts.fromPartial(object.putOpts)
        : undefined
    return message
  },
}

function createBasePutBlockResponse(): PutBlockResponse {
  return { ref: undefined, existed: false, error: '' }
}

export const PutBlockResponse = {
  encode(
    message: PutBlockResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(10).fork()).ldelim()
    }
    if (message.existed === true) {
      writer.uint32(16).bool(message.existed)
    }
    if (message.error !== '') {
      writer.uint32(26).string(message.error)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PutBlockResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePutBlockResponse()
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

          message.existed = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
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
  // Transform<PutBlockResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PutBlockResponse | PutBlockResponse[]>
      | Iterable<PutBlockResponse | PutBlockResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PutBlockResponse.encode(p).finish()]
        }
      } else {
        yield* [PutBlockResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PutBlockResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PutBlockResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PutBlockResponse.decode(p)]
        }
      } else {
        yield* [PutBlockResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): PutBlockResponse {
    return {
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
      existed: isSet(object.existed)
        ? globalThis.Boolean(object.existed)
        : false,
      error: isSet(object.error) ? globalThis.String(object.error) : '',
    }
  },

  toJSON(message: PutBlockResponse): unknown {
    const obj: any = {}
    if (message.ref !== undefined) {
      obj.ref = BlockRef.toJSON(message.ref)
    }
    if (message.existed === true) {
      obj.existed = message.existed
    }
    if (message.error !== '') {
      obj.error = message.error
    }
    return obj
  },

  create<I extends Exact<DeepPartial<PutBlockResponse>, I>>(
    base?: I,
  ): PutBlockResponse {
    return PutBlockResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<PutBlockResponse>, I>>(
    object: I,
  ): PutBlockResponse {
    const message = createBasePutBlockResponse()
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
        : undefined
    message.existed = object.existed ?? false
    message.error = object.error ?? ''
    return message
  },
}

function createBaseGetBlockRequest(): GetBlockRequest {
  return { ref: undefined }
}

export const GetBlockRequest = {
  encode(
    message: GetBlockRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetBlockRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetBlockRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.ref = BlockRef.decode(reader, reader.uint32())
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
  // Transform<GetBlockRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetBlockRequest | GetBlockRequest[]>
      | Iterable<GetBlockRequest | GetBlockRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetBlockRequest.encode(p).finish()]
        }
      } else {
        yield* [GetBlockRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetBlockRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetBlockRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetBlockRequest.decode(p)]
        }
      } else {
        yield* [GetBlockRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetBlockRequest {
    return {
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: GetBlockRequest): unknown {
    const obj: any = {}
    if (message.ref !== undefined) {
      obj.ref = BlockRef.toJSON(message.ref)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetBlockRequest>, I>>(
    base?: I,
  ): GetBlockRequest {
    return GetBlockRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetBlockRequest>, I>>(
    object: I,
  ): GetBlockRequest {
    const message = createBaseGetBlockRequest()
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
        : undefined
    return message
  },
}

function createBaseGetBlockResponse(): GetBlockResponse {
  return { exists: false, data: new Uint8Array(0), error: '' }
}

export const GetBlockResponse = {
  encode(
    message: GetBlockResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.exists === true) {
      writer.uint32(8).bool(message.exists)
    }
    if (message.data.length !== 0) {
      writer.uint32(18).bytes(message.data)
    }
    if (message.error !== '') {
      writer.uint32(26).string(message.error)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetBlockResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetBlockResponse()
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
          if (tag !== 18) {
            break
          }

          message.data = reader.bytes()
          continue
        case 3:
          if (tag !== 26) {
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
  // Transform<GetBlockResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetBlockResponse | GetBlockResponse[]>
      | Iterable<GetBlockResponse | GetBlockResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetBlockResponse.encode(p).finish()]
        }
      } else {
        yield* [GetBlockResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetBlockResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetBlockResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetBlockResponse.decode(p)]
        }
      } else {
        yield* [GetBlockResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetBlockResponse {
    return {
      exists: isSet(object.exists) ? globalThis.Boolean(object.exists) : false,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
      error: isSet(object.error) ? globalThis.String(object.error) : '',
    }
  },

  toJSON(message: GetBlockResponse): unknown {
    const obj: any = {}
    if (message.exists === true) {
      obj.exists = message.exists
    }
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    if (message.error !== '') {
      obj.error = message.error
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetBlockResponse>, I>>(
    base?: I,
  ): GetBlockResponse {
    return GetBlockResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetBlockResponse>, I>>(
    object: I,
  ): GetBlockResponse {
    const message = createBaseGetBlockResponse()
    message.exists = object.exists ?? false
    message.data = object.data ?? new Uint8Array(0)
    message.error = object.error ?? ''
    return message
  },
}

function createBaseGetBlockExistsRequest(): GetBlockExistsRequest {
  return { ref: undefined }
}

export const GetBlockExistsRequest = {
  encode(
    message: GetBlockExistsRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GetBlockExistsRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetBlockExistsRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.ref = BlockRef.decode(reader, reader.uint32())
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
  // Transform<GetBlockExistsRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetBlockExistsRequest | GetBlockExistsRequest[]>
      | Iterable<GetBlockExistsRequest | GetBlockExistsRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetBlockExistsRequest.encode(p).finish()]
        }
      } else {
        yield* [GetBlockExistsRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetBlockExistsRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetBlockExistsRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetBlockExistsRequest.decode(p)]
        }
      } else {
        yield* [GetBlockExistsRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetBlockExistsRequest {
    return {
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: GetBlockExistsRequest): unknown {
    const obj: any = {}
    if (message.ref !== undefined) {
      obj.ref = BlockRef.toJSON(message.ref)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetBlockExistsRequest>, I>>(
    base?: I,
  ): GetBlockExistsRequest {
    return GetBlockExistsRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetBlockExistsRequest>, I>>(
    object: I,
  ): GetBlockExistsRequest {
    const message = createBaseGetBlockExistsRequest()
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
        : undefined
    return message
  },
}

function createBaseGetBlockExistsResponse(): GetBlockExistsResponse {
  return { exists: false, error: '' }
}

export const GetBlockExistsResponse = {
  encode(
    message: GetBlockExistsResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.exists === true) {
      writer.uint32(8).bool(message.exists)
    }
    if (message.error !== '') {
      writer.uint32(18).string(message.error)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GetBlockExistsResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetBlockExistsResponse()
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
          if (tag !== 18) {
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
  // Transform<GetBlockExistsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetBlockExistsResponse | GetBlockExistsResponse[]>
      | Iterable<GetBlockExistsResponse | GetBlockExistsResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetBlockExistsResponse.encode(p).finish()]
        }
      } else {
        yield* [GetBlockExistsResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetBlockExistsResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetBlockExistsResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetBlockExistsResponse.decode(p)]
        }
      } else {
        yield* [GetBlockExistsResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetBlockExistsResponse {
    return {
      exists: isSet(object.exists) ? globalThis.Boolean(object.exists) : false,
      error: isSet(object.error) ? globalThis.String(object.error) : '',
    }
  },

  toJSON(message: GetBlockExistsResponse): unknown {
    const obj: any = {}
    if (message.exists === true) {
      obj.exists = message.exists
    }
    if (message.error !== '') {
      obj.error = message.error
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetBlockExistsResponse>, I>>(
    base?: I,
  ): GetBlockExistsResponse {
    return GetBlockExistsResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetBlockExistsResponse>, I>>(
    object: I,
  ): GetBlockExistsResponse {
    const message = createBaseGetBlockExistsResponse()
    message.exists = object.exists ?? false
    message.error = object.error ?? ''
    return message
  },
}

function createBaseRmBlockRequest(): RmBlockRequest {
  return { ref: undefined }
}

export const RmBlockRequest = {
  encode(
    message: RmBlockRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RmBlockRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRmBlockRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.ref = BlockRef.decode(reader, reader.uint32())
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
  // Transform<RmBlockRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RmBlockRequest | RmBlockRequest[]>
      | Iterable<RmBlockRequest | RmBlockRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RmBlockRequest.encode(p).finish()]
        }
      } else {
        yield* [RmBlockRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RmBlockRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RmBlockRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RmBlockRequest.decode(p)]
        }
      } else {
        yield* [RmBlockRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): RmBlockRequest {
    return {
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: RmBlockRequest): unknown {
    const obj: any = {}
    if (message.ref !== undefined) {
      obj.ref = BlockRef.toJSON(message.ref)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RmBlockRequest>, I>>(
    base?: I,
  ): RmBlockRequest {
    return RmBlockRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RmBlockRequest>, I>>(
    object: I,
  ): RmBlockRequest {
    const message = createBaseRmBlockRequest()
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
        : undefined
    return message
  },
}

function createBaseRmBlockResponse(): RmBlockResponse {
  return { error: '' }
}

export const RmBlockResponse = {
  encode(
    message: RmBlockResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RmBlockResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRmBlockResponse()
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
  // Transform<RmBlockResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RmBlockResponse | RmBlockResponse[]>
      | Iterable<RmBlockResponse | RmBlockResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RmBlockResponse.encode(p).finish()]
        }
      } else {
        yield* [RmBlockResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RmBlockResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RmBlockResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RmBlockResponse.decode(p)]
        }
      } else {
        yield* [RmBlockResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): RmBlockResponse {
    return { error: isSet(object.error) ? globalThis.String(object.error) : '' }
  },

  toJSON(message: RmBlockResponse): unknown {
    const obj: any = {}
    if (message.error !== '') {
      obj.error = message.error
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RmBlockResponse>, I>>(
    base?: I,
  ): RmBlockResponse {
    return RmBlockResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RmBlockResponse>, I>>(
    object: I,
  ): RmBlockResponse {
    const message = createBaseRmBlockResponse()
    message.error = object.error ?? ''
    return message
  },
}

/** BlockStore wraps a BlockStore interface with a RPC service. */
export interface BlockStore {
  /** PutBlock requests to put a block into the store. */
  PutBlock(
    request: PutBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<PutBlockResponse>
  /** GetBlock requests to lookup a block from the store. */
  GetBlock(
    request: GetBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetBlockResponse>
  /** GetBlockExists requests to check if a block exists in the store. */
  GetBlockExists(
    request: GetBlockExistsRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetBlockExistsResponse>
  /**
   * RmBlock requests to remove a block from the store.
   * Does not return an error if the block was not present.
   * In some cases, will return before confirming delete.
   */
  RmBlock(
    request: RmBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<RmBlockResponse>
}

export const BlockStoreServiceName = 'block.rpc.BlockStore'
export class BlockStoreClientImpl implements BlockStore {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || BlockStoreServiceName
    this.rpc = rpc
    this.PutBlock = this.PutBlock.bind(this)
    this.GetBlock = this.GetBlock.bind(this)
    this.GetBlockExists = this.GetBlockExists.bind(this)
    this.RmBlock = this.RmBlock.bind(this)
  }
  PutBlock(
    request: PutBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<PutBlockResponse> {
    const data = PutBlockRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'PutBlock',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      PutBlockResponse.decode(_m0.Reader.create(data)),
    )
  }

  GetBlock(
    request: GetBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetBlockResponse> {
    const data = GetBlockRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'GetBlock',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      GetBlockResponse.decode(_m0.Reader.create(data)),
    )
  }

  GetBlockExists(
    request: GetBlockExistsRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetBlockExistsResponse> {
    const data = GetBlockExistsRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'GetBlockExists',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      GetBlockExistsResponse.decode(_m0.Reader.create(data)),
    )
  }

  RmBlock(
    request: RmBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<RmBlockResponse> {
    const data = RmBlockRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'RmBlock',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      RmBlockResponse.decode(_m0.Reader.create(data)),
    )
  }
}

/** BlockStore wraps a BlockStore interface with a RPC service. */
export type BlockStoreDefinition = typeof BlockStoreDefinition
export const BlockStoreDefinition = {
  name: 'BlockStore',
  fullName: 'block.rpc.BlockStore',
  methods: {
    /** PutBlock requests to put a block into the store. */
    putBlock: {
      name: 'PutBlock',
      requestType: PutBlockRequest,
      requestStream: false,
      responseType: PutBlockResponse,
      responseStream: false,
      options: {},
    },
    /** GetBlock requests to lookup a block from the store. */
    getBlock: {
      name: 'GetBlock',
      requestType: GetBlockRequest,
      requestStream: false,
      responseType: GetBlockResponse,
      responseStream: false,
      options: {},
    },
    /** GetBlockExists requests to check if a block exists in the store. */
    getBlockExists: {
      name: 'GetBlockExists',
      requestType: GetBlockExistsRequest,
      requestStream: false,
      responseType: GetBlockExistsResponse,
      responseStream: false,
      options: {},
    },
    /**
     * RmBlock requests to remove a block from the store.
     * Does not return an error if the block was not present.
     * In some cases, will return before confirming delete.
     */
    rmBlock: {
      name: 'RmBlock',
      requestType: RmBlockRequest,
      requestStream: false,
      responseType: RmBlockResponse,
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
