/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.dist";

/** DistManifestBundle is a set of DistManifest. */
export interface DistManifestBundle {
  /** Manifests is the list of dist manifest. */
  manifests: DistManifest[];
}

/** DistManifest is a distribution bundle manifest. */
export interface DistManifest {
  /** PlatformId is the distribution platform ID. */
  platformId: string;
}

function createBaseDistManifestBundle(): DistManifestBundle {
  return { manifests: [] };
}

export const DistManifestBundle = {
  encode(message: DistManifestBundle, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.manifests) {
      DistManifest.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DistManifestBundle {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDistManifestBundle();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.manifests.push(DistManifest.decode(reader, reader.uint32()));
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<DistManifestBundle, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DistManifestBundle | DistManifestBundle[]>
      | Iterable<DistManifestBundle | DistManifestBundle[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DistManifestBundle.encode(p).finish()];
        }
      } else {
        yield* [DistManifestBundle.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DistManifestBundle>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DistManifestBundle> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DistManifestBundle.decode(p)];
        }
      } else {
        yield* [DistManifestBundle.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): DistManifestBundle {
    return {
      manifests: Array.isArray(object?.manifests) ? object.manifests.map((e: any) => DistManifest.fromJSON(e)) : [],
    };
  },

  toJSON(message: DistManifestBundle): unknown {
    const obj: any = {};
    if (message.manifests) {
      obj.manifests = message.manifests.map((e) => e ? DistManifest.toJSON(e) : undefined);
    } else {
      obj.manifests = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<DistManifestBundle>, I>>(base?: I): DistManifestBundle {
    return DistManifestBundle.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<DistManifestBundle>, I>>(object: I): DistManifestBundle {
    const message = createBaseDistManifestBundle();
    message.manifests = object.manifests?.map((e) => DistManifest.fromPartial(e)) || [];
    return message;
  },
};

function createBaseDistManifest(): DistManifest {
  return { platformId: "" };
}

export const DistManifest = {
  encode(message: DistManifest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.platformId !== "") {
      writer.uint32(10).string(message.platformId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DistManifest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDistManifest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.platformId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<DistManifest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<DistManifest | DistManifest[]> | Iterable<DistManifest | DistManifest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DistManifest.encode(p).finish()];
        }
      } else {
        yield* [DistManifest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DistManifest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DistManifest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DistManifest.decode(p)];
        }
      } else {
        yield* [DistManifest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): DistManifest {
    return { platformId: isSet(object.platformId) ? String(object.platformId) : "" };
  },

  toJSON(message: DistManifest): unknown {
    const obj: any = {};
    message.platformId !== undefined && (obj.platformId = message.platformId);
    return obj;
  },

  create<I extends Exact<DeepPartial<DistManifest>, I>>(base?: I): DistManifest {
    return DistManifest.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<DistManifest>, I>>(object: I): DistManifest {
    const message = createBaseDistManifest();
    message.platformId = object.platformId ?? "";
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
