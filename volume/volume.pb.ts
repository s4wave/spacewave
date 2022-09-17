/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { Info } from "../../controllerbus/controller/controller.pb.js";
import { BucketInfo } from "../bucket/bucket.pb.js";

export const protobufPackage = "volume";

/** VolumeInfo contains basic information about a volume. */
export interface VolumeInfo {
  /** VolumeId is the volume ID as determined by the controller. */
  volumeId: string;
  /** PeerId is the peer ID of the volume. */
  peerId: string;
  /** PeerPub is the pem public key of the volume. */
  peerPub: string;
  /** ControllerInfo is information about the volume controller. */
  controllerInfo: Info | undefined;
}

/** VolumeBucketInfo is information about a bucket in a volume. */
export interface VolumeBucketInfo {
  /** BucketInfo is the bucket information. */
  bucketInfo:
    | BucketInfo
    | undefined;
  /** VolumeInfo is the volume containing the bucket instance. */
  volumeInfo: VolumeInfo | undefined;
}

/** ListBucketsRequest is a list buckets directive in proto form. */
export interface ListBucketsRequest {
  /**
   * BucketId limits information to a specific bucket.
   * Can be empty.
   */
  bucketId: string;
  /**
   * VolumeRe limits to specific volumes by regex.
   * Can be empty.
   */
  volumeRe: string;
}

function createBaseVolumeInfo(): VolumeInfo {
  return { volumeId: "", peerId: "", peerPub: "", controllerInfo: undefined };
}

export const VolumeInfo = {
  encode(message: VolumeInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.volumeId !== "") {
      writer.uint32(10).string(message.volumeId);
    }
    if (message.peerId !== "") {
      writer.uint32(18).string(message.peerId);
    }
    if (message.peerPub !== "") {
      writer.uint32(26).string(message.peerPub);
    }
    if (message.controllerInfo !== undefined) {
      Info.encode(message.controllerInfo, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): VolumeInfo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseVolumeInfo();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.volumeId = reader.string();
          break;
        case 2:
          message.peerId = reader.string();
          break;
        case 3:
          message.peerPub = reader.string();
          break;
        case 4:
          message.controllerInfo = Info.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<VolumeInfo, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<VolumeInfo | VolumeInfo[]> | Iterable<VolumeInfo | VolumeInfo[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeInfo.encode(p).finish()];
        }
      } else {
        yield* [VolumeInfo.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, VolumeInfo>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<VolumeInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeInfo.decode(p)];
        }
      } else {
        yield* [VolumeInfo.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): VolumeInfo {
    return {
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : "",
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      peerPub: isSet(object.peerPub) ? String(object.peerPub) : "",
      controllerInfo: isSet(object.controllerInfo) ? Info.fromJSON(object.controllerInfo) : undefined,
    };
  },

  toJSON(message: VolumeInfo): unknown {
    const obj: any = {};
    message.volumeId !== undefined && (obj.volumeId = message.volumeId);
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.peerPub !== undefined && (obj.peerPub = message.peerPub);
    message.controllerInfo !== undefined &&
      (obj.controllerInfo = message.controllerInfo ? Info.toJSON(message.controllerInfo) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<VolumeInfo>, I>>(object: I): VolumeInfo {
    const message = createBaseVolumeInfo();
    message.volumeId = object.volumeId ?? "";
    message.peerId = object.peerId ?? "";
    message.peerPub = object.peerPub ?? "";
    message.controllerInfo = (object.controllerInfo !== undefined && object.controllerInfo !== null)
      ? Info.fromPartial(object.controllerInfo)
      : undefined;
    return message;
  },
};

function createBaseVolumeBucketInfo(): VolumeBucketInfo {
  return { bucketInfo: undefined, volumeInfo: undefined };
}

export const VolumeBucketInfo = {
  encode(message: VolumeBucketInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bucketInfo !== undefined) {
      BucketInfo.encode(message.bucketInfo, writer.uint32(10).fork()).ldelim();
    }
    if (message.volumeInfo !== undefined) {
      VolumeInfo.encode(message.volumeInfo, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): VolumeBucketInfo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseVolumeBucketInfo();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.bucketInfo = BucketInfo.decode(reader, reader.uint32());
          break;
        case 2:
          message.volumeInfo = VolumeInfo.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<VolumeBucketInfo, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<VolumeBucketInfo | VolumeBucketInfo[]> | Iterable<VolumeBucketInfo | VolumeBucketInfo[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeBucketInfo.encode(p).finish()];
        }
      } else {
        yield* [VolumeBucketInfo.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, VolumeBucketInfo>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<VolumeBucketInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeBucketInfo.decode(p)];
        }
      } else {
        yield* [VolumeBucketInfo.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): VolumeBucketInfo {
    return {
      bucketInfo: isSet(object.bucketInfo) ? BucketInfo.fromJSON(object.bucketInfo) : undefined,
      volumeInfo: isSet(object.volumeInfo) ? VolumeInfo.fromJSON(object.volumeInfo) : undefined,
    };
  },

  toJSON(message: VolumeBucketInfo): unknown {
    const obj: any = {};
    message.bucketInfo !== undefined &&
      (obj.bucketInfo = message.bucketInfo ? BucketInfo.toJSON(message.bucketInfo) : undefined);
    message.volumeInfo !== undefined &&
      (obj.volumeInfo = message.volumeInfo ? VolumeInfo.toJSON(message.volumeInfo) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<VolumeBucketInfo>, I>>(object: I): VolumeBucketInfo {
    const message = createBaseVolumeBucketInfo();
    message.bucketInfo = (object.bucketInfo !== undefined && object.bucketInfo !== null)
      ? BucketInfo.fromPartial(object.bucketInfo)
      : undefined;
    message.volumeInfo = (object.volumeInfo !== undefined && object.volumeInfo !== null)
      ? VolumeInfo.fromPartial(object.volumeInfo)
      : undefined;
    return message;
  },
};

function createBaseListBucketsRequest(): ListBucketsRequest {
  return { bucketId: "", volumeRe: "" };
}

export const ListBucketsRequest = {
  encode(message: ListBucketsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bucketId !== "") {
      writer.uint32(10).string(message.bucketId);
    }
    if (message.volumeRe !== "") {
      writer.uint32(18).string(message.volumeRe);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListBucketsRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListBucketsRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.bucketId = reader.string();
          break;
        case 2:
          message.volumeRe = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ListBucketsRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListBucketsRequest | ListBucketsRequest[]>
      | Iterable<ListBucketsRequest | ListBucketsRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketsRequest.encode(p).finish()];
        }
      } else {
        yield* [ListBucketsRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListBucketsRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ListBucketsRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketsRequest.decode(p)];
        }
      } else {
        yield* [ListBucketsRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ListBucketsRequest {
    return {
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : "",
      volumeRe: isSet(object.volumeRe) ? String(object.volumeRe) : "",
    };
  },

  toJSON(message: ListBucketsRequest): unknown {
    const obj: any = {};
    message.bucketId !== undefined && (obj.bucketId = message.bucketId);
    message.volumeRe !== undefined && (obj.volumeRe = message.volumeRe);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ListBucketsRequest>, I>>(object: I): ListBucketsRequest {
    const message = createBaseListBucketsRequest();
    message.bucketId = object.bucketId ?? "";
    message.volumeRe = object.volumeRe ?? "";
    return message;
  },
};

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
