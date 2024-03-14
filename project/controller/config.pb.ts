/* eslint-disable */
import { Backoff } from "@go/github.com/aperturerobotics/util/backoff/backoff.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { BuilderConfig, BuilderResult } from "../../manifest/builder/builder.pb.js";
import { ProjectConfig } from "../project.pb.js";

export const protobufPackage = "bldr.project.controller";

/** Config is the Project controller configuration. */
export interface Config {
  /** SourcePath is the path to the source code working dir. */
  sourcePath: string;
  /**
   * WorkingPath is the path to use for codegen and working state.
   * Usually source_path/.bldr
   * We expect the bldr dist sources to be under working_path/bldr
   */
  workingPath: string;
  /** ProjectConfig contains the project configuration. */
  projectConfig:
    | ProjectConfig
    | undefined;
  /**
   * BuildBackoff is the backoff config for building manifests.
   * If unset, defaults to reasonable defaults.
   */
  buildBackoff:
    | Backoff
    | undefined;
  /** Watch enables watching for changes. */
  watch: boolean;
  /** Start enables loading the plugins in the "start" portion of the config. */
  start: boolean;
  /**
   * FetchManifestRemote is the remote to use for the FetchManifest request.
   * When FetchManifest is applied, we will compile the plugin manifest to the remote.
   * If unset, we don't service FetchManifest directives.
   */
  fetchManifestRemote: string;
}

/** ManifestBuilderConfig is a configuration for a ManifestBuilder. */
export interface ManifestBuilderConfig {
  /** ManifestId is the manifest identifier to build. */
  manifestId: string;
  /** BuildType is the type of build this is. */
  buildType: string;
  /** PlatformId is the platform ID to build. */
  platformId: string;
  /** RemoteId is the identifier of the remote to attach to. */
  remoteId: string;
}

/** ManifestBuilderResult is the result of a ManifestBuilder build. */
export interface ManifestBuilderResult {
  /** BuilderConfig was the config passed to the builder. */
  builderConfig:
    | BuilderConfig
    | undefined;
  /** BuilderResult is the result of running the builder. */
  builderResult: BuilderResult | undefined;
}

function createBaseConfig(): Config {
  return {
    sourcePath: "",
    workingPath: "",
    projectConfig: undefined,
    buildBackoff: undefined,
    watch: false,
    start: false,
    fetchManifestRemote: "",
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.sourcePath !== "") {
      writer.uint32(10).string(message.sourcePath);
    }
    if (message.workingPath !== "") {
      writer.uint32(18).string(message.workingPath);
    }
    if (message.projectConfig !== undefined) {
      ProjectConfig.encode(message.projectConfig, writer.uint32(26).fork()).ldelim();
    }
    if (message.buildBackoff !== undefined) {
      Backoff.encode(message.buildBackoff, writer.uint32(34).fork()).ldelim();
    }
    if (message.watch !== false) {
      writer.uint32(40).bool(message.watch);
    }
    if (message.start !== false) {
      writer.uint32(48).bool(message.start);
    }
    if (message.fetchManifestRemote !== "") {
      writer.uint32(58).string(message.fetchManifestRemote);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.sourcePath = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.workingPath = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.projectConfig = ProjectConfig.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.buildBackoff = Backoff.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.watch = reader.bool();
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.start = reader.bool();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.fetchManifestRemote = reader.string();
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
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config.encode(p).finish()];
        }
      } else {
        yield* [Config.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config.decode(p)];
        }
      } else {
        yield* [Config.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      sourcePath: isSet(object.sourcePath) ? globalThis.String(object.sourcePath) : "",
      workingPath: isSet(object.workingPath) ? globalThis.String(object.workingPath) : "",
      projectConfig: isSet(object.projectConfig) ? ProjectConfig.fromJSON(object.projectConfig) : undefined,
      buildBackoff: isSet(object.buildBackoff) ? Backoff.fromJSON(object.buildBackoff) : undefined,
      watch: isSet(object.watch) ? globalThis.Boolean(object.watch) : false,
      start: isSet(object.start) ? globalThis.Boolean(object.start) : false,
      fetchManifestRemote: isSet(object.fetchManifestRemote) ? globalThis.String(object.fetchManifestRemote) : "",
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    if (message.sourcePath !== "") {
      obj.sourcePath = message.sourcePath;
    }
    if (message.workingPath !== "") {
      obj.workingPath = message.workingPath;
    }
    if (message.projectConfig !== undefined) {
      obj.projectConfig = ProjectConfig.toJSON(message.projectConfig);
    }
    if (message.buildBackoff !== undefined) {
      obj.buildBackoff = Backoff.toJSON(message.buildBackoff);
    }
    if (message.watch !== false) {
      obj.watch = message.watch;
    }
    if (message.start !== false) {
      obj.start = message.start;
    }
    if (message.fetchManifestRemote !== "") {
      obj.fetchManifestRemote = message.fetchManifestRemote;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.sourcePath = object.sourcePath ?? "";
    message.workingPath = object.workingPath ?? "";
    message.projectConfig = (object.projectConfig !== undefined && object.projectConfig !== null)
      ? ProjectConfig.fromPartial(object.projectConfig)
      : undefined;
    message.buildBackoff = (object.buildBackoff !== undefined && object.buildBackoff !== null)
      ? Backoff.fromPartial(object.buildBackoff)
      : undefined;
    message.watch = object.watch ?? false;
    message.start = object.start ?? false;
    message.fetchManifestRemote = object.fetchManifestRemote ?? "";
    return message;
  },
};

function createBaseManifestBuilderConfig(): ManifestBuilderConfig {
  return { manifestId: "", buildType: "", platformId: "", remoteId: "" };
}

export const ManifestBuilderConfig = {
  encode(message: ManifestBuilderConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.manifestId !== "") {
      writer.uint32(10).string(message.manifestId);
    }
    if (message.buildType !== "") {
      writer.uint32(18).string(message.buildType);
    }
    if (message.platformId !== "") {
      writer.uint32(26).string(message.platformId);
    }
    if (message.remoteId !== "") {
      writer.uint32(34).string(message.remoteId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ManifestBuilderConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseManifestBuilderConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.manifestId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.buildType = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.platformId = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.remoteId = reader.string();
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
  // Transform<ManifestBuilderConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ManifestBuilderConfig | ManifestBuilderConfig[]>
      | Iterable<ManifestBuilderConfig | ManifestBuilderConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestBuilderConfig.encode(p).finish()];
        }
      } else {
        yield* [ManifestBuilderConfig.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ManifestBuilderConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ManifestBuilderConfig> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestBuilderConfig.decode(p)];
        }
      } else {
        yield* [ManifestBuilderConfig.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): ManifestBuilderConfig {
    return {
      manifestId: isSet(object.manifestId) ? globalThis.String(object.manifestId) : "",
      buildType: isSet(object.buildType) ? globalThis.String(object.buildType) : "",
      platformId: isSet(object.platformId) ? globalThis.String(object.platformId) : "",
      remoteId: isSet(object.remoteId) ? globalThis.String(object.remoteId) : "",
    };
  },

  toJSON(message: ManifestBuilderConfig): unknown {
    const obj: any = {};
    if (message.manifestId !== "") {
      obj.manifestId = message.manifestId;
    }
    if (message.buildType !== "") {
      obj.buildType = message.buildType;
    }
    if (message.platformId !== "") {
      obj.platformId = message.platformId;
    }
    if (message.remoteId !== "") {
      obj.remoteId = message.remoteId;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ManifestBuilderConfig>, I>>(base?: I): ManifestBuilderConfig {
    return ManifestBuilderConfig.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ManifestBuilderConfig>, I>>(object: I): ManifestBuilderConfig {
    const message = createBaseManifestBuilderConfig();
    message.manifestId = object.manifestId ?? "";
    message.buildType = object.buildType ?? "";
    message.platformId = object.platformId ?? "";
    message.remoteId = object.remoteId ?? "";
    return message;
  },
};

function createBaseManifestBuilderResult(): ManifestBuilderResult {
  return { builderConfig: undefined, builderResult: undefined };
}

export const ManifestBuilderResult = {
  encode(message: ManifestBuilderResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.builderConfig !== undefined) {
      BuilderConfig.encode(message.builderConfig, writer.uint32(10).fork()).ldelim();
    }
    if (message.builderResult !== undefined) {
      BuilderResult.encode(message.builderResult, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ManifestBuilderResult {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseManifestBuilderResult();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.builderConfig = BuilderConfig.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.builderResult = BuilderResult.decode(reader, reader.uint32());
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
  // Transform<ManifestBuilderResult, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ManifestBuilderResult | ManifestBuilderResult[]>
      | Iterable<ManifestBuilderResult | ManifestBuilderResult[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestBuilderResult.encode(p).finish()];
        }
      } else {
        yield* [ManifestBuilderResult.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ManifestBuilderResult>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ManifestBuilderResult> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestBuilderResult.decode(p)];
        }
      } else {
        yield* [ManifestBuilderResult.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): ManifestBuilderResult {
    return {
      builderConfig: isSet(object.builderConfig) ? BuilderConfig.fromJSON(object.builderConfig) : undefined,
      builderResult: isSet(object.builderResult) ? BuilderResult.fromJSON(object.builderResult) : undefined,
    };
  },

  toJSON(message: ManifestBuilderResult): unknown {
    const obj: any = {};
    if (message.builderConfig !== undefined) {
      obj.builderConfig = BuilderConfig.toJSON(message.builderConfig);
    }
    if (message.builderResult !== undefined) {
      obj.builderResult = BuilderResult.toJSON(message.builderResult);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ManifestBuilderResult>, I>>(base?: I): ManifestBuilderResult {
    return ManifestBuilderResult.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ManifestBuilderResult>, I>>(object: I): ManifestBuilderResult {
    const message = createBaseManifestBuilderResult();
    message.builderConfig = (object.builderConfig !== undefined && object.builderConfig !== null)
      ? BuilderConfig.fromPartial(object.builderConfig)
      : undefined;
    message.builderResult = (object.builderResult !== undefined && object.builderResult !== null)
      ? BuilderResult.fromPartial(object.builderResult)
      : undefined;
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
