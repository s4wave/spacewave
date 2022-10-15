/* eslint-disable */
import { RpcStreamPacket } from "@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "rpc.kvtx";

/** KvtxTransactionRequest is a request in a KvtxTransaction rpc. */
export interface KvtxTransactionRequest {
  body?: { $case: "init"; init: KvtxTransactionInit } | { $case: "commit"; commit: boolean } | {
    $case: "discard";
    discard: boolean;
  };
}

/** KvtxTransactionInit is the message sent to init a kvtx transaction. */
export interface KvtxTransactionInit {
  /**
   * Write indicates if this should be a write transaction.
   * If unset, the store will be read-only.
   */
  write: boolean;
}

/** KvtxTransactionResponse is a response to the KvtxTransaction rpc. */
export interface KvtxTransactionResponse {
  body?: { $case: "ack"; ack: KvtxTransactionAck } | { $case: "complete"; complete: KvtxTransactionComplete };
}

/** KvtxTransactionAck contains information about opening the transaction. */
export interface KvtxTransactionAck {
  /** Error is any error opening the transaction. */
  error: string;
  /**
   * TransactionId is the identifier to use for the RpcStream.
   * If error != "" this will be empty.
   */
  transactionId: string;
}

/** KvtxTransactionComplete contains information about the result of a tx. */
export interface KvtxTransactionComplete {
  /**
   * Error is any error completing the transaction.
   * If this is set, usually discarded=true as well.
   */
  error: string;
  /**
   * Committed indicates we successfully committed the transaction.
   * If error != "" this will be false.
   */
  committed: boolean;
  /**
   * Discarded indicates the transaction is/was discarded.
   * No changes will be applied.
   */
  discarded: boolean;
}

/** KeyCountRequest is a request for the number of keys in the store. */
export interface KeyCountRequest {
}

/** KeyCountResponse is a response to the KeyCountRequest. */
export interface KeyCountResponse {
  /** KeyCount is the number of keys in the store. */
  keyCount: Long;
}

/** KvtxKeyRequest is a request that accepts a single key as a parameter. */
export interface KvtxKeyRequest {
  /** Key is the key to lookup. */
  key: Uint8Array;
}

/** KvtxKeyDataResponse responds to a request for data from a KvtxOps store. */
export interface KvtxKeyDataResponse {
  /**
   * Error is any error accessing the key.
   * Will be empty if the key was unset.
   */
  error: string;
  /** Found indicates the key was found. */
  found: boolean;
  /** Data contains the data, if found=true. */
  data: Uint8Array;
}

/** KvtxKeyExistsResponse responds to a request to check if a key exists. */
export interface KvtxKeyExistsResponse {
  /**
   * Error is any error accessing the key.
   * Will be empty if the key was unset.
   */
  error: string;
  /** Found indicates the key was found. */
  found: boolean;
}

/** KvtxSetKeyRequest is a request to set a key to a value. */
export interface KvtxSetKeyRequest {
  /** Key is the key to set. */
  key: Uint8Array;
  /** Value is the value to set it to. */
  value: Uint8Array;
}

/** KvtxSetKeyResponse is the response to setting a key to a value. */
export interface KvtxSetKeyResponse {
  /**
   * Error is any error setting the key.
   * If empty, the operation succeeded.
   */
  error: string;
}

/** KvtxDeleteKeyRequest is a request to delete a key from the store. */
export interface KvtxDeleteKeyRequest {
  /** Key is the key to delete. */
  key: Uint8Array;
}

/** KvtxDeleteKeyResponse is the response to deleting a key from the store. */
export interface KvtxDeleteKeyResponse {
  /**
   * Error is any error removing the key.
   * If empty, the operation succeeded.
   */
  error: string;
}

function createBaseKvtxTransactionRequest(): KvtxTransactionRequest {
  return { body: undefined };
}

export const KvtxTransactionRequest = {
  encode(message: KvtxTransactionRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.body?.$case === "init") {
      KvtxTransactionInit.encode(message.body.init, writer.uint32(10).fork()).ldelim();
    }
    if (message.body?.$case === "commit") {
      writer.uint32(16).bool(message.body.commit);
    }
    if (message.body?.$case === "discard") {
      writer.uint32(24).bool(message.body.discard);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxTransactionRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxTransactionRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.body = { $case: "init", init: KvtxTransactionInit.decode(reader, reader.uint32()) };
          break;
        case 2:
          message.body = { $case: "commit", commit: reader.bool() };
          break;
        case 3:
          message.body = { $case: "discard", discard: reader.bool() };
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionRequest | KvtxTransactionRequest[]>
      | Iterable<KvtxTransactionRequest | KvtxTransactionRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionRequest.encode(p).finish()];
        }
      } else {
        yield* [KvtxTransactionRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxTransactionRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionRequest.decode(p)];
        }
      } else {
        yield* [KvtxTransactionRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxTransactionRequest {
    return {
      body: isSet(object.init)
        ? { $case: "init", init: KvtxTransactionInit.fromJSON(object.init) }
        : isSet(object.commit)
        ? { $case: "commit", commit: Boolean(object.commit) }
        : isSet(object.discard)
        ? { $case: "discard", discard: Boolean(object.discard) }
        : undefined,
    };
  },

  toJSON(message: KvtxTransactionRequest): unknown {
    const obj: any = {};
    message.body?.$case === "init" &&
      (obj.init = message.body?.init ? KvtxTransactionInit.toJSON(message.body?.init) : undefined);
    message.body?.$case === "commit" && (obj.commit = message.body?.commit);
    message.body?.$case === "discard" && (obj.discard = message.body?.discard);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionRequest>, I>>(object: I): KvtxTransactionRequest {
    const message = createBaseKvtxTransactionRequest();
    if (object.body?.$case === "init" && object.body?.init !== undefined && object.body?.init !== null) {
      message.body = { $case: "init", init: KvtxTransactionInit.fromPartial(object.body.init) };
    }
    if (object.body?.$case === "commit" && object.body?.commit !== undefined && object.body?.commit !== null) {
      message.body = { $case: "commit", commit: object.body.commit };
    }
    if (object.body?.$case === "discard" && object.body?.discard !== undefined && object.body?.discard !== null) {
      message.body = { $case: "discard", discard: object.body.discard };
    }
    return message;
  },
};

function createBaseKvtxTransactionInit(): KvtxTransactionInit {
  return { write: false };
}

export const KvtxTransactionInit = {
  encode(message: KvtxTransactionInit, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.write === true) {
      writer.uint32(8).bool(message.write);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxTransactionInit {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxTransactionInit();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.write = reader.bool();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionInit, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionInit | KvtxTransactionInit[]>
      | Iterable<KvtxTransactionInit | KvtxTransactionInit[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionInit.encode(p).finish()];
        }
      } else {
        yield* [KvtxTransactionInit.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionInit>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxTransactionInit> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionInit.decode(p)];
        }
      } else {
        yield* [KvtxTransactionInit.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxTransactionInit {
    return { write: isSet(object.write) ? Boolean(object.write) : false };
  },

  toJSON(message: KvtxTransactionInit): unknown {
    const obj: any = {};
    message.write !== undefined && (obj.write = message.write);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionInit>, I>>(object: I): KvtxTransactionInit {
    const message = createBaseKvtxTransactionInit();
    message.write = object.write ?? false;
    return message;
  },
};

function createBaseKvtxTransactionResponse(): KvtxTransactionResponse {
  return { body: undefined };
}

export const KvtxTransactionResponse = {
  encode(message: KvtxTransactionResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.body?.$case === "ack") {
      KvtxTransactionAck.encode(message.body.ack, writer.uint32(10).fork()).ldelim();
    }
    if (message.body?.$case === "complete") {
      KvtxTransactionComplete.encode(message.body.complete, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxTransactionResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxTransactionResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.body = { $case: "ack", ack: KvtxTransactionAck.decode(reader, reader.uint32()) };
          break;
        case 2:
          message.body = { $case: "complete", complete: KvtxTransactionComplete.decode(reader, reader.uint32()) };
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionResponse | KvtxTransactionResponse[]>
      | Iterable<KvtxTransactionResponse | KvtxTransactionResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionResponse.encode(p).finish()];
        }
      } else {
        yield* [KvtxTransactionResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxTransactionResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionResponse.decode(p)];
        }
      } else {
        yield* [KvtxTransactionResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxTransactionResponse {
    return {
      body: isSet(object.ack)
        ? { $case: "ack", ack: KvtxTransactionAck.fromJSON(object.ack) }
        : isSet(object.complete)
        ? { $case: "complete", complete: KvtxTransactionComplete.fromJSON(object.complete) }
        : undefined,
    };
  },

  toJSON(message: KvtxTransactionResponse): unknown {
    const obj: any = {};
    message.body?.$case === "ack" &&
      (obj.ack = message.body?.ack ? KvtxTransactionAck.toJSON(message.body?.ack) : undefined);
    message.body?.$case === "complete" &&
      (obj.complete = message.body?.complete ? KvtxTransactionComplete.toJSON(message.body?.complete) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionResponse>, I>>(object: I): KvtxTransactionResponse {
    const message = createBaseKvtxTransactionResponse();
    if (object.body?.$case === "ack" && object.body?.ack !== undefined && object.body?.ack !== null) {
      message.body = { $case: "ack", ack: KvtxTransactionAck.fromPartial(object.body.ack) };
    }
    if (object.body?.$case === "complete" && object.body?.complete !== undefined && object.body?.complete !== null) {
      message.body = { $case: "complete", complete: KvtxTransactionComplete.fromPartial(object.body.complete) };
    }
    return message;
  },
};

function createBaseKvtxTransactionAck(): KvtxTransactionAck {
  return { error: "", transactionId: "" };
}

export const KvtxTransactionAck = {
  encode(message: KvtxTransactionAck, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.error !== "") {
      writer.uint32(10).string(message.error);
    }
    if (message.transactionId !== "") {
      writer.uint32(18).string(message.transactionId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxTransactionAck {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxTransactionAck();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string();
          break;
        case 2:
          message.transactionId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionAck, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionAck | KvtxTransactionAck[]>
      | Iterable<KvtxTransactionAck | KvtxTransactionAck[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionAck.encode(p).finish()];
        }
      } else {
        yield* [KvtxTransactionAck.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionAck>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxTransactionAck> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionAck.decode(p)];
        }
      } else {
        yield* [KvtxTransactionAck.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxTransactionAck {
    return {
      error: isSet(object.error) ? String(object.error) : "",
      transactionId: isSet(object.transactionId) ? String(object.transactionId) : "",
    };
  },

  toJSON(message: KvtxTransactionAck): unknown {
    const obj: any = {};
    message.error !== undefined && (obj.error = message.error);
    message.transactionId !== undefined && (obj.transactionId = message.transactionId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionAck>, I>>(object: I): KvtxTransactionAck {
    const message = createBaseKvtxTransactionAck();
    message.error = object.error ?? "";
    message.transactionId = object.transactionId ?? "";
    return message;
  },
};

function createBaseKvtxTransactionComplete(): KvtxTransactionComplete {
  return { error: "", committed: false, discarded: false };
}

export const KvtxTransactionComplete = {
  encode(message: KvtxTransactionComplete, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.error !== "") {
      writer.uint32(10).string(message.error);
    }
    if (message.committed === true) {
      writer.uint32(16).bool(message.committed);
    }
    if (message.discarded === true) {
      writer.uint32(24).bool(message.discarded);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxTransactionComplete {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxTransactionComplete();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string();
          break;
        case 2:
          message.committed = reader.bool();
          break;
        case 3:
          message.discarded = reader.bool();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxTransactionComplete, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxTransactionComplete | KvtxTransactionComplete[]>
      | Iterable<KvtxTransactionComplete | KvtxTransactionComplete[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionComplete.encode(p).finish()];
        }
      } else {
        yield* [KvtxTransactionComplete.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxTransactionComplete>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxTransactionComplete> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxTransactionComplete.decode(p)];
        }
      } else {
        yield* [KvtxTransactionComplete.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxTransactionComplete {
    return {
      error: isSet(object.error) ? String(object.error) : "",
      committed: isSet(object.committed) ? Boolean(object.committed) : false,
      discarded: isSet(object.discarded) ? Boolean(object.discarded) : false,
    };
  },

  toJSON(message: KvtxTransactionComplete): unknown {
    const obj: any = {};
    message.error !== undefined && (obj.error = message.error);
    message.committed !== undefined && (obj.committed = message.committed);
    message.discarded !== undefined && (obj.discarded = message.discarded);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxTransactionComplete>, I>>(object: I): KvtxTransactionComplete {
    const message = createBaseKvtxTransactionComplete();
    message.error = object.error ?? "";
    message.committed = object.committed ?? false;
    message.discarded = object.discarded ?? false;
    return message;
  },
};

function createBaseKeyCountRequest(): KeyCountRequest {
  return {};
}

export const KeyCountRequest = {
  encode(_: KeyCountRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KeyCountRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKeyCountRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KeyCountRequest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<KeyCountRequest | KeyCountRequest[]> | Iterable<KeyCountRequest | KeyCountRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyCountRequest.encode(p).finish()];
        }
      } else {
        yield* [KeyCountRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KeyCountRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KeyCountRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyCountRequest.decode(p)];
        }
      } else {
        yield* [KeyCountRequest.decode(pkt)];
      }
    }
  },

  fromJSON(_: any): KeyCountRequest {
    return {};
  },

  toJSON(_: KeyCountRequest): unknown {
    const obj: any = {};
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KeyCountRequest>, I>>(_: I): KeyCountRequest {
    const message = createBaseKeyCountRequest();
    return message;
  },
};

function createBaseKeyCountResponse(): KeyCountResponse {
  return { keyCount: Long.UZERO };
}

export const KeyCountResponse = {
  encode(message: KeyCountResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (!message.keyCount.isZero()) {
      writer.uint32(8).uint64(message.keyCount);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KeyCountResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKeyCountResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.keyCount = reader.uint64() as Long;
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KeyCountResponse, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<KeyCountResponse | KeyCountResponse[]> | Iterable<KeyCountResponse | KeyCountResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyCountResponse.encode(p).finish()];
        }
      } else {
        yield* [KeyCountResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KeyCountResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KeyCountResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyCountResponse.decode(p)];
        }
      } else {
        yield* [KeyCountResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KeyCountResponse {
    return { keyCount: isSet(object.keyCount) ? Long.fromValue(object.keyCount) : Long.UZERO };
  },

  toJSON(message: KeyCountResponse): unknown {
    const obj: any = {};
    message.keyCount !== undefined && (obj.keyCount = (message.keyCount || Long.UZERO).toString());
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KeyCountResponse>, I>>(object: I): KeyCountResponse {
    const message = createBaseKeyCountResponse();
    message.keyCount = (object.keyCount !== undefined && object.keyCount !== null)
      ? Long.fromValue(object.keyCount)
      : Long.UZERO;
    return message;
  },
};

function createBaseKvtxKeyRequest(): KvtxKeyRequest {
  return { key: new Uint8Array() };
}

export const KvtxKeyRequest = {
  encode(message: KvtxKeyRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key.length !== 0) {
      writer.uint32(10).bytes(message.key);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxKeyRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxKeyRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.bytes();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxKeyRequest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<KvtxKeyRequest | KvtxKeyRequest[]> | Iterable<KvtxKeyRequest | KvtxKeyRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyRequest.encode(p).finish()];
        }
      } else {
        yield* [KvtxKeyRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxKeyRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxKeyRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyRequest.decode(p)];
        }
      } else {
        yield* [KvtxKeyRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxKeyRequest {
    return { key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array() };
  },

  toJSON(message: KvtxKeyRequest): unknown {
    const obj: any = {};
    message.key !== undefined &&
      (obj.key = base64FromBytes(message.key !== undefined ? message.key : new Uint8Array()));
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxKeyRequest>, I>>(object: I): KvtxKeyRequest {
    const message = createBaseKvtxKeyRequest();
    message.key = object.key ?? new Uint8Array();
    return message;
  },
};

function createBaseKvtxKeyDataResponse(): KvtxKeyDataResponse {
  return { error: "", found: false, data: new Uint8Array() };
}

export const KvtxKeyDataResponse = {
  encode(message: KvtxKeyDataResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.error !== "") {
      writer.uint32(10).string(message.error);
    }
    if (message.found === true) {
      writer.uint32(16).bool(message.found);
    }
    if (message.data.length !== 0) {
      writer.uint32(26).bytes(message.data);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxKeyDataResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxKeyDataResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string();
          break;
        case 2:
          message.found = reader.bool();
          break;
        case 3:
          message.data = reader.bytes();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxKeyDataResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxKeyDataResponse | KvtxKeyDataResponse[]>
      | Iterable<KvtxKeyDataResponse | KvtxKeyDataResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyDataResponse.encode(p).finish()];
        }
      } else {
        yield* [KvtxKeyDataResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxKeyDataResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxKeyDataResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyDataResponse.decode(p)];
        }
      } else {
        yield* [KvtxKeyDataResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxKeyDataResponse {
    return {
      error: isSet(object.error) ? String(object.error) : "",
      found: isSet(object.found) ? Boolean(object.found) : false,
      data: isSet(object.data) ? bytesFromBase64(object.data) : new Uint8Array(),
    };
  },

  toJSON(message: KvtxKeyDataResponse): unknown {
    const obj: any = {};
    message.error !== undefined && (obj.error = message.error);
    message.found !== undefined && (obj.found = message.found);
    message.data !== undefined &&
      (obj.data = base64FromBytes(message.data !== undefined ? message.data : new Uint8Array()));
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxKeyDataResponse>, I>>(object: I): KvtxKeyDataResponse {
    const message = createBaseKvtxKeyDataResponse();
    message.error = object.error ?? "";
    message.found = object.found ?? false;
    message.data = object.data ?? new Uint8Array();
    return message;
  },
};

function createBaseKvtxKeyExistsResponse(): KvtxKeyExistsResponse {
  return { error: "", found: false };
}

export const KvtxKeyExistsResponse = {
  encode(message: KvtxKeyExistsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.error !== "") {
      writer.uint32(10).string(message.error);
    }
    if (message.found === true) {
      writer.uint32(16).bool(message.found);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxKeyExistsResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxKeyExistsResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string();
          break;
        case 2:
          message.found = reader.bool();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxKeyExistsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxKeyExistsResponse | KvtxKeyExistsResponse[]>
      | Iterable<KvtxKeyExistsResponse | KvtxKeyExistsResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyExistsResponse.encode(p).finish()];
        }
      } else {
        yield* [KvtxKeyExistsResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxKeyExistsResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxKeyExistsResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxKeyExistsResponse.decode(p)];
        }
      } else {
        yield* [KvtxKeyExistsResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxKeyExistsResponse {
    return {
      error: isSet(object.error) ? String(object.error) : "",
      found: isSet(object.found) ? Boolean(object.found) : false,
    };
  },

  toJSON(message: KvtxKeyExistsResponse): unknown {
    const obj: any = {};
    message.error !== undefined && (obj.error = message.error);
    message.found !== undefined && (obj.found = message.found);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxKeyExistsResponse>, I>>(object: I): KvtxKeyExistsResponse {
    const message = createBaseKvtxKeyExistsResponse();
    message.error = object.error ?? "";
    message.found = object.found ?? false;
    return message;
  },
};

function createBaseKvtxSetKeyRequest(): KvtxSetKeyRequest {
  return { key: new Uint8Array(), value: new Uint8Array() };
}

export const KvtxSetKeyRequest = {
  encode(message: KvtxSetKeyRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key.length !== 0) {
      writer.uint32(10).bytes(message.key);
    }
    if (message.value.length !== 0) {
      writer.uint32(18).bytes(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxSetKeyRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxSetKeyRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.bytes();
          break;
        case 2:
          message.value = reader.bytes();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxSetKeyRequest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<KvtxSetKeyRequest | KvtxSetKeyRequest[]> | Iterable<KvtxSetKeyRequest | KvtxSetKeyRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxSetKeyRequest.encode(p).finish()];
        }
      } else {
        yield* [KvtxSetKeyRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxSetKeyRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxSetKeyRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxSetKeyRequest.decode(p)];
        }
      } else {
        yield* [KvtxSetKeyRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxSetKeyRequest {
    return {
      key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array(),
      value: isSet(object.value) ? bytesFromBase64(object.value) : new Uint8Array(),
    };
  },

  toJSON(message: KvtxSetKeyRequest): unknown {
    const obj: any = {};
    message.key !== undefined &&
      (obj.key = base64FromBytes(message.key !== undefined ? message.key : new Uint8Array()));
    message.value !== undefined &&
      (obj.value = base64FromBytes(message.value !== undefined ? message.value : new Uint8Array()));
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxSetKeyRequest>, I>>(object: I): KvtxSetKeyRequest {
    const message = createBaseKvtxSetKeyRequest();
    message.key = object.key ?? new Uint8Array();
    message.value = object.value ?? new Uint8Array();
    return message;
  },
};

function createBaseKvtxSetKeyResponse(): KvtxSetKeyResponse {
  return { error: "" };
}

export const KvtxSetKeyResponse = {
  encode(message: KvtxSetKeyResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.error !== "") {
      writer.uint32(10).string(message.error);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxSetKeyResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxSetKeyResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxSetKeyResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxSetKeyResponse | KvtxSetKeyResponse[]>
      | Iterable<KvtxSetKeyResponse | KvtxSetKeyResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxSetKeyResponse.encode(p).finish()];
        }
      } else {
        yield* [KvtxSetKeyResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxSetKeyResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxSetKeyResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxSetKeyResponse.decode(p)];
        }
      } else {
        yield* [KvtxSetKeyResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxSetKeyResponse {
    return { error: isSet(object.error) ? String(object.error) : "" };
  },

  toJSON(message: KvtxSetKeyResponse): unknown {
    const obj: any = {};
    message.error !== undefined && (obj.error = message.error);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxSetKeyResponse>, I>>(object: I): KvtxSetKeyResponse {
    const message = createBaseKvtxSetKeyResponse();
    message.error = object.error ?? "";
    return message;
  },
};

function createBaseKvtxDeleteKeyRequest(): KvtxDeleteKeyRequest {
  return { key: new Uint8Array() };
}

export const KvtxDeleteKeyRequest = {
  encode(message: KvtxDeleteKeyRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key.length !== 0) {
      writer.uint32(10).bytes(message.key);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxDeleteKeyRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxDeleteKeyRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.bytes();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxDeleteKeyRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxDeleteKeyRequest | KvtxDeleteKeyRequest[]>
      | Iterable<KvtxDeleteKeyRequest | KvtxDeleteKeyRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxDeleteKeyRequest.encode(p).finish()];
        }
      } else {
        yield* [KvtxDeleteKeyRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxDeleteKeyRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxDeleteKeyRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxDeleteKeyRequest.decode(p)];
        }
      } else {
        yield* [KvtxDeleteKeyRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxDeleteKeyRequest {
    return { key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array() };
  },

  toJSON(message: KvtxDeleteKeyRequest): unknown {
    const obj: any = {};
    message.key !== undefined &&
      (obj.key = base64FromBytes(message.key !== undefined ? message.key : new Uint8Array()));
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxDeleteKeyRequest>, I>>(object: I): KvtxDeleteKeyRequest {
    const message = createBaseKvtxDeleteKeyRequest();
    message.key = object.key ?? new Uint8Array();
    return message;
  },
};

function createBaseKvtxDeleteKeyResponse(): KvtxDeleteKeyResponse {
  return { error: "" };
}

export const KvtxDeleteKeyResponse = {
  encode(message: KvtxDeleteKeyResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.error !== "") {
      writer.uint32(10).string(message.error);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KvtxDeleteKeyResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKvtxDeleteKeyResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KvtxDeleteKeyResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KvtxDeleteKeyResponse | KvtxDeleteKeyResponse[]>
      | Iterable<KvtxDeleteKeyResponse | KvtxDeleteKeyResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxDeleteKeyResponse.encode(p).finish()];
        }
      } else {
        yield* [KvtxDeleteKeyResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KvtxDeleteKeyResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<KvtxDeleteKeyResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KvtxDeleteKeyResponse.decode(p)];
        }
      } else {
        yield* [KvtxDeleteKeyResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): KvtxDeleteKeyResponse {
    return { error: isSet(object.error) ? String(object.error) : "" };
  },

  toJSON(message: KvtxDeleteKeyResponse): unknown {
    const obj: any = {};
    message.error !== undefined && (obj.error = message.error);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<KvtxDeleteKeyResponse>, I>>(object: I): KvtxDeleteKeyResponse {
    const message = createBaseKvtxDeleteKeyResponse();
    message.error = object.error ?? "";
    return message;
  },
};

/** Kvtx proxies a Kvtx store via RPC. */
export interface Kvtx {
  /**
   * KvtxTransaction executes a key/value transaction.
   * Returns a stream of messages about the transaction status.
   * The transaction will be discarded if this call is canceled before Commit.
   */
  KvtxTransaction(request: AsyncIterable<KvtxTransactionRequest>): AsyncIterable<KvtxTransactionResponse>;
  /**
   * KvtxTransactionRpc is a rpc request for an ongoing KvtxTransaction.
   * Exposes service: KvtxOps
   * Component ID: transaction_id from KvtxTransaction call.
   */
  KvtxTransactionRpc(request: AsyncIterable<RpcStreamPacket>): AsyncIterable<RpcStreamPacket>;
}

export class KvtxClientImpl implements Kvtx {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "rpc.kvtx.Kvtx";
    this.rpc = rpc;
    this.KvtxTransaction = this.KvtxTransaction.bind(this);
    this.KvtxTransactionRpc = this.KvtxTransactionRpc.bind(this);
  }
  KvtxTransaction(request: AsyncIterable<KvtxTransactionRequest>): AsyncIterable<KvtxTransactionResponse> {
    const data = KvtxTransactionRequest.encodeTransform(request);
    const result = this.rpc.bidirectionalStreamingRequest(this.service, "KvtxTransaction", data);
    return KvtxTransactionResponse.decodeTransform(result);
  }

  KvtxTransactionRpc(request: AsyncIterable<RpcStreamPacket>): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request);
    const result = this.rpc.bidirectionalStreamingRequest(this.service, "KvtxTransactionRpc", data);
    return RpcStreamPacket.decodeTransform(result);
  }
}

/** Kvtx proxies a Kvtx store via RPC. */
export type KvtxDefinition = typeof KvtxDefinition;
export const KvtxDefinition = {
  name: "Kvtx",
  fullName: "rpc.kvtx.Kvtx",
  methods: {
    /**
     * KvtxTransaction executes a key/value transaction.
     * Returns a stream of messages about the transaction status.
     * The transaction will be discarded if this call is canceled before Commit.
     */
    kvtxTransaction: {
      name: "KvtxTransaction",
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
      name: "KvtxTransactionRpc",
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
  },
} as const;

/**
 * KvtxOps exposes a KvtxOps object with a service.
 * Wraps the kvtx.TxOps interface.
 */
export interface KvtxOps {
  /** KeyCount returns the number of keys in the store. */
  KeyCount(request: KeyCountRequest): Promise<KeyCountResponse>;
  /** KeyData returns data for a key. */
  KeyData(request: KvtxKeyRequest): Promise<KvtxKeyDataResponse>;
  /** KeyExists checks if a key exists. */
  KeyExists(request: KvtxKeyRequest): Promise<KvtxKeyExistsResponse>;
  /** SetKey sets the value of a key. */
  SetKey(request: KvtxSetKeyRequest): Promise<KvtxSetKeyResponse>;
  /** DeleteKey removes a key from the store. */
  DeleteKey(request: KvtxDeleteKeyRequest): Promise<KvtxDeleteKeyResponse>;
}

export class KvtxOpsClientImpl implements KvtxOps {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "rpc.kvtx.KvtxOps";
    this.rpc = rpc;
    this.KeyCount = this.KeyCount.bind(this);
    this.KeyData = this.KeyData.bind(this);
    this.KeyExists = this.KeyExists.bind(this);
    this.SetKey = this.SetKey.bind(this);
    this.DeleteKey = this.DeleteKey.bind(this);
  }
  KeyCount(request: KeyCountRequest): Promise<KeyCountResponse> {
    const data = KeyCountRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "KeyCount", data);
    return promise.then((data) => KeyCountResponse.decode(new _m0.Reader(data)));
  }

  KeyData(request: KvtxKeyRequest): Promise<KvtxKeyDataResponse> {
    const data = KvtxKeyRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "KeyData", data);
    return promise.then((data) => KvtxKeyDataResponse.decode(new _m0.Reader(data)));
  }

  KeyExists(request: KvtxKeyRequest): Promise<KvtxKeyExistsResponse> {
    const data = KvtxKeyRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "KeyExists", data);
    return promise.then((data) => KvtxKeyExistsResponse.decode(new _m0.Reader(data)));
  }

  SetKey(request: KvtxSetKeyRequest): Promise<KvtxSetKeyResponse> {
    const data = KvtxSetKeyRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "SetKey", data);
    return promise.then((data) => KvtxSetKeyResponse.decode(new _m0.Reader(data)));
  }

  DeleteKey(request: KvtxDeleteKeyRequest): Promise<KvtxDeleteKeyResponse> {
    const data = KvtxDeleteKeyRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "DeleteKey", data);
    return promise.then((data) => KvtxDeleteKeyResponse.decode(new _m0.Reader(data)));
  }
}

/**
 * KvtxOps exposes a KvtxOps object with a service.
 * Wraps the kvtx.TxOps interface.
 */
export type KvtxOpsDefinition = typeof KvtxOpsDefinition;
export const KvtxOpsDefinition = {
  name: "KvtxOps",
  fullName: "rpc.kvtx.KvtxOps",
  methods: {
    /** KeyCount returns the number of keys in the store. */
    keyCount: {
      name: "KeyCount",
      requestType: KeyCountRequest,
      requestStream: false,
      responseType: KeyCountResponse,
      responseStream: false,
      options: {},
    },
    /** KeyData returns data for a key. */
    keyData: {
      name: "KeyData",
      requestType: KvtxKeyRequest,
      requestStream: false,
      responseType: KvtxKeyDataResponse,
      responseStream: false,
      options: {},
    },
    /** KeyExists checks if a key exists. */
    keyExists: {
      name: "KeyExists",
      requestType: KvtxKeyRequest,
      requestStream: false,
      responseType: KvtxKeyExistsResponse,
      responseStream: false,
      options: {},
    },
    /** SetKey sets the value of a key. */
    setKey: {
      name: "SetKey",
      requestType: KvtxSetKeyRequest,
      requestStream: false,
      responseType: KvtxSetKeyResponse,
      responseStream: false,
      options: {},
    },
    /** DeleteKey removes a key from the store. */
    deleteKey: {
      name: "DeleteKey",
      requestType: KvtxDeleteKeyRequest,
      requestStream: false,
      responseType: KvtxDeleteKeyResponse,
      responseStream: false,
      options: {},
    },
  },
} as const;

interface Rpc {
  request(service: string, method: string, data: Uint8Array): Promise<Uint8Array>;
  clientStreamingRequest(service: string, method: string, data: AsyncIterable<Uint8Array>): Promise<Uint8Array>;
  serverStreamingRequest(service: string, method: string, data: Uint8Array): AsyncIterable<Uint8Array>;
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
  ): AsyncIterable<Uint8Array>;
}

declare var self: any | undefined;
declare var window: any | undefined;
declare var global: any | undefined;
var globalThis: any = (() => {
  if (typeof globalThis !== "undefined") {
    return globalThis;
  }
  if (typeof self !== "undefined") {
    return self;
  }
  if (typeof window !== "undefined") {
    return window;
  }
  if (typeof global !== "undefined") {
    return global;
  }
  throw "Unable to locate global object";
})();

function bytesFromBase64(b64: string): Uint8Array {
  if (globalThis.Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, "base64"));
  } else {
    const bin = globalThis.atob(b64);
    const arr = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i);
    }
    return arr;
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (globalThis.Buffer) {
    return globalThis.Buffer.from(arr).toString("base64");
  } else {
    const bin: string[] = [];
    arr.forEach((byte) => {
      bin.push(String.fromCharCode(byte));
    });
    return globalThis.btoa(bin.join(""));
  }
}

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends Long ? string | number | Long : T extends Array<infer U> ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string } ? { [K in keyof Omit<T, "$case">]?: DeepPartial<T[K]> } & { $case: T["$case"] }
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any;
  _m0.configure();
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
