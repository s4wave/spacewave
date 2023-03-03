/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import { Config } from "@go/github.com/aperturerobotics/hydra/block/transform/transform.pb.js";
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.project";

/** ProjectConfig is a bldr project configuration. */
export interface ProjectConfig {
  /**
   * Id is the project identifier.
   * Must be a valid-dns-label.
   * Used to construct the application storage.
   */
  id: string;
  /** Start contains configuration for bldr start... commands. */
  start:
    | StartConfig
    | undefined;
  /** Release contains configuration for bldr release... commands. */
  release:
    | ReleaseConfig
    | undefined;
  /**
   * Plugins contains the mapping between plugin ID and plugin builder.
   * The controller will be built when a plugin is requested via LoadPlugin.
   * The ControllerConfig must be a plugin build controller Config.
   */
  plugins: { [key: string]: PluginConfig };
}

export interface ProjectConfig_PluginsEntry {
  key: string;
  value: PluginConfig | undefined;
}

/** PluginConfig is a configuration for building a plugin. */
export interface PluginConfig {
  /** Builder is the configuration for the plugin builder. */
  builder:
    | ControllerConfig
    | undefined;
  /**
   * Rev is the plugin revision to build.
   *
   * The controller will always scan for the latest plugin manifest for the
   * plugin, and add 1 to the most recent revision number when building.
   *
   * However, if there is no existing manifest in the store, or if you want to
   * override the minimum revision number, this field can be used.
   */
  rev: Long;
}

/** StartConfig configures starting the program. */
export interface StartConfig {
  /** Plugins is the list of plugin IDs to load on startup. */
  plugins: string[];
}

/** ReleaseConfig configures distributing the bundle and plugins. */
export interface ReleaseConfig {
  /**
   * Targets contains the set of release targets.
   * Key is the id of the target.
   */
  targets: { [key: string]: ReleaseTargetConfig };
}

export interface ReleaseConfig_TargetsEntry {
  key: string;
  value: ReleaseTargetConfig | undefined;
}

/** ReleaseTargetConfig configures a release target. */
export interface ReleaseTargetConfig {
  /**
   * HostConfigSet is a ConfigSet to apply to the devtool when releasing.
   * This ConfigSet is applied to the devtool bus.
   * Often used to mount the destination release World.
   */
  hostConfigSet: { [key: string]: ControllerConfig };
  /**
   * Plugins is the list of plugins to release.
   * Will be bundled together into the dist binary and/or plugin manifest bundle.
   * Any unset values are inherited from the ReleaseTargetConfig.
   */
  plugins: { [key: string]: PluginReleaseConfig };
  /**
   * EngineId is the world engine id to deploy to.
   * If unset, deploys to the devtool world engine.
   */
  engineId: string;
  /**
   * PluginHostKey is the PluginHost object to deploy to.
   * Overrided by object_key if set.
   * If both plugin_host_key and object_key are unset, uses devtool plugin host key.
   */
  pluginHostKey: string;
  /**
   * ObjectKey is the object key to deploy to.
   * If set, overrides plugin_host_key.
   */
  objectKey: string;
}

export interface ReleaseTargetConfig_HostConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

export interface ReleaseTargetConfig_PluginsEntry {
  key: string;
  value: PluginReleaseConfig | undefined;
}

/** PluginReleaseConfig configures releasing a plugin. */
export interface PluginReleaseConfig {
  /**
   * PrevReleaseRef is an ObjectRef to the previous PluginRelease.
   *
   * If set, we will copy the transform config from this ref.
   * If transform_config is set, it will override this value.
   *
   * If both prev_release_ref and transform_config are unset, uses the transform
   * config from the devtool world.
   *
   * Optional.
   */
  prevReleaseRef:
    | ObjectRef
    | undefined;
  /**
   * TransformConf is the transform configuration to use.
   *
   * If set, overrides the transform configuration in prev_release_ref.
   *
   * If both prev_release_ref and transform_config are unset, uses the transform
   * config from the devtool world.
   */
  transformConf: Config | undefined;
}

function createBaseProjectConfig(): ProjectConfig {
  return { id: "", start: undefined, release: undefined, plugins: {} };
}

export const ProjectConfig = {
  encode(message: ProjectConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.start !== undefined) {
      StartConfig.encode(message.start, writer.uint32(18).fork()).ldelim();
    }
    if (message.release !== undefined) {
      ReleaseConfig.encode(message.release, writer.uint32(26).fork()).ldelim();
    }
    Object.entries(message.plugins).forEach(([key, value]) => {
      ProjectConfig_PluginsEntry.encode({ key: key as any, value }, writer.uint32(34).fork()).ldelim();
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
          message.id = reader.string();
          break;
        case 2:
          message.start = StartConfig.decode(reader, reader.uint32());
          break;
        case 3:
          message.release = ReleaseConfig.decode(reader, reader.uint32());
          break;
        case 4:
          const entry4 = ProjectConfig_PluginsEntry.decode(reader, reader.uint32());
          if (entry4.value !== undefined) {
            message.plugins[entry4.key] = entry4.value;
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
      id: isSet(object.id) ? String(object.id) : "",
      start: isSet(object.start) ? StartConfig.fromJSON(object.start) : undefined,
      release: isSet(object.release) ? ReleaseConfig.fromJSON(object.release) : undefined,
      plugins: isObject(object.plugins)
        ? Object.entries(object.plugins).reduce<{ [key: string]: PluginConfig }>((acc, [key, value]) => {
          acc[key] = PluginConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: ProjectConfig): unknown {
    const obj: any = {};
    message.id !== undefined && (obj.id = message.id);
    message.start !== undefined && (obj.start = message.start ? StartConfig.toJSON(message.start) : undefined);
    message.release !== undefined &&
      (obj.release = message.release ? ReleaseConfig.toJSON(message.release) : undefined);
    obj.plugins = {};
    if (message.plugins) {
      Object.entries(message.plugins).forEach(([k, v]) => {
        obj.plugins[k] = PluginConfig.toJSON(v);
      });
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig>, I>>(base?: I): ProjectConfig {
    return ProjectConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig>, I>>(object: I): ProjectConfig {
    const message = createBaseProjectConfig();
    message.id = object.id ?? "";
    message.start = (object.start !== undefined && object.start !== null)
      ? StartConfig.fromPartial(object.start)
      : undefined;
    message.release = (object.release !== undefined && object.release !== null)
      ? ReleaseConfig.fromPartial(object.release)
      : undefined;
    message.plugins = Object.entries(object.plugins ?? {}).reduce<{ [key: string]: PluginConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = PluginConfig.fromPartial(value);
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
      PluginConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
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
          message.value = PluginConfig.decode(reader, reader.uint32());
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
      value: isSet(object.value) ? PluginConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_PluginsEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? PluginConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig_PluginsEntry>, I>>(base?: I): ProjectConfig_PluginsEntry {
    return ProjectConfig_PluginsEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_PluginsEntry>, I>>(object: I): ProjectConfig_PluginsEntry {
    const message = createBaseProjectConfig_PluginsEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? PluginConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePluginConfig(): PluginConfig {
  return { builder: undefined, rev: Long.UZERO };
}

export const PluginConfig = {
  encode(message: PluginConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.builder !== undefined) {
      ControllerConfig.encode(message.builder, writer.uint32(10).fork()).ldelim();
    }
    if (!message.rev.isZero()) {
      writer.uint32(16).uint64(message.rev);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.builder = ControllerConfig.decode(reader, reader.uint32());
          break;
        case 2:
          message.rev = reader.uint64() as Long;
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PluginConfig | PluginConfig[]> | Iterable<PluginConfig | PluginConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginConfig.encode(p).finish()];
        }
      } else {
        yield* [PluginConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginConfig.decode(p)];
        }
      } else {
        yield* [PluginConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PluginConfig {
    return {
      builder: isSet(object.builder) ? ControllerConfig.fromJSON(object.builder) : undefined,
      rev: isSet(object.rev) ? Long.fromValue(object.rev) : Long.UZERO,
    };
  },

  toJSON(message: PluginConfig): unknown {
    const obj: any = {};
    message.builder !== undefined &&
      (obj.builder = message.builder ? ControllerConfig.toJSON(message.builder) : undefined);
    message.rev !== undefined && (obj.rev = (message.rev || Long.UZERO).toString());
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginConfig>, I>>(base?: I): PluginConfig {
    return PluginConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PluginConfig>, I>>(object: I): PluginConfig {
    const message = createBasePluginConfig();
    message.builder = (object.builder !== undefined && object.builder !== null)
      ? ControllerConfig.fromPartial(object.builder)
      : undefined;
    message.rev = (object.rev !== undefined && object.rev !== null) ? Long.fromValue(object.rev) : Long.UZERO;
    return message;
  },
};

function createBaseStartConfig(): StartConfig {
  return { plugins: [] };
}

export const StartConfig = {
  encode(message: StartConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.plugins) {
      writer.uint32(10).string(v!);
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
          message.plugins.push(reader.string());
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
    return { plugins: Array.isArray(object?.plugins) ? object.plugins.map((e: any) => String(e)) : [] };
  },

  toJSON(message: StartConfig): unknown {
    const obj: any = {};
    if (message.plugins) {
      obj.plugins = message.plugins.map((e) => e);
    } else {
      obj.plugins = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<StartConfig>, I>>(base?: I): StartConfig {
    return StartConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<StartConfig>, I>>(object: I): StartConfig {
    const message = createBaseStartConfig();
    message.plugins = object.plugins?.map((e) => e) || [];
    return message;
  },
};

function createBaseReleaseConfig(): ReleaseConfig {
  return { targets: {} };
}

export const ReleaseConfig = {
  encode(message: ReleaseConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.targets).forEach(([key, value]) => {
      ReleaseConfig_TargetsEntry.encode({ key: key as any, value }, writer.uint32(10).fork()).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReleaseConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReleaseConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          const entry1 = ReleaseConfig_TargetsEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.targets[entry1.key] = entry1.value;
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
  // Transform<ReleaseConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ReleaseConfig | ReleaseConfig[]> | Iterable<ReleaseConfig | ReleaseConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseConfig.encode(p).finish()];
        }
      } else {
        yield* [ReleaseConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReleaseConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReleaseConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseConfig.decode(p)];
        }
      } else {
        yield* [ReleaseConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ReleaseConfig {
    return {
      targets: isObject(object.targets)
        ? Object.entries(object.targets).reduce<{ [key: string]: ReleaseTargetConfig }>((acc, [key, value]) => {
          acc[key] = ReleaseTargetConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: ReleaseConfig): unknown {
    const obj: any = {};
    obj.targets = {};
    if (message.targets) {
      Object.entries(message.targets).forEach(([k, v]) => {
        obj.targets[k] = ReleaseTargetConfig.toJSON(v);
      });
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ReleaseConfig>, I>>(base?: I): ReleaseConfig {
    return ReleaseConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ReleaseConfig>, I>>(object: I): ReleaseConfig {
    const message = createBaseReleaseConfig();
    message.targets = Object.entries(object.targets ?? {}).reduce<{ [key: string]: ReleaseTargetConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ReleaseTargetConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    return message;
  },
};

function createBaseReleaseConfig_TargetsEntry(): ReleaseConfig_TargetsEntry {
  return { key: "", value: undefined };
}

export const ReleaseConfig_TargetsEntry = {
  encode(message: ReleaseConfig_TargetsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ReleaseTargetConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReleaseConfig_TargetsEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReleaseConfig_TargetsEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = ReleaseTargetConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ReleaseConfig_TargetsEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ReleaseConfig_TargetsEntry | ReleaseConfig_TargetsEntry[]>
      | Iterable<ReleaseConfig_TargetsEntry | ReleaseConfig_TargetsEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseConfig_TargetsEntry.encode(p).finish()];
        }
      } else {
        yield* [ReleaseConfig_TargetsEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReleaseConfig_TargetsEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReleaseConfig_TargetsEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseConfig_TargetsEntry.decode(p)];
        }
      } else {
        yield* [ReleaseConfig_TargetsEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ReleaseConfig_TargetsEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ReleaseTargetConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ReleaseConfig_TargetsEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ReleaseTargetConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ReleaseConfig_TargetsEntry>, I>>(base?: I): ReleaseConfig_TargetsEntry {
    return ReleaseConfig_TargetsEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ReleaseConfig_TargetsEntry>, I>>(object: I): ReleaseConfig_TargetsEntry {
    const message = createBaseReleaseConfig_TargetsEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ReleaseTargetConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseReleaseTargetConfig(): ReleaseTargetConfig {
  return { hostConfigSet: {}, plugins: {}, engineId: "", pluginHostKey: "", objectKey: "" };
}

export const ReleaseTargetConfig = {
  encode(message: ReleaseTargetConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.hostConfigSet).forEach(([key, value]) => {
      ReleaseTargetConfig_HostConfigSetEntry.encode({ key: key as any, value }, writer.uint32(10).fork()).ldelim();
    });
    Object.entries(message.plugins).forEach(([key, value]) => {
      ReleaseTargetConfig_PluginsEntry.encode({ key: key as any, value }, writer.uint32(18).fork()).ldelim();
    });
    if (message.engineId !== "") {
      writer.uint32(26).string(message.engineId);
    }
    if (message.pluginHostKey !== "") {
      writer.uint32(34).string(message.pluginHostKey);
    }
    if (message.objectKey !== "") {
      writer.uint32(42).string(message.objectKey);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReleaseTargetConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReleaseTargetConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          const entry1 = ReleaseTargetConfig_HostConfigSetEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.hostConfigSet[entry1.key] = entry1.value;
          }
          break;
        case 2:
          const entry2 = ReleaseTargetConfig_PluginsEntry.decode(reader, reader.uint32());
          if (entry2.value !== undefined) {
            message.plugins[entry2.key] = entry2.value;
          }
          break;
        case 3:
          message.engineId = reader.string();
          break;
        case 4:
          message.pluginHostKey = reader.string();
          break;
        case 5:
          message.objectKey = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ReleaseTargetConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ReleaseTargetConfig | ReleaseTargetConfig[]>
      | Iterable<ReleaseTargetConfig | ReleaseTargetConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseTargetConfig.encode(p).finish()];
        }
      } else {
        yield* [ReleaseTargetConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReleaseTargetConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReleaseTargetConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseTargetConfig.decode(p)];
        }
      } else {
        yield* [ReleaseTargetConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ReleaseTargetConfig {
    return {
      hostConfigSet: isObject(object.hostConfigSet)
        ? Object.entries(object.hostConfigSet).reduce<{ [key: string]: ControllerConfig }>((acc, [key, value]) => {
          acc[key] = ControllerConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      plugins: isObject(object.plugins)
        ? Object.entries(object.plugins).reduce<{ [key: string]: PluginReleaseConfig }>((acc, [key, value]) => {
          acc[key] = PluginReleaseConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      pluginHostKey: isSet(object.pluginHostKey) ? String(object.pluginHostKey) : "",
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
    };
  },

  toJSON(message: ReleaseTargetConfig): unknown {
    const obj: any = {};
    obj.hostConfigSet = {};
    if (message.hostConfigSet) {
      Object.entries(message.hostConfigSet).forEach(([k, v]) => {
        obj.hostConfigSet[k] = ControllerConfig.toJSON(v);
      });
    }
    obj.plugins = {};
    if (message.plugins) {
      Object.entries(message.plugins).forEach(([k, v]) => {
        obj.plugins[k] = PluginReleaseConfig.toJSON(v);
      });
    }
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.pluginHostKey !== undefined && (obj.pluginHostKey = message.pluginHostKey);
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    return obj;
  },

  create<I extends Exact<DeepPartial<ReleaseTargetConfig>, I>>(base?: I): ReleaseTargetConfig {
    return ReleaseTargetConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ReleaseTargetConfig>, I>>(object: I): ReleaseTargetConfig {
    const message = createBaseReleaseTargetConfig();
    message.hostConfigSet = Object.entries(object.hostConfigSet ?? {}).reduce<{ [key: string]: ControllerConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ControllerConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.plugins = Object.entries(object.plugins ?? {}).reduce<{ [key: string]: PluginReleaseConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = PluginReleaseConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.engineId = object.engineId ?? "";
    message.pluginHostKey = object.pluginHostKey ?? "";
    message.objectKey = object.objectKey ?? "";
    return message;
  },
};

function createBaseReleaseTargetConfig_HostConfigSetEntry(): ReleaseTargetConfig_HostConfigSetEntry {
  return { key: "", value: undefined };
}

export const ReleaseTargetConfig_HostConfigSetEntry = {
  encode(message: ReleaseTargetConfig_HostConfigSetEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReleaseTargetConfig_HostConfigSetEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReleaseTargetConfig_HostConfigSetEntry();
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
  // Transform<ReleaseTargetConfig_HostConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ReleaseTargetConfig_HostConfigSetEntry | ReleaseTargetConfig_HostConfigSetEntry[]>
      | Iterable<ReleaseTargetConfig_HostConfigSetEntry | ReleaseTargetConfig_HostConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseTargetConfig_HostConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [ReleaseTargetConfig_HostConfigSetEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReleaseTargetConfig_HostConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReleaseTargetConfig_HostConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseTargetConfig_HostConfigSetEntry.decode(p)];
        }
      } else {
        yield* [ReleaseTargetConfig_HostConfigSetEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ReleaseTargetConfig_HostConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ReleaseTargetConfig_HostConfigSetEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ReleaseTargetConfig_HostConfigSetEntry>, I>>(
    base?: I,
  ): ReleaseTargetConfig_HostConfigSetEntry {
    return ReleaseTargetConfig_HostConfigSetEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ReleaseTargetConfig_HostConfigSetEntry>, I>>(
    object: I,
  ): ReleaseTargetConfig_HostConfigSetEntry {
    const message = createBaseReleaseTargetConfig_HostConfigSetEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ControllerConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseReleaseTargetConfig_PluginsEntry(): ReleaseTargetConfig_PluginsEntry {
  return { key: "", value: undefined };
}

export const ReleaseTargetConfig_PluginsEntry = {
  encode(message: ReleaseTargetConfig_PluginsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      PluginReleaseConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReleaseTargetConfig_PluginsEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReleaseTargetConfig_PluginsEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = PluginReleaseConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ReleaseTargetConfig_PluginsEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ReleaseTargetConfig_PluginsEntry | ReleaseTargetConfig_PluginsEntry[]>
      | Iterable<ReleaseTargetConfig_PluginsEntry | ReleaseTargetConfig_PluginsEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseTargetConfig_PluginsEntry.encode(p).finish()];
        }
      } else {
        yield* [ReleaseTargetConfig_PluginsEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReleaseTargetConfig_PluginsEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReleaseTargetConfig_PluginsEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReleaseTargetConfig_PluginsEntry.decode(p)];
        }
      } else {
        yield* [ReleaseTargetConfig_PluginsEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ReleaseTargetConfig_PluginsEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? PluginReleaseConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ReleaseTargetConfig_PluginsEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? PluginReleaseConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ReleaseTargetConfig_PluginsEntry>, I>>(
    base?: I,
  ): ReleaseTargetConfig_PluginsEntry {
    return ReleaseTargetConfig_PluginsEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ReleaseTargetConfig_PluginsEntry>, I>>(
    object: I,
  ): ReleaseTargetConfig_PluginsEntry {
    const message = createBaseReleaseTargetConfig_PluginsEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? PluginReleaseConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePluginReleaseConfig(): PluginReleaseConfig {
  return { prevReleaseRef: undefined, transformConf: undefined };
}

export const PluginReleaseConfig = {
  encode(message: PluginReleaseConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.prevReleaseRef !== undefined) {
      ObjectRef.encode(message.prevReleaseRef, writer.uint32(10).fork()).ldelim();
    }
    if (message.transformConf !== undefined) {
      Config.encode(message.transformConf, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginReleaseConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginReleaseConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.prevReleaseRef = ObjectRef.decode(reader, reader.uint32());
          break;
        case 2:
          message.transformConf = Config.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginReleaseConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PluginReleaseConfig | PluginReleaseConfig[]>
      | Iterable<PluginReleaseConfig | PluginReleaseConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginReleaseConfig.encode(p).finish()];
        }
      } else {
        yield* [PluginReleaseConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginReleaseConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginReleaseConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginReleaseConfig.decode(p)];
        }
      } else {
        yield* [PluginReleaseConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PluginReleaseConfig {
    return {
      prevReleaseRef: isSet(object.prevReleaseRef) ? ObjectRef.fromJSON(object.prevReleaseRef) : undefined,
      transformConf: isSet(object.transformConf) ? Config.fromJSON(object.transformConf) : undefined,
    };
  },

  toJSON(message: PluginReleaseConfig): unknown {
    const obj: any = {};
    message.prevReleaseRef !== undefined &&
      (obj.prevReleaseRef = message.prevReleaseRef ? ObjectRef.toJSON(message.prevReleaseRef) : undefined);
    message.transformConf !== undefined &&
      (obj.transformConf = message.transformConf ? Config.toJSON(message.transformConf) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginReleaseConfig>, I>>(base?: I): PluginReleaseConfig {
    return PluginReleaseConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PluginReleaseConfig>, I>>(object: I): PluginReleaseConfig {
    const message = createBasePluginReleaseConfig();
    message.prevReleaseRef = (object.prevReleaseRef !== undefined && object.prevReleaseRef !== null)
      ? ObjectRef.fromPartial(object.prevReleaseRef)
      : undefined;
    message.transformConf = (object.transformConf !== undefined && object.transformConf !== null)
      ? Config.fromPartial(object.transformConf)
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
