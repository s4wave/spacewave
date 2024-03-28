/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../../../store/kvkey/kvkey.pb.js'

export const protobufPackage = 'block.store.kvfile.http'

/**
 * Config configures the block store kvfile via http controller.
 *
 * Reads from kvfile at an HTTP URL and exposes a block store.
 * This store controller always runs in read-only mooe.
 */
export interface Config {
  /** BlockStoreId is the block store id to use on the bus. */
  blockStoreId: string
  /** Url is the url where the kvfile is located. */
  url: string
  /** BucketIds is a list of bucket ids to serve LookupBlockFromNetwork directives. */
  bucketIds: string[]
  /** SkipNotFound skips returning a value if the block was not found. */
  skipNotFound: boolean
  /** Verbose enables verbose logging of the block store. */
  verbose: boolean
  /** DisableCache disables the browser cache (if possible). */
  disableCache: boolean
  /**
   * KvKeyOpts are key/value key constants.
   * Optional.
   */
  kvKeyOpts: Config1 | undefined
  /**
   * MinRequestSize sets the minimum size to use for http range requests.
   * Enables buffering in memory if set.
   */
  minRequestSize: Long
}

function createBaseConfig(): Config {
  return {
    blockStoreId: '',
    url: '',
    bucketIds: [],
    skipNotFound: false,
    verbose: false,
    disableCache: false,
    kvKeyOpts: undefined,
    minRequestSize: Long.UZERO,
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
    if (message.url !== '') {
      writer.uint32(18).string(message.url)
    }
    for (const v of message.bucketIds) {
      writer.uint32(26).string(v!)
    }
    if (message.skipNotFound !== false) {
      writer.uint32(32).bool(message.skipNotFound)
    }
    if (message.verbose !== false) {
      writer.uint32(40).bool(message.verbose)
    }
    if (message.disableCache !== false) {
      writer.uint32(48).bool(message.disableCache)
    }
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(58).fork()).ldelim()
    }
    if (!message.minRequestSize.equals(Long.UZERO)) {
      writer.uint32(64).uint64(message.minRequestSize)
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

          message.url = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.bucketIds.push(reader.string())
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.skipNotFound = reader.bool()
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.verbose = reader.bool()
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.disableCache = reader.bool()
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.kvKeyOpts = Config1.decode(reader, reader.uint32())
          continue
        case 8:
          if (tag !== 64) {
            break
          }

          message.minRequestSize = reader.uint64() as Long
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
      url: isSet(object.url) ? globalThis.String(object.url) : '',
      bucketIds: globalThis.Array.isArray(object?.bucketIds)
        ? object.bucketIds.map((e: any) => globalThis.String(e))
        : [],
      skipNotFound: isSet(object.skipNotFound)
        ? globalThis.Boolean(object.skipNotFound)
        : false,
      verbose: isSet(object.verbose)
        ? globalThis.Boolean(object.verbose)
        : false,
      disableCache: isSet(object.disableCache)
        ? globalThis.Boolean(object.disableCache)
        : false,
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      minRequestSize: isSet(object.minRequestSize)
        ? Long.fromValue(object.minRequestSize)
        : Long.UZERO,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.blockStoreId !== '') {
      obj.blockStoreId = message.blockStoreId
    }
    if (message.url !== '') {
      obj.url = message.url
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
    if (message.disableCache !== false) {
      obj.disableCache = message.disableCache
    }
    if (message.kvKeyOpts !== undefined) {
      obj.kvKeyOpts = Config1.toJSON(message.kvKeyOpts)
    }
    if (!message.minRequestSize.equals(Long.UZERO)) {
      obj.minRequestSize = (message.minRequestSize || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.blockStoreId = object.blockStoreId ?? ''
    message.url = object.url ?? ''
    message.bucketIds = object.bucketIds?.map((e) => e) || []
    message.skipNotFound = object.skipNotFound ?? false
    message.verbose = object.verbose ?? false
    message.disableCache = object.disableCache ?? false
    message.kvKeyOpts =
      object.kvKeyOpts !== undefined && object.kvKeyOpts !== null
        ? Config1.fromPartial(object.kvKeyOpts)
        : undefined
    message.minRequestSize =
      object.minRequestSize !== undefined && object.minRequestSize !== null
        ? Long.fromValue(object.minRequestSize)
        : Long.UZERO
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
