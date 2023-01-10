/* eslint-disable */
import { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'kvtx.rpc'

/** KvtxTransactionRequest is a request in a KvtxTransaction rpc. */
export interface KvtxTransactionRequest {
  body?:
    | { $case: 'init'; init: KvtxTransactionInit }
    | { $case: 'commit'; commit: boolean }
    | {
        $case: 'discard'
        discard: boolean
      }
}

/** KvtxTransactionInit is the message sent to init a kvtx transaction. */
export interface KvtxTransactionInit {
  /**
   * Write indicates if this should be a write transaction.
   * If unset, the store will be read-only.
   */
  write: boolean
}

/** KvtxTransactionResponse is a response to the KvtxTransaction rpc. */
export interface KvtxTransactionResponse {
  body?:
    | { $case: 'ack'; ack: KvtxTransactionAck }
    | { $case: 'complete'; complete: KvtxTransactionComplete }
}

/** KvtxTransactionAck contains information about opening the transaction. */
export interface KvtxTransactionAck {
  /** Error is any error opening the transaction. */
  error: string
  /**
   * TransactionId is the identifier to use for the RpcStream.
   * If error != "" this will be empty.
   */
  transactionId: string
}

/** KvtxTransactionComplete contains information about the result of a tx. */
export interface KvtxTransactionComplete {
  /**
   * Error is any error completing the transaction.
   * If this is set, usually discarded=true as well.
   */
  error: string
  /**
   * Committed indicates we successfully committed the transaction.
   * If error != "" this will be false.
   */
  committed: boolean
  /**
   * Discarded indicates the transaction is/was discarded.
   * No changes will be applied.
   */
  discarded: boolean
}

/** KeyCountRequest is a request for the number of keys in the store. */
export interface KeyCountRequest {}

/** KeyCountResponse is a response to the KeyCountRequest. */
export interface KeyCountResponse {
  /** KeyCount is the number of keys in the store. */
  keyCount: Long
}

/** KvtxKeyRequest is a request that accepts a single key as a parameter. */
export interface KvtxKeyRequest {
  /** Key is the key to lookup. */
  key: Uint8Array
}

/** KvtxKeyDataResponse responds to a request for data from a KvtxOps store. */
export interface KvtxKeyDataResponse {
  /**
   * Error is any error accessing the key.
   * Will be empty if the key was unset.
   */
  error: string
  /** Found indicates the key was found. */
  found: boolean
  /** Data contains the data, if found=true. */
  data: Uint8Array
}

/** KvtxKeyExistsResponse responds to a request to check if a key exists. */
export interface KvtxKeyExistsResponse {
  /**
   * Error is any error accessing the key.
   * Will be empty if the key was unset.
   */
  error: string
  /** Found indicates the key was found. */
  found: boolean
}

/** KvtxSetKeyRequest is a request to set a key to a value. */
export interface KvtxSetKeyRequest {
  /** Key is the key to set. */
  key: Uint8Array
  /** Value is the value to set it to. */
  value: Uint8Array
}

/** KvtxSetKeyResponse is the response to setting a key to a value. */
export interface KvtxSetKeyResponse {
  /**
   * Error is any error setting the key.
   * If empty, the operation succeeded.
   */
  error: string
}

/** KvtxDeleteKeyRequest is a request to delete a key from the store. */
export interface KvtxDeleteKeyRequest {
  /** Key is the key to delete. */
  key: Uint8Array
}

/** KvtxDeleteKeyResponse is the response to deleting a key from the store. */
export interface KvtxDeleteKeyResponse {
  /**
   * Error is any error removing the key.
   * If empty, the operation succeeded.
   */
  error: string
}

/** KvtxScanPrefixRequest is a request to scan for key/value pairs by prefix. */
export interface KvtxScanPrefixRequest {
  /**
   * Prefix is the key prefix to scan.
   * If empty, returns all key/value pairs.
   */
  prefix: Uint8Array
  /** OnlyKeys looks up the keys, without the values. */
  onlyKeys: boolean
}

/** KvtxScanPrefixResponse is the response to deleting a key from the store. */
export interface KvtxScanPrefixResponse {
  /**
   * Error is any error scanning the key/value pairs.
   * If set, this is the final message in the stream.
   */
  error: string
  /** Key is the key for this key/value pair. */
  key: Uint8Array
  /** Value is the value for this key/value pair. */
  value: Uint8Array
}

/** KvtxIterateRequest is a request to open an iterator on a kvtx store. */
export interface KvtxIterateRequest {
  body?:
    | { $case: 'init'; init: KvtxIterateInit }
    | { $case: 'lookupValue'; lookupValue: boolean }
    | { $case: 'next'; next: boolean }
    | { $case: 'seek'; seek: Uint8Array }
    | { $case: 'seekBeginning'; seekBeginning: boolean }
    | { $case: 'close'; close: boolean }
}

/** KvtxIterateInit are the arguments for initializing a iterator. */
export interface KvtxIterateInit {
  /** Prefix is the key prefix to filter for. */
  prefix: Uint8Array
  /** Sort sorts the results by key. */
  sort: boolean
  /** Reverse reverses the order of iteration. */
  reverse: boolean
}

/** KvtxIterateResponse is a response to an iterate request message. */
export interface KvtxIterateResponse {
  body?:
    | { $case: 'ack'; ack: boolean }
    | { $case: 'reqError'; reqError: string }
    | { $case: 'status'; status: KvtxIterateStatus }
    | { $case: 'value'; value: Uint8Array }
    | { $case: 'closed'; closed: boolean }
}

/** KvtxIterateStatus contains an update to the iterator status. */
export interface KvtxIterateStatus {
  /** Error indicates the iterator is released with an error. */
  error: string
  /** Valid indicates the iterator points to a valid entry. */
  valid: boolean
  /**
   * Key is the current entry key.
   * If len(key) == 0, no change.
   */
  key: Uint8Array
}

function createBaseKvtxTransactionRequest(): KvtxTransactionRequest {
  return { body: undefined }
}

export const KvtxTransactionRequest = {
  encode(
    message: KvtxTransactionRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.body?.$case === 'init') {
      KvtxTransactionInit.encode(
        message.body.init,
        writer.uint32(10).fork()
      ).ldelim()
    }
    if (message.body?.$case === 'commit') {
      writer.uint32(16).bool(message.body.commit)
    }
    if (message.body?.$case === 'discard') {
      writer.uint32(24).bool(message.body.discard)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): KvtxTransactionRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxTransactionRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.body = {
            $case: 'init',
            init: KvtxTransactionInit.decode(reader, reader.uint32()),
          }
          break
        case 2:
          message.body = { $case: 'commit', commit: reader.bool() }
          break
        case 3:
          message.body = { $case: 'discard', discard: reader.bool() }
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionRequest | KvtxTransactionRequest[]>
      | Iterable<KvtxTransactionRequest | KvtxTransactionRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionRequest.encode(p).finish()]
        }
      } else {
        yield* [KvtxTransactionRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxTransactionRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionRequest.decode(p)]
        }
      } else {
        yield* [KvtxTransactionRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxTransactionRequest {
    return {
      body: isSet(object.init)
        ? { $case: 'init', init: KvtxTransactionInit.fromJSON(object.init) }
        : isSet(object.commit)
        ? { $case: 'commit', commit: Boolean(object.commit) }
        : isSet(object.discard)
        ? { $case: 'discard', discard: Boolean(object.discard) }
        : undefined,
    }
  },

  toJSON(message: KvtxTransactionRequest): unknown {
    const obj: any = {}
    message.body?.$case === 'init' &&
      (obj.init = message.body?.init
        ? KvtxTransactionInit.toJSON(message.body?.init)
        : undefined)
    message.body?.$case === 'commit' && (obj.commit = message.body?.commit)
    message.body?.$case === 'discard' && (obj.discard = message.body?.discard)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxTransactionRequest>, I>>(
    base?: I
  ): KvtxTransactionRequest {
    return KvtxTransactionRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionRequest>, I>>(
    object: I
  ): KvtxTransactionRequest {
    const message = createBaseKvtxTransactionRequest()
    if (
      object.body?.$case === 'init' &&
      object.body?.init !== undefined &&
      object.body?.init !== null
    ) {
      message.body = {
        $case: 'init',
        init: KvtxTransactionInit.fromPartial(object.body.init),
      }
    }
    if (
      object.body?.$case === 'commit' &&
      object.body?.commit !== undefined &&
      object.body?.commit !== null
    ) {
      message.body = { $case: 'commit', commit: object.body.commit }
    }
    if (
      object.body?.$case === 'discard' &&
      object.body?.discard !== undefined &&
      object.body?.discard !== null
    ) {
      message.body = { $case: 'discard', discard: object.body.discard }
    }
    return message
  },
}

function createBaseKvtxTransactionInit(): KvtxTransactionInit {
  return { write: false }
}

export const KvtxTransactionInit = {
  encode(
    message: KvtxTransactionInit,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.write === true) {
      writer.uint32(8).bool(message.write)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxTransactionInit {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxTransactionInit()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.write = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionInit, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionInit | KvtxTransactionInit[]>
      | Iterable<KvtxTransactionInit | KvtxTransactionInit[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionInit.encode(p).finish()]
        }
      } else {
        yield* [KvtxTransactionInit.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionInit>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxTransactionInit> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionInit.decode(p)]
        }
      } else {
        yield* [KvtxTransactionInit.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxTransactionInit {
    return { write: isSet(object.write) ? Boolean(object.write) : false }
  },

  toJSON(message: KvtxTransactionInit): unknown {
    const obj: any = {}
    message.write !== undefined && (obj.write = message.write)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxTransactionInit>, I>>(
    base?: I
  ): KvtxTransactionInit {
    return KvtxTransactionInit.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionInit>, I>>(
    object: I
  ): KvtxTransactionInit {
    const message = createBaseKvtxTransactionInit()
    message.write = object.write ?? false
    return message
  },
}

function createBaseKvtxTransactionResponse(): KvtxTransactionResponse {
  return { body: undefined }
}

export const KvtxTransactionResponse = {
  encode(
    message: KvtxTransactionResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.body?.$case === 'ack') {
      KvtxTransactionAck.encode(
        message.body.ack,
        writer.uint32(10).fork()
      ).ldelim()
    }
    if (message.body?.$case === 'complete') {
      KvtxTransactionComplete.encode(
        message.body.complete,
        writer.uint32(18).fork()
      ).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): KvtxTransactionResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxTransactionResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.body = {
            $case: 'ack',
            ack: KvtxTransactionAck.decode(reader, reader.uint32()),
          }
          break
        case 2:
          message.body = {
            $case: 'complete',
            complete: KvtxTransactionComplete.decode(reader, reader.uint32()),
          }
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionResponse | KvtxTransactionResponse[]>
      | Iterable<KvtxTransactionResponse | KvtxTransactionResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionResponse.encode(p).finish()]
        }
      } else {
        yield* [KvtxTransactionResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxTransactionResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionResponse.decode(p)]
        }
      } else {
        yield* [KvtxTransactionResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxTransactionResponse {
    return {
      body: isSet(object.ack)
        ? { $case: 'ack', ack: KvtxTransactionAck.fromJSON(object.ack) }
        : isSet(object.complete)
        ? {
            $case: 'complete',
            complete: KvtxTransactionComplete.fromJSON(object.complete),
          }
        : undefined,
    }
  },

  toJSON(message: KvtxTransactionResponse): unknown {
    const obj: any = {}
    message.body?.$case === 'ack' &&
      (obj.ack = message.body?.ack
        ? KvtxTransactionAck.toJSON(message.body?.ack)
        : undefined)
    message.body?.$case === 'complete' &&
      (obj.complete = message.body?.complete
        ? KvtxTransactionComplete.toJSON(message.body?.complete)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxTransactionResponse>, I>>(
    base?: I
  ): KvtxTransactionResponse {
    return KvtxTransactionResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionResponse>, I>>(
    object: I
  ): KvtxTransactionResponse {
    const message = createBaseKvtxTransactionResponse()
    if (
      object.body?.$case === 'ack' &&
      object.body?.ack !== undefined &&
      object.body?.ack !== null
    ) {
      message.body = {
        $case: 'ack',
        ack: KvtxTransactionAck.fromPartial(object.body.ack),
      }
    }
    if (
      object.body?.$case === 'complete' &&
      object.body?.complete !== undefined &&
      object.body?.complete !== null
    ) {
      message.body = {
        $case: 'complete',
        complete: KvtxTransactionComplete.fromPartial(object.body.complete),
      }
    }
    return message
  },
}

function createBaseKvtxTransactionAck(): KvtxTransactionAck {
  return { error: '', transactionId: '' }
}

export const KvtxTransactionAck = {
  encode(
    message: KvtxTransactionAck,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    if (message.transactionId !== '') {
      writer.uint32(18).string(message.transactionId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxTransactionAck {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxTransactionAck()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string()
          break
        case 2:
          message.transactionId = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionAck, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionAck | KvtxTransactionAck[]>
      | Iterable<KvtxTransactionAck | KvtxTransactionAck[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionAck.encode(p).finish()]
        }
      } else {
        yield* [KvtxTransactionAck.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionAck>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxTransactionAck> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionAck.decode(p)]
        }
      } else {
        yield* [KvtxTransactionAck.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxTransactionAck {
    return {
      error: isSet(object.error) ? String(object.error) : '',
      transactionId: isSet(object.transactionId)
        ? String(object.transactionId)
        : '',
    }
  },

  toJSON(message: KvtxTransactionAck): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    message.transactionId !== undefined &&
      (obj.transactionId = message.transactionId)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxTransactionAck>, I>>(
    base?: I
  ): KvtxTransactionAck {
    return KvtxTransactionAck.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionAck>, I>>(
    object: I
  ): KvtxTransactionAck {
    const message = createBaseKvtxTransactionAck()
    message.error = object.error ?? ''
    message.transactionId = object.transactionId ?? ''
    return message
  },
}

function createBaseKvtxTransactionComplete(): KvtxTransactionComplete {
  return { error: '', committed: false, discarded: false }
}

export const KvtxTransactionComplete = {
  encode(
    message: KvtxTransactionComplete,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    if (message.committed === true) {
      writer.uint32(16).bool(message.committed)
    }
    if (message.discarded === true) {
      writer.uint32(24).bool(message.discarded)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): KvtxTransactionComplete {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxTransactionComplete()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string()
          break
        case 2:
          message.committed = reader.bool()
          break
        case 3:
          message.discarded = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionComplete, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionComplete | KvtxTransactionComplete[]>
      | Iterable<KvtxTransactionComplete | KvtxTransactionComplete[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionComplete.encode(p).finish()]
        }
      } else {
        yield* [KvtxTransactionComplete.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionComplete>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxTransactionComplete> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionComplete.decode(p)]
        }
      } else {
        yield* [KvtxTransactionComplete.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxTransactionComplete {
    return {
      error: isSet(object.error) ? String(object.error) : '',
      committed: isSet(object.committed) ? Boolean(object.committed) : false,
      discarded: isSet(object.discarded) ? Boolean(object.discarded) : false,
    }
  },

  toJSON(message: KvtxTransactionComplete): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    message.committed !== undefined && (obj.committed = message.committed)
    message.discarded !== undefined && (obj.discarded = message.discarded)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxTransactionComplete>, I>>(
    base?: I
  ): KvtxTransactionComplete {
    return KvtxTransactionComplete.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionComplete>, I>>(
    object: I
  ): KvtxTransactionComplete {
    const message = createBaseKvtxTransactionComplete()
    message.error = object.error ?? ''
    message.committed = object.committed ?? false
    message.discarded = object.discarded ?? false
    return message
  },
}

function createBaseKeyCountRequest(): KeyCountRequest {
  return {}
}

export const KeyCountRequest = {
  encode(
    _: KeyCountRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KeyCountRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKeyCountRequest()
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
  // Transform<KeyCountRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KeyCountRequest | KeyCountRequest[]>
      | Iterable<KeyCountRequest | KeyCountRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyCountRequest.encode(p).finish()]
        }
      } else {
        yield* [KeyCountRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KeyCountRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KeyCountRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyCountRequest.decode(p)]
        }
      } else {
        yield* [KeyCountRequest.decode(pkt)]
      }
    }
  },

  fromJSON(_: any): KeyCountRequest {
    return {}
  },

  toJSON(_: KeyCountRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<KeyCountRequest>, I>>(
    base?: I
  ): KeyCountRequest {
    return KeyCountRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KeyCountRequest>, I>>(
    _: I
  ): KeyCountRequest {
    const message = createBaseKeyCountRequest()
    return message
  },
}

function createBaseKeyCountResponse(): KeyCountResponse {
  return { keyCount: Long.UZERO }
}

export const KeyCountResponse = {
  encode(
    message: KeyCountResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (!message.keyCount.isZero()) {
      writer.uint32(8).uint64(message.keyCount)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KeyCountResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKeyCountResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.keyCount = reader.uint64() as Long
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KeyCountResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KeyCountResponse | KeyCountResponse[]>
      | Iterable<KeyCountResponse | KeyCountResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyCountResponse.encode(p).finish()]
        }
      } else {
        yield* [KeyCountResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KeyCountResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KeyCountResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyCountResponse.decode(p)]
        }
      } else {
        yield* [KeyCountResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KeyCountResponse {
    return {
      keyCount: isSet(object.keyCount)
        ? Long.fromValue(object.keyCount)
        : Long.UZERO,
    }
  },

  toJSON(message: KeyCountResponse): unknown {
    const obj: any = {}
    message.keyCount !== undefined &&
      (obj.keyCount = (message.keyCount || Long.UZERO).toString())
    return obj
  },

  create<I extends Exact<DeepPartial<KeyCountResponse>, I>>(
    base?: I
  ): KeyCountResponse {
    return KeyCountResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KeyCountResponse>, I>>(
    object: I
  ): KeyCountResponse {
    const message = createBaseKeyCountResponse()
    message.keyCount =
      object.keyCount !== undefined && object.keyCount !== null
        ? Long.fromValue(object.keyCount)
        : Long.UZERO
    return message
  },
}

function createBaseKvtxKeyRequest(): KvtxKeyRequest {
  return { key: new Uint8Array() }
}

export const KvtxKeyRequest = {
  encode(
    message: KvtxKeyRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.key.length !== 0) {
      writer.uint32(10).bytes(message.key)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxKeyRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxKeyRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.key = reader.bytes()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxKeyRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxKeyRequest | KvtxKeyRequest[]>
      | Iterable<KvtxKeyRequest | KvtxKeyRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyRequest.encode(p).finish()]
        }
      } else {
        yield* [KvtxKeyRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxKeyRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxKeyRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyRequest.decode(p)]
        }
      } else {
        yield* [KvtxKeyRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxKeyRequest {
    return {
      key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array(),
    }
  },

  toJSON(message: KvtxKeyRequest): unknown {
    const obj: any = {}
    message.key !== undefined &&
      (obj.key = base64FromBytes(
        message.key !== undefined ? message.key : new Uint8Array()
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxKeyRequest>, I>>(
    base?: I
  ): KvtxKeyRequest {
    return KvtxKeyRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxKeyRequest>, I>>(
    object: I
  ): KvtxKeyRequest {
    const message = createBaseKvtxKeyRequest()
    message.key = object.key ?? new Uint8Array()
    return message
  },
}

function createBaseKvtxKeyDataResponse(): KvtxKeyDataResponse {
  return { error: '', found: false, data: new Uint8Array() }
}

export const KvtxKeyDataResponse = {
  encode(
    message: KvtxKeyDataResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    if (message.found === true) {
      writer.uint32(16).bool(message.found)
    }
    if (message.data.length !== 0) {
      writer.uint32(26).bytes(message.data)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxKeyDataResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxKeyDataResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string()
          break
        case 2:
          message.found = reader.bool()
          break
        case 3:
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
  // Transform<KvtxKeyDataResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxKeyDataResponse | KvtxKeyDataResponse[]>
      | Iterable<KvtxKeyDataResponse | KvtxKeyDataResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyDataResponse.encode(p).finish()]
        }
      } else {
        yield* [KvtxKeyDataResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxKeyDataResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxKeyDataResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyDataResponse.decode(p)]
        }
      } else {
        yield* [KvtxKeyDataResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxKeyDataResponse {
    return {
      error: isSet(object.error) ? String(object.error) : '',
      found: isSet(object.found) ? Boolean(object.found) : false,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(),
    }
  },

  toJSON(message: KvtxKeyDataResponse): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    message.found !== undefined && (obj.found = message.found)
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxKeyDataResponse>, I>>(
    base?: I
  ): KvtxKeyDataResponse {
    return KvtxKeyDataResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxKeyDataResponse>, I>>(
    object: I
  ): KvtxKeyDataResponse {
    const message = createBaseKvtxKeyDataResponse()
    message.error = object.error ?? ''
    message.found = object.found ?? false
    message.data = object.data ?? new Uint8Array()
    return message
  },
}

function createBaseKvtxKeyExistsResponse(): KvtxKeyExistsResponse {
  return { error: '', found: false }
}

export const KvtxKeyExistsResponse = {
  encode(
    message: KvtxKeyExistsResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    if (message.found === true) {
      writer.uint32(16).bool(message.found)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): KvtxKeyExistsResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxKeyExistsResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string()
          break
        case 2:
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
  // Transform<KvtxKeyExistsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxKeyExistsResponse | KvtxKeyExistsResponse[]>
      | Iterable<KvtxKeyExistsResponse | KvtxKeyExistsResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyExistsResponse.encode(p).finish()]
        }
      } else {
        yield* [KvtxKeyExistsResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxKeyExistsResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxKeyExistsResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyExistsResponse.decode(p)]
        }
      } else {
        yield* [KvtxKeyExistsResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxKeyExistsResponse {
    return {
      error: isSet(object.error) ? String(object.error) : '',
      found: isSet(object.found) ? Boolean(object.found) : false,
    }
  },

  toJSON(message: KvtxKeyExistsResponse): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    message.found !== undefined && (obj.found = message.found)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxKeyExistsResponse>, I>>(
    base?: I
  ): KvtxKeyExistsResponse {
    return KvtxKeyExistsResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxKeyExistsResponse>, I>>(
    object: I
  ): KvtxKeyExistsResponse {
    const message = createBaseKvtxKeyExistsResponse()
    message.error = object.error ?? ''
    message.found = object.found ?? false
    return message
  },
}

function createBaseKvtxSetKeyRequest(): KvtxSetKeyRequest {
  return { key: new Uint8Array(), value: new Uint8Array() }
}

export const KvtxSetKeyRequest = {
  encode(
    message: KvtxSetKeyRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.key.length !== 0) {
      writer.uint32(10).bytes(message.key)
    }
    if (message.value.length !== 0) {
      writer.uint32(18).bytes(message.value)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxSetKeyRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxSetKeyRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.key = reader.bytes()
          break
        case 2:
          message.value = reader.bytes()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxSetKeyRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxSetKeyRequest | KvtxSetKeyRequest[]>
      | Iterable<KvtxSetKeyRequest | KvtxSetKeyRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxSetKeyRequest.encode(p).finish()]
        }
      } else {
        yield* [KvtxSetKeyRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxSetKeyRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxSetKeyRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxSetKeyRequest.decode(p)]
        }
      } else {
        yield* [KvtxSetKeyRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxSetKeyRequest {
    return {
      key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array(),
      value: isSet(object.value)
        ? bytesFromBase64(object.value)
        : new Uint8Array(),
    }
  },

  toJSON(message: KvtxSetKeyRequest): unknown {
    const obj: any = {}
    message.key !== undefined &&
      (obj.key = base64FromBytes(
        message.key !== undefined ? message.key : new Uint8Array()
      ))
    message.value !== undefined &&
      (obj.value = base64FromBytes(
        message.value !== undefined ? message.value : new Uint8Array()
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxSetKeyRequest>, I>>(
    base?: I
  ): KvtxSetKeyRequest {
    return KvtxSetKeyRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxSetKeyRequest>, I>>(
    object: I
  ): KvtxSetKeyRequest {
    const message = createBaseKvtxSetKeyRequest()
    message.key = object.key ?? new Uint8Array()
    message.value = object.value ?? new Uint8Array()
    return message
  },
}

function createBaseKvtxSetKeyResponse(): KvtxSetKeyResponse {
  return { error: '' }
}

export const KvtxSetKeyResponse = {
  encode(
    message: KvtxSetKeyResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxSetKeyResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxSetKeyResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxSetKeyResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxSetKeyResponse | KvtxSetKeyResponse[]>
      | Iterable<KvtxSetKeyResponse | KvtxSetKeyResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxSetKeyResponse.encode(p).finish()]
        }
      } else {
        yield* [KvtxSetKeyResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxSetKeyResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxSetKeyResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxSetKeyResponse.decode(p)]
        }
      } else {
        yield* [KvtxSetKeyResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxSetKeyResponse {
    return { error: isSet(object.error) ? String(object.error) : '' }
  },

  toJSON(message: KvtxSetKeyResponse): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxSetKeyResponse>, I>>(
    base?: I
  ): KvtxSetKeyResponse {
    return KvtxSetKeyResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxSetKeyResponse>, I>>(
    object: I
  ): KvtxSetKeyResponse {
    const message = createBaseKvtxSetKeyResponse()
    message.error = object.error ?? ''
    return message
  },
}

function createBaseKvtxDeleteKeyRequest(): KvtxDeleteKeyRequest {
  return { key: new Uint8Array() }
}

export const KvtxDeleteKeyRequest = {
  encode(
    message: KvtxDeleteKeyRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.key.length !== 0) {
      writer.uint32(10).bytes(message.key)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): KvtxDeleteKeyRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxDeleteKeyRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.key = reader.bytes()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxDeleteKeyRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxDeleteKeyRequest | KvtxDeleteKeyRequest[]>
      | Iterable<KvtxDeleteKeyRequest | KvtxDeleteKeyRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxDeleteKeyRequest.encode(p).finish()]
        }
      } else {
        yield* [KvtxDeleteKeyRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxDeleteKeyRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxDeleteKeyRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxDeleteKeyRequest.decode(p)]
        }
      } else {
        yield* [KvtxDeleteKeyRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxDeleteKeyRequest {
    return {
      key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array(),
    }
  },

  toJSON(message: KvtxDeleteKeyRequest): unknown {
    const obj: any = {}
    message.key !== undefined &&
      (obj.key = base64FromBytes(
        message.key !== undefined ? message.key : new Uint8Array()
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxDeleteKeyRequest>, I>>(
    base?: I
  ): KvtxDeleteKeyRequest {
    return KvtxDeleteKeyRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxDeleteKeyRequest>, I>>(
    object: I
  ): KvtxDeleteKeyRequest {
    const message = createBaseKvtxDeleteKeyRequest()
    message.key = object.key ?? new Uint8Array()
    return message
  },
}

function createBaseKvtxDeleteKeyResponse(): KvtxDeleteKeyResponse {
  return { error: '' }
}

export const KvtxDeleteKeyResponse = {
  encode(
    message: KvtxDeleteKeyResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): KvtxDeleteKeyResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxDeleteKeyResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxDeleteKeyResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxDeleteKeyResponse | KvtxDeleteKeyResponse[]>
      | Iterable<KvtxDeleteKeyResponse | KvtxDeleteKeyResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxDeleteKeyResponse.encode(p).finish()]
        }
      } else {
        yield* [KvtxDeleteKeyResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxDeleteKeyResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxDeleteKeyResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxDeleteKeyResponse.decode(p)]
        }
      } else {
        yield* [KvtxDeleteKeyResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxDeleteKeyResponse {
    return { error: isSet(object.error) ? String(object.error) : '' }
  },

  toJSON(message: KvtxDeleteKeyResponse): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxDeleteKeyResponse>, I>>(
    base?: I
  ): KvtxDeleteKeyResponse {
    return KvtxDeleteKeyResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxDeleteKeyResponse>, I>>(
    object: I
  ): KvtxDeleteKeyResponse {
    const message = createBaseKvtxDeleteKeyResponse()
    message.error = object.error ?? ''
    return message
  },
}

function createBaseKvtxScanPrefixRequest(): KvtxScanPrefixRequest {
  return { prefix: new Uint8Array(), onlyKeys: false }
}

export const KvtxScanPrefixRequest = {
  encode(
    message: KvtxScanPrefixRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.prefix.length !== 0) {
      writer.uint32(10).bytes(message.prefix)
    }
    if (message.onlyKeys === true) {
      writer.uint32(16).bool(message.onlyKeys)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): KvtxScanPrefixRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxScanPrefixRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.prefix = reader.bytes()
          break
        case 2:
          message.onlyKeys = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxScanPrefixRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxScanPrefixRequest | KvtxScanPrefixRequest[]>
      | Iterable<KvtxScanPrefixRequest | KvtxScanPrefixRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxScanPrefixRequest.encode(p).finish()]
        }
      } else {
        yield* [KvtxScanPrefixRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxScanPrefixRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxScanPrefixRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxScanPrefixRequest.decode(p)]
        }
      } else {
        yield* [KvtxScanPrefixRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxScanPrefixRequest {
    return {
      prefix: isSet(object.prefix)
        ? bytesFromBase64(object.prefix)
        : new Uint8Array(),
      onlyKeys: isSet(object.onlyKeys) ? Boolean(object.onlyKeys) : false,
    }
  },

  toJSON(message: KvtxScanPrefixRequest): unknown {
    const obj: any = {}
    message.prefix !== undefined &&
      (obj.prefix = base64FromBytes(
        message.prefix !== undefined ? message.prefix : new Uint8Array()
      ))
    message.onlyKeys !== undefined && (obj.onlyKeys = message.onlyKeys)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxScanPrefixRequest>, I>>(
    base?: I
  ): KvtxScanPrefixRequest {
    return KvtxScanPrefixRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxScanPrefixRequest>, I>>(
    object: I
  ): KvtxScanPrefixRequest {
    const message = createBaseKvtxScanPrefixRequest()
    message.prefix = object.prefix ?? new Uint8Array()
    message.onlyKeys = object.onlyKeys ?? false
    return message
  },
}

function createBaseKvtxScanPrefixResponse(): KvtxScanPrefixResponse {
  return { error: '', key: new Uint8Array(), value: new Uint8Array() }
}

export const KvtxScanPrefixResponse = {
  encode(
    message: KvtxScanPrefixResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    if (message.key.length !== 0) {
      writer.uint32(18).bytes(message.key)
    }
    if (message.value.length !== 0) {
      writer.uint32(26).bytes(message.value)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): KvtxScanPrefixResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxScanPrefixResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string()
          break
        case 2:
          message.key = reader.bytes()
          break
        case 3:
          message.value = reader.bytes()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxScanPrefixResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxScanPrefixResponse | KvtxScanPrefixResponse[]>
      | Iterable<KvtxScanPrefixResponse | KvtxScanPrefixResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxScanPrefixResponse.encode(p).finish()]
        }
      } else {
        yield* [KvtxScanPrefixResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxScanPrefixResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxScanPrefixResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxScanPrefixResponse.decode(p)]
        }
      } else {
        yield* [KvtxScanPrefixResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxScanPrefixResponse {
    return {
      error: isSet(object.error) ? String(object.error) : '',
      key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array(),
      value: isSet(object.value)
        ? bytesFromBase64(object.value)
        : new Uint8Array(),
    }
  },

  toJSON(message: KvtxScanPrefixResponse): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    message.key !== undefined &&
      (obj.key = base64FromBytes(
        message.key !== undefined ? message.key : new Uint8Array()
      ))
    message.value !== undefined &&
      (obj.value = base64FromBytes(
        message.value !== undefined ? message.value : new Uint8Array()
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxScanPrefixResponse>, I>>(
    base?: I
  ): KvtxScanPrefixResponse {
    return KvtxScanPrefixResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxScanPrefixResponse>, I>>(
    object: I
  ): KvtxScanPrefixResponse {
    const message = createBaseKvtxScanPrefixResponse()
    message.error = object.error ?? ''
    message.key = object.key ?? new Uint8Array()
    message.value = object.value ?? new Uint8Array()
    return message
  },
}

function createBaseKvtxIterateRequest(): KvtxIterateRequest {
  return { body: undefined }
}

export const KvtxIterateRequest = {
  encode(
    message: KvtxIterateRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.body?.$case === 'init') {
      KvtxIterateInit.encode(
        message.body.init,
        writer.uint32(10).fork()
      ).ldelim()
    }
    if (message.body?.$case === 'lookupValue') {
      writer.uint32(16).bool(message.body.lookupValue)
    }
    if (message.body?.$case === 'next') {
      writer.uint32(24).bool(message.body.next)
    }
    if (message.body?.$case === 'seek') {
      writer.uint32(34).bytes(message.body.seek)
    }
    if (message.body?.$case === 'seekBeginning') {
      writer.uint32(40).bool(message.body.seekBeginning)
    }
    if (message.body?.$case === 'close') {
      writer.uint32(48).bool(message.body.close)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxIterateRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxIterateRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.body = {
            $case: 'init',
            init: KvtxIterateInit.decode(reader, reader.uint32()),
          }
          break
        case 2:
          message.body = { $case: 'lookupValue', lookupValue: reader.bool() }
          break
        case 3:
          message.body = { $case: 'next', next: reader.bool() }
          break
        case 4:
          message.body = { $case: 'seek', seek: reader.bytes() }
          break
        case 5:
          message.body = {
            $case: 'seekBeginning',
            seekBeginning: reader.bool(),
          }
          break
        case 6:
          message.body = { $case: 'close', close: reader.bool() }
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxIterateRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxIterateRequest | KvtxIterateRequest[]>
      | Iterable<KvtxIterateRequest | KvtxIterateRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxIterateRequest.encode(p).finish()]
        }
      } else {
        yield* [KvtxIterateRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxIterateRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxIterateRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxIterateRequest.decode(p)]
        }
      } else {
        yield* [KvtxIterateRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxIterateRequest {
    return {
      body: isSet(object.init)
        ? { $case: 'init', init: KvtxIterateInit.fromJSON(object.init) }
        : isSet(object.lookupValue)
        ? { $case: 'lookupValue', lookupValue: Boolean(object.lookupValue) }
        : isSet(object.next)
        ? { $case: 'next', next: Boolean(object.next) }
        : isSet(object.seek)
        ? { $case: 'seek', seek: bytesFromBase64(object.seek) }
        : isSet(object.seekBeginning)
        ? {
            $case: 'seekBeginning',
            seekBeginning: Boolean(object.seekBeginning),
          }
        : isSet(object.close)
        ? { $case: 'close', close: Boolean(object.close) }
        : undefined,
    }
  },

  toJSON(message: KvtxIterateRequest): unknown {
    const obj: any = {}
    message.body?.$case === 'init' &&
      (obj.init = message.body?.init
        ? KvtxIterateInit.toJSON(message.body?.init)
        : undefined)
    message.body?.$case === 'lookupValue' &&
      (obj.lookupValue = message.body?.lookupValue)
    message.body?.$case === 'next' && (obj.next = message.body?.next)
    message.body?.$case === 'seek' &&
      (obj.seek =
        message.body?.seek !== undefined
          ? base64FromBytes(message.body?.seek)
          : undefined)
    message.body?.$case === 'seekBeginning' &&
      (obj.seekBeginning = message.body?.seekBeginning)
    message.body?.$case === 'close' && (obj.close = message.body?.close)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxIterateRequest>, I>>(
    base?: I
  ): KvtxIterateRequest {
    return KvtxIterateRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxIterateRequest>, I>>(
    object: I
  ): KvtxIterateRequest {
    const message = createBaseKvtxIterateRequest()
    if (
      object.body?.$case === 'init' &&
      object.body?.init !== undefined &&
      object.body?.init !== null
    ) {
      message.body = {
        $case: 'init',
        init: KvtxIterateInit.fromPartial(object.body.init),
      }
    }
    if (
      object.body?.$case === 'lookupValue' &&
      object.body?.lookupValue !== undefined &&
      object.body?.lookupValue !== null
    ) {
      message.body = {
        $case: 'lookupValue',
        lookupValue: object.body.lookupValue,
      }
    }
    if (
      object.body?.$case === 'next' &&
      object.body?.next !== undefined &&
      object.body?.next !== null
    ) {
      message.body = { $case: 'next', next: object.body.next }
    }
    if (
      object.body?.$case === 'seek' &&
      object.body?.seek !== undefined &&
      object.body?.seek !== null
    ) {
      message.body = { $case: 'seek', seek: object.body.seek }
    }
    if (
      object.body?.$case === 'seekBeginning' &&
      object.body?.seekBeginning !== undefined &&
      object.body?.seekBeginning !== null
    ) {
      message.body = {
        $case: 'seekBeginning',
        seekBeginning: object.body.seekBeginning,
      }
    }
    if (
      object.body?.$case === 'close' &&
      object.body?.close !== undefined &&
      object.body?.close !== null
    ) {
      message.body = { $case: 'close', close: object.body.close }
    }
    return message
  },
}

function createBaseKvtxIterateInit(): KvtxIterateInit {
  return { prefix: new Uint8Array(), sort: false, reverse: false }
}

export const KvtxIterateInit = {
  encode(
    message: KvtxIterateInit,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.prefix.length !== 0) {
      writer.uint32(10).bytes(message.prefix)
    }
    if (message.sort === true) {
      writer.uint32(16).bool(message.sort)
    }
    if (message.reverse === true) {
      writer.uint32(24).bool(message.reverse)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxIterateInit {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxIterateInit()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.prefix = reader.bytes()
          break
        case 2:
          message.sort = reader.bool()
          break
        case 3:
          message.reverse = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxIterateInit, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxIterateInit | KvtxIterateInit[]>
      | Iterable<KvtxIterateInit | KvtxIterateInit[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxIterateInit.encode(p).finish()]
        }
      } else {
        yield* [KvtxIterateInit.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxIterateInit>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxIterateInit> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxIterateInit.decode(p)]
        }
      } else {
        yield* [KvtxIterateInit.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxIterateInit {
    return {
      prefix: isSet(object.prefix)
        ? bytesFromBase64(object.prefix)
        : new Uint8Array(),
      sort: isSet(object.sort) ? Boolean(object.sort) : false,
      reverse: isSet(object.reverse) ? Boolean(object.reverse) : false,
    }
  },

  toJSON(message: KvtxIterateInit): unknown {
    const obj: any = {}
    message.prefix !== undefined &&
      (obj.prefix = base64FromBytes(
        message.prefix !== undefined ? message.prefix : new Uint8Array()
      ))
    message.sort !== undefined && (obj.sort = message.sort)
    message.reverse !== undefined && (obj.reverse = message.reverse)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxIterateInit>, I>>(
    base?: I
  ): KvtxIterateInit {
    return KvtxIterateInit.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxIterateInit>, I>>(
    object: I
  ): KvtxIterateInit {
    const message = createBaseKvtxIterateInit()
    message.prefix = object.prefix ?? new Uint8Array()
    message.sort = object.sort ?? false
    message.reverse = object.reverse ?? false
    return message
  },
}

function createBaseKvtxIterateResponse(): KvtxIterateResponse {
  return { body: undefined }
}

export const KvtxIterateResponse = {
  encode(
    message: KvtxIterateResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.body?.$case === 'ack') {
      writer.uint32(8).bool(message.body.ack)
    }
    if (message.body?.$case === 'reqError') {
      writer.uint32(18).string(message.body.reqError)
    }
    if (message.body?.$case === 'status') {
      KvtxIterateStatus.encode(
        message.body.status,
        writer.uint32(26).fork()
      ).ldelim()
    }
    if (message.body?.$case === 'value') {
      writer.uint32(34).bytes(message.body.value)
    }
    if (message.body?.$case === 'closed') {
      writer.uint32(40).bool(message.body.closed)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxIterateResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxIterateResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.body = { $case: 'ack', ack: reader.bool() }
          break
        case 2:
          message.body = { $case: 'reqError', reqError: reader.string() }
          break
        case 3:
          message.body = {
            $case: 'status',
            status: KvtxIterateStatus.decode(reader, reader.uint32()),
          }
          break
        case 4:
          message.body = { $case: 'value', value: reader.bytes() }
          break
        case 5:
          message.body = { $case: 'closed', closed: reader.bool() }
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxIterateResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxIterateResponse | KvtxIterateResponse[]>
      | Iterable<KvtxIterateResponse | KvtxIterateResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxIterateResponse.encode(p).finish()]
        }
      } else {
        yield* [KvtxIterateResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxIterateResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxIterateResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxIterateResponse.decode(p)]
        }
      } else {
        yield* [KvtxIterateResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxIterateResponse {
    return {
      body: isSet(object.ack)
        ? { $case: 'ack', ack: Boolean(object.ack) }
        : isSet(object.reqError)
        ? { $case: 'reqError', reqError: String(object.reqError) }
        : isSet(object.status)
        ? { $case: 'status', status: KvtxIterateStatus.fromJSON(object.status) }
        : isSet(object.value)
        ? { $case: 'value', value: bytesFromBase64(object.value) }
        : isSet(object.closed)
        ? { $case: 'closed', closed: Boolean(object.closed) }
        : undefined,
    }
  },

  toJSON(message: KvtxIterateResponse): unknown {
    const obj: any = {}
    message.body?.$case === 'ack' && (obj.ack = message.body?.ack)
    message.body?.$case === 'reqError' &&
      (obj.reqError = message.body?.reqError)
    message.body?.$case === 'status' &&
      (obj.status = message.body?.status
        ? KvtxIterateStatus.toJSON(message.body?.status)
        : undefined)
    message.body?.$case === 'value' &&
      (obj.value =
        message.body?.value !== undefined
          ? base64FromBytes(message.body?.value)
          : undefined)
    message.body?.$case === 'closed' && (obj.closed = message.body?.closed)
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxIterateResponse>, I>>(
    base?: I
  ): KvtxIterateResponse {
    return KvtxIterateResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxIterateResponse>, I>>(
    object: I
  ): KvtxIterateResponse {
    const message = createBaseKvtxIterateResponse()
    if (
      object.body?.$case === 'ack' &&
      object.body?.ack !== undefined &&
      object.body?.ack !== null
    ) {
      message.body = { $case: 'ack', ack: object.body.ack }
    }
    if (
      object.body?.$case === 'reqError' &&
      object.body?.reqError !== undefined &&
      object.body?.reqError !== null
    ) {
      message.body = { $case: 'reqError', reqError: object.body.reqError }
    }
    if (
      object.body?.$case === 'status' &&
      object.body?.status !== undefined &&
      object.body?.status !== null
    ) {
      message.body = {
        $case: 'status',
        status: KvtxIterateStatus.fromPartial(object.body.status),
      }
    }
    if (
      object.body?.$case === 'value' &&
      object.body?.value !== undefined &&
      object.body?.value !== null
    ) {
      message.body = { $case: 'value', value: object.body.value }
    }
    if (
      object.body?.$case === 'closed' &&
      object.body?.closed !== undefined &&
      object.body?.closed !== null
    ) {
      message.body = { $case: 'closed', closed: object.body.closed }
    }
    return message
  },
}

function createBaseKvtxIterateStatus(): KvtxIterateStatus {
  return { error: '', valid: false, key: new Uint8Array() }
}

export const KvtxIterateStatus = {
  encode(
    message: KvtxIterateStatus,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    if (message.valid === true) {
      writer.uint32(16).bool(message.valid)
    }
    if (message.key.length !== 0) {
      writer.uint32(26).bytes(message.key)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxIterateStatus {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKvtxIterateStatus()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string()
          break
        case 2:
          message.valid = reader.bool()
          break
        case 3:
          message.key = reader.bytes()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxIterateStatus, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxIterateStatus | KvtxIterateStatus[]>
      | Iterable<KvtxIterateStatus | KvtxIterateStatus[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxIterateStatus.encode(p).finish()]
        }
      } else {
        yield* [KvtxIterateStatus.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxIterateStatus>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KvtxIterateStatus> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxIterateStatus.decode(p)]
        }
      } else {
        yield* [KvtxIterateStatus.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KvtxIterateStatus {
    return {
      error: isSet(object.error) ? String(object.error) : '',
      valid: isSet(object.valid) ? Boolean(object.valid) : false,
      key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array(),
    }
  },

  toJSON(message: KvtxIterateStatus): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    message.valid !== undefined && (obj.valid = message.valid)
    message.key !== undefined &&
      (obj.key = base64FromBytes(
        message.key !== undefined ? message.key : new Uint8Array()
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<KvtxIterateStatus>, I>>(
    base?: I
  ): KvtxIterateStatus {
    return KvtxIterateStatus.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KvtxIterateStatus>, I>>(
    object: I
  ): KvtxIterateStatus {
    const message = createBaseKvtxIterateStatus()
    message.error = object.error ?? ''
    message.valid = object.valid ?? false
    message.key = object.key ?? new Uint8Array()
    return message
  },
}

/** Kvtx proxies a Kvtx store via RPC. */
export interface Kvtx {
  /**
   * KvtxTransaction executes a key/value transaction.
   * Returns a stream of messages about the transaction status.
   * The transaction will be discarded if this call is canceled before Commit.
   */
  KvtxTransaction(
    request: AsyncIterable<KvtxTransactionRequest>
  ): AsyncIterable<KvtxTransactionResponse>
  /**
   * KvtxTransactionRpc is a rpc request for an ongoing KvtxTransaction.
   * Exposes service: KvtxOps
   * Component ID: transaction_id from KvtxTransaction call.
   */
  KvtxTransactionRpc(
    request: AsyncIterable<RpcStreamPacket>
  ): AsyncIterable<RpcStreamPacket>
}

export class KvtxClientImpl implements Kvtx {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || 'kvtx.rpc.Kvtx'
    this.rpc = rpc
    this.KvtxTransaction = this.KvtxTransaction.bind(this)
    this.KvtxTransactionRpc = this.KvtxTransactionRpc.bind(this)
  }
  KvtxTransaction(
    request: AsyncIterable<KvtxTransactionRequest>
  ): AsyncIterable<KvtxTransactionResponse> {
    const data = KvtxTransactionRequest.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'KvtxTransaction',
      data
    )
    return KvtxTransactionResponse.decodeTransform(result)
  }

  KvtxTransactionRpc(
    request: AsyncIterable<RpcStreamPacket>
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'KvtxTransactionRpc',
      data
    )
    return RpcStreamPacket.decodeTransform(result)
  }
}

/** Kvtx proxies a Kvtx store via RPC. */
export type KvtxDefinition = typeof KvtxDefinition
export const KvtxDefinition = {
  name: 'Kvtx',
  fullName: 'kvtx.rpc.Kvtx',
  methods: {
    /**
     * KvtxTransaction executes a key/value transaction.
     * Returns a stream of messages about the transaction status.
     * The transaction will be discarded if this call is canceled before Commit.
     */
    kvtxTransaction: {
      name: 'KvtxTransaction',
      requestType: KvtxTransactionRequest,
      requestStream: true,
      responseType: KvtxTransactionResponse,
      responseStream: true,
      options: {},
    },
    /**
     * KvtxTransactionRpc is a rpc request for an ongoing KvtxTransaction.
     * Exposes service: KvtxOps
     * Component ID: transaction_id from KvtxTransaction call.
     */
    kvtxTransactionRpc: {
      name: 'KvtxTransactionRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
  },
} as const

/**
 * KvtxOps exposes a KvtxOps object with a service.
 * Wraps the kvtx.TxOps interface.
 */
export interface KvtxOps {
  /** KeyCount returns the number of keys in the store. */
  KeyCount(request: KeyCountRequest): Promise<KeyCountResponse>
  /** KeyData returns data for a key. */
  KeyData(request: KvtxKeyRequest): Promise<KvtxKeyDataResponse>
  /** KeyExists checks if a key exists. */
  KeyExists(request: KvtxKeyRequest): Promise<KvtxKeyExistsResponse>
  /** SetKey sets the value of a key. */
  SetKey(request: KvtxSetKeyRequest): Promise<KvtxSetKeyResponse>
  /** DeleteKey removes a key from the store. */
  DeleteKey(request: KvtxDeleteKeyRequest): Promise<KvtxDeleteKeyResponse>
  /** ScanPrefix scans for key/value pairs with a key prefix. */
  ScanPrefix(
    request: KvtxScanPrefixRequest
  ): AsyncIterable<KvtxScanPrefixResponse>
  /**
   * Iterate iterates over the Kvtx store.
   * Uses a request/reply approach:
   *  - First message is sent by caller w/ the iterate arguments.
   *  - Server replies with the Ack message.
   *  - Subsequent messages are request/reply, one request to one reply.
   */
  Iterate(
    request: AsyncIterable<KvtxIterateRequest>
  ): AsyncIterable<KvtxIterateResponse>
}

export class KvtxOpsClientImpl implements KvtxOps {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || 'kvtx.rpc.KvtxOps'
    this.rpc = rpc
    this.KeyCount = this.KeyCount.bind(this)
    this.KeyData = this.KeyData.bind(this)
    this.KeyExists = this.KeyExists.bind(this)
    this.SetKey = this.SetKey.bind(this)
    this.DeleteKey = this.DeleteKey.bind(this)
    this.ScanPrefix = this.ScanPrefix.bind(this)
    this.Iterate = this.Iterate.bind(this)
  }
  KeyCount(request: KeyCountRequest): Promise<KeyCountResponse> {
    const data = KeyCountRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'KeyCount', data)
    return promise.then((data) => KeyCountResponse.decode(new _m0.Reader(data)))
  }

  KeyData(request: KvtxKeyRequest): Promise<KvtxKeyDataResponse> {
    const data = KvtxKeyRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'KeyData', data)
    return promise.then((data) =>
      KvtxKeyDataResponse.decode(new _m0.Reader(data))
    )
  }

  KeyExists(request: KvtxKeyRequest): Promise<KvtxKeyExistsResponse> {
    const data = KvtxKeyRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'KeyExists', data)
    return promise.then((data) =>
      KvtxKeyExistsResponse.decode(new _m0.Reader(data))
    )
  }

  SetKey(request: KvtxSetKeyRequest): Promise<KvtxSetKeyResponse> {
    const data = KvtxSetKeyRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'SetKey', data)
    return promise.then((data) =>
      KvtxSetKeyResponse.decode(new _m0.Reader(data))
    )
  }

  DeleteKey(request: KvtxDeleteKeyRequest): Promise<KvtxDeleteKeyResponse> {
    const data = KvtxDeleteKeyRequest.encode(request).finish()
    const promise = this.rpc.request(this.service, 'DeleteKey', data)
    return promise.then((data) =>
      KvtxDeleteKeyResponse.decode(new _m0.Reader(data))
    )
  }

  ScanPrefix(
    request: KvtxScanPrefixRequest
  ): AsyncIterable<KvtxScanPrefixResponse> {
    const data = KvtxScanPrefixRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'ScanPrefix',
      data
    )
    return KvtxScanPrefixResponse.decodeTransform(result)
  }

  Iterate(
    request: AsyncIterable<KvtxIterateRequest>
  ): AsyncIterable<KvtxIterateResponse> {
    const data = KvtxIterateRequest.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'Iterate',
      data
    )
    return KvtxIterateResponse.decodeTransform(result)
  }
}

/**
 * KvtxOps exposes a KvtxOps object with a service.
 * Wraps the kvtx.TxOps interface.
 */
export type KvtxOpsDefinition = typeof KvtxOpsDefinition
export const KvtxOpsDefinition = {
  name: 'KvtxOps',
  fullName: 'kvtx.rpc.KvtxOps',
  methods: {
    /** KeyCount returns the number of keys in the store. */
    keyCount: {
      name: 'KeyCount',
      requestType: KeyCountRequest,
      requestStream: false,
      responseType: KeyCountResponse,
      responseStream: false,
      options: {},
    },
    /** KeyData returns data for a key. */
    keyData: {
      name: 'KeyData',
      requestType: KvtxKeyRequest,
      requestStream: false,
      responseType: KvtxKeyDataResponse,
      responseStream: false,
      options: {},
    },
    /** KeyExists checks if a key exists. */
    keyExists: {
      name: 'KeyExists',
      requestType: KvtxKeyRequest,
      requestStream: false,
      responseType: KvtxKeyExistsResponse,
      responseStream: false,
      options: {},
    },
    /** SetKey sets the value of a key. */
    setKey: {
      name: 'SetKey',
      requestType: KvtxSetKeyRequest,
      requestStream: false,
      responseType: KvtxSetKeyResponse,
      responseStream: false,
      options: {},
    },
    /** DeleteKey removes a key from the store. */
    deleteKey: {
      name: 'DeleteKey',
      requestType: KvtxDeleteKeyRequest,
      requestStream: false,
      responseType: KvtxDeleteKeyResponse,
      responseStream: false,
      options: {},
    },
    /** ScanPrefix scans for key/value pairs with a key prefix. */
    scanPrefix: {
      name: 'ScanPrefix',
      requestType: KvtxScanPrefixRequest,
      requestStream: false,
      responseType: KvtxScanPrefixResponse,
      responseStream: true,
      options: {},
    },
    /**
     * Iterate iterates over the Kvtx store.
     * Uses a request/reply approach:
     *  - First message is sent by caller w/ the iterate arguments.
     *  - Server replies with the Ack message.
     *  - Subsequent messages are request/reply, one request to one reply.
     */
    iterate: {
      name: 'Iterate',
      requestType: KvtxIterateRequest,
      requestStream: true,
      responseType: KvtxIterateResponse,
      responseStream: true,
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
