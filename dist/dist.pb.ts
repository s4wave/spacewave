/* eslint-disable */
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
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
  /**
   * DistWorldRef is the root object ref to the static assets kvfile block-backed world.
   * Contains the transform configuration.
   */
  distWorldRef:
    | ObjectRef
    | undefined;
  /** DistObjectKey is the root object key to search for manifests in the dist world. */
  distObjectKey: string;
}

function createBaseDistMeta(): DistMeta {
  return { projectId: "", platformId: "", startupPlugins: [], distWorldRef: undefined, distObjectKey: "" };
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
    if (message.distWorldRef !== undefined) {
      ObjectRef.encode(message.distWorldRef, writer.uint32(34).fork()).ldelim();
    }
    if (message.distObjectKey !== "") {
      writer.uint32(42).string(message.distObjectKey);
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
        case 4:
          if (tag !== 34) {
            break;
          }

          message.distWorldRef = ObjectRef.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.distObjectKey = reader.string();
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
      distWorldRef: isSet(object.distWorldRef) ? ObjectRef.fromJSON(object.distWorldRef) : undefined,
      distObjectKey: isSet(object.distObjectKey) ? globalThis.String(object.distObjectKey) : "",
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
    if (message.distWorldRef !== undefined) {
      obj.distWorldRef = ObjectRef.toJSON(message.distWorldRef);
    }
    if (message.distObjectKey !== "") {
      obj.distObjectKey = message.distObjectKey;
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
    message.distWorldRef = (object.distWorldRef !== undefined && object.distWorldRef !== null)
      ? ObjectRef.fromPartial(object.distWorldRef)
      : undefined;
    message.distObjectKey = object.distObjectKey ?? "";
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
