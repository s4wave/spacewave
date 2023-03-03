/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { PluginManifestMeta } from "../plugin.pb.js";

export const protobufPackage = "bldr.plugin.builder";

/** PluginBuilderConfig is common configuration for a builder routine. */
export interface PluginBuilderConfig {
  /** PluginManifestMeta is the plugin manifest metadata. */
  pluginManifestMeta:
    | PluginManifestMeta
    | undefined;
  /** SourcePath is the path to the project source root. */
  sourcePath: string;
  /** DistSourcePath is the path to the bldr dist source root. */
  distSourcePath: string;
  /** WorkingPath is the path to use for codegen and working state. */
  workingPath: string;
  /** EngineId is the world engine to store the manifest. */
  engineId: string;
  /** ObjectKey is the key to store the plugin manifest. */
  objectKey: string;
  /** LinkObjectKeys is the list of object keys to link to the manifest. */
  linkObjectKeys: string[];
  /** PeerId is the peer ID to use for world transactions. */
  peerId: string;
  /**
   * DisableWatch disables watching for changes in source files.
   * If unset, watches source files for changes to trigger rebuild.
   */
  disableWatch: boolean;
}

function createBasePluginBuilderConfig(): PluginBuilderConfig {
  return {
    pluginManifestMeta: undefined,
    sourcePath: "",
    distSourcePath: "",
    workingPath: "",
    engineId: "",
    objectKey: "",
    linkObjectKeys: [],
    peerId: "",
    disableWatch: false,
  };
}

export const PluginBuilderConfig = {
  encode(message: PluginBuilderConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginManifestMeta !== undefined) {
      PluginManifestMeta.encode(message.pluginManifestMeta, writer.uint32(10).fork()).ldelim();
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
    if (message.disableWatch === true) {
      writer.uint32(72).bool(message.disableWatch);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginBuilderConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginBuilderConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginManifestMeta = PluginManifestMeta.decode(reader, reader.uint32());
          break;
        case 2:
          message.sourcePath = reader.string();
          break;
        case 3:
          message.distSourcePath = reader.string();
          break;
        case 4:
          message.workingPath = reader.string();
          break;
        case 5:
          message.engineId = reader.string();
          break;
        case 6:
          message.objectKey = reader.string();
          break;
        case 7:
          message.linkObjectKeys.push(reader.string());
          break;
        case 8:
          message.peerId = reader.string();
          break;
        case 9:
          message.disableWatch = reader.bool();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginBuilderConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PluginBuilderConfig | PluginBuilderConfig[]>
      | Iterable<PluginBuilderConfig | PluginBuilderConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginBuilderConfig.encode(p).finish()];
        }
      } else {
        yield* [PluginBuilderConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginBuilderConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginBuilderConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginBuilderConfig.decode(p)];
        }
      } else {
        yield* [PluginBuilderConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PluginBuilderConfig {
    return {
      pluginManifestMeta: isSet(object.pluginManifestMeta)
        ? PluginManifestMeta.fromJSON(object.pluginManifestMeta)
        : undefined,
      sourcePath: isSet(object.sourcePath) ? String(object.sourcePath) : "",
      distSourcePath: isSet(object.distSourcePath) ? String(object.distSourcePath) : "",
      workingPath: isSet(object.workingPath) ? String(object.workingPath) : "",
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      disableWatch: isSet(object.disableWatch) ? Boolean(object.disableWatch) : false,
    };
  },

  toJSON(message: PluginBuilderConfig): unknown {
    const obj: any = {};
    message.pluginManifestMeta !== undefined &&
      (obj.pluginManifestMeta = message.pluginManifestMeta
        ? PluginManifestMeta.toJSON(message.pluginManifestMeta)
        : undefined);
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
    message.disableWatch !== undefined && (obj.disableWatch = message.disableWatch);
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginBuilderConfig>, I>>(base?: I): PluginBuilderConfig {
    return PluginBuilderConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PluginBuilderConfig>, I>>(object: I): PluginBuilderConfig {
    const message = createBasePluginBuilderConfig();
    message.pluginManifestMeta = (object.pluginManifestMeta !== undefined && object.pluginManifestMeta !== null)
      ? PluginManifestMeta.fromPartial(object.pluginManifestMeta)
      : undefined;
    message.sourcePath = object.sourcePath ?? "";
    message.distSourcePath = object.distSourcePath ?? "";
    message.workingPath = object.workingPath ?? "";
    message.engineId = object.engineId ?? "";
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    message.peerId = object.peerId ?? "";
    message.disableWatch = object.disableWatch ?? false;
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
