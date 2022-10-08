/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "plugin.builder";

/** PluginBuilderConfig is common configuration for a builder routine. */
export interface PluginBuilderConfig {
  /** PluginId is the plugin ID to build. */
  pluginId: string;
  /** EngineId is the world engine to store the manifest. */
  engineId: string;
  /** PluginHostKey is the plugin host object to link the manifest to. */
  pluginHostKey: string;
  /** PeerId is the peer ID to use for world transactions. */
  peerId: string;
  /** PlatformId is the platform ID to build for. */
  platformId: string;
  /** SourcePath is the path to the project source root. */
  sourcePath: string;
  /** WorkingPath is the path to use for codegen and working state. */
  workingPath: string;
}

function createBasePluginBuilderConfig(): PluginBuilderConfig {
  return { pluginId: "", engineId: "", pluginHostKey: "", peerId: "", platformId: "", sourcePath: "", workingPath: "" };
}

export const PluginBuilderConfig = {
  encode(message: PluginBuilderConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.engineId !== "") {
      writer.uint32(18).string(message.engineId);
    }
    if (message.pluginHostKey !== "") {
      writer.uint32(26).string(message.pluginHostKey);
    }
    if (message.peerId !== "") {
      writer.uint32(34).string(message.peerId);
    }
    if (message.platformId !== "") {
      writer.uint32(42).string(message.platformId);
    }
    if (message.sourcePath !== "") {
      writer.uint32(50).string(message.sourcePath);
    }
    if (message.workingPath !== "") {
      writer.uint32(58).string(message.workingPath);
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
          message.pluginId = reader.string();
          break;
        case 2:
          message.engineId = reader.string();
          break;
        case 3:
          message.pluginHostKey = reader.string();
          break;
        case 4:
          message.peerId = reader.string();
          break;
        case 5:
          message.platformId = reader.string();
          break;
        case 6:
          message.sourcePath = reader.string();
          break;
        case 7:
          message.workingPath = reader.string();
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
      pluginId: isSet(object.pluginId) ? String(object.pluginId) : "",
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      pluginHostKey: isSet(object.pluginHostKey) ? String(object.pluginHostKey) : "",
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      platformId: isSet(object.platformId) ? String(object.platformId) : "",
      sourcePath: isSet(object.sourcePath) ? String(object.sourcePath) : "",
      workingPath: isSet(object.workingPath) ? String(object.workingPath) : "",
    };
  },

  toJSON(message: PluginBuilderConfig): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.pluginHostKey !== undefined && (obj.pluginHostKey = message.pluginHostKey);
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.platformId !== undefined && (obj.platformId = message.platformId);
    message.sourcePath !== undefined && (obj.sourcePath = message.sourcePath);
    message.workingPath !== undefined && (obj.workingPath = message.workingPath);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<PluginBuilderConfig>, I>>(object: I): PluginBuilderConfig {
    const message = createBasePluginBuilderConfig();
    message.pluginId = object.pluginId ?? "";
    message.engineId = object.engineId ?? "";
    message.pluginHostKey = object.pluginHostKey ?? "";
    message.peerId = object.peerId ?? "";
    message.platformId = object.platformId ?? "";
    message.sourcePath = object.sourcePath ?? "";
    message.workingPath = object.workingPath ?? "";
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
