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
   * Manifests contains the mapping between manifest ID and manifest config.
   * The controller will be built when a manifest is requested via LoadManifest.
   * The ControllerConfig must be a manifest build controller Config.
   */
  manifests: { [key: string]: ManifestConfig };
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

export interface ProjectConfig_ManifestsEntry {
  key: string;
  value: ManifestConfig | undefined;
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

/** ManifestConfig is a configuration for building a manifest. */
export interface ManifestConfig {
  /** Builder is the configuration for the manifest builder. */
  builder:
    | ControllerConfig
    | undefined;
  /**
   * Rev is the manifest revision to build.
   *
   * The controller will always scan for the latest manifest and add 1 to the
   * most recent revision number when building.
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

/** BuildConfig configures a build target. */
export interface BuildConfig {
  /** Manifests is the list of manifest IDs to build. */
  manifests: string[];
  /** PlatformIds is the list of platforms to target. */
  platformIds: string[];
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
  /** LinkObjectKeys is the list of keys to link from with the <manifest> predicate. */
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
   * ManifestStorage overrides the storage for the given list of manifests.
   * Any unset values are inherited from the PublishConfig.
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
  return { id: "", start: undefined, manifests: {}, build: {}, repositories: {}, publish: {} };
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
    Object.entries(message.repositories).forEach(([key, value]) => {
      ProjectConfig_RepositoriesEntry.encode({ key: key as any, value }, writer.uint32(42).fork()).ldelim();
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

          const entry5 = ProjectConfig_RepositoriesEntry.decode(reader, reader.uint32());
          if (entry5.value !== undefined) {
            message.repositories[entry5.key] = entry5.value;
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig_RepositoriesEntry();
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

          message.value = RepositoryConfig.decode(reader, reader.uint32());
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRepositoryConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          const entry1 = RepositoryConfig_HostConfigSetEntry.decode(reader, reader.uint32());
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

          message.objectKey = reader.string();
          continue;
        case 4:
          if (tag != 34) {
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRepositoryConfig_HostConfigSetEntry();
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
  return { builds: [], repositories: [], objectKey: "", manifestStorage: {} };
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
    Object.entries(message.manifestStorage).forEach(([key, value]) => {
      PublishConfig_ManifestStorageEntry.encode({ key: key as any, value }, writer.uint32(34).fork()).ldelim();
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

          message.builds.push(reader.string());
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.repositories.push(reader.string());
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.objectKey = reader.string();
          continue;
        case 4:
          if (tag != 34) {
            break;
          }

          const entry4 = PublishConfig_ManifestStorageEntry.decode(reader, reader.uint32());
          if (entry4.value !== undefined) {
            message.manifestStorage[entry4.key] = entry4.value;
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
      builds: Array.isArray(object?.builds) ? object.builds.map((e: any) => String(e)) : [],
      repositories: Array.isArray(object?.repositories) ? object.repositories.map((e: any) => String(e)) : [],
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
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
    message.builds = object.builds?.map((e) => e) || [];
    message.repositories = object.repositories?.map((e) => e) || [];
    message.objectKey = object.objectKey ?? "";
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

          message.prevRef = ObjectRef.decode(reader, reader.uint32());
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

          message.objectKey = reader.string();
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
