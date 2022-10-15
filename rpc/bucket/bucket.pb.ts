/* eslint-disable */
import { BucketInfo, Config } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "rpc.bucket";

/** ApplyBucketConfigRequest requests running volumes ingest a bucket config. */
export interface ApplyBucketConfigRequest {
  /** Config is the bucket config. */
  config: Config | undefined;
}

/** ApplyBucketConfigResponse returns results of the request. */
export interface ApplyBucketConfigResponse {
  /** Updated indicates the configuration was updated. */
  updated: boolean;
  /** Prev is the previous configuration, if any. */
  prev:
    | Config
    | undefined;
  /** Curr is the current configuration, if any. */
  curr: Config | undefined;
}

/** GetBucketConfigRequest requests to look up a bucket config in a volume. */
export interface GetBucketConfigRequest {
  /** BucketId is the identifier of the bucket to look up. */
  bucketId: string;
}

/** GetBucketConfigResponse responds to the request for a bucket config. */
export interface GetBucketConfigResponse {
  /** Config is the bucket config, if found. */
  config: Config | undefined;
}

/** GetBucketInfoRequest requests to bucket information from a volume. */
export interface GetBucketInfoRequest {
  /** BucketId is the identifier of the bucket to look up. */
  bucketId: string;
}

/** GetBucketInfoResponse responds to the request for bucket info. */
export interface GetBucketInfoResponse {
  /**
   * BucketInfo is the bucket information, if found.
   * Otherwise returns empty.
   */
  bucketInfo: BucketInfo | undefined;
}

/** ListBucketInfoRequest requests to bucket information from a volume. */
export interface ListBucketInfoRequest {
  /** BucketIdRe is an optional regex to filter the list by. */
  bucketIdRe: string;
}

/** ListBucketInfoResponse is the response to the request for bucket infos. */
export interface ListBucketInfoResponse {
  /** BucketInfo is the bucket information list. */
  bucketInfo: BucketInfo[];
}

function createBaseApplyBucketConfigRequest(): ApplyBucketConfigRequest {
  return { config: undefined };
}

export const ApplyBucketConfigRequest = {
  encode(message: ApplyBucketConfigRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.config !== undefined) {
      Config.encode(message.config, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ApplyBucketConfigRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseApplyBucketConfigRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.config = Config.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ApplyBucketConfigRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ApplyBucketConfigRequest | ApplyBucketConfigRequest[]>
      | Iterable<ApplyBucketConfigRequest | ApplyBucketConfigRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigRequest.encode(p).finish()];
        }
      } else {
        yield* [ApplyBucketConfigRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ApplyBucketConfigRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ApplyBucketConfigRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigRequest.decode(p)];
        }
      } else {
        yield* [ApplyBucketConfigRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ApplyBucketConfigRequest {
    return { config: isSet(object.config) ? Config.fromJSON(object.config) : undefined };
  },

  toJSON(message: ApplyBucketConfigRequest): unknown {
    const obj: any = {};
    message.config !== undefined && (obj.config = message.config ? Config.toJSON(message.config) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ApplyBucketConfigRequest>, I>>(object: I): ApplyBucketConfigRequest {
    const message = createBaseApplyBucketConfigRequest();
    message.config = (object.config !== undefined && object.config !== null)
      ? Config.fromPartial(object.config)
      : undefined;
    return message;
  },
};

function createBaseApplyBucketConfigResponse(): ApplyBucketConfigResponse {
  return { updated: false, prev: undefined, curr: undefined };
}

export const ApplyBucketConfigResponse = {
  encode(message: ApplyBucketConfigResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.updated === true) {
      writer.uint32(8).bool(message.updated);
    }
    if (message.prev !== undefined) {
      Config.encode(message.prev, writer.uint32(18).fork()).ldelim();
    }
    if (message.curr !== undefined) {
      Config.encode(message.curr, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ApplyBucketConfigResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseApplyBucketConfigResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.updated = reader.bool();
          break;
        case 2:
          message.prev = Config.decode(reader, reader.uint32());
          break;
        case 3:
          message.curr = Config.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ApplyBucketConfigResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ApplyBucketConfigResponse | ApplyBucketConfigResponse[]>
      | Iterable<ApplyBucketConfigResponse | ApplyBucketConfigResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigResponse.encode(p).finish()];
        }
      } else {
        yield* [ApplyBucketConfigResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ApplyBucketConfigResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ApplyBucketConfigResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigResponse.decode(p)];
        }
      } else {
        yield* [ApplyBucketConfigResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ApplyBucketConfigResponse {
    return {
      updated: isSet(object.updated) ? Boolean(object.updated) : false,
      prev: isSet(object.prev) ? Config.fromJSON(object.prev) : undefined,
      curr: isSet(object.curr) ? Config.fromJSON(object.curr) : undefined,
    };
  },

  toJSON(message: ApplyBucketConfigResponse): unknown {
    const obj: any = {};
    message.updated !== undefined && (obj.updated = message.updated);
    message.prev !== undefined && (obj.prev = message.prev ? Config.toJSON(message.prev) : undefined);
    message.curr !== undefined && (obj.curr = message.curr ? Config.toJSON(message.curr) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ApplyBucketConfigResponse>, I>>(object: I): ApplyBucketConfigResponse {
    const message = createBaseApplyBucketConfigResponse();
    message.updated = object.updated ?? false;
    message.prev = (object.prev !== undefined && object.prev !== null) ? Config.fromPartial(object.prev) : undefined;
    message.curr = (object.curr !== undefined && object.curr !== null) ? Config.fromPartial(object.curr) : undefined;
    return message;
  },
};

function createBaseGetBucketConfigRequest(): GetBucketConfigRequest {
  return { bucketId: "" };
}

export const GetBucketConfigRequest = {
  encode(message: GetBucketConfigRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bucketId !== "") {
      writer.uint32(10).string(message.bucketId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetBucketConfigRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetBucketConfigRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.bucketId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetBucketConfigRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetBucketConfigRequest | GetBucketConfigRequest[]>
      | Iterable<GetBucketConfigRequest | GetBucketConfigRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetBucketConfigRequest.encode(p).finish()];
        }
      } else {
        yield* [GetBucketConfigRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetBucketConfigRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetBucketConfigRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetBucketConfigRequest.decode(p)];
        }
      } else {
        yield* [GetBucketConfigRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): GetBucketConfigRequest {
    return { bucketId: isSet(object.bucketId) ? String(object.bucketId) : "" };
  },

  toJSON(message: GetBucketConfigRequest): unknown {
    const obj: any = {};
    message.bucketId !== undefined && (obj.bucketId = message.bucketId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<GetBucketConfigRequest>, I>>(object: I): GetBucketConfigRequest {
    const message = createBaseGetBucketConfigRequest();
    message.bucketId = object.bucketId ?? "";
    return message;
  },
};

function createBaseGetBucketConfigResponse(): GetBucketConfigResponse {
  return { config: undefined };
}

export const GetBucketConfigResponse = {
  encode(message: GetBucketConfigResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.config !== undefined) {
      Config.encode(message.config, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetBucketConfigResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetBucketConfigResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.config = Config.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetBucketConfigResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetBucketConfigResponse | GetBucketConfigResponse[]>
      | Iterable<GetBucketConfigResponse | GetBucketConfigResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetBucketConfigResponse.encode(p).finish()];
        }
      } else {
        yield* [GetBucketConfigResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetBucketConfigResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetBucketConfigResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetBucketConfigResponse.decode(p)];
        }
      } else {
        yield* [GetBucketConfigResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): GetBucketConfigResponse {
    return { config: isSet(object.config) ? Config.fromJSON(object.config) : undefined };
  },

  toJSON(message: GetBucketConfigResponse): unknown {
    const obj: any = {};
    message.config !== undefined && (obj.config = message.config ? Config.toJSON(message.config) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<GetBucketConfigResponse>, I>>(object: I): GetBucketConfigResponse {
    const message = createBaseGetBucketConfigResponse();
    message.config = (object.config !== undefined && object.config !== null)
      ? Config.fromPartial(object.config)
      : undefined;
    return message;
  },
};

function createBaseGetBucketInfoRequest(): GetBucketInfoRequest {
  return { bucketId: "" };
}

export const GetBucketInfoRequest = {
  encode(message: GetBucketInfoRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bucketId !== "") {
      writer.uint32(10).string(message.bucketId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetBucketInfoRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetBucketInfoRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.bucketId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetBucketInfoRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetBucketInfoRequest | GetBucketInfoRequest[]>
      | Iterable<GetBucketInfoRequest | GetBucketInfoRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetBucketInfoRequest.encode(p).finish()];
        }
      } else {
        yield* [GetBucketInfoRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetBucketInfoRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetBucketInfoRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetBucketInfoRequest.decode(p)];
        }
      } else {
        yield* [GetBucketInfoRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): GetBucketInfoRequest {
    return { bucketId: isSet(object.bucketId) ? String(object.bucketId) : "" };
  },

  toJSON(message: GetBucketInfoRequest): unknown {
    const obj: any = {};
    message.bucketId !== undefined && (obj.bucketId = message.bucketId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<GetBucketInfoRequest>, I>>(object: I): GetBucketInfoRequest {
    const message = createBaseGetBucketInfoRequest();
    message.bucketId = object.bucketId ?? "";
    return message;
  },
};

function createBaseGetBucketInfoResponse(): GetBucketInfoResponse {
  return { bucketInfo: undefined };
}

export const GetBucketInfoResponse = {
  encode(message: GetBucketInfoResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bucketInfo !== undefined) {
      BucketInfo.encode(message.bucketInfo, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetBucketInfoResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetBucketInfoResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.bucketInfo = BucketInfo.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetBucketInfoResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetBucketInfoResponse | GetBucketInfoResponse[]>
      | Iterable<GetBucketInfoResponse | GetBucketInfoResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetBucketInfoResponse.encode(p).finish()];
        }
      } else {
        yield* [GetBucketInfoResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetBucketInfoResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetBucketInfoResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetBucketInfoResponse.decode(p)];
        }
      } else {
        yield* [GetBucketInfoResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): GetBucketInfoResponse {
    return { bucketInfo: isSet(object.bucketInfo) ? BucketInfo.fromJSON(object.bucketInfo) : undefined };
  },

  toJSON(message: GetBucketInfoResponse): unknown {
    const obj: any = {};
    message.bucketInfo !== undefined &&
      (obj.bucketInfo = message.bucketInfo ? BucketInfo.toJSON(message.bucketInfo) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<GetBucketInfoResponse>, I>>(object: I): GetBucketInfoResponse {
    const message = createBaseGetBucketInfoResponse();
    message.bucketInfo = (object.bucketInfo !== undefined && object.bucketInfo !== null)
      ? BucketInfo.fromPartial(object.bucketInfo)
      : undefined;
    return message;
  },
};

function createBaseListBucketInfoRequest(): ListBucketInfoRequest {
  return { bucketIdRe: "" };
}

export const ListBucketInfoRequest = {
  encode(message: ListBucketInfoRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bucketIdRe !== "") {
      writer.uint32(10).string(message.bucketIdRe);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListBucketInfoRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListBucketInfoRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.bucketIdRe = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ListBucketInfoRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListBucketInfoRequest | ListBucketInfoRequest[]>
      | Iterable<ListBucketInfoRequest | ListBucketInfoRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketInfoRequest.encode(p).finish()];
        }
      } else {
        yield* [ListBucketInfoRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListBucketInfoRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ListBucketInfoRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketInfoRequest.decode(p)];
        }
      } else {
        yield* [ListBucketInfoRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ListBucketInfoRequest {
    return { bucketIdRe: isSet(object.bucketIdRe) ? String(object.bucketIdRe) : "" };
  },

  toJSON(message: ListBucketInfoRequest): unknown {
    const obj: any = {};
    message.bucketIdRe !== undefined && (obj.bucketIdRe = message.bucketIdRe);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ListBucketInfoRequest>, I>>(object: I): ListBucketInfoRequest {
    const message = createBaseListBucketInfoRequest();
    message.bucketIdRe = object.bucketIdRe ?? "";
    return message;
  },
};

function createBaseListBucketInfoResponse(): ListBucketInfoResponse {
  return { bucketInfo: [] };
}

export const ListBucketInfoResponse = {
  encode(message: ListBucketInfoResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.bucketInfo) {
      BucketInfo.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListBucketInfoResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListBucketInfoResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.bucketInfo.push(BucketInfo.decode(reader, reader.uint32()));
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ListBucketInfoResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListBucketInfoResponse | ListBucketInfoResponse[]>
      | Iterable<ListBucketInfoResponse | ListBucketInfoResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketInfoResponse.encode(p).finish()];
        }
      } else {
        yield* [ListBucketInfoResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListBucketInfoResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ListBucketInfoResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketInfoResponse.decode(p)];
        }
      } else {
        yield* [ListBucketInfoResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ListBucketInfoResponse {
    return {
      bucketInfo: Array.isArray(object?.bucketInfo) ? object.bucketInfo.map((e: any) => BucketInfo.fromJSON(e)) : [],
    };
  },

  toJSON(message: ListBucketInfoResponse): unknown {
    const obj: any = {};
    if (message.bucketInfo) {
      obj.bucketInfo = message.bucketInfo.map((e) => e ? BucketInfo.toJSON(e) : undefined);
    } else {
      obj.bucketInfo = [];
    }
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ListBucketInfoResponse>, I>>(object: I): ListBucketInfoResponse {
    const message = createBaseListBucketInfoResponse();
    message.bucketInfo = object.bucketInfo?.map((e) => BucketInfo.fromPartial(e)) || [];
    return message;
  },
};

/** BucketStore implements the bucket storage on a ProxyVolume. */
export interface BucketStore {
  /** GetBucketConfig gets the bucket config with the highest revision for the ID. */
  GetBucketConfig(request: GetBucketConfigRequest): Promise<GetBucketConfigResponse>;
  /** ApplyBucketConfig requests to apply a bucket config to this volume only. */
  ApplyBucketConfig(request: ApplyBucketConfigRequest): Promise<ApplyBucketConfigResponse>;
  /** GetBucketInfo returns bucket information. */
  GetBucketInfo(request: GetBucketInfoRequest): Promise<GetBucketInfoResponse>;
  /** ListBucketInfo returns a list of bucket infos with an optional regex. */
  ListBucketInfo(request: ListBucketInfoRequest): Promise<ListBucketInfoResponse>;
}

export class BucketStoreClientImpl implements BucketStore {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "rpc.bucket.BucketStore";
    this.rpc = rpc;
    this.GetBucketConfig = this.GetBucketConfig.bind(this);
    this.ApplyBucketConfig = this.ApplyBucketConfig.bind(this);
    this.GetBucketInfo = this.GetBucketInfo.bind(this);
    this.ListBucketInfo = this.ListBucketInfo.bind(this);
  }
  GetBucketConfig(request: GetBucketConfigRequest): Promise<GetBucketConfigResponse> {
    const data = GetBucketConfigRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "GetBucketConfig", data);
    return promise.then((data) => GetBucketConfigResponse.decode(new _m0.Reader(data)));
  }

  ApplyBucketConfig(request: ApplyBucketConfigRequest): Promise<ApplyBucketConfigResponse> {
    const data = ApplyBucketConfigRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "ApplyBucketConfig", data);
    return promise.then((data) => ApplyBucketConfigResponse.decode(new _m0.Reader(data)));
  }

  GetBucketInfo(request: GetBucketInfoRequest): Promise<GetBucketInfoResponse> {
    const data = GetBucketInfoRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "GetBucketInfo", data);
    return promise.then((data) => GetBucketInfoResponse.decode(new _m0.Reader(data)));
  }

  ListBucketInfo(request: ListBucketInfoRequest): Promise<ListBucketInfoResponse> {
    const data = ListBucketInfoRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "ListBucketInfo", data);
    return promise.then((data) => ListBucketInfoResponse.decode(new _m0.Reader(data)));
  }
}

/** BucketStore implements the bucket storage on a ProxyVolume. */
export type BucketStoreDefinition = typeof BucketStoreDefinition;
export const BucketStoreDefinition = {
  name: "BucketStore",
  fullName: "rpc.bucket.BucketStore",
  methods: {
    /** GetBucketConfig gets the bucket config with the highest revision for the ID. */
    getBucketConfig: {
      name: "GetBucketConfig",
      requestType: GetBucketConfigRequest,
      requestStream: false,
      responseType: GetBucketConfigResponse,
      responseStream: false,
      options: {},
    },
    /** ApplyBucketConfig requests to apply a bucket config to this volume only. */
    applyBucketConfig: {
      name: "ApplyBucketConfig",
      requestType: ApplyBucketConfigRequest,
      requestStream: false,
      responseType: ApplyBucketConfigResponse,
      responseStream: false,
      options: {},
    },
    /** GetBucketInfo returns bucket information. */
    getBucketInfo: {
      name: "GetBucketInfo",
      requestType: GetBucketInfoRequest,
      requestStream: false,
      responseType: GetBucketInfoResponse,
      responseStream: false,
      options: {},
    },
    /** ListBucketInfo returns a list of bucket infos with an optional regex. */
    listBucketInfo: {
      name: "ListBucketInfo",
      requestType: ListBucketInfoRequest,
      requestStream: false,
      responseType: ListBucketInfoResponse,
      responseStream: false,
      options: {},
    },
  },
} as const;

interface Rpc {
  request(service: string, method: string, data: Uint8Array): Promise<Uint8Array>;
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
