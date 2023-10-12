/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.dist";

/** DistMeta is metadata embedded in a distribution entrypoint. */
export interface DistMeta {
  /**
   * ProjectId is the project identifier.
   * Must be a valid-dns-label.
   * Used to construct the application storage and dist bundle filenames.
   */
  projectId: string;
  /** PlatformId is the destination platform ID. */
  platformId: string;
  /** StartupPlugins is the list of plugins to run on startup. */
  startupPlugins: string[];
}

function createBaseDistMeta(): DistMeta {
  return { projectId: "", platformId: "", startupPlugins: [] };
}

export const DistMeta = {
  encode(message: DistMeta, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.projectId !== "") {
      writer.uint32(10).string(message.projectId);
    }
    if (message.platformId !== "") {
      writer.uint32(18).string(message.platformId);
    }
    for (const v of message.startupPlugins) {
      writer.uint32(26).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DistMeta {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDistMeta();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.projectId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.platformId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.startupPlugins.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<DistMeta, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<DistMeta | DistMeta[]> | Iterable<DistMeta | DistMeta[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [DistMeta.encode(p).finish()];
        }
      } else {
        yield* [DistMeta.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DistMeta>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DistMeta> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [DistMeta.decode(p)];
        }
      } else {
        yield* [DistMeta.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): DistMeta {
    return {
      projectId: isSet(object.projectId) ? globalThis.String(object.projectId) : "",
      platformId: isSet(object.platformId) ? globalThis.String(object.platformId) : "",
      startupPlugins: globalThis.Array.isArray(object?.startupPlugins)
        ? object.startupPlugins.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: DistMeta): unknown {
    const obj: any = {};
    if (message.projectId !== "") {
      obj.projectId = message.projectId;
    }
    if (message.platformId !== "") {
      obj.platformId = message.platformId;
    }
    if (message.startupPlugins?.length) {
      obj.startupPlugins = message.startupPlugins;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<DistMeta>, I>>(base?: I): DistMeta {
    return DistMeta.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<DistMeta>, I>>(object: I): DistMeta {
    const message = createBaseDistMeta();
    message.projectId = object.projectId ?? "";
    message.platformId = object.platformId ?? "";
    message.startupPlugins = object.startupPlugins?.map((e) => e) || [];
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends Long ? string | number | Long : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
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
