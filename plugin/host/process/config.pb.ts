/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "plugin.host.process";

/** Config is the Process PluginHost controller configuration. */
export interface Config {
  /** EngineId is the world engine id to attach to. */
  engineId: string;
  /**
   * ObjectKey is the root object to attach to.
   * If not exists, waits for it to exist.
   *
   * Searches for <manifest> links from this object.
   */
  objectKey: string;
  /** PeerId is the peer ID to use for world transactions. */
  peerId: string;
  /**
   * VolumeId is the identifier of the volume on the plugin host bus.
   * This volume is available for the plugin to use via the volume proxy.
   */
  volumeId: string;
  /**
   * AlwaysFetchManifest will always create a FetchManifest directive even if
   * the manifest already exists. Used in dev mode.
   */
  alwaysFetchManifest: boolean;
  /**
   * DisableStoreManifest disables storing manifests fetched with FetchManifest.
   * This is used if we are watching the same world as the manifest compiler.
   */
  disableStoreManifest: boolean;
  /** StateDir is the directory to use for state. */
  stateDir: string;
  /** DistDir is the directory to use for plugin distribution files */
  distDir: string;
}

function createBaseConfig(): Config {
  return {
    engineId: "",
    objectKey: "",
    peerId: "",
    volumeId: "",
    alwaysFetchManifest: false,
    disableStoreManifest: false,
    stateDir: "",
    distDir: "",
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.engineId !== "") {
      writer.uint32(10).string(message.engineId);
    }
    if (message.objectKey !== "") {
      writer.uint32(18).string(message.objectKey);
    }
    if (message.peerId !== "") {
      writer.uint32(26).string(message.peerId);
    }
    if (message.volumeId !== "") {
      writer.uint32(34).string(message.volumeId);
    }
    if (message.alwaysFetchManifest === true) {
      writer.uint32(40).bool(message.alwaysFetchManifest);
    }
    if (message.disableStoreManifest === true) {
      writer.uint32(48).bool(message.disableStoreManifest);
    }
    if (message.stateDir !== "") {
      writer.uint32(58).string(message.stateDir);
    }
    if (message.distDir !== "") {
      writer.uint32(66).string(message.distDir);
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

          message.engineId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.objectKey = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.peerId = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.volumeId = reader.string();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.alwaysFetchManifest = reader.bool();
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.disableStoreManifest = reader.bool();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.stateDir = reader.string();
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.distDir = reader.string();
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
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : "",
      alwaysFetchManifest: isSet(object.alwaysFetchManifest) ? Boolean(object.alwaysFetchManifest) : false,
      disableStoreManifest: isSet(object.disableStoreManifest) ? Boolean(object.disableStoreManifest) : false,
      stateDir: isSet(object.stateDir) ? String(object.stateDir) : "",
      distDir: isSet(object.distDir) ? String(object.distDir) : "",
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.volumeId !== undefined && (obj.volumeId = message.volumeId);
    message.alwaysFetchManifest !== undefined && (obj.alwaysFetchManifest = message.alwaysFetchManifest);
    message.disableStoreManifest !== undefined && (obj.disableStoreManifest = message.disableStoreManifest);
    message.stateDir !== undefined && (obj.stateDir = message.stateDir);
    message.distDir !== undefined && (obj.distDir = message.distDir);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.engineId = object.engineId ?? "";
    message.objectKey = object.objectKey ?? "";
    message.peerId = object.peerId ?? "";
    message.volumeId = object.volumeId ?? "";
    message.alwaysFetchManifest = object.alwaysFetchManifest ?? false;
    message.disableStoreManifest = object.disableStoreManifest ?? false;
    message.stateDir = object.stateDir ?? "";
    message.distDir = object.distDir ?? "";
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
