/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { Config as Config1 } from "../../store/kvkey/kvkey.pb.js";
import { Config as Config3 } from "../../store/kvtx/kv_tx.pb.js";
import { Config as Config2 } from "../controller/controller.pb.js";

export const protobufPackage = "volume.bolt";

/** Config is the bolt volume controller config. */
export interface Config {
  /** Path is the file to store the data in. */
  path: string;
  /** KvKeyOpts are key/value options.. */
  kvKeyOpts:
    | Config1
    | undefined;
  /** Verbose indicates we should log every operation. */
  verbose: boolean;
  /** VolumeConfig is the volume controller config. */
  volumeConfig:
    | Config2
    | undefined;
  /** StoreConfig is the store queue configuration for kvtx. */
  storeConfig:
    | Config3
    | undefined;
  /**
   * NoGenerateKey indicates to skip generating a private key.
   * This has no effect if a key already exists.
   */
  noGenerateKey: boolean;
  /**
   * Sync indicates to sync after every write.
   * Reduces write performance but increases data safety.
   */
  sync: boolean;
  /**
   * FreelistSync enables syncing the freelist to disk.
   * Reduces write performance but increases recovery performance.
   */
  freelistSync: boolean;
}

function createBaseConfig(): Config {
  return {
    path: "",
    kvKeyOpts: undefined,
    verbose: false,
    volumeConfig: undefined,
    storeConfig: undefined,
    noGenerateKey: false,
    sync: false,
    freelistSync: false,
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.path !== "") {
      writer.uint32(10).string(message.path);
    }
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(18).fork()).ldelim();
    }
    if (message.verbose === true) {
      writer.uint32(24).bool(message.verbose);
    }
    if (message.volumeConfig !== undefined) {
      Config2.encode(message.volumeConfig, writer.uint32(42).fork()).ldelim();
    }
    if (message.storeConfig !== undefined) {
      Config3.encode(message.storeConfig, writer.uint32(50).fork()).ldelim();
    }
    if (message.noGenerateKey === true) {
      writer.uint32(56).bool(message.noGenerateKey);
    }
    if (message.sync === true) {
      writer.uint32(64).bool(message.sync);
    }
    if (message.freelistSync === true) {
      writer.uint32(72).bool(message.freelistSync);
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
          message.path = reader.string();
          break;
        case 2:
          message.kvKeyOpts = Config1.decode(reader, reader.uint32());
          break;
        case 3:
          message.verbose = reader.bool();
          break;
        case 5:
          message.volumeConfig = Config2.decode(reader, reader.uint32());
          break;
        case 6:
          message.storeConfig = Config3.decode(reader, reader.uint32());
          break;
        case 7:
          message.noGenerateKey = reader.bool();
          break;
        case 8:
          message.sync = reader.bool();
          break;
        case 9:
          message.freelistSync = reader.bool();
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
      path: isSet(object.path) ? String(object.path) : "",
      kvKeyOpts: isSet(object.kvKeyOpts) ? Config1.fromJSON(object.kvKeyOpts) : undefined,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
      volumeConfig: isSet(object.volumeConfig) ? Config2.fromJSON(object.volumeConfig) : undefined,
      storeConfig: isSet(object.storeConfig) ? Config3.fromJSON(object.storeConfig) : undefined,
      noGenerateKey: isSet(object.noGenerateKey) ? Boolean(object.noGenerateKey) : false,
      sync: isSet(object.sync) ? Boolean(object.sync) : false,
      freelistSync: isSet(object.freelistSync) ? Boolean(object.freelistSync) : false,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.path !== undefined && (obj.path = message.path);
    message.kvKeyOpts !== undefined &&
      (obj.kvKeyOpts = message.kvKeyOpts ? Config1.toJSON(message.kvKeyOpts) : undefined);
    message.verbose !== undefined && (obj.verbose = message.verbose);
    message.volumeConfig !== undefined &&
      (obj.volumeConfig = message.volumeConfig ? Config2.toJSON(message.volumeConfig) : undefined);
    message.storeConfig !== undefined &&
      (obj.storeConfig = message.storeConfig ? Config3.toJSON(message.storeConfig) : undefined);
    message.noGenerateKey !== undefined && (obj.noGenerateKey = message.noGenerateKey);
    message.sync !== undefined && (obj.sync = message.sync);
    message.freelistSync !== undefined && (obj.freelistSync = message.freelistSync);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.path = object.path ?? "";
    message.kvKeyOpts = (object.kvKeyOpts !== undefined && object.kvKeyOpts !== null)
      ? Config1.fromPartial(object.kvKeyOpts)
      : undefined;
    message.verbose = object.verbose ?? false;
    message.volumeConfig = (object.volumeConfig !== undefined && object.volumeConfig !== null)
      ? Config2.fromPartial(object.volumeConfig)
      : undefined;
    message.storeConfig = (object.storeConfig !== undefined && object.storeConfig !== null)
      ? Config3.fromPartial(object.storeConfig)
      : undefined;
    message.noGenerateKey = object.noGenerateKey ?? false;
    message.sync = object.sync ?? false;
    message.freelistSync = object.freelistSync ?? false;
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
