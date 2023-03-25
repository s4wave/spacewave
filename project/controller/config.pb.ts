/* eslint-disable */
import { Backoff } from "@go/github.com/aperturerobotics/util/backoff/backoff.pb.js";
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
   * We expect the bldr dist sources to be under working_path/bldr
   */
  workingPath: string;
  /** ProjectConfig contains the project configuration. */
  projectConfig:
    | ProjectConfig
    | undefined;
  /** EngineId is the world engine to store the manifests. */
  engineId: string;
  /**
   * LinkObjectKeys is the list of object keys to link to the manifests.
   * The first key in the list is used as the base key for new manifests.
   * Must have at least one key.
   */
  linkObjectKeys: string[];
  /** PeerId is the peer id to use for world transactions. */
  peerId: string;
  /**
   * StartProject indicates the controller should start the project ConfigSet
   * and startup plugins from the "start" section of the project config.
   */
  startProject: boolean;
  /**
   * BuildBackoff is the backoff config for building manifests.
   * If unset, defaults to reasonable defaults.
   */
  buildBackoff:
    | Backoff
    | undefined;
  /** Watch enables watching for changes. */
  watch: boolean;
}

function createBaseConfig(): Config {
  return {
    sourcePath: "",
    workingPath: "",
    projectConfig: undefined,
    engineId: "",
    linkObjectKeys: [],
    peerId: "",
    startProject: false,
    buildBackoff: undefined,
    watch: false,
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
    if (message.engineId !== "") {
      writer.uint32(34).string(message.engineId);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(42).string(v!);
    }
    if (message.peerId !== "") {
      writer.uint32(50).string(message.peerId);
    }
    if (message.startProject === true) {
      writer.uint32(56).bool(message.startProject);
    }
    if (message.buildBackoff !== undefined) {
      Backoff.encode(message.buildBackoff, writer.uint32(66).fork()).ldelim();
    }
    if (message.watch === true) {
      writer.uint32(72).bool(message.watch);
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
          if (tag != 10) {
            break;
          }

          message.sourcePath = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.workingPath = reader.string();
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.projectConfig = ProjectConfig.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag != 34) {
            break;
          }

          message.engineId = reader.string();
          continue;
        case 5:
          if (tag != 42) {
            break;
          }

          message.linkObjectKeys.push(reader.string());
          continue;
        case 6:
          if (tag != 50) {
            break;
          }

          message.peerId = reader.string();
          continue;
        case 7:
          if (tag != 56) {
            break;
          }

          message.startProject = reader.bool();
          continue;
        case 8:
          if (tag != 66) {
            break;
          }

          message.buildBackoff = Backoff.decode(reader, reader.uint32());
          continue;
        case 9:
          if (tag != 72) {
            break;
          }

          message.watch = reader.bool();
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
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      linkObjectKeys: Array.isArray(object?.linkObjectKeys) ? object.linkObjectKeys.map((e: any) => String(e)) : [],
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      startProject: isSet(object.startProject) ? Boolean(object.startProject) : false,
      buildBackoff: isSet(object.buildBackoff) ? Backoff.fromJSON(object.buildBackoff) : undefined,
      watch: isSet(object.watch) ? Boolean(object.watch) : false,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.sourcePath !== undefined && (obj.sourcePath = message.sourcePath);
    message.workingPath !== undefined && (obj.workingPath = message.workingPath);
    message.projectConfig !== undefined &&
      (obj.projectConfig = message.projectConfig ? ProjectConfig.toJSON(message.projectConfig) : undefined);
    message.engineId !== undefined && (obj.engineId = message.engineId);
    if (message.linkObjectKeys) {
      obj.linkObjectKeys = message.linkObjectKeys.map((e) => e);
    } else {
      obj.linkObjectKeys = [];
    }
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.startProject !== undefined && (obj.startProject = message.startProject);
    message.buildBackoff !== undefined &&
      (obj.buildBackoff = message.buildBackoff ? Backoff.toJSON(message.buildBackoff) : undefined);
    message.watch !== undefined && (obj.watch = message.watch);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.sourcePath = object.sourcePath ?? "";
    message.workingPath = object.workingPath ?? "";
    message.projectConfig = (object.projectConfig !== undefined && object.projectConfig !== null)
      ? ProjectConfig.fromPartial(object.projectConfig)
      : undefined;
    message.engineId = object.engineId ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    message.peerId = object.peerId ?? "";
    message.startProject = object.startProject ?? false;
    message.buildBackoff = (object.buildBackoff !== undefined && object.buildBackoff !== null)
      ? Backoff.fromPartial(object.buildBackoff)
      : undefined;
    message.watch = object.watch ?? false;
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
