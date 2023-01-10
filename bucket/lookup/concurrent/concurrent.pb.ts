/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../bucket.pb.js'

export const protobufPackage = 'lookup.concurrent'

/** NotFoundBehavior controls what happens when a block was not found locally. */
export enum NotFoundBehavior {
  /** NotFoundBehavior_NONE - NotFoundBehavior_NONE does nothing when we don't find a block. */
  NotFoundBehavior_NONE = 0,
  /** NotFoundBehavior_LOOKUP_DIRECTIVE - NotFoundBehavior_LOOKUP_DIRECTIVE uses a lookup directive. */
  NotFoundBehavior_LOOKUP_DIRECTIVE = 1,
  UNRECOGNIZED = -1,
}

export function notFoundBehaviorFromJSON(object: any): NotFoundBehavior {
  switch (object) {
    case 0:
    case 'NotFoundBehavior_NONE':
      return NotFoundBehavior.NotFoundBehavior_NONE
    case 1:
    case 'NotFoundBehavior_LOOKUP_DIRECTIVE':
      return NotFoundBehavior.NotFoundBehavior_LOOKUP_DIRECTIVE
    case -1:
    case 'UNRECOGNIZED':
    default:
      return NotFoundBehavior.UNRECOGNIZED
  }
}

export function notFoundBehaviorToJSON(object: NotFoundBehavior): string {
  switch (object) {
    case NotFoundBehavior.NotFoundBehavior_NONE:
      return 'NotFoundBehavior_NONE'
    case NotFoundBehavior.NotFoundBehavior_LOOKUP_DIRECTIVE:
      return 'NotFoundBehavior_LOOKUP_DIRECTIVE'
    case NotFoundBehavior.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * PutBlockBehavior controls what to do when we write-back a block.
 * This is used to write when we don't know a specific target volume.
 */
export enum PutBlockBehavior {
  /** PutBlockBehavior_NONE - PutBlockBehavior_NONE does nothing with the incoming block. */
  PutBlockBehavior_NONE = 0,
  /** PutBlockBehavior_ALL_VOLUMES - PutBlockBehavior_ALL_VOLUMES writes the incoming block to all volumes. */
  PutBlockBehavior_ALL_VOLUMES = 1,
  UNRECOGNIZED = -1,
}

export function putBlockBehaviorFromJSON(object: any): PutBlockBehavior {
  switch (object) {
    case 0:
    case 'PutBlockBehavior_NONE':
      return PutBlockBehavior.PutBlockBehavior_NONE
    case 1:
    case 'PutBlockBehavior_ALL_VOLUMES':
      return PutBlockBehavior.PutBlockBehavior_ALL_VOLUMES
    case -1:
    case 'UNRECOGNIZED':
    default:
      return PutBlockBehavior.UNRECOGNIZED
  }
}

export function putBlockBehaviorToJSON(object: PutBlockBehavior): string {
  switch (object) {
    case PutBlockBehavior.PutBlockBehavior_NONE:
      return 'PutBlockBehavior_NONE'
    case PutBlockBehavior.PutBlockBehavior_ALL_VOLUMES:
      return 'PutBlockBehavior_ALL_VOLUMES'
    case PutBlockBehavior.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Config is the example lookup config. */
export interface Config {
  /** BucketConf is the bucket configuration. */
  bucketConf: Config1 | undefined
  /** NotFoundBehavior controls the not-found action. */
  notFoundBehavior: NotFoundBehavior
  /** PutBlockBehavior is the write-back behavior action. */
  putBlockBehavior: PutBlockBehavior
}

function createBaseConfig(): Config {
  return { bucketConf: undefined, notFoundBehavior: 0, putBlockBehavior: 0 }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.bucketConf !== undefined) {
      Config1.encode(message.bucketConf, writer.uint32(10).fork()).ldelim()
    }
    if (message.notFoundBehavior !== 0) {
      writer.uint32(16).int32(message.notFoundBehavior)
    }
    if (message.putBlockBehavior !== 0) {
      writer.uint32(24).int32(message.putBlockBehavior)
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
          message.bucketConf = Config1.decode(reader, reader.uint32())
          break
        case 2:
          message.notFoundBehavior = reader.int32() as any
          break
        case 3:
          message.putBlockBehavior = reader.int32() as any
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
      bucketConf: isSet(object.bucketConf)
        ? Config1.fromJSON(object.bucketConf)
        : undefined,
      notFoundBehavior: isSet(object.notFoundBehavior)
        ? notFoundBehaviorFromJSON(object.notFoundBehavior)
        : 0,
      putBlockBehavior: isSet(object.putBlockBehavior)
        ? putBlockBehaviorFromJSON(object.putBlockBehavior)
        : 0,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.bucketConf !== undefined &&
      (obj.bucketConf = message.bucketConf
        ? Config1.toJSON(message.bucketConf)
        : undefined)
    message.notFoundBehavior !== undefined &&
      (obj.notFoundBehavior = notFoundBehaviorToJSON(message.notFoundBehavior))
    message.putBlockBehavior !== undefined &&
      (obj.putBlockBehavior = putBlockBehaviorToJSON(message.putBlockBehavior))
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.bucketConf =
      object.bucketConf !== undefined && object.bucketConf !== null
        ? Config1.fromPartial(object.bucketConf)
        : undefined
    message.notFoundBehavior = object.notFoundBehavior ?? 0
    message.putBlockBehavior = object.putBlockBehavior ?? 0
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
