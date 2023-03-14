/* eslint-disable */
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { ManifestRef } from "../manifest.pb.js";

export const protobufPackage = "bldr.manifest.world";

/** StoreManifestOp stores a Manifest to an object key. */
export interface StoreManifestOp {
  /** ObjectKey is the object key to set. */
  objectKey: string;
  /** LinkObjectKeys is the list of keys to link from with the <manifest> predicate. */
  linkObjectKeys: string[];
  /** ManifestRef is the manifest reference. */
  manifestRef: ManifestRef | undefined;
}

/**
 * ExtractManifestBundleOp stores a ManifestBundle to an object key.
 * Extracts the manifests to Manifest objects with <manifest> links.
 */
export interface ExtractManifestBundleOp {
  /** ObjectKey is the object key to set. */
  objectKey: string;
  /** LinkObjectKeys is the list of keys to link from with the <manifest> predicate. */
  linkObjectKeys: string[];
  /** ManifestBundle is the root reference to the ManifestBundle. */
  manifestBundle: ObjectRef | undefined;
}

function createBaseStoreManifestOp(): StoreManifestOp {
  return { objectKey: "", linkObjectKeys: [], manifestRef: undefined };
}

export const StoreManifestOp = {
  encode(message: StoreManifestOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(18).string(v!);
    }
    if (message.manifestRef !== undefined) {
      ManifestRef.encode(message.manifestRef, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StoreManifestOp {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStoreManifestOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.objectKey = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.linkObjectKeys.push(reader.string());
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.manifestRef = ManifestRef.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<StoreManifestOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<StoreManifestOp | StoreManifestOp[]> | Iterable<StoreManifestOp | StoreManifestOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StoreManifestOp.encode(p).finish()];
        }
      } else {
        yield* [StoreManifestOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, StoreManifestOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<StoreManifestOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StoreManifestOp.decode(p)];
        }
      } else {
        yield* [StoreManifestOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): StoreManifestOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
      manifestRef: isSet(object.manifestRef) ? ManifestRef.fromJSON(object.manifestRef) : undefined,
    };
  },

  toJSON(message: StoreManifestOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    if (message.linkObjectKeys) {
      obj.linkObjectKeys = message.linkObjectKeys.map((e) => e);
    } else {
      obj.linkObjectKeys = [];
    }
    message.manifestRef !== undefined &&
      (obj.manifestRef = message.manifestRef ? ManifestRef.toJSON(message.manifestRef) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<StoreManifestOp>, I>>(base?: I): StoreManifestOp {
    return StoreManifestOp.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<StoreManifestOp>, I>>(object: I): StoreManifestOp {
    const message = createBaseStoreManifestOp();
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    message.manifestRef = (object.manifestRef !== undefined && object.manifestRef !== null)
      ? ManifestRef.fromPartial(object.manifestRef)
      : undefined;
    return message;
  },
};

function createBaseExtractManifestBundleOp(): ExtractManifestBundleOp {
  return { objectKey: "", linkObjectKeys: [], manifestBundle: undefined };
}

export const ExtractManifestBundleOp = {
  encode(message: ExtractManifestBundleOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(18).string(v!);
    }
    if (message.manifestBundle !== undefined) {
      ObjectRef.encode(message.manifestBundle, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ExtractManifestBundleOp {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseExtractManifestBundleOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.objectKey = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.linkObjectKeys.push(reader.string());
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.manifestBundle = ObjectRef.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ExtractManifestBundleOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ExtractManifestBundleOp | ExtractManifestBundleOp[]>
      | Iterable<ExtractManifestBundleOp | ExtractManifestBundleOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExtractManifestBundleOp.encode(p).finish()];
        }
      } else {
        yield* [ExtractManifestBundleOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ExtractManifestBundleOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ExtractManifestBundleOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExtractManifestBundleOp.decode(p)];
        }
      } else {
        yield* [ExtractManifestBundleOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ExtractManifestBundleOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
      manifestBundle: isSet(object.manifestBundle) ? ObjectRef.fromJSON(object.manifestBundle) : undefined,
    };
  },

  toJSON(message: ExtractManifestBundleOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    if (message.linkObjectKeys) {
      obj.linkObjectKeys = message.linkObjectKeys.map((e) => e);
    } else {
      obj.linkObjectKeys = [];
    }
    message.manifestBundle !== undefined &&
      (obj.manifestBundle = message.manifestBundle ? ObjectRef.toJSON(message.manifestBundle) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ExtractManifestBundleOp>, I>>(base?: I): ExtractManifestBundleOp {
    return ExtractManifestBundleOp.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ExtractManifestBundleOp>, I>>(object: I): ExtractManifestBundleOp {
    const message = createBaseExtractManifestBundleOp();
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    message.manifestBundle = (object.manifestBundle !== undefined && object.manifestBundle !== null)
      ? ObjectRef.fromPartial(object.manifestBundle)
      : undefined;
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
