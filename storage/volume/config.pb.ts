/* eslint-disable */
import { Config as Config1 } from "@go/github.com/aperturerobotics/hydra/volume/controller/controller.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "storage.volume";

/** Config configures a volume attached to a Storage. */
export interface Config {
  /**
   * StorageId is the id to use to look up the storage on the bus.
   * usually "default"
   */
  storageId: string;
  /**
   * StorageVolumeId is the id to pass to the storage constructor for the volume.
   * e.x. my-app-dist
   */
  storageVolumeId: string;
  /** VolumeConfig is the volume controller config. */
  volumeConfig: Config1 | undefined;
}

function createBaseConfig(): Config {
  return { storageId: "", storageVolumeId: "", volumeConfig: undefined };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.storageId !== "") {
      writer.uint32(10).string(message.storageId);
    }
    if (message.storageVolumeId !== "") {
      writer.uint32(18).string(message.storageVolumeId);
    }
    if (message.volumeConfig !== undefined) {
      Config1.encode(message.volumeConfig, writer.uint32(26).fork()).ldelim();
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

          message.storageId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.storageVolumeId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.volumeConfig = Config1.decode(reader, reader.uint32());
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
      storageId: isSet(object.storageId) ? globalThis.String(object.storageId) : "",
      storageVolumeId: isSet(object.storageVolumeId) ? globalThis.String(object.storageVolumeId) : "",
      volumeConfig: isSet(object.volumeConfig) ? Config1.fromJSON(object.volumeConfig) : undefined,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    if (message.storageId !== "") {
      obj.storageId = message.storageId;
    }
    if (message.storageVolumeId !== "") {
      obj.storageVolumeId = message.storageVolumeId;
    }
    if (message.volumeConfig !== undefined) {
      obj.volumeConfig = Config1.toJSON(message.volumeConfig);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.storageId = object.storageId ?? "";
    message.storageVolumeId = object.storageVolumeId ?? "";
    message.volumeConfig = (object.volumeConfig !== undefined && object.volumeConfig !== null)
      ? Config1.fromPartial(object.volumeConfig)
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
