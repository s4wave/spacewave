/* eslint-disable */
import { Backoff } from "@go/github.com/aperturerobotics/bifrost/util/backoff/backoff.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { ProjectConfig } from "../project.pb.js";

export const protobufPackage = "bldr.project.controller";

/** Config is the Project controller configuration. */
export interface Config {
  /** SourcePath is the path to the source code working dir. */
  sourcePath: string;
  /**
   * WorkingPath is the path to use for codegen and working state.
   * Usually source_path/.bldr
   */
  workingPath: string;
  /** ProjectConfig contains the project configuration. */
  projectConfig:
    | ProjectConfig
    | undefined;
  /**
   * StartProject indicates the controller should start the project ConfigSet
   * and startup plugins.
   */
  startProject: boolean;
  /** EngineId is the world engine to store the manifests. */
  engineId: string;
  /** PluginHostKey is the plugin host object to link the manifest to. */
  pluginHostKey: string;
  /** PlatformId is the platform ID to build for. */
  platformId: string;
  /**
   * BuildBackoff is the backoff config for building plugins.
   * If unset, defaults to reasonable defaults.
   */
  buildBackoff: Backoff | undefined;
}

function createBaseConfig(): Config {
  return {
    sourcePath: "",
    workingPath: "",
    projectConfig: undefined,
    startProject: false,
    engineId: "",
    pluginHostKey: "",
    platformId: "",
    buildBackoff: undefined,
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
    if (message.startProject === true) {
      writer.uint32(32).bool(message.startProject);
    }
    if (message.engineId !== "") {
      writer.uint32(42).string(message.engineId);
    }
    if (message.pluginHostKey !== "") {
      writer.uint32(50).string(message.pluginHostKey);
    }
    if (message.platformId !== "") {
      writer.uint32(58).string(message.platformId);
    }
    if (message.buildBackoff !== undefined) {
      Backoff.encode(message.buildBackoff, writer.uint32(66).fork()).ldelim();
    }
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
          message.sourcePath = reader.string();
          break;
        case 2:
          message.workingPath = reader.string();
          break;
        case 3:
          message.projectConfig = ProjectConfig.decode(reader, reader.uint32());
          break;
        case 4:
          message.startProject = reader.bool();
          break;
        case 5:
          message.engineId = reader.string();
          break;
        case 6:
          message.pluginHostKey = reader.string();
          break;
        case 7:
          message.platformId = reader.string();
          break;
        case 8:
          message.buildBackoff = Backoff.decode(reader, reader.uint32());
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
      sourcePath: isSet(object.sourcePath) ? String(object.sourcePath) : "",
      workingPath: isSet(object.workingPath) ? String(object.workingPath) : "",
      projectConfig: isSet(object.projectConfig) ? ProjectConfig.fromJSON(object.projectConfig) : undefined,
      startProject: isSet(object.startProject) ? Boolean(object.startProject) : false,
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      pluginHostKey: isSet(object.pluginHostKey) ? String(object.pluginHostKey) : "",
      platformId: isSet(object.platformId) ? String(object.platformId) : "",
      buildBackoff: isSet(object.buildBackoff) ? Backoff.fromJSON(object.buildBackoff) : undefined,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.sourcePath !== undefined && (obj.sourcePath = message.sourcePath);
    message.workingPath !== undefined && (obj.workingPath = message.workingPath);
    message.projectConfig !== undefined &&
      (obj.projectConfig = message.projectConfig ? ProjectConfig.toJSON(message.projectConfig) : undefined);
    message.startProject !== undefined && (obj.startProject = message.startProject);
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.pluginHostKey !== undefined && (obj.pluginHostKey = message.pluginHostKey);
    message.platformId !== undefined && (obj.platformId = message.platformId);
    message.buildBackoff !== undefined &&
      (obj.buildBackoff = message.buildBackoff ? Backoff.toJSON(message.buildBackoff) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.sourcePath = object.sourcePath ?? "";
    message.workingPath = object.workingPath ?? "";
    message.projectConfig = (object.projectConfig !== undefined && object.projectConfig !== null)
      ? ProjectConfig.fromPartial(object.projectConfig)
      : undefined;
    message.startProject = object.startProject ?? false;
    message.engineId = object.engineId ?? "";
    message.pluginHostKey = object.pluginHostKey ?? "";
    message.platformId = object.platformId ?? "";
    message.buildBackoff = (object.buildBackoff !== undefined && object.buildBackoff !== null)
      ? Backoff.fromPartial(object.buildBackoff)
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
