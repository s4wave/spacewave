/* eslint-disable */
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { PluginManifestMeta } from "../plugin.pb.js";

export const protobufPackage = "plugin.host";

/** StorePluginManifestOp stores a PluginManifest to an object key. */
export interface StorePluginManifestOp {
  /** ObjectKey is the object key to set. */
  objectKey: string;
  /** LinkObjectKeys is the list of keys to link from with the <plugin> predicate. */
  linkObjectKeys: string[];
  /**
   * PluginManifestMeta is the plugin manifest metadata.
   * If different from the meta field in the linked manifest, returns an error.
   * Can be unset to skip checking the meta field.
   */
  pluginManifestMeta:
    | PluginManifestMeta
    | undefined;
  /** PluginManifest is the root reference to the PluginManifest. */
  pluginManifest: ObjectRef | undefined;
}

/**
 * ExtractPluginManifestBundleOp stores a PluginManifestBundle to an object key.
 * Extracts the plugin manifests to PluginManifest objects with <plugin> links.
 */
export interface ExtractPluginManifestBundleOp {
  /** ObjectKey is the object key to set. */
  objectKey: string;
  /** LinkObjectKeys is the list of keys to link from with the <plugin> predicate. */
  linkObjectKeys: string[];
  /** PluginManifestBundle is the root reference to the PluginManifestBundle. */
  pluginManifestBundle: ObjectRef | undefined;
}

function createBaseStorePluginManifestOp(): StorePluginManifestOp {
  return { objectKey: "", linkObjectKeys: [], pluginManifestMeta: undefined, pluginManifest: undefined };
}

export const StorePluginManifestOp = {
  encode(message: StorePluginManifestOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(18).string(v!);
    }
    if (message.pluginManifestMeta !== undefined) {
      PluginManifestMeta.encode(message.pluginManifestMeta, writer.uint32(26).fork()).ldelim();
    }
    if (message.pluginManifest !== undefined) {
      ObjectRef.encode(message.pluginManifest, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StorePluginManifestOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStorePluginManifestOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.linkObjectKeys.push(reader.string());
          break;
        case 3:
          message.pluginManifestMeta = PluginManifestMeta.decode(reader, reader.uint32());
          break;
        case 4:
          message.pluginManifest = ObjectRef.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<StorePluginManifestOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<StorePluginManifestOp | StorePluginManifestOp[]>
      | Iterable<StorePluginManifestOp | StorePluginManifestOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StorePluginManifestOp.encode(p).finish()];
        }
      } else {
        yield* [StorePluginManifestOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, StorePluginManifestOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<StorePluginManifestOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StorePluginManifestOp.decode(p)];
        }
      } else {
        yield* [StorePluginManifestOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): StorePluginManifestOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
      pluginManifestMeta: isSet(object.pluginManifestMeta)
        ? PluginManifestMeta.fromJSON(object.pluginManifestMeta)
        : undefined,
      pluginManifest: isSet(object.pluginManifest) ? ObjectRef.fromJSON(object.pluginManifest) : undefined,
    };
  },

  toJSON(message: StorePluginManifestOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    if (message.linkObjectKeys) {
      obj.linkObjectKeys = message.linkObjectKeys.map((e) => e);
    } else {
      obj.linkObjectKeys = [];
    }
    message.pluginManifestMeta !== undefined &&
      (obj.pluginManifestMeta = message.pluginManifestMeta
        ? PluginManifestMeta.toJSON(message.pluginManifestMeta)
        : undefined);
    message.pluginManifest !== undefined &&
      (obj.pluginManifest = message.pluginManifest ? ObjectRef.toJSON(message.pluginManifest) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<StorePluginManifestOp>, I>>(base?: I): StorePluginManifestOp {
    return StorePluginManifestOp.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<StorePluginManifestOp>, I>>(object: I): StorePluginManifestOp {
    const message = createBaseStorePluginManifestOp();
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    message.pluginManifestMeta = (object.pluginManifestMeta !== undefined && object.pluginManifestMeta !== null)
      ? PluginManifestMeta.fromPartial(object.pluginManifestMeta)
      : undefined;
    message.pluginManifest = (object.pluginManifest !== undefined && object.pluginManifest !== null)
      ? ObjectRef.fromPartial(object.pluginManifest)
      : undefined;
    return message;
  },
};

function createBaseExtractPluginManifestBundleOp(): ExtractPluginManifestBundleOp {
  return { objectKey: "", linkObjectKeys: [], pluginManifestBundle: undefined };
}

export const ExtractPluginManifestBundleOp = {
  encode(message: ExtractPluginManifestBundleOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(18).string(v!);
    }
    if (message.pluginManifestBundle !== undefined) {
      ObjectRef.encode(message.pluginManifestBundle, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ExtractPluginManifestBundleOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseExtractPluginManifestBundleOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.linkObjectKeys.push(reader.string());
          break;
        case 3:
          message.pluginManifestBundle = ObjectRef.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ExtractPluginManifestBundleOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ExtractPluginManifestBundleOp | ExtractPluginManifestBundleOp[]>
      | Iterable<ExtractPluginManifestBundleOp | ExtractPluginManifestBundleOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExtractPluginManifestBundleOp.encode(p).finish()];
        }
      } else {
        yield* [ExtractPluginManifestBundleOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ExtractPluginManifestBundleOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ExtractPluginManifestBundleOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExtractPluginManifestBundleOp.decode(p)];
        }
      } else {
        yield* [ExtractPluginManifestBundleOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ExtractPluginManifestBundleOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
      pluginManifestBundle: isSet(object.pluginManifestBundle)
        ? ObjectRef.fromJSON(object.pluginManifestBundle)
        : undefined,
    };
  },

  toJSON(message: ExtractPluginManifestBundleOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    if (message.linkObjectKeys) {
      obj.linkObjectKeys = message.linkObjectKeys.map((e) => e);
    } else {
      obj.linkObjectKeys = [];
    }
    message.pluginManifestBundle !== undefined &&
      (obj.pluginManifestBundle = message.pluginManifestBundle
        ? ObjectRef.toJSON(message.pluginManifestBundle)
        : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ExtractPluginManifestBundleOp>, I>>(base?: I): ExtractPluginManifestBundleOp {
    return ExtractPluginManifestBundleOp.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ExtractPluginManifestBundleOp>, I>>(
    object: I,
  ): ExtractPluginManifestBundleOp {
    const message = createBaseExtractPluginManifestBundleOp();
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    message.pluginManifestBundle = (object.pluginManifestBundle !== undefined && object.pluginManifestBundle !== null)
      ? ObjectRef.fromPartial(object.pluginManifestBundle)
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
