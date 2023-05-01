/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import { Config } from "@go/github.com/aperturerobotics/hydra/block/transform/transform.pb.js";
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import { Timestamp } from "@go/github.com/aperturerobotics/timestamp/timestamp.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.project";

/** ProjectConfig is a bldr project configuration. */
export interface ProjectConfig {
  /**
   * Id is the project identifier.
   * Must be a valid-dns-label.
   * Used to construct the application storage and dist bundle filenames.
   * The project id can usually be overridden in the manifest compilers as well.
   */
  id: string;
  /** Start contains configuration for bldr start... commands. */
  start:
    | StartConfig
    | undefined;
  /**
   * Manifests contains the mapping between manifest ID and manifest config.
   * The controller will be built when a manifest is requested via LoadManifest.
   * The ControllerConfig must be a manifest build controller Config.
   */
  manifests: { [key: string]: ManifestConfig };
  /** Build contains the list of build target configs. */
  build: { [key: string]: BuildConfig };
  /**
   * Remotes contains definitions of destinations to build/publish to.
   * A default remote named "devtool" is automatically added.
   */
  remotes: { [key: string]: RemoteConfig };
  /**
   * Publish contains configuration for build + publish manifests to destinations.
   * Contains configuration for bldr publish... commands.
   */
  publish: { [key: string]: PublishConfig };
}

export interface ProjectConfig_ManifestsEntry {
  key: string;
  value: ManifestConfig | undefined;
}

export interface ProjectConfig_BuildEntry {
  key: string;
  value: BuildConfig | undefined;
}

export interface ProjectConfig_RemotesEntry {
  key: string;
  value: RemoteConfig | undefined;
}

export interface ProjectConfig_PublishEntry {
  key: string;
  value: PublishConfig | undefined;
}

/** ManifestConfig is a configuration for building a manifest. */
export interface ManifestConfig {
  /** Builder is the configuration for the manifest builder. */
  builder:
    | ControllerConfig
    | undefined;
  /**
   * Rev is the manifest rev to build.
   *
   * The controller will always scan for the latest manifest and add 1 to the
   * most recent rev number when building.
   *
   * However, if there is no existing manifest in the store, or if you want to
   * override the minimum rev number, this field can be used.
   *
   * This version will be used in the devtool storage.
   */
  rev: Long;
}

/** StartConfig configures starting the program. */
export interface StartConfig {
  /** Plugins is the list of plugin IDs to load on startup. */
  plugins: string[];
  /** DisableBuild disables running the manifest builders to resolve FetchManifest. */
  disableBuild: boolean;
}

/** BuildConfig configures a build target. */
export interface BuildConfig {
  /** Manifests is the list of manifest IDs to build. */
  manifests: string[];
  /** PlatformIds is the list of platforms to target. */
  platformIds: string[];
}

/** RemoteConfig configures a location where manifests and source can be stored. */
export interface RemoteConfig {
  /**
   * HostConfigSet is a ConfigSet to apply to the devtool to access the world.
   * This ConfigSet is applied to the devtool bus.
   */
  hostConfigSet: { [key: string]: ControllerConfig };
  /** EngineId is the world engine id to deploy to. */
  engineId: string;
  /**
   * PeerId is the peer id to use for world transactions.
   * The peer controller must be running with the same id.
   */
  peerId: string;
  /**
   * ObjectKey is the object key to deploy to.
   * Creates the manifests with this key as a prefix.
   */
  objectKey: string;
  /** LinkObjectKeys is a link of object keys to link to ObjectKey with <manifest>. */
  linkObjectKeys: string[];
}

export interface RemoteConfig_HostConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

/** PublishConfig configures a publish target. */
export interface PublishConfig {
  /**
   * SourceObjectKeys is the list of object keys to collect manifests from.
   * Gathers any manifest linked to with the <manifest> tag recursively.
   * If empty uses objectKey from source remote config.
   */
  sourceObjectKeys: string[];
  /**
   * Manifests is the list of manifests to publish.
   * If empty publishes all manifests matched by object keys.
   */
  manifests: string[];
  /** AllManifestRevs copies all manifest revisions instead of just the latest. */
  allManifestRevs: boolean;
  /**
   * PlatformIds is the list of platforms to target.
   * If empty publishes all platform ids matched by object keys.
   */
  platformIds: string[];
  /** Remotes is the list of remotes to publish to. */
  remotes: string[];
  /**
   * DestObjectKey is the destination object key to deploy to.
   * Creates the manifests with this key as a prefix.
   * If unset, uses the object key from the destination remote config(s).
   */
  destObjectKey: string;
  /**
   * Storage overrides the storage for all manifests.
   * Overrides the settings from the destination world.
   * If empty, uses the destination world storage transform.
   */
  storage:
    | PublishStorageConfig
    | undefined;
  /**
   * ManifestStorage maps manifest ID to publish storage configuration.
   * Overrides the settings in the storage property, per-manifest.
   * If empty, uses the destination world storage transform.
   */
  manifestStorage: { [key: string]: PublishStorageConfig };
}

export interface PublishConfig_ManifestStorageEntry {
  key: string;
  value: PublishStorageConfig | undefined;
}

/** PublishStorageConfig configures adjusting the storage transform config for an asset. */
export interface PublishStorageConfig {
  /**
   * TransformConfFromRef is an ObjectRef to inherit the transform config from.
   *
   * If set, we will copy the transform config from this ref.
   * If transform_config is set, it will override this value.
   * NOTE: only the transform field will be used, not the transformRef field.
   *
   * If both transform_from_ref and transform are unset, uses the transform
   * config config from the parent world.
   *
   * Optional.
   */
  transformConfFromRef:
    | ObjectRef
    | undefined;
  /**
   * TransformConf is the transform configuration to use.
   *
   * If set, overrides the transform configuration in transform_from_ref.
   *
   * If both transform_from_ref and transform are unset, uses the transform
   * config from the parent world.
   */
  transformConf:
    | Config
    | undefined;
  /**
   * Timestamp sets the timestamp to use for unixfs timestamps.
   * If empty, uses the current timestamp
   */
  timestamp: Timestamp | undefined;
}

function createBaseProjectConfig(): ProjectConfig {
  return { id: "", start: undefined, manifests: {}, build: {}, remotes: {}, publish: {} };
}

export const ProjectConfig = {
  encode(message: ProjectConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.start !== undefined) {
      StartConfig.encode(message.start, writer.uint32(18).fork()).ldelim();
    }
    Object.entries(message.manifests).forEach(([key, value]) => {
      ProjectConfig_ManifestsEntry.encode({ key: key as any, value }, writer.uint32(26).fork()).ldelim();
    });
    Object.entries(message.build).forEach(([key, value]) => {
      ProjectConfig_BuildEntry.encode({ key: key as any, value }, writer.uint32(34).fork()).ldelim();
    });
    Object.entries(message.remotes).forEach(([key, value]) => {
      ProjectConfig_RemotesEntry.encode({ key: key as any, value }, writer.uint32(42).fork()).ldelim();
    });
    Object.entries(message.publish).forEach(([key, value]) => {
      ProjectConfig_PublishEntry.encode({ key: key as any, value }, writer.uint32(50).fork()).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.id = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.start = StartConfig.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          const entry3 = ProjectConfig_ManifestsEntry.decode(reader, reader.uint32());
          if (entry3.value !== undefined) {
            message.manifests[entry3.key] = entry3.value;
          }
          continue;
        case 4:
          if (tag != 34) {
            break;
          }

          const entry4 = ProjectConfig_BuildEntry.decode(reader, reader.uint32());
          if (entry4.value !== undefined) {
            message.build[entry4.key] = entry4.value;
          }
          continue;
        case 5:
          if (tag != 42) {
            break;
          }

          const entry5 = ProjectConfig_RemotesEntry.decode(reader, reader.uint32());
          if (entry5.value !== undefined) {
            message.remotes[entry5.key] = entry5.value;
          }
          continue;
        case 6:
          if (tag != 50) {
            break;
          }

          const entry6 = ProjectConfig_PublishEntry.decode(reader, reader.uint32());
          if (entry6.value !== undefined) {
            message.publish[entry6.key] = entry6.value;
          }
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
      manifests: isObject(object.manifests)
        ? Object.entries(object.manifests).reduce<{ [key: string]: ManifestConfig }>((acc, [key, value]) => {
          acc[key] = ManifestConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      build: isObject(object.build)
        ? Object.entries(object.build).reduce<{ [key: string]: BuildConfig }>((acc, [key, value]) => {
          acc[key] = BuildConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      remotes: isObject(object.remotes)
        ? Object.entries(object.remotes).reduce<{ [key: string]: RemoteConfig }>((acc, [key, value]) => {
          acc[key] = RemoteConfig.fromJSON(value);
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
    obj.manifests = {};
    if (message.manifests) {
      Object.entries(message.manifests).forEach(([k, v]) => {
        obj.manifests[k] = ManifestConfig.toJSON(v);
      });
    }
    obj.build = {};
    if (message.build) {
      Object.entries(message.build).forEach(([k, v]) => {
        obj.build[k] = BuildConfig.toJSON(v);
      });
    }
    obj.remotes = {};
    if (message.remotes) {
      Object.entries(message.remotes).forEach(([k, v]) => {
        obj.remotes[k] = RemoteConfig.toJSON(v);
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
    message.manifests = Object.entries(object.manifests ?? {}).reduce<{ [key: string]: ManifestConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ManifestConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.build = Object.entries(object.build ?? {}).reduce<{ [key: string]: BuildConfig }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = BuildConfig.fromPartial(value);
      }
      return acc;
    }, {});
    message.remotes = Object.entries(object.remotes ?? {}).reduce<{ [key: string]: RemoteConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = RemoteConfig.fromPartial(value);
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

function createBaseProjectConfig_ManifestsEntry(): ProjectConfig_ManifestsEntry {
  return { key: "", value: undefined };
}

export const ProjectConfig_ManifestsEntry = {
  encode(message: ProjectConfig_ManifestsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ManifestConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig_ManifestsEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_ManifestsEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.value = ManifestConfig.decode(reader, reader.uint32());
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
  // Transform<ProjectConfig_ManifestsEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ProjectConfig_ManifestsEntry | ProjectConfig_ManifestsEntry[]>
      | Iterable<ProjectConfig_ManifestsEntry | ProjectConfig_ManifestsEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_ManifestsEntry.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig_ManifestsEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig_ManifestsEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig_ManifestsEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_ManifestsEntry.decode(p)];
        }
      } else {
        yield* [ProjectConfig_ManifestsEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig_ManifestsEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ManifestConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_ManifestsEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ManifestConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig_ManifestsEntry>, I>>(base?: I): ProjectConfig_ManifestsEntry {
    return ProjectConfig_ManifestsEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_ManifestsEntry>, I>>(object: I): ProjectConfig_ManifestsEntry {
    const message = createBaseProjectConfig_ManifestsEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ManifestConfig.fromPartial(object.value)
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_BuildEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.value = BuildConfig.decode(reader, reader.uint32());
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

function createBaseProjectConfig_RemotesEntry(): ProjectConfig_RemotesEntry {
  return { key: "", value: undefined };
}

export const ProjectConfig_RemotesEntry = {
  encode(message: ProjectConfig_RemotesEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      RemoteConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig_RemotesEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_RemotesEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.value = RemoteConfig.decode(reader, reader.uint32());
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
  // Transform<ProjectConfig_RemotesEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ProjectConfig_RemotesEntry | ProjectConfig_RemotesEntry[]>
      | Iterable<ProjectConfig_RemotesEntry | ProjectConfig_RemotesEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_RemotesEntry.encode(p).finish()];
        }
      } else {
        yield* [ProjectConfig_RemotesEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ProjectConfig_RemotesEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ProjectConfig_RemotesEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ProjectConfig_RemotesEntry.decode(p)];
        }
      } else {
        yield* [ProjectConfig_RemotesEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ProjectConfig_RemotesEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? RemoteConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ProjectConfig_RemotesEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? RemoteConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig_RemotesEntry>, I>>(base?: I): ProjectConfig_RemotesEntry {
    return ProjectConfig_RemotesEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ProjectConfig_RemotesEntry>, I>>(object: I): ProjectConfig_RemotesEntry {
    const message = createBaseProjectConfig_RemotesEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? RemoteConfig.fromPartial(object.value)
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_PublishEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.value = PublishConfig.decode(reader, reader.uint32());
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

function createBaseManifestConfig(): ManifestConfig {
  return { builder: undefined, rev: Long.UZERO };
}

export const ManifestConfig = {
  encode(message: ManifestConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.builder !== undefined) {
      ControllerConfig.encode(message.builder, writer.uint32(10).fork()).ldelim();
    }
    if (!message.rev.isZero()) {
      writer.uint32(16).uint64(message.rev);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ManifestConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseManifestConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.builder = ControllerConfig.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag != 16) {
            break;
          }

          message.rev = reader.uint64() as Long;
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
  // Transform<ManifestConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ManifestConfig | ManifestConfig[]> | Iterable<ManifestConfig | ManifestConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ManifestConfig.encode(p).finish()];
        }
      } else {
        yield* [ManifestConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ManifestConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ManifestConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ManifestConfig.decode(p)];
        }
      } else {
        yield* [ManifestConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ManifestConfig {
    return {
      builder: isSet(object.builder) ? ControllerConfig.fromJSON(object.builder) : undefined,
      rev: isSet(object.rev) ? Long.fromValue(object.rev) : Long.UZERO,
    };
  },

  toJSON(message: ManifestConfig): unknown {
    const obj: any = {};
    message.builder !== undefined &&
      (obj.builder = message.builder ? ControllerConfig.toJSON(message.builder) : undefined);
    message.rev !== undefined && (obj.rev = (message.rev || Long.UZERO).toString());
    return obj;
  },

  create<I extends Exact<DeepPartial<ManifestConfig>, I>>(base?: I): ManifestConfig {
    return ManifestConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ManifestConfig>, I>>(object: I): ManifestConfig {
    const message = createBaseManifestConfig();
    message.builder = (object.builder !== undefined && object.builder !== null)
      ? ControllerConfig.fromPartial(object.builder)
      : undefined;
    message.rev = (object.rev !== undefined && object.rev !== null) ? Long.fromValue(object.rev) : Long.UZERO;
    return message;
  },
};

function createBaseStartConfig(): StartConfig {
  return { plugins: [], disableBuild: false };
}

export const StartConfig = {
  encode(message: StartConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.plugins) {
      writer.uint32(10).string(v!);
    }
    if (message.disableBuild === true) {
      writer.uint32(16).bool(message.disableBuild);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StartConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStartConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.plugins.push(reader.string());
          continue;
        case 2:
          if (tag != 16) {
            break;
          }

          message.disableBuild = reader.bool();
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
      plugins: Array.isArray(object?.plugins) ? object.plugins.map((e: any) => String(e)) : [],
      disableBuild: isSet(object.disableBuild) ? Boolean(object.disableBuild) : false,
    };
  },

  toJSON(message: StartConfig): unknown {
    const obj: any = {};
    if (message.plugins) {
      obj.plugins = message.plugins.map((e) => e);
    } else {
      obj.plugins = [];
    }
    message.disableBuild !== undefined && (obj.disableBuild = message.disableBuild);
    return obj;
  },

  create<I extends Exact<DeepPartial<StartConfig>, I>>(base?: I): StartConfig {
    return StartConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<StartConfig>, I>>(object: I): StartConfig {
    const message = createBaseStartConfig();
    message.plugins = object.plugins?.map((e) => e) || [];
    message.disableBuild = object.disableBuild ?? false;
    return message;
  },
};

function createBaseBuildConfig(): BuildConfig {
  return { manifests: [], platformIds: [] };
}

export const BuildConfig = {
  encode(message: BuildConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.manifests) {
      writer.uint32(10).string(v!);
    }
    for (const v of message.platformIds) {
      writer.uint32(18).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.manifests.push(reader.string());
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.platformIds.push(reader.string());
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
      manifests: Array.isArray(object?.manifests) ? object.manifests.map((e: any) => String(e)) : [],
      platformIds: Array.isArray(object?.platformIds) ? object.platformIds.map((e: any) => String(e)) : [],
    };
  },

  toJSON(message: BuildConfig): unknown {
    const obj: any = {};
    if (message.manifests) {
      obj.manifests = message.manifests.map((e) => e);
    } else {
      obj.manifests = [];
    }
    if (message.platformIds) {
      obj.platformIds = message.platformIds.map((e) => e);
    } else {
      obj.platformIds = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildConfig>, I>>(base?: I): BuildConfig {
    return BuildConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<BuildConfig>, I>>(object: I): BuildConfig {
    const message = createBaseBuildConfig();
    message.manifests = object.manifests?.map((e) => e) || [];
    message.platformIds = object.platformIds?.map((e) => e) || [];
    return message;
  },
};

function createBaseRemoteConfig(): RemoteConfig {
  return { hostConfigSet: {}, engineId: "", peerId: "", objectKey: "", linkObjectKeys: [] };
}

export const RemoteConfig = {
  encode(message: RemoteConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.hostConfigSet).forEach(([key, value]) => {
      RemoteConfig_HostConfigSetEntry.encode({ key: key as any, value }, writer.uint32(10).fork()).ldelim();
    });
    if (message.engineId !== "") {
      writer.uint32(18).string(message.engineId);
    }
    if (message.peerId !== "") {
      writer.uint32(26).string(message.peerId);
    }
    if (message.objectKey !== "") {
      writer.uint32(34).string(message.objectKey);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(42).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RemoteConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRemoteConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          const entry1 = RemoteConfig_HostConfigSetEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.hostConfigSet[entry1.key] = entry1.value;
          }
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.engineId = reader.string();
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.peerId = reader.string();
          continue;
        case 4:
          if (tag != 34) {
            break;
          }

          message.objectKey = reader.string();
          continue;
        case 5:
          if (tag != 42) {
            break;
          }

          message.linkObjectKeys.push(reader.string());
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
  // Transform<RemoteConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<RemoteConfig | RemoteConfig[]> | Iterable<RemoteConfig | RemoteConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoteConfig.encode(p).finish()];
        }
      } else {
        yield* [RemoteConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoteConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoteConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoteConfig.decode(p)];
        }
      } else {
        yield* [RemoteConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): RemoteConfig {
    return {
      hostConfigSet: isObject(object.hostConfigSet)
        ? Object.entries(object.hostConfigSet).reduce<{ [key: string]: ControllerConfig }>((acc, [key, value]) => {
          acc[key] = ControllerConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
    };
  },

  toJSON(message: RemoteConfig): unknown {
    const obj: any = {};
    obj.hostConfigSet = {};
    if (message.hostConfigSet) {
      Object.entries(message.hostConfigSet).forEach(([k, v]) => {
        obj.hostConfigSet[k] = ControllerConfig.toJSON(v);
      });
    }
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    if (message.linkObjectKeys) {
      obj.linkObjectKeys = message.linkObjectKeys.map((e) => e);
    } else {
      obj.linkObjectKeys = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<RemoteConfig>, I>>(base?: I): RemoteConfig {
    return RemoteConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<RemoteConfig>, I>>(object: I): RemoteConfig {
    const message = createBaseRemoteConfig();
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
    message.peerId = object.peerId ?? "";
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    return message;
  },
};

function createBaseRemoteConfig_HostConfigSetEntry(): RemoteConfig_HostConfigSetEntry {
  return { key: "", value: undefined };
}

export const RemoteConfig_HostConfigSetEntry = {
  encode(message: RemoteConfig_HostConfigSetEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RemoteConfig_HostConfigSetEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRemoteConfig_HostConfigSetEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.value = ControllerConfig.decode(reader, reader.uint32());
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
  // Transform<RemoteConfig_HostConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoteConfig_HostConfigSetEntry | RemoteConfig_HostConfigSetEntry[]>
      | Iterable<RemoteConfig_HostConfigSetEntry | RemoteConfig_HostConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoteConfig_HostConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [RemoteConfig_HostConfigSetEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoteConfig_HostConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoteConfig_HostConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoteConfig_HostConfigSetEntry.decode(p)];
        }
      } else {
        yield* [RemoteConfig_HostConfigSetEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): RemoteConfig_HostConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: RemoteConfig_HostConfigSetEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<RemoteConfig_HostConfigSetEntry>, I>>(base?: I): RemoteConfig_HostConfigSetEntry {
    return RemoteConfig_HostConfigSetEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<RemoteConfig_HostConfigSetEntry>, I>>(
    object: I,
  ): RemoteConfig_HostConfigSetEntry {
    const message = createBaseRemoteConfig_HostConfigSetEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ControllerConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePublishConfig(): PublishConfig {
  return {
    sourceObjectKeys: [],
    manifests: [],
    allManifestRevs: false,
    platformIds: [],
    remotes: [],
    destObjectKey: "",
    storage: undefined,
    manifestStorage: {},
  };
}

export const PublishConfig = {
  encode(message: PublishConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.sourceObjectKeys) {
      writer.uint32(10).string(v!);
    }
    for (const v of message.manifests) {
      writer.uint32(18).string(v!);
    }
    if (message.allManifestRevs === true) {
      writer.uint32(24).bool(message.allManifestRevs);
    }
    for (const v of message.platformIds) {
      writer.uint32(34).string(v!);
    }
    for (const v of message.remotes) {
      writer.uint32(42).string(v!);
    }
    if (message.destObjectKey !== "") {
      writer.uint32(50).string(message.destObjectKey);
    }
    if (message.storage !== undefined) {
      PublishStorageConfig.encode(message.storage, writer.uint32(58).fork()).ldelim();
    }
    Object.entries(message.manifestStorage).forEach(([key, value]) => {
      PublishConfig_ManifestStorageEntry.encode({ key: key as any, value }, writer.uint32(66).fork()).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PublishConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePublishConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.sourceObjectKeys.push(reader.string());
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.manifests.push(reader.string());
          continue;
        case 3:
          if (tag != 24) {
            break;
          }

          message.allManifestRevs = reader.bool();
          continue;
        case 4:
          if (tag != 34) {
            break;
          }

          message.platformIds.push(reader.string());
          continue;
        case 5:
          if (tag != 42) {
            break;
          }

          message.remotes.push(reader.string());
          continue;
        case 6:
          if (tag != 50) {
            break;
          }

          message.destObjectKey = reader.string();
          continue;
        case 7:
          if (tag != 58) {
            break;
          }

          message.storage = PublishStorageConfig.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag != 66) {
            break;
          }

          const entry8 = PublishConfig_ManifestStorageEntry.decode(reader, reader.uint32());
          if (entry8.value !== undefined) {
            message.manifestStorage[entry8.key] = entry8.value;
          }
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
      sourceObjectKeys: Array.isArray(object?.sourceObjectKeys)
        ? object.sourceObjectKeys.map((e: any) => String(e))
        : [],
      manifests: Array.isArray(object?.manifests) ? object.manifests.map((e: any) => String(e)) : [],
      allManifestRevs: isSet(object.allManifestRevs) ? Boolean(object.allManifestRevs) : false,
      platformIds: Array.isArray(object?.platformIds) ? object.platformIds.map((e: any) => String(e)) : [],
      remotes: Array.isArray(object?.remotes) ? object.remotes.map((e: any) => String(e)) : [],
      destObjectKey: isSet(object.destObjectKey) ? String(object.destObjectKey) : "",
      storage: isSet(object.storage) ? PublishStorageConfig.fromJSON(object.storage) : undefined,
      manifestStorage: isObject(object.manifestStorage)
        ? Object.entries(object.manifestStorage).reduce<{ [key: string]: PublishStorageConfig }>(
          (acc, [key, value]) => {
            acc[key] = PublishStorageConfig.fromJSON(value);
            return acc;
          },
          {},
        )
        : {},
    };
  },

  toJSON(message: PublishConfig): unknown {
    const obj: any = {};
    if (message.sourceObjectKeys) {
      obj.sourceObjectKeys = message.sourceObjectKeys.map((e) => e);
    } else {
      obj.sourceObjectKeys = [];
    }
    if (message.manifests) {
      obj.manifests = message.manifests.map((e) => e);
    } else {
      obj.manifests = [];
    }
    message.allManifestRevs !== undefined && (obj.allManifestRevs = message.allManifestRevs);
    if (message.platformIds) {
      obj.platformIds = message.platformIds.map((e) => e);
    } else {
      obj.platformIds = [];
    }
    if (message.remotes) {
      obj.remotes = message.remotes.map((e) => e);
    } else {
      obj.remotes = [];
    }
    message.destObjectKey !== undefined && (obj.destObjectKey = message.destObjectKey);
    message.storage !== undefined &&
      (obj.storage = message.storage ? PublishStorageConfig.toJSON(message.storage) : undefined);
    obj.manifestStorage = {};
    if (message.manifestStorage) {
      Object.entries(message.manifestStorage).forEach(([k, v]) => {
        obj.manifestStorage[k] = PublishStorageConfig.toJSON(v);
      });
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<PublishConfig>, I>>(base?: I): PublishConfig {
    return PublishConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PublishConfig>, I>>(object: I): PublishConfig {
    const message = createBasePublishConfig();
    message.sourceObjectKeys = object.sourceObjectKeys?.map((e) => e) || [];
    message.manifests = object.manifests?.map((e) => e) || [];
    message.allManifestRevs = object.allManifestRevs ?? false;
    message.platformIds = object.platformIds?.map((e) => e) || [];
    message.remotes = object.remotes?.map((e) => e) || [];
    message.destObjectKey = object.destObjectKey ?? "";
    message.storage = (object.storage !== undefined && object.storage !== null)
      ? PublishStorageConfig.fromPartial(object.storage)
      : undefined;
    message.manifestStorage = Object.entries(object.manifestStorage ?? {}).reduce<
      { [key: string]: PublishStorageConfig }
    >((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = PublishStorageConfig.fromPartial(value);
      }
      return acc;
    }, {});
    return message;
  },
};

function createBasePublishConfig_ManifestStorageEntry(): PublishConfig_ManifestStorageEntry {
  return { key: "", value: undefined };
}

export const PublishConfig_ManifestStorageEntry = {
  encode(message: PublishConfig_ManifestStorageEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      PublishStorageConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PublishConfig_ManifestStorageEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePublishConfig_ManifestStorageEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.value = PublishStorageConfig.decode(reader, reader.uint32());
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
  // Transform<PublishConfig_ManifestStorageEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PublishConfig_ManifestStorageEntry | PublishConfig_ManifestStorageEntry[]>
      | Iterable<PublishConfig_ManifestStorageEntry | PublishConfig_ManifestStorageEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishConfig_ManifestStorageEntry.encode(p).finish()];
        }
      } else {
        yield* [PublishConfig_ManifestStorageEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PublishConfig_ManifestStorageEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PublishConfig_ManifestStorageEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PublishConfig_ManifestStorageEntry.decode(p)];
        }
      } else {
        yield* [PublishConfig_ManifestStorageEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PublishConfig_ManifestStorageEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? PublishStorageConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: PublishConfig_ManifestStorageEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? PublishStorageConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PublishConfig_ManifestStorageEntry>, I>>(
    base?: I,
  ): PublishConfig_ManifestStorageEntry {
    return PublishConfig_ManifestStorageEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PublishConfig_ManifestStorageEntry>, I>>(
    object: I,
  ): PublishConfig_ManifestStorageEntry {
    const message = createBasePublishConfig_ManifestStorageEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? PublishStorageConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePublishStorageConfig(): PublishStorageConfig {
  return { transformConfFromRef: undefined, transformConf: undefined, timestamp: undefined };
}

export const PublishStorageConfig = {
  encode(message: PublishStorageConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.transformConfFromRef !== undefined) {
      ObjectRef.encode(message.transformConfFromRef, writer.uint32(10).fork()).ldelim();
    }
    if (message.transformConf !== undefined) {
      Config.encode(message.transformConf, writer.uint32(18).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PublishStorageConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePublishStorageConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.transformConfFromRef = ObjectRef.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.transformConf = Config.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32());
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
      transformConfFromRef: isSet(object.transformConfFromRef)
        ? ObjectRef.fromJSON(object.transformConfFromRef)
        : undefined,
      transformConf: isSet(object.transformConf) ? Config.fromJSON(object.transformConf) : undefined,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: PublishStorageConfig): unknown {
    const obj: any = {};
    message.transformConfFromRef !== undefined &&
      (obj.transformConfFromRef = message.transformConfFromRef
        ? ObjectRef.toJSON(message.transformConfFromRef)
        : undefined);
    message.transformConf !== undefined &&
      (obj.transformConf = message.transformConf ? Config.toJSON(message.transformConf) : undefined);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PublishStorageConfig>, I>>(base?: I): PublishStorageConfig {
    return PublishStorageConfig.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PublishStorageConfig>, I>>(object: I): PublishStorageConfig {
    const message = createBasePublishStorageConfig();
    message.transformConfFromRef = (object.transformConfFromRef !== undefined && object.transformConfFromRef !== null)
      ? ObjectRef.fromPartial(object.transformConfFromRef)
      : undefined;
    message.transformConf = (object.transformConf !== undefined && object.transformConf !== null)
      ? Config.fromPartial(object.transformConf)
      : undefined;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
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
