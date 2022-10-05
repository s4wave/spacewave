/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.project";

/** ProjectConfig is a bldr project configuration. */
export interface ProjectConfig {
  /** Start contains configuration for bldr start... commands. */
  start:
    | StartConfig
    | undefined;
  /**
   * Plugins contains the mapping between plugin ID and plugin fetcher.
   * The controller will be loaded when a plugin is requested via LoadPlugin.
   * The ControllerConfig must be a plugin build controller Config.
   */
  plugins: { [key: string]: ControllerConfig };
}

export interface ProjectConfig_PluginsEntry {
  key: string;
  value: ControllerConfig | undefined;
}

/** StartConfig configures the Start commands. */
export interface StartConfig {
  /** LoadPluginIds is the list of plugin IDs to load on startup. */
  loadPluginIds: string[];
  /** ConfigSetYaml is a ConfigSet yaml to apply on startup. */
  configSetYaml: string;
}

function createBaseProjectConfig(): ProjectConfig {
  return { start: undefined, plugins: {} };
}

export const ProjectConfig = {
  encode(message: ProjectConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.start !== undefined) {
      StartConfig.encode(message.start, writer.uint32(10).fork()).ldelim();
    }
    Object.entries(message.plugins).forEach(([key, value]) => {
      ProjectConfig_PluginsEntry.encode({ key: key as any, value }, writer.uint32(18).fork()).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.start = StartConfig.decode(reader, reader.uint32());
          break;
        case 2:
          const entry2 = ProjectConfig_PluginsEntry.decode(reader, reader.uint32());
          if (entry2.value !== undefined) {
            message.plugins[entry2.key] = entry2.value;
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
  // Transform<ProjectConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ProjectConfig | ProjectConfig[]> | Iterable<ProjectConfig | ProjectConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig.decode(p)];
        }
      } else {
        yield* [ProjectConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig {
    return {
      start: isSet(object.start) ? StartConfig.fromJSON(object.start) : undefined,
      plugins: isObject(object.plugins)
        ? Object.entries(object.plugins).reduce<{ [key: string]: ControllerConfig }>((acc, [key, value]) => {
          acc[key] = ControllerConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: ProjectConfig): unknown {
    const obj: any = {};
    message.start !== undefined && (obj.start = message.start ? StartConfig.toJSON(message.start) : undefined);
    obj.plugins = {};
    if (message.plugins) {
      Object.entries(message.plugins).forEach(([k, v]) => {
        obj.plugins[k] = ControllerConfig.toJSON(v);
      });
    }
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig>, I>>(object: I): ProjectConfig {
    const message = createBaseProjectConfig();
    message.start = (object.start !== undefined && object.start !== null)
      ? StartConfig.fromPartial(object.start)
      : undefined;
    message.plugins = Object.entries(object.plugins ?? {}).reduce<{ [key: string]: ControllerConfig }>(
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

function createBaseProjectConfig_PluginsEntry(): ProjectConfig_PluginsEntry {
  return { key: "", value: undefined };
}

export const ProjectConfig_PluginsEntry = {
  encode(message: ProjectConfig_PluginsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig_PluginsEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_PluginsEntry();
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
  // Transform<ProjectConfig_PluginsEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ProjectConfig_PluginsEntry | ProjectConfig_PluginsEntry[]>
      | Iterable<ProjectConfig_PluginsEntry | ProjectConfig_PluginsEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_PluginsEntry.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig_PluginsEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig_PluginsEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig_PluginsEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_PluginsEntry.decode(p)];
        }
      } else {
        yield* [ProjectConfig_PluginsEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig_PluginsEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_PluginsEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_PluginsEntry>, I>>(object: I): ProjectConfig_PluginsEntry {
    const message = createBaseProjectConfig_PluginsEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ControllerConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseStartConfig(): StartConfig {
  return { loadPluginIds: [], configSetYaml: "" };
}

export const StartConfig = {
  encode(message: StartConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.loadPluginIds) {
      writer.uint32(10).string(v!);
    }
    if (message.configSetYaml !== "") {
      writer.uint32(18).string(message.configSetYaml);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StartConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStartConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.loadPluginIds.push(reader.string());
          break;
        case 2:
          message.configSetYaml = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<StartConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<StartConfig | StartConfig[]> | Iterable<StartConfig | StartConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StartConfig.encode(p).finish()];
        }
      } else {
        yield* [StartConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, StartConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<StartConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StartConfig.decode(p)];
        }
      } else {
        yield* [StartConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): StartConfig {
    return {
      loadPluginIds: Array.isArray(object?.loadPluginIds) ? object.loadPluginIds.map((e: any) => String(e)) : [],
      configSetYaml: isSet(object.configSetYaml) ? String(object.configSetYaml) : "",
    };
  },

  toJSON(message: StartConfig): unknown {
    const obj: any = {};
    if (message.loadPluginIds) {
      obj.loadPluginIds = message.loadPluginIds.map((e) => e);
    } else {
      obj.loadPluginIds = [];
    }
    message.configSetYaml !== undefined && (obj.configSetYaml = message.configSetYaml);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<StartConfig>, I>>(object: I): StartConfig {
    const message = createBaseStartConfig();
    message.loadPluginIds = object.loadPluginIds?.map((e) => e) || [];
    message.configSetYaml = object.configSetYaml ?? "";
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
