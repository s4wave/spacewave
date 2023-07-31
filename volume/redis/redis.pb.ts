/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../store/kvkey/kvkey.pb.js'
import { Config as Config3 } from '../../store/kvtx/kvtx.pb.js'
import { ClientConfig } from '../../store/kvtx/redis/redis.pb.js'
import { Config as Config2 } from '../controller/controller.pb.js'

export const protobufPackage = 'volume.redis'

/**
 * Config is the redis volume controller config.
 * Url is the only mandatory flag.
 */
export interface Config {
  /** Client configures the redis client. */
  client: ClientConfig | undefined
  /** KvKeyOpts are key/value options. */
  kvKeyOpts: Config1 | undefined
  /**
   * NoGenerateKey indicates the controller should not generate a private key if
   * one is not already present. Setting this to false will cause the system to
   * create a new private key if one is not present in the store at startup. If
   * no key is in the store at startup and this is true, returns an error.
   */
  noGenerateKey: boolean
  /**
   * NoWriteKey indicates the controller should not write a private key to
   * storage if it generates one. This results in an ephemeral volume peer
   * identity if there is no key present in the store already.
   *
   * Has no effect if the store has a peer private key.
   */
  noWriteKey: boolean
  /** Verbose indicates we should log every operation. */
  verbose: boolean
  /** VolumeConfig is the volume controller config. */
  volumeConfig: Config2 | undefined
  /** StoreConfig is the store configuration for kvtx. */
  storeConfig: Config3 | undefined
}

function createBaseConfig(): Config {
  return {
    client: undefined,
    kvKeyOpts: undefined,
    noGenerateKey: false,
    noWriteKey: false,
    verbose: false,
    volumeConfig: undefined,
    storeConfig: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.client !== undefined) {
      ClientConfig.encode(message.client, writer.uint32(10).fork()).ldelim()
    }
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(18).fork()).ldelim()
    }
    if (message.noGenerateKey === true) {
      writer.uint32(24).bool(message.noGenerateKey)
    }
    if (message.noWriteKey === true) {
      writer.uint32(56).bool(message.noWriteKey)
    }
    if (message.verbose === true) {
      writer.uint32(32).bool(message.verbose)
    }
    if (message.volumeConfig !== undefined) {
      Config2.encode(message.volumeConfig, writer.uint32(42).fork()).ldelim()
    }
    if (message.storeConfig !== undefined) {
      Config3.encode(message.storeConfig, writer.uint32(50).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.client = ClientConfig.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.kvKeyOpts = Config1.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.noGenerateKey = reader.bool()
          continue
        case 7:
          if (tag !== 56) {
            break
          }

          message.noWriteKey = reader.bool()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.verbose = reader.bool()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.volumeConfig = Config2.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.storeConfig = Config3.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      client: isSet(object.client)
        ? ClientConfig.fromJSON(object.client)
        : undefined,
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      noGenerateKey: isSet(object.noGenerateKey)
        ? Boolean(object.noGenerateKey)
        : false,
      noWriteKey: isSet(object.noWriteKey) ? Boolean(object.noWriteKey) : false,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
      volumeConfig: isSet(object.volumeConfig)
        ? Config2.fromJSON(object.volumeConfig)
        : undefined,
      storeConfig: isSet(object.storeConfig)
        ? Config3.fromJSON(object.storeConfig)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.client !== undefined) {
      obj.client = ClientConfig.toJSON(message.client)
    }
    if (message.kvKeyOpts !== undefined) {
      obj.kvKeyOpts = Config1.toJSON(message.kvKeyOpts)
    }
    if (message.noGenerateKey === true) {
      obj.noGenerateKey = message.noGenerateKey
    }
    if (message.noWriteKey === true) {
      obj.noWriteKey = message.noWriteKey
    }
    if (message.verbose === true) {
      obj.verbose = message.verbose
    }
    if (message.volumeConfig !== undefined) {
      obj.volumeConfig = Config2.toJSON(message.volumeConfig)
    }
    if (message.storeConfig !== undefined) {
      obj.storeConfig = Config3.toJSON(message.storeConfig)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.client =
      object.client !== undefined && object.client !== null
        ? ClientConfig.fromPartial(object.client)
        : undefined
    message.kvKeyOpts =
      object.kvKeyOpts !== undefined && object.kvKeyOpts !== null
        ? Config1.fromPartial(object.kvKeyOpts)
        : undefined
    message.noGenerateKey = object.noGenerateKey ?? false
    message.noWriteKey = object.noWriteKey ?? false
    message.verbose = object.verbose ?? false
    message.volumeConfig =
      object.volumeConfig !== undefined && object.volumeConfig !== null
        ? Config2.fromPartial(object.volumeConfig)
        : undefined
    message.storeConfig =
      object.storeConfig !== undefined && object.storeConfig !== null
        ? Config3.fromPartial(object.storeConfig)
        : undefined
    return message
  },
}

type Builtin =
  | Date
  | Function
  | Uint8Array
  | string
  | number
  | boolean
  | undefined

export type DeepPartial<T> = T extends Builtin
  ? T
  : T extends Long
  ? string | number | Long
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string }
  ? { [K in keyof Omit<T, '$case'>]?: DeepPartial<T[K]> } & {
      $case: T['$case']
    }
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

type KeysOfUnion<T> = T extends T ? keyof T : never
export type Exact<P, I extends P> = P extends Builtin
  ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & {
      [K in Exclude<keyof I, KeysOfUnion<P>>]: never
    }

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
