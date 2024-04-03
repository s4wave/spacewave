/* eslint-disable */
import {
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../../store/kvkey/kvkey.pb.js'

export const protobufPackage = 'block.store.inmem'

/** Config configures the inmem block store controller. */
export interface Config {
  /** BlockStoreId is the block store id to use on the bus. */
  blockStoreId: string
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
  /**
   * HashGet enables hashing values for Get requests.
   * This reduces performance but ensures data integrity.
   * As inmem is an in-memory cache, it shouldn't be necessary to use this.
   */
  hashGet: boolean
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
    kvKeyOpts: undefined,
    forceHashType: 0,
    hashGet: false,
    bucketIds: [],
    skipNotFound: false,
    verbose: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.blockStoreId !== '') {
      writer.uint32(10).string(message.blockStoreId)
    }
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(18).fork()).ldelim()
    }
    if (message.forceHashType !== 0) {
      writer.uint32(24).int32(message.forceHashType)
    }
    if (message.hashGet !== false) {
      writer.uint32(32).bool(message.hashGet)
    }
    for (const v of message.bucketIds) {
      writer.uint32(42).string(v!)
    }
    if (message.skipNotFound !== false) {
      writer.uint32(48).bool(message.skipNotFound)
    }
    if (message.verbose !== false) {
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
          if (tag !== 10) {
            break
          }

          message.blockStoreId = reader.string()
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

          message.forceHashType = reader.int32() as any
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.hashGet = reader.bool()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.bucketIds.push(reader.string())
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.skipNotFound = reader.bool()
          continue
        case 7:
          if (tag !== 56) {
            break
          }

          message.verbose = reader.bool()
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
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt as any).finish()]
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
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      blockStoreId: isSet(object.blockStoreId)
        ? globalThis.String(object.blockStoreId)
        : '',
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      forceHashType: isSet(object.forceHashType)
        ? hashTypeFromJSON(object.forceHashType)
        : 0,
      hashGet: isSet(object.hashGet)
        ? globalThis.Boolean(object.hashGet)
        : false,
      bucketIds: globalThis.Array.isArray(object?.bucketIds)
        ? object.bucketIds.map((e: any) => globalThis.String(e))
        : [],
      skipNotFound: isSet(object.skipNotFound)
        ? globalThis.Boolean(object.skipNotFound)
        : false,
      verbose: isSet(object.verbose)
        ? globalThis.Boolean(object.verbose)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.blockStoreId !== '') {
      obj.blockStoreId = message.blockStoreId
    }
    if (message.kvKeyOpts !== undefined) {
      obj.kvKeyOpts = Config1.toJSON(message.kvKeyOpts)
    }
    if (message.forceHashType !== 0) {
      obj.forceHashType = hashTypeToJSON(message.forceHashType)
    }
    if (message.hashGet !== false) {
      obj.hashGet = message.hashGet
    }
    if (message.bucketIds?.length) {
      obj.bucketIds = message.bucketIds
    }
    if (message.skipNotFound !== false) {
      obj.skipNotFound = message.skipNotFound
    }
    if (message.verbose !== false) {
      obj.verbose = message.verbose
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.blockStoreId = object.blockStoreId ?? ''
    message.kvKeyOpts =
      object.kvKeyOpts !== undefined && object.kvKeyOpts !== null
        ? Config1.fromPartial(object.kvKeyOpts)
        : undefined
    message.forceHashType = object.forceHashType ?? 0
    message.hashGet = object.hashGet ?? false
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
    : T extends globalThis.Array<infer U>
      ? globalThis.Array<DeepPartial<U>>
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
