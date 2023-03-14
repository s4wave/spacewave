/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { ManifestMeta } from "../manifest.pb.js";

export const protobufPackage = "bldr.manifest.builder";

/** BuilderConfig is common configuration for a manifest builder routine. */
export interface BuilderConfig {
  /** ManifestMeta is the metadata of the manifest to build. */
  manifestMeta:
    | ManifestMeta
    | undefined;
  /** SourcePath is the path to the project source root. */
  sourcePath: string;
  /** DistSourcePath is the path to the bldr dist source root. */
  distSourcePath: string;
  /** WorkingPath is the path to use for codegen and working state. */
  workingPath: string;
  /** EngineId is the world engine to store the manifest. */
  engineId: string;
  /** ObjectKey is the key to store the manifest. */
  objectKey: string;
  /** LinkObjectKeys is the list of object keys to link to the manifest. */
  linkObjectKeys: string[];
  /** PeerId is the peer ID to use for world transactions. */
  peerId: string;
}

function createBaseBuilderConfig(): BuilderConfig {
  return {
    manifestMeta: undefined,
    sourcePath: "",
    distSourcePath: "",
    workingPath: "",
    engineId: "",
    objectKey: "",
    linkObjectKeys: [],
    peerId: "",
  };
}

export const BuilderConfig = {
  encode(message: BuilderConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.manifestMeta !== undefined) {
      ManifestMeta.encode(message.manifestMeta, writer.uint32(10).fork()).ldelim();
    }
    if (message.sourcePath !== "") {
      writer.uint32(18).string(message.sourcePath);
    }
    if (message.distSourcePath !== "") {
      writer.uint32(26).string(message.distSourcePath);
    }
    if (message.workingPath !== "") {
      writer.uint32(34).string(message.workingPath);
    }
    if (message.engineId !== "") {
      writer.uint32(42).string(message.engineId);
    }
    if (message.objectKey !== "") {
      writer.uint32(50).string(message.objectKey);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(58).string(v!);
    }
    if (message.peerId !== "") {
      writer.uint32(66).string(message.peerId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.manifestMeta = ManifestMeta.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.sourcePath = reader.string();
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.distSourcePath = reader.string();
          continue;
        case 4:
          if (tag != 34) {
            break;
          }

          message.workingPath = reader.string();
          continue;
        case 5:
          if (tag != 42) {
            break;
          }

          message.engineId = reader.string();
          continue;
        case 6:
          if (tag != 50) {
            break;
          }

          message.objectKey = reader.string();
          continue;
        case 7:
          if (tag != 58) {
            break;
          }

          message.linkObjectKeys.push(reader.string());
          continue;
        case 8:
          if (tag != 66) {
            break;
          }

          message.peerId = reader.string();
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
  // Transform<BuilderConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BuilderConfig | BuilderConfig[]> | Iterable<BuilderConfig | BuilderConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BuilderConfig.encode(p).finish()];
        }
      } else {
        yield* [BuilderConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BuilderConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BuilderConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BuilderConfig.decode(p)];
        }
      } else {
        yield* [BuilderConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): BuilderConfig {
    return {
      manifestMeta: isSet(object.manifestMeta) ? ManifestMeta.fromJSON(object.manifestMeta) : undefined,
      sourcePath: isSet(object.sourcePath) ? String(object.sourcePath) : "",
      distSourcePath: isSet(object.distSourcePath) ? String(object.distSourcePath) : "",
      workingPath: isSet(object.workingPath) ? String(object.workingPath) : "",
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
    };
  },

  toJSON(message: BuilderConfig): unknown {
    const obj: any = {};
    message.manifestMeta !== undefined &&
      (obj.manifestMeta = message.manifestMeta ? ManifestMeta.toJSON(message.manifestMeta) : undefined);
    message.sourcePath !== undefined && (obj.sourcePath = message.sourcePath);
    message.distSourcePath !== undefined && (obj.distSourcePath = message.distSourcePath);
    message.workingPath !== undefined && (obj.workingPath = message.workingPath);
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    if (message.linkObjectKeys) {
      obj.linkObjectKeys = message.linkObjectKeys.map((e) => e);
    } else {
      obj.linkObjectKeys = [];
    }
    message.peerId !== undefined && (obj.peerId = message.peerId);
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig>, I>>(base?: I): BuilderConfig {
    return BuilderConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<BuilderConfig>, I>>(object: I): BuilderConfig {
    const message = createBaseBuilderConfig();
    message.manifestMeta = (object.manifestMeta !== undefined && object.manifestMeta !== null)
      ? ManifestMeta.fromPartial(object.manifestMeta)
      : undefined;
    message.sourcePath = object.sourcePath ?? "";
    message.distSourcePath = object.distSourcePath ?? "";
    message.workingPath = object.workingPath ?? "";
    message.engineId = object.engineId ?? "";
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    message.peerId = object.peerId ?? "";
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
