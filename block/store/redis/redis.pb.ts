/* eslint-disable */
import {
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../../store/kvkey/kvkey.pb.js'
import { ClientConfig } from '../../../store/kvtx/redis/redis.pb.js'

export const protobufPackage = 'block.store.redis'

/** Config configures the Redis block store controller. */
export interface Config {
  /** BlockStoreId is the block store id to use on the bus. */
  blockStoreId: string
  /** Client configures the redis client. */
  client: ClientConfig | undefined
  /**
   * KvKeyOpts are key/value key constants.
   * Optional.
   */
  kvKeyOpts: Config1 | undefined
  /**
   * ForceHashType forces writing the given hash type to the store.
   * If unset, accepts any hash type.
   */
  forceHashType: HashType
  /** BucketIds is a list of bucket ids to serve LookupBlockFromNetwork directives. */
  bucketIds: string[]
  /** SkipNotFound skips returning a value if the block was not found. */
  skipNotFound: boolean
  /** Verbose enables verbose logging of the block store. */
  verbose: boolean
}

function createBaseConfig(): Config {
  return {
    blockStoreId: '',
    client: undefined,
    kvKeyOpts: undefined,
    forceHashType: 0,
    bucketIds: [],
    skipNotFound: false,
    verbose: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.blockStoreId !== '') {
      writer.uint32(10).string(message.blockStoreId)
    }
    if (message.client !== undefined) {
      ClientConfig.encode(message.client, writer.uint32(18).fork()).ldelim()
    }
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(26).fork()).ldelim()
    }
    if (message.forceHashType !== 0) {
      writer.uint32(32).int32(message.forceHashType)
    }
    for (const v of message.bucketIds) {
      writer.uint32(42).string(v!)
    }
    if (message.skipNotFound === true) {
      writer.uint32(48).bool(message.skipNotFound)
    }
    if (message.verbose === true) {
      writer.uint32(56).bool(message.verbose)
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

          message.blockStoreId = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.client = ClientConfig.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.kvKeyOpts = Config1.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag != 32) {
            break
          }

          message.forceHashType = reader.int32() as any
          continue
        case 5:
          if (tag != 42) {
            break
          }

          message.bucketIds.push(reader.string())
          continue
        case 6:
          if (tag != 48) {
            break
          }

          message.skipNotFound = reader.bool()
          continue
        case 7:
          if (tag != 56) {
            break
          }

          message.verbose = reader.bool()
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
      blockStoreId: isSet(object.blockStoreId)
        ? String(object.blockStoreId)
        : '',
      client: isSet(object.client)
        ? ClientConfig.fromJSON(object.client)
        : undefined,
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      forceHashType: isSet(object.forceHashType)
        ? hashTypeFromJSON(object.forceHashType)
        : 0,
      bucketIds: Array.isArray(object?.bucketIds)
        ? object.bucketIds.map((e: any) => String(e))
        : [],
      skipNotFound: isSet(object.skipNotFound)
        ? Boolean(object.skipNotFound)
        : false,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.blockStoreId !== undefined &&
      (obj.blockStoreId = message.blockStoreId)
    message.client !== undefined &&
      (obj.client = message.client
        ? ClientConfig.toJSON(message.client)
        : undefined)
    message.kvKeyOpts !== undefined &&
      (obj.kvKeyOpts = message.kvKeyOpts
        ? Config1.toJSON(message.kvKeyOpts)
        : undefined)
    message.forceHashType !== undefined &&
      (obj.forceHashType = hashTypeToJSON(message.forceHashType))
    if (message.bucketIds) {
      obj.bucketIds = message.bucketIds.map((e) => e)
    } else {
      obj.bucketIds = []
    }
    message.skipNotFound !== undefined &&
      (obj.skipNotFound = message.skipNotFound)
    message.verbose !== undefined && (obj.verbose = message.verbose)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.blockStoreId = object.blockStoreId ?? ''
    message.client =
      object.client !== undefined && object.client !== null
        ? ClientConfig.fromPartial(object.client)
        : undefined
    message.kvKeyOpts =
      object.kvKeyOpts !== undefined && object.kvKeyOpts !== null
        ? Config1.fromPartial(object.kvKeyOpts)
        : undefined
    message.forceHashType = object.forceHashType ?? 0
    message.bucketIds = object.bucketIds?.map((e) => e) || []
    message.skipNotFound = object.skipNotFound ?? false
    message.verbose = object.verbose ?? false
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
