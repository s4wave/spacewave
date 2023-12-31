/* eslint-disable */
import { Timestamp } from '@aperturerobotics/ts-proto-common-types/google/protobuf/timestamp.pb.js'
import { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'mqueue.rpc'

/** RmMqueueRequest requests to remove a message queue and its contents. */
export interface RmMqueueRequest {
  /** MqueueId is the message queue to remove. */
  mqueueId: Uint8Array
}

/** RmMqueueResponse is the response to removing a message queue. */
export interface RmMqueueResponse {
  /**
   * Error is any error removing the message queue.
   * Will be empty if the queue did not exist.
   */
  error: string
}

/** ListMqueuesRequest requests to list message queues with a id prefix. */
export interface ListMqueuesRequest {
  /** Prefix is the message queue id prefix to filter by. */
  prefix: Uint8Array
  /**
   * Filled indicates to filter the IDs to only queues with a pending message.
   *
   * Note: if !filled, implementation might not return queues that are empty.
   * If filled is set, implementation must only return filled queues.
   */
  filled: boolean
}

/** ListMqueuesResponse is the response to listing message queues. */
export interface ListMqueuesResponse {
  /** Error is any error listing message queues. */
  error: string
  /** MqueueIds is the list of message queue ids. */
  mqueueIds: Uint8Array[]
}

/** PeekRequest is a request to peek the next message. */
export interface PeekRequest {}

/** PeekResponse responds to a request to peek the next message. */
export interface PeekResponse {
  /**
   * Error is any error accessing the key.
   * Will be empty if the key was unset.
   */
  error: string
  /** Found indicates there was a message. */
  found: boolean
  /** Msg contains the message, if found=true. */
  msg: MqueueMsg | undefined
}

/** AckRequest is a request to ack a message. */
export interface AckRequest {
  /** Id is the message id to acknowledge. */
  id: Long
}

/** AckResponse is the response to acking a message. */
export interface AckResponse {
  /**
   * Error is any error acknowledging the message.
   * If empty, the operation succeeded.
   */
  error: string
}

/** PushRequest is a request to push a message to the queue. */
export interface PushRequest {
  /** Data is the contents of the message to push. */
  data: Uint8Array
}

/** PushResponse is the response to pushing a message. */
export interface PushResponse {
  /**
   * Error is any error pushing the message.
   * If empty, the operation succeeded.
   */
  error: string
  /**
   * Msg contains the pushed message, if error="".
   * note: the data field will be empty.
   */
  msg: MqueueMsg | undefined
}

/** MqueueMsg is a message with associated metadata. */
export interface MqueueMsg {
  /** Id contains the message id. */
  id: Long
  /** Timestamp contains the message timestamp. */
  timestamp: Date | undefined
  /** Data contains the message data. */
  data: Uint8Array
}

/** WaitRequest is a request to wait for the next message. */
export interface WaitRequest {
  /**
   * Ack indicates to ack the message when returning it.
   * Note: message may be dropped in transit.
   * You may want to ack with a second call instead.
   */
  ack: boolean
}

/** WaitResponse is the response to waiting for a message. */
export interface WaitResponse {
  /** Msg contains the message. */
  msg: MqueueMsg | undefined
}

/** DeleteQueueRequest is a request to delete the queue. */
export interface DeleteQueueRequest {}

/** DeleteQueueResponse is the response to deleting a message queue. */
export interface DeleteQueueResponse {
  /**
   * Error is any error deleting the queue.
   * If empty, the operation succeeded.
   */
  error: string
}

function createBaseRmMqueueRequest(): RmMqueueRequest {
  return { mqueueId: new Uint8Array(0) }
}

export const RmMqueueRequest = {
  encode(
    message: RmMqueueRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.mqueueId.length !== 0) {
      writer.uint32(10).bytes(message.mqueueId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RmMqueueRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRmMqueueRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.mqueueId = reader.bytes()
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
  // Transform<RmMqueueRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RmMqueueRequest | RmMqueueRequest[]>
      | Iterable<RmMqueueRequest | RmMqueueRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RmMqueueRequest.encode(p).finish()]
        }
      } else {
        yield* [RmMqueueRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RmMqueueRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RmMqueueRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RmMqueueRequest.decode(p)]
        }
      } else {
        yield* [RmMqueueRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): RmMqueueRequest {
    return {
      mqueueId: isSet(object.mqueueId)
        ? bytesFromBase64(object.mqueueId)
        : new Uint8Array(0),
    }
  },

  toJSON(message: RmMqueueRequest): unknown {
    const obj: any = {}
    if (message.mqueueId.length !== 0) {
      obj.mqueueId = base64FromBytes(message.mqueueId)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RmMqueueRequest>, I>>(
    base?: I,
  ): RmMqueueRequest {
    return RmMqueueRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RmMqueueRequest>, I>>(
    object: I,
  ): RmMqueueRequest {
    const message = createBaseRmMqueueRequest()
    message.mqueueId = object.mqueueId ?? new Uint8Array(0)
    return message
  },
}

function createBaseRmMqueueResponse(): RmMqueueResponse {
  return { error: '' }
}

export const RmMqueueResponse = {
  encode(
    message: RmMqueueResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RmMqueueResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRmMqueueResponse()
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
  // Transform<RmMqueueResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RmMqueueResponse | RmMqueueResponse[]>
      | Iterable<RmMqueueResponse | RmMqueueResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RmMqueueResponse.encode(p).finish()]
        }
      } else {
        yield* [RmMqueueResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RmMqueueResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RmMqueueResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RmMqueueResponse.decode(p)]
        }
      } else {
        yield* [RmMqueueResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): RmMqueueResponse {
    return { error: isSet(object.error) ? globalThis.String(object.error) : '' }
  },

  toJSON(message: RmMqueueResponse): unknown {
    const obj: any = {}
    if (message.error !== '') {
      obj.error = message.error
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RmMqueueResponse>, I>>(
    base?: I,
  ): RmMqueueResponse {
    return RmMqueueResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RmMqueueResponse>, I>>(
    object: I,
  ): RmMqueueResponse {
    const message = createBaseRmMqueueResponse()
    message.error = object.error ?? ''
    return message
  },
}

function createBaseListMqueuesRequest(): ListMqueuesRequest {
  return { prefix: new Uint8Array(0), filled: false }
}

export const ListMqueuesRequest = {
  encode(
    message: ListMqueuesRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.prefix.length !== 0) {
      writer.uint32(10).bytes(message.prefix)
    }
    if (message.filled === true) {
      writer.uint32(16).bool(message.filled)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListMqueuesRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListMqueuesRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.prefix = reader.bytes()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.filled = reader.bool()
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
  // Transform<ListMqueuesRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListMqueuesRequest | ListMqueuesRequest[]>
      | Iterable<ListMqueuesRequest | ListMqueuesRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListMqueuesRequest.encode(p).finish()]
        }
      } else {
        yield* [ListMqueuesRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListMqueuesRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ListMqueuesRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListMqueuesRequest.decode(p)]
        }
      } else {
        yield* [ListMqueuesRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ListMqueuesRequest {
    return {
      prefix: isSet(object.prefix)
        ? bytesFromBase64(object.prefix)
        : new Uint8Array(0),
      filled: isSet(object.filled) ? globalThis.Boolean(object.filled) : false,
    }
  },

  toJSON(message: ListMqueuesRequest): unknown {
    const obj: any = {}
    if (message.prefix.length !== 0) {
      obj.prefix = base64FromBytes(message.prefix)
    }
    if (message.filled === true) {
      obj.filled = message.filled
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ListMqueuesRequest>, I>>(
    base?: I,
  ): ListMqueuesRequest {
    return ListMqueuesRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ListMqueuesRequest>, I>>(
    object: I,
  ): ListMqueuesRequest {
    const message = createBaseListMqueuesRequest()
    message.prefix = object.prefix ?? new Uint8Array(0)
    message.filled = object.filled ?? false
    return message
  },
}

function createBaseListMqueuesResponse(): ListMqueuesResponse {
  return { error: '', mqueueIds: [] }
}

export const ListMqueuesResponse = {
  encode(
    message: ListMqueuesResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    for (const v of message.mqueueIds) {
      writer.uint32(18).bytes(v!)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListMqueuesResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListMqueuesResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.error = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.mqueueIds.push(reader.bytes())
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
  // Transform<ListMqueuesResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListMqueuesResponse | ListMqueuesResponse[]>
      | Iterable<ListMqueuesResponse | ListMqueuesResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListMqueuesResponse.encode(p).finish()]
        }
      } else {
        yield* [ListMqueuesResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListMqueuesResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ListMqueuesResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ListMqueuesResponse.decode(p)]
        }
      } else {
        yield* [ListMqueuesResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ListMqueuesResponse {
    return {
      error: isSet(object.error) ? globalThis.String(object.error) : '',
      mqueueIds: globalThis.Array.isArray(object?.mqueueIds)
        ? object.mqueueIds.map((e: any) => bytesFromBase64(e))
        : [],
    }
  },

  toJSON(message: ListMqueuesResponse): unknown {
    const obj: any = {}
    if (message.error !== '') {
      obj.error = message.error
    }
    if (message.mqueueIds?.length) {
      obj.mqueueIds = message.mqueueIds.map((e) => base64FromBytes(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ListMqueuesResponse>, I>>(
    base?: I,
  ): ListMqueuesResponse {
    return ListMqueuesResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ListMqueuesResponse>, I>>(
    object: I,
  ): ListMqueuesResponse {
    const message = createBaseListMqueuesResponse()
    message.error = object.error ?? ''
    message.mqueueIds = object.mqueueIds?.map((e) => e) || []
    return message
  },
}

function createBasePeekRequest(): PeekRequest {
  return {}
}

export const PeekRequest = {
  encode(_: PeekRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PeekRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePeekRequest()
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
  // Transform<PeekRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PeekRequest | PeekRequest[]>
      | Iterable<PeekRequest | PeekRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PeekRequest.encode(p).finish()]
        }
      } else {
        yield* [PeekRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PeekRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PeekRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PeekRequest.decode(p)]
        }
      } else {
        yield* [PeekRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): PeekRequest {
    return {}
  },

  toJSON(_: PeekRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<PeekRequest>, I>>(base?: I): PeekRequest {
    return PeekRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<PeekRequest>, I>>(_: I): PeekRequest {
    const message = createBasePeekRequest()
    return message
  },
}

function createBasePeekResponse(): PeekResponse {
  return { error: '', found: false, msg: undefined }
}

export const PeekResponse = {
  encode(
    message: PeekResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    if (message.found === true) {
      writer.uint32(16).bool(message.found)
    }
    if (message.msg !== undefined) {
      MqueueMsg.encode(message.msg, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PeekResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePeekResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.error = reader.string()
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

          message.msg = MqueueMsg.decode(reader, reader.uint32())
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
  // Transform<PeekResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PeekResponse | PeekResponse[]>
      | Iterable<PeekResponse | PeekResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PeekResponse.encode(p).finish()]
        }
      } else {
        yield* [PeekResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PeekResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PeekResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PeekResponse.decode(p)]
        }
      } else {
        yield* [PeekResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): PeekResponse {
    return {
      error: isSet(object.error) ? globalThis.String(object.error) : '',
      found: isSet(object.found) ? globalThis.Boolean(object.found) : false,
      msg: isSet(object.msg) ? MqueueMsg.fromJSON(object.msg) : undefined,
    }
  },

  toJSON(message: PeekResponse): unknown {
    const obj: any = {}
    if (message.error !== '') {
      obj.error = message.error
    }
    if (message.found === true) {
      obj.found = message.found
    }
    if (message.msg !== undefined) {
      obj.msg = MqueueMsg.toJSON(message.msg)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<PeekResponse>, I>>(
    base?: I,
  ): PeekResponse {
    return PeekResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<PeekResponse>, I>>(
    object: I,
  ): PeekResponse {
    const message = createBasePeekResponse()
    message.error = object.error ?? ''
    message.found = object.found ?? false
    message.msg =
      object.msg !== undefined && object.msg !== null
        ? MqueueMsg.fromPartial(object.msg)
        : undefined
    return message
  },
}

function createBaseAckRequest(): AckRequest {
  return { id: Long.UZERO }
}

export const AckRequest = {
  encode(
    message: AckRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.id.isZero()) {
      writer.uint32(8).uint64(message.id)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): AckRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseAckRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.id = reader.uint64() as Long
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
  // Transform<AckRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<AckRequest | AckRequest[]>
      | Iterable<AckRequest | AckRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [AckRequest.encode(p).finish()]
        }
      } else {
        yield* [AckRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, AckRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<AckRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [AckRequest.decode(p)]
        }
      } else {
        yield* [AckRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): AckRequest {
    return { id: isSet(object.id) ? Long.fromValue(object.id) : Long.UZERO }
  },

  toJSON(message: AckRequest): unknown {
    const obj: any = {}
    if (!message.id.isZero()) {
      obj.id = (message.id || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<AckRequest>, I>>(base?: I): AckRequest {
    return AckRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<AckRequest>, I>>(
    object: I,
  ): AckRequest {
    const message = createBaseAckRequest()
    message.id =
      object.id !== undefined && object.id !== null
        ? Long.fromValue(object.id)
        : Long.UZERO
    return message
  },
}

function createBaseAckResponse(): AckResponse {
  return { error: '' }
}

export const AckResponse = {
  encode(
    message: AckResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): AckResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseAckResponse()
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
  // Transform<AckResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<AckResponse | AckResponse[]>
      | Iterable<AckResponse | AckResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [AckResponse.encode(p).finish()]
        }
      } else {
        yield* [AckResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, AckResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<AckResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [AckResponse.decode(p)]
        }
      } else {
        yield* [AckResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): AckResponse {
    return { error: isSet(object.error) ? globalThis.String(object.error) : '' }
  },

  toJSON(message: AckResponse): unknown {
    const obj: any = {}
    if (message.error !== '') {
      obj.error = message.error
    }
    return obj
  },

  create<I extends Exact<DeepPartial<AckResponse>, I>>(base?: I): AckResponse {
    return AckResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<AckResponse>, I>>(
    object: I,
  ): AckResponse {
    const message = createBaseAckResponse()
    message.error = object.error ?? ''
    return message
  },
}

function createBasePushRequest(): PushRequest {
  return { data: new Uint8Array(0) }
}

export const PushRequest = {
  encode(
    message: PushRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.data.length !== 0) {
      writer.uint32(10).bytes(message.data)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PushRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePushRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
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
  // Transform<PushRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PushRequest | PushRequest[]>
      | Iterable<PushRequest | PushRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PushRequest.encode(p).finish()]
        }
      } else {
        yield* [PushRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PushRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PushRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PushRequest.decode(p)]
        }
      } else {
        yield* [PushRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): PushRequest {
    return {
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
    }
  },

  toJSON(message: PushRequest): unknown {
    const obj: any = {}
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<PushRequest>, I>>(base?: I): PushRequest {
    return PushRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<PushRequest>, I>>(
    object: I,
  ): PushRequest {
    const message = createBasePushRequest()
    message.data = object.data ?? new Uint8Array(0)
    return message
  },
}

function createBasePushResponse(): PushResponse {
  return { error: '', msg: undefined }
}

export const PushResponse = {
  encode(
    message: PushResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    if (message.msg !== undefined) {
      MqueueMsg.encode(message.msg, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PushResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePushResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.error = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.msg = MqueueMsg.decode(reader, reader.uint32())
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
  // Transform<PushResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PushResponse | PushResponse[]>
      | Iterable<PushResponse | PushResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PushResponse.encode(p).finish()]
        }
      } else {
        yield* [PushResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PushResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PushResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [PushResponse.decode(p)]
        }
      } else {
        yield* [PushResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): PushResponse {
    return {
      error: isSet(object.error) ? globalThis.String(object.error) : '',
      msg: isSet(object.msg) ? MqueueMsg.fromJSON(object.msg) : undefined,
    }
  },

  toJSON(message: PushResponse): unknown {
    const obj: any = {}
    if (message.error !== '') {
      obj.error = message.error
    }
    if (message.msg !== undefined) {
      obj.msg = MqueueMsg.toJSON(message.msg)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<PushResponse>, I>>(
    base?: I,
  ): PushResponse {
    return PushResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<PushResponse>, I>>(
    object: I,
  ): PushResponse {
    const message = createBasePushResponse()
    message.error = object.error ?? ''
    message.msg =
      object.msg !== undefined && object.msg !== null
        ? MqueueMsg.fromPartial(object.msg)
        : undefined
    return message
  },
}

function createBaseMqueueMsg(): MqueueMsg {
  return { id: Long.UZERO, timestamp: undefined, data: new Uint8Array(0) }
}

export const MqueueMsg = {
  encode(
    message: MqueueMsg,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.id.isZero()) {
      writer.uint32(8).uint64(message.id)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(
        toTimestamp(message.timestamp),
        writer.uint32(18).fork(),
      ).ldelim()
    }
    if (message.data.length !== 0) {
      writer.uint32(26).bytes(message.data)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MqueueMsg {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseMqueueMsg()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.id = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.timestamp = fromTimestamp(
            Timestamp.decode(reader, reader.uint32()),
          )
          continue
        case 3:
          if (tag !== 26) {
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
  // Transform<MqueueMsg, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<MqueueMsg | MqueueMsg[]>
      | Iterable<MqueueMsg | MqueueMsg[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [MqueueMsg.encode(p).finish()]
        }
      } else {
        yield* [MqueueMsg.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MqueueMsg>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<MqueueMsg> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [MqueueMsg.decode(p)]
        }
      } else {
        yield* [MqueueMsg.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): MqueueMsg {
    return {
      id: isSet(object.id) ? Long.fromValue(object.id) : Long.UZERO,
      timestamp: isSet(object.timestamp)
        ? fromJsonTimestamp(object.timestamp)
        : undefined,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
    }
  },

  toJSON(message: MqueueMsg): unknown {
    const obj: any = {}
    if (!message.id.isZero()) {
      obj.id = (message.id || Long.UZERO).toString()
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = message.timestamp.toISOString()
    }
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<MqueueMsg>, I>>(base?: I): MqueueMsg {
    return MqueueMsg.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<MqueueMsg>, I>>(
    object: I,
  ): MqueueMsg {
    const message = createBaseMqueueMsg()
    message.id =
      object.id !== undefined && object.id !== null
        ? Long.fromValue(object.id)
        : Long.UZERO
    message.timestamp = object.timestamp ?? undefined
    message.data = object.data ?? new Uint8Array(0)
    return message
  },
}

function createBaseWaitRequest(): WaitRequest {
  return { ack: false }
}

export const WaitRequest = {
  encode(
    message: WaitRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.ack === true) {
      writer.uint32(8).bool(message.ack)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WaitRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWaitRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.ack = reader.bool()
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
  // Transform<WaitRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WaitRequest | WaitRequest[]>
      | Iterable<WaitRequest | WaitRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WaitRequest.encode(p).finish()]
        }
      } else {
        yield* [WaitRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WaitRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WaitRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WaitRequest.decode(p)]
        }
      } else {
        yield* [WaitRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WaitRequest {
    return { ack: isSet(object.ack) ? globalThis.Boolean(object.ack) : false }
  },

  toJSON(message: WaitRequest): unknown {
    const obj: any = {}
    if (message.ack === true) {
      obj.ack = message.ack
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WaitRequest>, I>>(base?: I): WaitRequest {
    return WaitRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WaitRequest>, I>>(
    object: I,
  ): WaitRequest {
    const message = createBaseWaitRequest()
    message.ack = object.ack ?? false
    return message
  },
}

function createBaseWaitResponse(): WaitResponse {
  return { msg: undefined }
}

export const WaitResponse = {
  encode(
    message: WaitResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.msg !== undefined) {
      MqueueMsg.encode(message.msg, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WaitResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWaitResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.msg = MqueueMsg.decode(reader, reader.uint32())
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
  // Transform<WaitResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WaitResponse | WaitResponse[]>
      | Iterable<WaitResponse | WaitResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WaitResponse.encode(p).finish()]
        }
      } else {
        yield* [WaitResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WaitResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WaitResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WaitResponse.decode(p)]
        }
      } else {
        yield* [WaitResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WaitResponse {
    return {
      msg: isSet(object.msg) ? MqueueMsg.fromJSON(object.msg) : undefined,
    }
  },

  toJSON(message: WaitResponse): unknown {
    const obj: any = {}
    if (message.msg !== undefined) {
      obj.msg = MqueueMsg.toJSON(message.msg)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WaitResponse>, I>>(
    base?: I,
  ): WaitResponse {
    return WaitResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WaitResponse>, I>>(
    object: I,
  ): WaitResponse {
    const message = createBaseWaitResponse()
    message.msg =
      object.msg !== undefined && object.msg !== null
        ? MqueueMsg.fromPartial(object.msg)
        : undefined
    return message
  },
}

function createBaseDeleteQueueRequest(): DeleteQueueRequest {
  return {}
}

export const DeleteQueueRequest = {
  encode(
    _: DeleteQueueRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DeleteQueueRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDeleteQueueRequest()
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
  // Transform<DeleteQueueRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DeleteQueueRequest | DeleteQueueRequest[]>
      | Iterable<DeleteQueueRequest | DeleteQueueRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [DeleteQueueRequest.encode(p).finish()]
        }
      } else {
        yield* [DeleteQueueRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DeleteQueueRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DeleteQueueRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [DeleteQueueRequest.decode(p)]
        }
      } else {
        yield* [DeleteQueueRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): DeleteQueueRequest {
    return {}
  },

  toJSON(_: DeleteQueueRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<DeleteQueueRequest>, I>>(
    base?: I,
  ): DeleteQueueRequest {
    return DeleteQueueRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<DeleteQueueRequest>, I>>(
    _: I,
  ): DeleteQueueRequest {
    const message = createBaseDeleteQueueRequest()
    return message
  },
}

function createBaseDeleteQueueResponse(): DeleteQueueResponse {
  return { error: '' }
}

export const DeleteQueueResponse = {
  encode(
    message: DeleteQueueResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DeleteQueueResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDeleteQueueResponse()
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
  // Transform<DeleteQueueResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DeleteQueueResponse | DeleteQueueResponse[]>
      | Iterable<DeleteQueueResponse | DeleteQueueResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [DeleteQueueResponse.encode(p).finish()]
        }
      } else {
        yield* [DeleteQueueResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DeleteQueueResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DeleteQueueResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [DeleteQueueResponse.decode(p)]
        }
      } else {
        yield* [DeleteQueueResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): DeleteQueueResponse {
    return { error: isSet(object.error) ? globalThis.String(object.error) : '' }
  },

  toJSON(message: DeleteQueueResponse): unknown {
    const obj: any = {}
    if (message.error !== '') {
      obj.error = message.error
    }
    return obj
  },

  create<I extends Exact<DeepPartial<DeleteQueueResponse>, I>>(
    base?: I,
  ): DeleteQueueResponse {
    return DeleteQueueResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<DeleteQueueResponse>, I>>(
    object: I,
  ): DeleteQueueResponse {
    const message = createBaseDeleteQueueResponse()
    message.error = object.error ?? ''
    return message
  },
}

/** MqueueStore implements a container storing message queues. */
export interface MqueueStore {
  /**
   * MqueueRpc is a rpc request for a MessageQueue by ID.
   * Exposes service: rpc.mqueue.QueueOps
   * Component ID: message queue id.
   */
  MqueueRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
  /**
   * ListMqueues lists message queues with the given ID prefix.
   *
   * Note: if !filled, implementation might not return queues that are empty.
   * If filled is set, implementation must only return filled queues.
   */
  ListMqueues(
    request: ListMqueuesRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListMqueuesResponse>
  /** RmMqueue deletes the message queue and all contents by ID. */
  RmMqueue(
    request: RmMqueueRequest,
    abortSignal?: AbortSignal,
  ): Promise<RmMqueueResponse>
}

export const MqueueStoreServiceName = 'mqueue.rpc.MqueueStore'
export class MqueueStoreClientImpl implements MqueueStore {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || MqueueStoreServiceName
    this.rpc = rpc
    this.MqueueRpc = this.MqueueRpc.bind(this)
    this.ListMqueues = this.ListMqueues.bind(this)
    this.RmMqueue = this.RmMqueue.bind(this)
  }
  MqueueRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'MqueueRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }

  ListMqueues(
    request: ListMqueuesRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListMqueuesResponse> {
    const data = ListMqueuesRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'ListMqueues',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      ListMqueuesResponse.decode(_m0.Reader.create(data)),
    )
  }

  RmMqueue(
    request: RmMqueueRequest,
    abortSignal?: AbortSignal,
  ): Promise<RmMqueueResponse> {
    const data = RmMqueueRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'RmMqueue',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      RmMqueueResponse.decode(_m0.Reader.create(data)),
    )
  }
}

/** MqueueStore implements a container storing message queues. */
export type MqueueStoreDefinition = typeof MqueueStoreDefinition
export const MqueueStoreDefinition = {
  name: 'MqueueStore',
  fullName: 'mqueue.rpc.MqueueStore',
  methods: {
    /**
     * MqueueRpc is a rpc request for a MessageQueue by ID.
     * Exposes service: rpc.mqueue.QueueOps
     * Component ID: message queue id.
     */
    mqueueRpc: {
      name: 'MqueueRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
    /**
     * ListMqueues lists message queues with the given ID prefix.
     *
     * Note: if !filled, implementation might not return queues that are empty.
     * If filled is set, implementation must only return filled queues.
     */
    listMqueues: {
      name: 'ListMqueues',
      requestType: ListMqueuesRequest,
      requestStream: false,
      responseType: ListMqueuesResponse,
      responseStream: false,
      options: {},
    },
    /** RmMqueue deletes the message queue and all contents by ID. */
    rmMqueue: {
      name: 'RmMqueue',
      requestType: RmMqueueRequest,
      requestStream: false,
      responseType: RmMqueueResponse,
      responseStream: false,
      options: {},
    },
  },
} as const

/**
 * QueueOps exposes a Queue object with a service.
 * Wraps the mqueue.Queue interface.
 */
export interface QueueOps {
  /** Peek returns the next message, if any. */
  Peek(request: PeekRequest, abortSignal?: AbortSignal): Promise<PeekResponse>
  /**
   * Ack acknowledges the message with the given ID.
   * If the latest message is not the one with the ID, does nothing.
   */
  Ack(request: AckRequest, abortSignal?: AbortSignal): Promise<AckResponse>
  /** Push pushes a message to the queue. */
  Push(request: PushRequest, abortSignal?: AbortSignal): Promise<PushResponse>
  /** Wait waits for the next message, or call cancellation. */
  Wait(request: WaitRequest, abortSignal?: AbortSignal): Promise<WaitResponse>
  /** DeleteQueue deletes the messages and metadata for the queue. */
  DeleteQueue(
    request: DeleteQueueRequest,
    abortSignal?: AbortSignal,
  ): Promise<DeleteQueueResponse>
}

export const QueueOpsServiceName = 'mqueue.rpc.QueueOps'
export class QueueOpsClientImpl implements QueueOps {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || QueueOpsServiceName
    this.rpc = rpc
    this.Peek = this.Peek.bind(this)
    this.Ack = this.Ack.bind(this)
    this.Push = this.Push.bind(this)
    this.Wait = this.Wait.bind(this)
    this.DeleteQueue = this.DeleteQueue.bind(this)
  }
  Peek(request: PeekRequest, abortSignal?: AbortSignal): Promise<PeekResponse> {
    const data = PeekRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'Peek',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) => PeekResponse.decode(_m0.Reader.create(data)))
  }

  Ack(request: AckRequest, abortSignal?: AbortSignal): Promise<AckResponse> {
    const data = AckRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'Ack',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) => AckResponse.decode(_m0.Reader.create(data)))
  }

  Push(request: PushRequest, abortSignal?: AbortSignal): Promise<PushResponse> {
    const data = PushRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'Push',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) => PushResponse.decode(_m0.Reader.create(data)))
  }

  Wait(request: WaitRequest, abortSignal?: AbortSignal): Promise<WaitResponse> {
    const data = WaitRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'Wait',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) => WaitResponse.decode(_m0.Reader.create(data)))
  }

  DeleteQueue(
    request: DeleteQueueRequest,
    abortSignal?: AbortSignal,
  ): Promise<DeleteQueueResponse> {
    const data = DeleteQueueRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'DeleteQueue',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      DeleteQueueResponse.decode(_m0.Reader.create(data)),
    )
  }
}

/**
 * QueueOps exposes a Queue object with a service.
 * Wraps the mqueue.Queue interface.
 */
export type QueueOpsDefinition = typeof QueueOpsDefinition
export const QueueOpsDefinition = {
  name: 'QueueOps',
  fullName: 'mqueue.rpc.QueueOps',
  methods: {
    /** Peek returns the next message, if any. */
    peek: {
      name: 'Peek',
      requestType: PeekRequest,
      requestStream: false,
      responseType: PeekResponse,
      responseStream: false,
      options: {},
    },
    /**
     * Ack acknowledges the message with the given ID.
     * If the latest message is not the one with the ID, does nothing.
     */
    ack: {
      name: 'Ack',
      requestType: AckRequest,
      requestStream: false,
      responseType: AckResponse,
      responseStream: false,
      options: {},
    },
    /** Push pushes a message to the queue. */
    push: {
      name: 'Push',
      requestType: PushRequest,
      requestStream: false,
      responseType: PushResponse,
      responseStream: false,
      options: {},
    },
    /** Wait waits for the next message, or call cancellation. */
    wait: {
      name: 'Wait',
      requestType: WaitRequest,
      requestStream: false,
      responseType: WaitResponse,
      responseStream: false,
      options: {},
    },
    /** DeleteQueue deletes the messages and metadata for the queue. */
    deleteQueue: {
      name: 'DeleteQueue',
      requestType: DeleteQueueRequest,
      requestStream: false,
      responseType: DeleteQueueResponse,
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
  if (globalThis.Buffer) {
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
  if (globalThis.Buffer) {
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

function toTimestamp(date: Date): Timestamp {
  const seconds = numberToLong(Math.trunc(date.getTime() / 1_000))
  const nanos = (date.getTime() % 1_000) * 1_000_000
  return { seconds, nanos }
}

function fromTimestamp(t: Timestamp): Date {
  let millis = (t.seconds.toNumber() || 0) * 1_000
  millis += (t.nanos || 0) / 1_000_000
  return new globalThis.Date(millis)
}

function fromJsonTimestamp(o: any): Date {
  if (o instanceof globalThis.Date) {
    return o
  } else if (typeof o === 'string') {
    return new globalThis.Date(o)
  } else {
    return fromTimestamp(Timestamp.fromJSON(o))
  }
}

function numberToLong(number: number) {
  return Long.fromNumber(number)
}

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
