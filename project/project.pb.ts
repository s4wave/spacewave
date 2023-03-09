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
  /**
   * Plugins contains the mapping between plugin ID and plugin config.
   * The controller will be built when a plugin is requested via LoadPlugin.
   * The ControllerConfig must be a plugin build controller Config.
   */
  plugin: { [key: string]: PluginConfig };
  /** Dists contains the mapping between dist ID and distribution config. */
  dist: { [key: string]: DistConfig };
  /** Builds contains the list of build target configs. */
  build: { [key: string]: BuildConfig };
  /** Repositories contains destinations to publish manifests. */
  repositories: { [key: string]: RepositoryConfig };
  /**
   * Publish contains the mapping between publish ID and publish config.
   * Contains configuration for bldr publish... commands.
   */
  publish: { [key: string]: PublishConfig };
}

export interface ProjectConfig_PluginEntry {
  key: string;
  value: PluginConfig | undefined;
}

export interface ProjectConfig_DistEntry {
  key: string;
  value: DistConfig | undefined;
}

export interface ProjectConfig_BuildEntry {
  key: string;
  value: BuildConfig | undefined;
}

export interface ProjectConfig_RepositoriesEntry {
  key: string;
  value: RepositoryConfig | undefined;
}

export interface ProjectConfig_PublishEntry {
  key: string;
  value: PublishConfig | undefined;
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
   *
   * This version will be used in the devtool storage.
   */
  rev: Long;
}

/** StartConfig configures starting the program. */
export interface StartConfig {
  /** Plugins is the list of plugin IDs to load on startup. */
  plugins: string[];
}

/** DistConfig configures distributing the program. */
export interface DistConfig {
  /** EmbedPlugins is the list of plugin IDs to embed. */
  embedPlugins: string[];
  /** StartPlugins is the list of plugin IDs to load on startup. */
  startPlugins: string[];
}

/** BuildConfig configures a build target. */
export interface BuildConfig {
  /** Plugins is the list of plugin IDs to build. */
  plugins: string[];
  /** Dists is the list of dist IDs to build. */
  dists: string[];
  /**
   * PluginPlatformIds is the list of plugin platforms to build.
   * Merged with the list of plugin platform IDs from distPlatformIDs.
   */
  pluginPlatformIds: string[];
  /** DistPlatformIds is the list of dist platforms to build. */
  distPlatformIds: string[];
}

/** RepositoryConfig configures a repository config target. */
export interface RepositoryConfig {
  /**
   * HostConfigSet is a ConfigSet to apply to the devtool when releasing.
   * This ConfigSet is applied to the devtool bus.
   * Often used to mount the destination release World.
   */
  hostConfigSet: { [key: string]: ControllerConfig };
  /**
   * EngineId is the world engine id to deploy to.
   * If unset, deploys to the devtool world engine.
   */
  engineId: string;
  /**
   * ObjectKey is the object key to deploy to.
   * Deploys a BuildManifestBundle.
   */
  objectKey: string;
  /** LinkObjectKeys is the list of keys to link from with the <plugin> predicate. */
  linkObjectKeys: string[];
}

export interface RepositoryConfig_HostConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

/** PublishConfig configures a publish target. */
export interface PublishConfig {
  /** Builds is the list of build targets to build. */
  builds: string[];
  /** Repositories is the list of repositories to publish to. */
  repositories: string[];
  /**
   * ObjectKey is the object key to deploy to.
   * Deploys a BuildManifestBundle.
   * Clears any existing objects at that key linked w/ that prefix.
   */
  objectKey: string;
  /**
   * PluginStorage overrides the storage for the given list of plugins.
   * Any unset values are inherited from the PublishConfig.
   */
  pluginStorage: { [key: string]: PublishStorageConfig };
  /**
   * DistStorage overrides the storage for the given list of dists.
   * Any unset values are inherited from the PublishConfig.
   */
  distStorage: { [key: string]: PublishStorageConfig };
}

export interface PublishConfig_PluginStorageEntry {
  key: string;
  value: PublishStorageConfig | undefined;
}

export interface PublishConfig_DistStorageEntry {
  key: string;
  value: PublishStorageConfig | undefined;
}

/** PublishStorageConfig configures adjusting the storage transform config for an asset. */
export interface PublishStorageConfig {
  /**
   * PrevRef is an ObjectRef to inherit the transform config from.
   *
   * If set, we will copy the transform config from this ref.
   * If transform_config is set, it will override this value.
   *
   * If both prev_ref and transform_config are unset, uses the transform config
   * from the previous release from the existing object.
   *
   * Optional.
   */
  prevRef:
    | ObjectRef
    | undefined;
  /**
   * TransformConf is the transform configuration to use.
   *
   * If set, overrides the transform configuration in prev_release_ref.
   *
   * If both prev_release_ref and transform_config are unset, uses the transform
   * config from the existing object.
   */
  transformConf:
    | Config
    | undefined;
  /**
   * ObjectKey is the object key to deploy to.
   * If set, overrides the default key.
   */
  objectKey: string;
}

function createBaseProjectConfig(): ProjectConfig {
  return { id: "", start: undefined, plugin: {}, dist: {}, build: {}, repositories: {}, publish: {} };
}

export const ProjectConfig = {
  encode(message: ProjectConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.start !== undefined) {
      StartConfig.encode(message.start, writer.uint32(18).fork()).ldelim();
    }
    Object.entries(message.plugin).forEach(([key, value]) => {
      ProjectConfig_PluginEntry.encode({ key: key as any, value }, writer.uint32(26).fork()).ldelim();
    });
    Object.entries(message.dist).forEach(([key, value]) => {
      ProjectConfig_DistEntry.encode({ key: key as any, value }, writer.uint32(34).fork()).ldelim();
    });
    Object.entries(message.build).forEach(([key, value]) => {
      ProjectConfig_BuildEntry.encode({ key: key as any, value }, writer.uint32(42).fork()).ldelim();
    });
    Object.entries(message.repositories).forEach(([key, value]) => {
      ProjectConfig_RepositoriesEntry.encode({ key: key as any, value }, writer.uint32(50).fork()).ldelim();
    });
    Object.entries(message.publish).forEach(([key, value]) => {
      ProjectConfig_PublishEntry.encode({ key: key as any, value }, writer.uint32(58).fork()).ldelim();
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
          const entry3 = ProjectConfig_PluginEntry.decode(reader, reader.uint32());
          if (entry3.value !== undefined) {
            message.plugin[entry3.key] = entry3.value;
          }
          break;
        case 4:
          const entry4 = ProjectConfig_DistEntry.decode(reader, reader.uint32());
          if (entry4.value !== undefined) {
            message.dist[entry4.key] = entry4.value;
          }
          break;
        case 5:
          const entry5 = ProjectConfig_BuildEntry.decode(reader, reader.uint32());
          if (entry5.value !== undefined) {
            message.build[entry5.key] = entry5.value;
          }
          break;
        case 6:
          const entry6 = ProjectConfig_RepositoriesEntry.decode(reader, reader.uint32());
          if (entry6.value !== undefined) {
            message.repositories[entry6.key] = entry6.value;
          }
          break;
        case 7:
          const entry7 = ProjectConfig_PublishEntry.decode(reader, reader.uint32());
          if (entry7.value !== undefined) {
            message.publish[entry7.key] = entry7.value;
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
      plugin: isObject(object.plugin)
        ? Object.entries(object.plugin).reduce<{ [key: string]: PluginConfig }>((acc, [key, value]) => {
          acc[key] = PluginConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      dist: isObject(object.dist)
        ? Object.entries(object.dist).reduce<{ [key: string]: DistConfig }>((acc, [key, value]) => {
          acc[key] = DistConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      build: isObject(object.build)
        ? Object.entries(object.build).reduce<{ [key: string]: BuildConfig }>((acc, [key, value]) => {
          acc[key] = BuildConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      repositories: isObject(object.repositories)
        ? Object.entries(object.repositories).reduce<{ [key: string]: RepositoryConfig }>((acc, [key, value]) => {
          acc[key] = RepositoryConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      publish: isObject(object.publish)
        ? Object.entries(object.publish).reduce<{ [key: string]: PublishConfig }>((acc, [key, value]) => {
          acc[key] = PublishConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: ProjectConfig): unknown {
    const obj: any = {};
    message.id !== undefined && (obj.id = message.id);
    message.start !== undefined && (obj.start = message.start ? StartConfig.toJSON(message.start) : undefined);
    obj.plugin = {};
    if (message.plugin) {
      Object.entries(message.plugin).forEach(([k, v]) => {
        obj.plugin[k] = PluginConfig.toJSON(v);
      });
    }
    obj.dist = {};
    if (message.dist) {
      Object.entries(message.dist).forEach(([k, v]) => {
        obj.dist[k] = DistConfig.toJSON(v);
      });
    }
    obj.build = {};
    if (message.build) {
      Object.entries(message.build).forEach(([k, v]) => {
        obj.build[k] = BuildConfig.toJSON(v);
      });
    }
    obj.repositories = {};
    if (message.repositories) {
      Object.entries(message.repositories).forEach(([k, v]) => {
        obj.repositories[k] = RepositoryConfig.toJSON(v);
      });
    }
    obj.publish = {};
    if (message.publish) {
      Object.entries(message.publish).forEach(([k, v]) => {
        obj.publish[k] = PublishConfig.toJSON(v);
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
    message.plugin = Object.entries(object.plugin ?? {}).reduce<{ [key: string]: PluginConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = PluginConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.dist = Object.entries(object.dist ?? {}).reduce<{ [key: string]: DistConfig }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = DistConfig.fromPartial(value);
      }
      return acc;
    }, {});
    message.build = Object.entries(object.build ?? {}).reduce<{ [key: string]: BuildConfig }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = BuildConfig.fromPartial(value);
      }
      return acc;
    }, {});
    message.repositories = Object.entries(object.repositories ?? {}).reduce<{ [key: string]: RepositoryConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = RepositoryConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.publish = Object.entries(object.publish ?? {}).reduce<{ [key: string]: PublishConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = PublishConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    return message;
  },
};

function createBaseProjectConfig_PluginEntry(): ProjectConfig_PluginEntry {
  return { key: "", value: undefined };
}

export const ProjectConfig_PluginEntry = {
  encode(message: ProjectConfig_PluginEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      PluginConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig_PluginEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_PluginEntry();
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
  // Transform<ProjectConfig_PluginEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ProjectConfig_PluginEntry | ProjectConfig_PluginEntry[]>
      | Iterable<ProjectConfig_PluginEntry | ProjectConfig_PluginEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_PluginEntry.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig_PluginEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig_PluginEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig_PluginEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_PluginEntry.decode(p)];
        }
      } else {
        yield* [ProjectConfig_PluginEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig_PluginEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? PluginConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_PluginEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? PluginConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig_PluginEntry>, I>>(base?: I): ProjectConfig_PluginEntry {
    return ProjectConfig_PluginEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_PluginEntry>, I>>(object: I): ProjectConfig_PluginEntry {
    const message = createBaseProjectConfig_PluginEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? PluginConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseProjectConfig_DistEntry(): ProjectConfig_DistEntry {
  return { key: "", value: undefined };
}

export const ProjectConfig_DistEntry = {
  encode(message: ProjectConfig_DistEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      DistConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig_DistEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_DistEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = DistConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ProjectConfig_DistEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ProjectConfig_DistEntry | ProjectConfig_DistEntry[]>
      | Iterable<ProjectConfig_DistEntry | ProjectConfig_DistEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_DistEntry.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig_DistEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig_DistEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig_DistEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_DistEntry.decode(p)];
        }
      } else {
        yield* [ProjectConfig_DistEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig_DistEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? DistConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_DistEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? DistConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig_DistEntry>, I>>(base?: I): ProjectConfig_DistEntry {
    return ProjectConfig_DistEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_DistEntry>, I>>(object: I): ProjectConfig_DistEntry {
    const message = createBaseProjectConfig_DistEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? DistConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseProjectConfig_BuildEntry(): ProjectConfig_BuildEntry {
  return { key: "", value: undefined };
}

export const ProjectConfig_BuildEntry = {
  encode(message: ProjectConfig_BuildEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      BuildConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig_BuildEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_BuildEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = BuildConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ProjectConfig_BuildEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ProjectConfig_BuildEntry | ProjectConfig_BuildEntry[]>
      | Iterable<ProjectConfig_BuildEntry | ProjectConfig_BuildEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_BuildEntry.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig_BuildEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig_BuildEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig_BuildEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_BuildEntry.decode(p)];
        }
      } else {
        yield* [ProjectConfig_BuildEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig_BuildEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? BuildConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_BuildEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? BuildConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig_BuildEntry>, I>>(base?: I): ProjectConfig_BuildEntry {
    return ProjectConfig_BuildEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_BuildEntry>, I>>(object: I): ProjectConfig_BuildEntry {
    const message = createBaseProjectConfig_BuildEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? BuildConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseProjectConfig_RepositoriesEntry(): ProjectConfig_RepositoriesEntry {
  return { key: "", value: undefined };
}

export const ProjectConfig_RepositoriesEntry = {
  encode(message: ProjectConfig_RepositoriesEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      RepositoryConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig_RepositoriesEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_RepositoriesEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = RepositoryConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ProjectConfig_RepositoriesEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ProjectConfig_RepositoriesEntry | ProjectConfig_RepositoriesEntry[]>
      | Iterable<ProjectConfig_RepositoriesEntry | ProjectConfig_RepositoriesEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_RepositoriesEntry.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig_RepositoriesEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig_RepositoriesEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig_RepositoriesEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_RepositoriesEntry.decode(p)];
        }
      } else {
        yield* [ProjectConfig_RepositoriesEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig_RepositoriesEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? RepositoryConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_RepositoriesEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? RepositoryConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig_RepositoriesEntry>, I>>(base?: I): ProjectConfig_RepositoriesEntry {
    return ProjectConfig_RepositoriesEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_RepositoriesEntry>, I>>(
    object: I,
  ): ProjectConfig_RepositoriesEntry {
    const message = createBaseProjectConfig_RepositoriesEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? RepositoryConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseProjectConfig_PublishEntry(): ProjectConfig_PublishEntry {
  return { key: "", value: undefined };
}

export const ProjectConfig_PublishEntry = {
  encode(message: ProjectConfig_PublishEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      PublishConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig_PublishEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_PublishEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = PublishConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ProjectConfig_PublishEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ProjectConfig_PublishEntry | ProjectConfig_PublishEntry[]>
      | Iterable<ProjectConfig_PublishEntry | ProjectConfig_PublishEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_PublishEntry.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig_PublishEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig_PublishEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig_PublishEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_PublishEntry.decode(p)];
        }
      } else {
        yield* [ProjectConfig_PublishEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig_PublishEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? PublishConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_PublishEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? PublishConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig_PublishEntry>, I>>(base?: I): ProjectConfig_PublishEntry {
    return ProjectConfig_PublishEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_PublishEntry>, I>>(object: I): ProjectConfig_PublishEntry {
    const message = createBaseProjectConfig_PublishEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? PublishConfig.fromPartial(object.value)
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

function createBaseDistConfig(): DistConfig {
  return { embedPlugins: [], startPlugins: [] };
}

export const DistConfig = {
  encode(message: DistConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.embedPlugins) {
      writer.uint32(10).string(v!);
    }
    for (const v of message.startPlugins) {
      writer.uint32(18).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DistConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDistConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.embedPlugins.push(reader.string());
          break;
        case 2:
          message.startPlugins.push(reader.string());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<DistConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<DistConfig | DistConfig[]> | Iterable<DistConfig | DistConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DistConfig.encode(p).finish()];
        }
      } else {
        yield* [DistConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DistConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DistConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DistConfig.decode(p)];
        }
      } else {
        yield* [DistConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): DistConfig {
    return {
      embedPlugins: Array.isArray(object?.embedPlugins) ? object.embedPlugins.map((e: any) => String(e)) : [],
      startPlugins: Array.isArray(object?.startPlugins) ? object.startPlugins.map((e: any) => String(e)) : [],
    };
  },

  toJSON(message: DistConfig): unknown {
    const obj: any = {};
    if (message.embedPlugins) {
      obj.embedPlugins = message.embedPlugins.map((e) => e);
    } else {
      obj.embedPlugins = [];
    }
    if (message.startPlugins) {
      obj.startPlugins = message.startPlugins.map((e) => e);
    } else {
      obj.startPlugins = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<DistConfig>, I>>(base?: I): DistConfig {
    return DistConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<DistConfig>, I>>(object: I): DistConfig {
    const message = createBaseDistConfig();
    message.embedPlugins = object.embedPlugins?.map((e) => e) || [];
    message.startPlugins = object.startPlugins?.map((e) => e) || [];
    return message;
  },
};

function createBaseBuildConfig(): BuildConfig {
  return { plugins: [], dists: [], pluginPlatformIds: [], distPlatformIds: [] };
}

export const BuildConfig = {
  encode(message: BuildConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.plugins) {
      writer.uint32(10).string(v!);
    }
    for (const v of message.dists) {
      writer.uint32(18).string(v!);
    }
    for (const v of message.pluginPlatformIds) {
      writer.uint32(26).string(v!);
    }
    for (const v of message.distPlatformIds) {
      writer.uint32(34).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.plugins.push(reader.string());
          break;
        case 2:
          message.dists.push(reader.string());
          break;
        case 3:
          message.pluginPlatformIds.push(reader.string());
          break;
        case 4:
          message.distPlatformIds.push(reader.string());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<BuildConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BuildConfig | BuildConfig[]> | Iterable<BuildConfig | BuildConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BuildConfig.encode(p).finish()];
        }
      } else {
        yield* [BuildConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BuildConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BuildConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BuildConfig.decode(p)];
        }
      } else {
        yield* [BuildConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): BuildConfig {
    return {
      plugins: Array.isArray(object?.plugins) ? object.plugins.map((e: any) => String(e)) : [],
      dists: Array.isArray(object?.dists) ? object.dists.map((e: any) => String(e)) : [],
      pluginPlatformIds: Array.isArray(object?.pluginPlatformIds)
        ? object.pluginPlatformIds.map((e: any) => String(e))
        : [],
      distPlatformIds: Array.isArray(object?.distPlatformIds) ? object.distPlatformIds.map((e: any) => String(e)) : [],
    };
  },

  toJSON(message: BuildConfig): unknown {
    const obj: any = {};
    if (message.plugins) {
      obj.plugins = message.plugins.map((e) => e);
    } else {
      obj.plugins = [];
    }
    if (message.dists) {
      obj.dists = message.dists.map((e) => e);
    } else {
      obj.dists = [];
    }
    if (message.pluginPlatformIds) {
      obj.pluginPlatformIds = message.pluginPlatformIds.map((e) => e);
    } else {
      obj.pluginPlatformIds = [];
    }
    if (message.distPlatformIds) {
      obj.distPlatformIds = message.distPlatformIds.map((e) => e);
    } else {
      obj.distPlatformIds = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildConfig>, I>>(base?: I): BuildConfig {
    return BuildConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<BuildConfig>, I>>(object: I): BuildConfig {
    const message = createBaseBuildConfig();
    message.plugins = object.plugins?.map((e) => e) || [];
    message.dists = object.dists?.map((e) => e) || [];
    message.pluginPlatformIds = object.pluginPlatformIds?.map((e) => e) || [];
    message.distPlatformIds = object.distPlatformIds?.map((e) => e) || [];
    return message;
  },
};

function createBaseRepositoryConfig(): RepositoryConfig {
  return { hostConfigSet: {}, engineId: "", objectKey: "", linkObjectKeys: [] };
}

export const RepositoryConfig = {
  encode(message: RepositoryConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.hostConfigSet).forEach(([key, value]) => {
      RepositoryConfig_HostConfigSetEntry.encode({ key: key as any, value }, writer.uint32(10).fork()).ldelim();
    });
    if (message.engineId !== "") {
      writer.uint32(18).string(message.engineId);
    }
    if (message.objectKey !== "") {
      writer.uint32(26).string(message.objectKey);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(34).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RepositoryConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRepositoryConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          const entry1 = RepositoryConfig_HostConfigSetEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.hostConfigSet[entry1.key] = entry1.value;
          }
          break;
        case 2:
          message.engineId = reader.string();
          break;
        case 3:
          message.objectKey = reader.string();
          break;
        case 4:
          message.linkObjectKeys.push(reader.string());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RepositoryConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<RepositoryConfig | RepositoryConfig[]> | Iterable<RepositoryConfig | RepositoryConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RepositoryConfig.encode(p).finish()];
        }
      } else {
        yield* [RepositoryConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RepositoryConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RepositoryConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RepositoryConfig.decode(p)];
        }
      } else {
        yield* [RepositoryConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): RepositoryConfig {
    return {
      hostConfigSet: isObject(object.hostConfigSet)
        ? Object.entries(object.hostConfigSet).reduce<{ [key: string]: ControllerConfig }>((acc, [key, value]) => {
          acc[key] = ControllerConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
    };
  },

  toJSON(message: RepositoryConfig): unknown {
    const obj: any = {};
    obj.hostConfigSet = {};
    if (message.hostConfigSet) {
      Object.entries(message.hostConfigSet).forEach(([k, v]) => {
        obj.hostConfigSet[k] = ControllerConfig.toJSON(v);
      });
    }
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    if (message.linkObjectKeys) {
      obj.linkObjectKeys = message.linkObjectKeys.map((e) => e);
    } else {
      obj.linkObjectKeys = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<RepositoryConfig>, I>>(base?: I): RepositoryConfig {
    return RepositoryConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<RepositoryConfig>, I>>(object: I): RepositoryConfig {
    const message = createBaseRepositoryConfig();
    message.hostConfigSet = Object.entries(object.hostConfigSet ?? {}).reduce<{ [key: string]: ControllerConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ControllerConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.engineId = object.engineId ?? "";
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    return message;
  },
};

function createBaseRepositoryConfig_HostConfigSetEntry(): RepositoryConfig_HostConfigSetEntry {
  return { key: "", value: undefined };
}

export const RepositoryConfig_HostConfigSetEntry = {
  encode(message: RepositoryConfig_HostConfigSetEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RepositoryConfig_HostConfigSetEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRepositoryConfig_HostConfigSetEntry();
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
  // Transform<RepositoryConfig_HostConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RepositoryConfig_HostConfigSetEntry | RepositoryConfig_HostConfigSetEntry[]>
      | Iterable<RepositoryConfig_HostConfigSetEntry | RepositoryConfig_HostConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RepositoryConfig_HostConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [RepositoryConfig_HostConfigSetEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RepositoryConfig_HostConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RepositoryConfig_HostConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RepositoryConfig_HostConfigSetEntry.decode(p)];
        }
      } else {
        yield* [RepositoryConfig_HostConfigSetEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): RepositoryConfig_HostConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: RepositoryConfig_HostConfigSetEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<RepositoryConfig_HostConfigSetEntry>, I>>(
    base?: I,
  ): RepositoryConfig_HostConfigSetEntry {
    return RepositoryConfig_HostConfigSetEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<RepositoryConfig_HostConfigSetEntry>, I>>(
    object: I,
  ): RepositoryConfig_HostConfigSetEntry {
    const message = createBaseRepositoryConfig_HostConfigSetEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ControllerConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePublishConfig(): PublishConfig {
  return { builds: [], repositories: [], objectKey: "", pluginStorage: {}, distStorage: {} };
}

export const PublishConfig = {
  encode(message: PublishConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.builds) {
      writer.uint32(10).string(v!);
    }
    for (const v of message.repositories) {
      writer.uint32(18).string(v!);
    }
    if (message.objectKey !== "") {
      writer.uint32(26).string(message.objectKey);
    }
    Object.entries(message.pluginStorage).forEach(([key, value]) => {
      PublishConfig_PluginStorageEntry.encode({ key: key as any, value }, writer.uint32(34).fork()).ldelim();
    });
    Object.entries(message.distStorage).forEach(([key, value]) => {
      PublishConfig_DistStorageEntry.encode({ key: key as any, value }, writer.uint32(42).fork()).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PublishConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePublishConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.builds.push(reader.string());
          break;
        case 2:
          message.repositories.push(reader.string());
          break;
        case 3:
          message.objectKey = reader.string();
          break;
        case 4:
          const entry4 = PublishConfig_PluginStorageEntry.decode(reader, reader.uint32());
          if (entry4.value !== undefined) {
            message.pluginStorage[entry4.key] = entry4.value;
          }
          break;
        case 5:
          const entry5 = PublishConfig_DistStorageEntry.decode(reader, reader.uint32());
          if (entry5.value !== undefined) {
            message.distStorage[entry5.key] = entry5.value;
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
  // Transform<PublishConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PublishConfig | PublishConfig[]> | Iterable<PublishConfig | PublishConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishConfig.encode(p).finish()];
        }
      } else {
        yield* [PublishConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PublishConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PublishConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishConfig.decode(p)];
        }
      } else {
        yield* [PublishConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PublishConfig {
    return {
      builds: Array.isArray(object?.builds) ? object.builds.map((e: any) => String(e)) : [],
      repositories: Array.isArray(object?.repositories) ? object.repositories.map((e: any) => String(e)) : [],
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      pluginStorage: isObject(object.pluginStorage)
        ? Object.entries(object.pluginStorage).reduce<{ [key: string]: PublishStorageConfig }>((acc, [key, value]) => {
          acc[key] = PublishStorageConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      distStorage: isObject(object.distStorage)
        ? Object.entries(object.distStorage).reduce<{ [key: string]: PublishStorageConfig }>((acc, [key, value]) => {
          acc[key] = PublishStorageConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: PublishConfig): unknown {
    const obj: any = {};
    if (message.builds) {
      obj.builds = message.builds.map((e) => e);
    } else {
      obj.builds = [];
    }
    if (message.repositories) {
      obj.repositories = message.repositories.map((e) => e);
    } else {
      obj.repositories = [];
    }
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    obj.pluginStorage = {};
    if (message.pluginStorage) {
      Object.entries(message.pluginStorage).forEach(([k, v]) => {
        obj.pluginStorage[k] = PublishStorageConfig.toJSON(v);
      });
    }
    obj.distStorage = {};
    if (message.distStorage) {
      Object.entries(message.distStorage).forEach(([k, v]) => {
        obj.distStorage[k] = PublishStorageConfig.toJSON(v);
      });
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<PublishConfig>, I>>(base?: I): PublishConfig {
    return PublishConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PublishConfig>, I>>(object: I): PublishConfig {
    const message = createBasePublishConfig();
    message.builds = object.builds?.map((e) => e) || [];
    message.repositories = object.repositories?.map((e) => e) || [];
    message.objectKey = object.objectKey ?? "";
    message.pluginStorage = Object.entries(object.pluginStorage ?? {}).reduce<{ [key: string]: PublishStorageConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = PublishStorageConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.distStorage = Object.entries(object.distStorage ?? {}).reduce<{ [key: string]: PublishStorageConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = PublishStorageConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    return message;
  },
};

function createBasePublishConfig_PluginStorageEntry(): PublishConfig_PluginStorageEntry {
  return { key: "", value: undefined };
}

export const PublishConfig_PluginStorageEntry = {
  encode(message: PublishConfig_PluginStorageEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      PublishStorageConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PublishConfig_PluginStorageEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePublishConfig_PluginStorageEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = PublishStorageConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PublishConfig_PluginStorageEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PublishConfig_PluginStorageEntry | PublishConfig_PluginStorageEntry[]>
      | Iterable<PublishConfig_PluginStorageEntry | PublishConfig_PluginStorageEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishConfig_PluginStorageEntry.encode(p).finish()];
        }
      } else {
        yield* [PublishConfig_PluginStorageEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PublishConfig_PluginStorageEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PublishConfig_PluginStorageEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishConfig_PluginStorageEntry.decode(p)];
        }
      } else {
        yield* [PublishConfig_PluginStorageEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PublishConfig_PluginStorageEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? PublishStorageConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: PublishConfig_PluginStorageEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? PublishStorageConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PublishConfig_PluginStorageEntry>, I>>(
    base?: I,
  ): PublishConfig_PluginStorageEntry {
    return PublishConfig_PluginStorageEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PublishConfig_PluginStorageEntry>, I>>(
    object: I,
  ): PublishConfig_PluginStorageEntry {
    const message = createBasePublishConfig_PluginStorageEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? PublishStorageConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePublishConfig_DistStorageEntry(): PublishConfig_DistStorageEntry {
  return { key: "", value: undefined };
}

export const PublishConfig_DistStorageEntry = {
  encode(message: PublishConfig_DistStorageEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      PublishStorageConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PublishConfig_DistStorageEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePublishConfig_DistStorageEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = PublishStorageConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PublishConfig_DistStorageEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PublishConfig_DistStorageEntry | PublishConfig_DistStorageEntry[]>
      | Iterable<PublishConfig_DistStorageEntry | PublishConfig_DistStorageEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishConfig_DistStorageEntry.encode(p).finish()];
        }
      } else {
        yield* [PublishConfig_DistStorageEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PublishConfig_DistStorageEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PublishConfig_DistStorageEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishConfig_DistStorageEntry.decode(p)];
        }
      } else {
        yield* [PublishConfig_DistStorageEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PublishConfig_DistStorageEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? PublishStorageConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: PublishConfig_DistStorageEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? PublishStorageConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PublishConfig_DistStorageEntry>, I>>(base?: I): PublishConfig_DistStorageEntry {
    return PublishConfig_DistStorageEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PublishConfig_DistStorageEntry>, I>>(
    object: I,
  ): PublishConfig_DistStorageEntry {
    const message = createBasePublishConfig_DistStorageEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? PublishStorageConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePublishStorageConfig(): PublishStorageConfig {
  return { prevRef: undefined, transformConf: undefined, objectKey: "" };
}

export const PublishStorageConfig = {
  encode(message: PublishStorageConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.prevRef !== undefined) {
      ObjectRef.encode(message.prevRef, writer.uint32(10).fork()).ldelim();
    }
    if (message.transformConf !== undefined) {
      Config.encode(message.transformConf, writer.uint32(18).fork()).ldelim();
    }
    if (message.objectKey !== "") {
      writer.uint32(26).string(message.objectKey);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PublishStorageConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePublishStorageConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.prevRef = ObjectRef.decode(reader, reader.uint32());
          break;
        case 2:
          message.transformConf = Config.decode(reader, reader.uint32());
          break;
        case 3:
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
  // Transform<PublishStorageConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PublishStorageConfig | PublishStorageConfig[]>
      | Iterable<PublishStorageConfig | PublishStorageConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishStorageConfig.encode(p).finish()];
        }
      } else {
        yield* [PublishStorageConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PublishStorageConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PublishStorageConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishStorageConfig.decode(p)];
        }
      } else {
        yield* [PublishStorageConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PublishStorageConfig {
    return {
      prevRef: isSet(object.prevRef) ? ObjectRef.fromJSON(object.prevRef) : undefined,
      transformConf: isSet(object.transformConf) ? Config.fromJSON(object.transformConf) : undefined,
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
    };
  },

  toJSON(message: PublishStorageConfig): unknown {
    const obj: any = {};
    message.prevRef !== undefined && (obj.prevRef = message.prevRef ? ObjectRef.toJSON(message.prevRef) : undefined);
    message.transformConf !== undefined &&
      (obj.transformConf = message.transformConf ? Config.toJSON(message.transformConf) : undefined);
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    return obj;
  },

  create<I extends Exact<DeepPartial<PublishStorageConfig>, I>>(base?: I): PublishStorageConfig {
    return PublishStorageConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PublishStorageConfig>, I>>(object: I): PublishStorageConfig {
    const message = createBasePublishStorageConfig();
    message.prevRef = (object.prevRef !== undefined && object.prevRef !== null)
      ? ObjectRef.fromPartial(object.prevRef)
      : undefined;
    message.transformConf = (object.transformConf !== undefined && object.transformConf !== null)
      ? Config.fromPartial(object.transformConf)
      : undefined;
    message.objectKey = object.objectKey ?? "";
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
