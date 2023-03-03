/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { DistManifestBundle } from "../dist/dist.pb.js";
import { PluginManifestBundle } from "../plugin/plugin.pb.js";

export const protobufPackage = "bldr.release";

/**
 * ReleaseManifest contains a bundle of dist entrypoints and plugins.
 * The Manifest represents a specific version.
 */
export interface ReleaseManifest {
  /** Plugins contains the list of plugin manifests. */
  plugins:
    | PluginManifestBundle
    | undefined;
  /** Dists contains the list of distribution manifests. */
  dists: DistManifestBundle | undefined;
}

function createBaseReleaseManifest(): ReleaseManifest {
  return { plugins: undefined, dists: undefined };
}

export const ReleaseManifest = {
  encode(message: ReleaseManifest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.plugins !== undefined) {
      PluginManifestBundle.encode(message.plugins, writer.uint32(10).fork()).ldelim();
    }
    if (message.dists !== undefined) {
      DistManifestBundle.encode(message.dists, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReleaseManifest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReleaseManifest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.plugins = PluginManifestBundle.decode(reader, reader.uint32());
          break;
        case 2:
          message.dists = DistManifestBundle.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ReleaseManifest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ReleaseManifest | ReleaseManifest[]> | Iterable<ReleaseManifest | ReleaseManifest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseManifest.encode(p).finish()];
        }
      } else {
        yield* [ReleaseManifest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReleaseManifest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReleaseManifest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseManifest.decode(p)];
        }
      } else {
        yield* [ReleaseManifest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ReleaseManifest {
    return {
      plugins: isSet(object.plugins) ? PluginManifestBundle.fromJSON(object.plugins) : undefined,
      dists: isSet(object.dists) ? DistManifestBundle.fromJSON(object.dists) : undefined,
    };
  },

  toJSON(message: ReleaseManifest): unknown {
    const obj: any = {};
    message.plugins !== undefined &&
      (obj.plugins = message.plugins ? PluginManifestBundle.toJSON(message.plugins) : undefined);
    message.dists !== undefined && (obj.dists = message.dists ? DistManifestBundle.toJSON(message.dists) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ReleaseManifest>, I>>(base?: I): ReleaseManifest {
    return ReleaseManifest.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ReleaseManifest>, I>>(object: I): ReleaseManifest {
    const message = createBaseReleaseManifest();
    message.plugins = (object.plugins !== undefined && object.plugins !== null)
      ? PluginManifestBundle.fromPartial(object.plugins)
      : undefined;
    message.dists = (object.dists !== undefined && object.dists !== null)
      ? DistManifestBundle.fromPartial(object.dists)
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
