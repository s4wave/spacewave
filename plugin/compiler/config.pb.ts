/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "plugin.compiler";

/** Config configures the plugin compiler controller. */
export interface Config {
  /** PluginId is the plugin ID to build. */
  pluginId: string;
  /** EngineId is the world engine to store the manifest. */
  engineId: string;
  /** PluginHostKey is the plugin host object to link the manifest to. */
  pluginHostKey: string;
  /** PlatformId is the platform ID to build for. */
  platformId: string;
  /** SourcePath is the path to the project source root. */
  sourcePath: string;
  /** WorkingPath is the path to use for codegen and working state. */
  workingPath: string;
  /**
   * GoPackages is the list of Go packages to scan for controller factories.
   * Looks for package-level functions:
   *  - NewFactory(b bus.Bus) controller.Factory
   *  - BuildFactories(b bus.Bus) []controller.Factory
   */
  goPackages: string[];
  /**
   * ConfigSet is a ConfigSet to apply on plugin startup.
   * This ConfigSet is applied to the plugin bus.
   * This will be included in the plugin binary.
   */
  configSet: { [key: string]: ControllerConfig };
}

export interface Config_ConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

function createBaseConfig(): Config {
  return {
    pluginId: "",
    engineId: "",
    pluginHostKey: "",
    platformId: "",
    sourcePath: "",
    workingPath: "",
    goPackages: [],
    configSet: {},
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.engineId !== "") {
      writer.uint32(18).string(message.engineId);
    }
    if (message.pluginHostKey !== "") {
      writer.uint32(26).string(message.pluginHostKey);
    }
    if (message.platformId !== "") {
      writer.uint32(34).string(message.platformId);
    }
    if (message.sourcePath !== "") {
      writer.uint32(42).string(message.sourcePath);
    }
    if (message.workingPath !== "") {
      writer.uint32(50).string(message.workingPath);
    }
    for (const v of message.goPackages) {
      writer.uint32(58).string(v!);
    }
    Object.entries(message.configSet).forEach(([key, value]) => {
      Config_ConfigSetEntry.encode({ key: key as any, value }, writer.uint32(66).fork()).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig();
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
          message.platformId = reader.string();
          break;
        case 5:
          message.sourcePath = reader.string();
          break;
        case 6:
          message.workingPath = reader.string();
          break;
        case 7:
          message.goPackages.push(reader.string());
          break;
        case 8:
          const entry8 = Config_ConfigSetEntry.decode(reader, reader.uint32());
          if (entry8.value !== undefined) {
            message.configSet[entry8.key] = entry8.value;
          }
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()];
        }
      } else {
        yield* [Config.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)];
        }
      } else {
        yield* [Config.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      pluginId: isSet(object.pluginId) ? String(object.pluginId) : "",
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      pluginHostKey: isSet(object.pluginHostKey) ? String(object.pluginHostKey) : "",
      platformId: isSet(object.platformId) ? String(object.platformId) : "",
      sourcePath: isSet(object.sourcePath) ? String(object.sourcePath) : "",
      workingPath: isSet(object.workingPath) ? String(object.workingPath) : "",
      goPackages: Array.isArray(object?.goPackages) ? object.goPackages.map((e: any) => String(e)) : [],
      configSet: isObject(object.configSet)
        ? Object.entries(object.configSet).reduce<{ [key: string]: ControllerConfig }>((acc, [key, value]) => {
          acc[key] = ControllerConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.pluginHostKey !== undefined && (obj.pluginHostKey = message.pluginHostKey);
    message.platformId !== undefined && (obj.platformId = message.platformId);
    message.sourcePath !== undefined && (obj.sourcePath = message.sourcePath);
    message.workingPath !== undefined && (obj.workingPath = message.workingPath);
    if (message.goPackages) {
      obj.goPackages = message.goPackages.map((e) => e);
    } else {
      obj.goPackages = [];
    }
    obj.configSet = {};
    if (message.configSet) {
      Object.entries(message.configSet).forEach(([k, v]) => {
        obj.configSet[k] = ControllerConfig.toJSON(v);
      });
    }
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.pluginId = object.pluginId ?? "";
    message.engineId = object.engineId ?? "";
    message.pluginHostKey = object.pluginHostKey ?? "";
    message.platformId = object.platformId ?? "";
    message.sourcePath = object.sourcePath ?? "";
    message.workingPath = object.workingPath ?? "";
    message.goPackages = object.goPackages?.map((e) => e) || [];
    message.configSet = Object.entries(object.configSet ?? {}).reduce<{ [key: string]: ControllerConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ControllerConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    return message;
  },
};

function createBaseConfig_ConfigSetEntry(): Config_ConfigSetEntry {
  return { key: "", value: undefined };
}

export const Config_ConfigSetEntry = {
  encode(message: Config_ConfigSetEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config_ConfigSetEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig_ConfigSetEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = ControllerConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config_ConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Config_ConfigSetEntry | Config_ConfigSetEntry[]>
      | Iterable<Config_ConfigSetEntry | Config_ConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_ConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [Config_ConfigSetEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_ConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_ConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_ConfigSetEntry.decode(p)];
        }
      } else {
        yield* [Config_ConfigSetEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config_ConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: Config_ConfigSetEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config_ConfigSetEntry>, I>>(object: I): Config_ConfigSetEntry {
    const message = createBaseConfig_ConfigSetEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ControllerConfig.fromPartial(object.value)
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
