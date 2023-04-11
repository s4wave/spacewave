/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../store/kvkey/kvkey.pb.js'
import { Config as Config3 } from '../../store/kvtx/kv_tx.pb.js'
import { Config as Config2 } from '../controller/controller.pb.js'

export const protobufPackage = 'volume.bolt'

/** Config is the bolt volume controller config. */
export interface Config {
  /** Path is the file to store the data in. */
  path: string
  /** KvKeyOpts are key/value options.. */
  kvKeyOpts: Config1 | undefined
  /** Verbose indicates we should log every operation. */
  verbose: boolean
  /** VolumeConfig is the volume controller config. */
  volumeConfig: Config2 | undefined
  /** StoreConfig is the store configuration for kvtx. */
  storeConfig: Config3 | undefined
  /**
   * NoGenerateKey indicates to skip generating a private key.
   * This has no effect if a key already exists.
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
  /**
   * Sync indicates to sync after every write.
   * Reduces write performance but increases data safety.
   */
  sync: boolean
  /**
   * FreelistSync enables syncing the freelist to disk.
   * Reduces write performance but increases recovery performance.
   */
  freelistSync: boolean
}

function createBaseConfig(): Config {
  return {
    path: '',
    kvKeyOpts: undefined,
    verbose: false,
    volumeConfig: undefined,
    storeConfig: undefined,
    noGenerateKey: false,
    noWriteKey: false,
    sync: false,
    freelistSync: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.path !== '') {
      writer.uint32(10).string(message.path)
    }
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(18).fork()).ldelim()
    }
    if (message.verbose === true) {
      writer.uint32(24).bool(message.verbose)
    }
    if (message.volumeConfig !== undefined) {
      Config2.encode(message.volumeConfig, writer.uint32(42).fork()).ldelim()
    }
    if (message.storeConfig !== undefined) {
      Config3.encode(message.storeConfig, writer.uint32(50).fork()).ldelim()
    }
    if (message.noGenerateKey === true) {
      writer.uint32(56).bool(message.noGenerateKey)
    }
    if (message.noWriteKey === true) {
      writer.uint32(80).bool(message.noWriteKey)
    }
    if (message.sync === true) {
      writer.uint32(64).bool(message.sync)
    }
    if (message.freelistSync === true) {
      writer.uint32(72).bool(message.freelistSync)
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
          if (tag != 10) {
            break
          }

          message.path = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.kvKeyOpts = Config1.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag != 24) {
            break
          }

          message.verbose = reader.bool()
          continue
        case 5:
          if (tag != 42) {
            break
          }

          message.volumeConfig = Config2.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag != 50) {
            break
          }

          message.storeConfig = Config3.decode(reader, reader.uint32())
          continue
        case 7:
          if (tag != 56) {
            break
          }

          message.noGenerateKey = reader.bool()
          continue
        case 10:
          if (tag != 80) {
            break
          }

          message.noWriteKey = reader.bool()
          continue
        case 8:
          if (tag != 64) {
            break
          }

          message.sync = reader.bool()
          continue
        case 9:
          if (tag != 72) {
            break
          }

          message.freelistSync = reader.bool()
          continue
      }
      if ((tag & 7) == 4 || tag == 0) {
        break
      }
      reader.skipType(tag & 7)
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
      path: isSet(object.path) ? String(object.path) : '',
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
      volumeConfig: isSet(object.volumeConfig)
        ? Config2.fromJSON(object.volumeConfig)
        : undefined,
      storeConfig: isSet(object.storeConfig)
        ? Config3.fromJSON(object.storeConfig)
        : undefined,
      noGenerateKey: isSet(object.noGenerateKey)
        ? Boolean(object.noGenerateKey)
        : false,
      noWriteKey: isSet(object.noWriteKey) ? Boolean(object.noWriteKey) : false,
      sync: isSet(object.sync) ? Boolean(object.sync) : false,
      freelistSync: isSet(object.freelistSync)
        ? Boolean(object.freelistSync)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.path !== undefined && (obj.path = message.path)
    message.kvKeyOpts !== undefined &&
      (obj.kvKeyOpts = message.kvKeyOpts
        ? Config1.toJSON(message.kvKeyOpts)
        : undefined)
    message.verbose !== undefined && (obj.verbose = message.verbose)
    message.volumeConfig !== undefined &&
      (obj.volumeConfig = message.volumeConfig
        ? Config2.toJSON(message.volumeConfig)
        : undefined)
    message.storeConfig !== undefined &&
      (obj.storeConfig = message.storeConfig
        ? Config3.toJSON(message.storeConfig)
        : undefined)
    message.noGenerateKey !== undefined &&
      (obj.noGenerateKey = message.noGenerateKey)
    message.noWriteKey !== undefined && (obj.noWriteKey = message.noWriteKey)
    message.sync !== undefined && (obj.sync = message.sync)
    message.freelistSync !== undefined &&
      (obj.freelistSync = message.freelistSync)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.path = object.path ?? ''
    message.kvKeyOpts =
      object.kvKeyOpts !== undefined && object.kvKeyOpts !== null
        ? Config1.fromPartial(object.kvKeyOpts)
        : undefined
    message.verbose = object.verbose ?? false
    message.volumeConfig =
      object.volumeConfig !== undefined && object.volumeConfig !== null
        ? Config2.fromPartial(object.volumeConfig)
        : undefined
    message.storeConfig =
      object.storeConfig !== undefined && object.storeConfig !== null
        ? Config3.fromPartial(object.storeConfig)
        : undefined
    message.noGenerateKey = object.noGenerateKey ?? false
    message.noWriteKey = object.noWriteKey ?? false
    message.sync = object.sync ?? false
    message.freelistSync = object.freelistSync ?? false
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
