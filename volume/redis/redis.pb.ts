/* eslint-disable */
import Long from 'long'
import { Config as Config1 } from '../../store/kvkey/kvkey.pb.js'
import { Config as Config2 } from '../controller/controller.pb.js'
import { Config as Config3 } from '../../store/kvtx/kv_tx.pb.js'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'volume.redis'

/**
 * Config is the redis volume controller config.
 * Url is the only mandatory flag.
 */
export interface Config {
  /** Url is the redis:// url to connect to. */
  url: string
  /** KvKeyOpts are key/value options. */
  kvKeyOpts: Config1 | undefined
  /**
   * NoGenerateKey indicates the controller should not generate a private key if
   * one is already present. Setting this to false will cause the system to
   * create a new private key if one is not present in the store at startup. If
   * no key is in the store at startup and this is true, an error will be
   * returned.
   */
  noGenerateKey: boolean
  /** Verbose indicates we should log every operation. */
  verbose: boolean
  /** VolumeConfig is the volume controller config. */
  volumeConfig: Config2 | undefined
  /** StoreConfig is the store queue configuration for kvtx. */
  storeConfig: Config3 | undefined
}

function createBaseConfig(): Config {
  return {
    url: '',
    kvKeyOpts: undefined,
    noGenerateKey: false,
    verbose: false,
    volumeConfig: undefined,
    storeConfig: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.url !== '') {
      writer.uint32(10).string(message.url)
    }
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(18).fork()).ldelim()
    }
    if (message.noGenerateKey === true) {
      writer.uint32(24).bool(message.noGenerateKey)
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
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.url = reader.string()
          break
        case 2:
          message.kvKeyOpts = Config1.decode(reader, reader.uint32())
          break
        case 3:
          message.noGenerateKey = reader.bool()
          break
        case 4:
          message.verbose = reader.bool()
          break
        case 5:
          message.volumeConfig = Config2.decode(reader, reader.uint32())
          break
        case 6:
          message.storeConfig = Config3.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>
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
      | Iterable<Uint8Array | Uint8Array[]>
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
      url: isSet(object.url) ? String(object.url) : '',
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      noGenerateKey: isSet(object.noGenerateKey)
        ? Boolean(object.noGenerateKey)
        : false,
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
    message.url !== undefined && (obj.url = message.url)
    message.kvKeyOpts !== undefined &&
      (obj.kvKeyOpts = message.kvKeyOpts
        ? Config1.toJSON(message.kvKeyOpts)
        : undefined)
    message.noGenerateKey !== undefined &&
      (obj.noGenerateKey = message.noGenerateKey)
    message.verbose !== undefined && (obj.verbose = message.verbose)
    message.volumeConfig !== undefined &&
      (obj.volumeConfig = message.volumeConfig
        ? Config2.toJSON(message.volumeConfig)
        : undefined)
    message.storeConfig !== undefined &&
      (obj.storeConfig = message.storeConfig
        ? Config3.toJSON(message.storeConfig)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.url = object.url ?? ''
    message.kvKeyOpts =
      object.kvKeyOpts !== undefined && object.kvKeyOpts !== null
        ? Config1.fromPartial(object.kvKeyOpts)
        : undefined
    message.noGenerateKey = object.noGenerateKey ?? false
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
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & Record<
        Exclude<keyof I, KeysOfUnion<P>>,
        never
      >

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
