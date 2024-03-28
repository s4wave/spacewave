/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../../bucket/bucket.pb.js'

export const protobufPackage = 'block.store.bucket'

/**
 * Config configures the block-store backed bucket controller.
 *
 * Exposes a statically configured bucket against a block store by id.
 */
export interface Config {
  /**
   * BlockStoreId configures the block store to use for blocks.
   * uses LookupBlockStore to lookup the block store on the bus.
   */
  blockStoreId: string
  /** BucketConfig is the bucket config to expose on the bus and back with the block store. */
  bucketConfig: Config1 | undefined
  /**
   * BucketStoreId configures the store id to use when filtering BuildBucketAPI directives.
   * If unset, defaults to the block_store_id.
   */
  bucketStoreId: string
  /**
   * NotFoundIfIdle returns a not found error if the block store was not found.
   * If unset, waits until the block store is available.
   */
  notFoundIfIdle: boolean
}

function createBaseConfig(): Config {
  return {
    blockStoreId: '',
    bucketConfig: undefined,
    bucketStoreId: '',
    notFoundIfIdle: false,
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
    if (message.bucketConfig !== undefined) {
      Config1.encode(message.bucketConfig, writer.uint32(18).fork()).ldelim()
    }
    if (message.bucketStoreId !== '') {
      writer.uint32(26).string(message.bucketStoreId)
    }
    if (message.notFoundIfIdle !== false) {
      writer.uint32(32).bool(message.notFoundIfIdle)
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

          message.bucketConfig = Config1.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.bucketStoreId = reader.string()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.notFoundIfIdle = reader.bool()
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
      bucketConfig: isSet(object.bucketConfig)
        ? Config1.fromJSON(object.bucketConfig)
        : undefined,
      bucketStoreId: isSet(object.bucketStoreId)
        ? globalThis.String(object.bucketStoreId)
        : '',
      notFoundIfIdle: isSet(object.notFoundIfIdle)
        ? globalThis.Boolean(object.notFoundIfIdle)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.blockStoreId !== '') {
      obj.blockStoreId = message.blockStoreId
    }
    if (message.bucketConfig !== undefined) {
      obj.bucketConfig = Config1.toJSON(message.bucketConfig)
    }
    if (message.bucketStoreId !== '') {
      obj.bucketStoreId = message.bucketStoreId
    }
    if (message.notFoundIfIdle !== false) {
      obj.notFoundIfIdle = message.notFoundIfIdle
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.blockStoreId = object.blockStoreId ?? ''
    message.bucketConfig =
      object.bucketConfig !== undefined && object.bucketConfig !== null
        ? Config1.fromPartial(object.bucketConfig)
        : undefined
    message.bucketStoreId = object.bucketStoreId ?? ''
    message.notFoundIfIdle = object.notFoundIfIdle ?? false
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
