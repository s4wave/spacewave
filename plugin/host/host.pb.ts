/* eslint-disable */
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "plugin.host";

/**
 * UpdatePluginManifest updates a PluginManifest and attaches it to the
 * associated PluginHost which fetched it (presumably on-demand from RunPlugin).
 */
export interface UpdatePluginManifestOp {
  /** PluginHostKey is the plugin host object key. */
  pluginHostKey: string;
  /** PluginId is the plugin identifier. */
  pluginId: string;
  /** PluginManifest is the root reference to the PluginManifest. */
  pluginManifest: ObjectRef | undefined;
}

function createBaseUpdatePluginManifestOp(): UpdatePluginManifestOp {
  return { pluginHostKey: "", pluginId: "", pluginManifest: undefined };
}

export const UpdatePluginManifestOp = {
  encode(message: UpdatePluginManifestOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginHostKey !== "") {
      writer.uint32(10).string(message.pluginHostKey);
    }
    if (message.pluginId !== "") {
      writer.uint32(18).string(message.pluginId);
    }
    if (message.pluginManifest !== undefined) {
      ObjectRef.encode(message.pluginManifest, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): UpdatePluginManifestOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseUpdatePluginManifestOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginHostKey = reader.string();
          break;
        case 2:
          message.pluginId = reader.string();
          break;
        case 3:
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
  // Transform<UpdatePluginManifestOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<UpdatePluginManifestOp | UpdatePluginManifestOp[]>
      | Iterable<UpdatePluginManifestOp | UpdatePluginManifestOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [UpdatePluginManifestOp.encode(p).finish()];
        }
      } else {
        yield* [UpdatePluginManifestOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, UpdatePluginManifestOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<UpdatePluginManifestOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [UpdatePluginManifestOp.decode(p)];
        }
      } else {
        yield* [UpdatePluginManifestOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): UpdatePluginManifestOp {
    return {
      pluginHostKey: isSet(object.pluginHostKey) ? String(object.pluginHostKey) : "",
      pluginId: isSet(object.pluginId) ? String(object.pluginId) : "",
      pluginManifest: isSet(object.pluginManifest) ? ObjectRef.fromJSON(object.pluginManifest) : undefined,
    };
  },

  toJSON(message: UpdatePluginManifestOp): unknown {
    const obj: any = {};
    message.pluginHostKey !== undefined && (obj.pluginHostKey = message.pluginHostKey);
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.pluginManifest !== undefined &&
      (obj.pluginManifest = message.pluginManifest ? ObjectRef.toJSON(message.pluginManifest) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<UpdatePluginManifestOp>, I>>(base?: I): UpdatePluginManifestOp {
    return UpdatePluginManifestOp.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<UpdatePluginManifestOp>, I>>(object: I): UpdatePluginManifestOp {
    const message = createBaseUpdatePluginManifestOp();
    message.pluginHostKey = object.pluginHostKey ?? "";
    message.pluginId = object.pluginId ?? "";
    message.pluginManifest = (object.pluginManifest !== undefined && object.pluginManifest !== null)
      ? ObjectRef.fromPartial(object.pluginManifest)
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
