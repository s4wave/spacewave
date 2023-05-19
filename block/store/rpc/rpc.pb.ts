/* eslint-disable */
import {
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'block.store.rpc'

/**
 * Config configures the block store rpc controller.
 * Executes a LookupRpcClient directive to build the rpc client.
 * Resolves LookupBlockStore directives.
 */
export interface Config {
  /**
   * BlockStoreId is the block store id to match on the bus.
   * Combined with block_store_ids if also set.
   */
  blockStoreId: string
  /**
   * BlockStoreIds is a list of the block store id to use on the bus.
   * Combined with block_store_id if also set.
   */
  blockStoreIds: string[]
  /**
   * ServiceId is the service id to lookup with LookupRpcClient.
   * Cannot be empty.
   */
  serviceId: string
  /**
   * ClientId is the client id to use with LookupRpcClient.
   * Can be empty.
   */
  clientId: string
  /** ReadOnly disables writing to the rpc store. */
  readOnly: boolean
  /**
   * ForceHashType forces writing the given hash type to the store.
   * If unset, accepts any hash type.
   */
  forceHashType: HashType
  /** BucketIds is a list of bucket ids to serve LookupBlockFromNetwork directives. */
  bucketIds: string[]
  /**
   * LookupOnStart creates the LookupRpcClient directive on startup.
   * If false, waits until at least one directive references it.
   */
  lookupOnStart: boolean
  /** SkipNotFound skips returning a value if the block was not found. */
  skipNotFound: boolean
  /** Verbose enables verbose logging of the block store. */
  verbose: boolean
}

function createBaseConfig(): Config {
  return {
    blockStoreId: '',
    blockStoreIds: [],
    serviceId: '',
    clientId: '',
    readOnly: false,
    forceHashType: 0,
    bucketIds: [],
    lookupOnStart: false,
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
    for (const v of message.blockStoreIds) {
      writer.uint32(18).string(v!)
    }
    if (message.serviceId !== '') {
      writer.uint32(26).string(message.serviceId)
    }
    if (message.clientId !== '') {
      writer.uint32(34).string(message.clientId)
    }
    if (message.readOnly === true) {
      writer.uint32(40).bool(message.readOnly)
    }
    if (message.forceHashType !== 0) {
      writer.uint32(48).int32(message.forceHashType)
    }
    for (const v of message.bucketIds) {
      writer.uint32(58).string(v!)
    }
    if (message.lookupOnStart === true) {
      writer.uint32(64).bool(message.lookupOnStart)
    }
    if (message.skipNotFound === true) {
      writer.uint32(72).bool(message.skipNotFound)
    }
    if (message.verbose === true) {
      writer.uint32(80).bool(message.verbose)
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

          message.blockStoreIds.push(reader.string())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.serviceId = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.clientId = reader.string()
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.readOnly = reader.bool()
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.forceHashType = reader.int32() as any
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.bucketIds.push(reader.string())
          continue
        case 8:
          if (tag !== 64) {
            break
          }

          message.lookupOnStart = reader.bool()
          continue
        case 9:
          if (tag !== 72) {
            break
          }

          message.skipNotFound = reader.bool()
          continue
        case 10:
          if (tag !== 80) {
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
      blockStoreIds: Array.isArray(object?.blockStoreIds)
        ? object.blockStoreIds.map((e: any) => String(e))
        : [],
      serviceId: isSet(object.serviceId) ? String(object.serviceId) : '',
      clientId: isSet(object.clientId) ? String(object.clientId) : '',
      readOnly: isSet(object.readOnly) ? Boolean(object.readOnly) : false,
      forceHashType: isSet(object.forceHashType)
        ? hashTypeFromJSON(object.forceHashType)
        : 0,
      bucketIds: Array.isArray(object?.bucketIds)
        ? object.bucketIds.map((e: any) => String(e))
        : [],
      lookupOnStart: isSet(object.lookupOnStart)
        ? Boolean(object.lookupOnStart)
        : false,
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
    if (message.blockStoreIds) {
      obj.blockStoreIds = message.blockStoreIds.map((e) => e)
    } else {
      obj.blockStoreIds = []
    }
    message.serviceId !== undefined && (obj.serviceId = message.serviceId)
    message.clientId !== undefined && (obj.clientId = message.clientId)
    message.readOnly !== undefined && (obj.readOnly = message.readOnly)
    message.forceHashType !== undefined &&
      (obj.forceHashType = hashTypeToJSON(message.forceHashType))
    if (message.bucketIds) {
      obj.bucketIds = message.bucketIds.map((e) => e)
    } else {
      obj.bucketIds = []
    }
    message.lookupOnStart !== undefined &&
      (obj.lookupOnStart = message.lookupOnStart)
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
    message.blockStoreIds = object.blockStoreIds?.map((e) => e) || []
    message.serviceId = object.serviceId ?? ''
    message.clientId = object.clientId ?? ''
    message.readOnly = object.readOnly ?? false
    message.forceHashType = object.forceHashType ?? 0
    message.bucketIds = object.bucketIds?.map((e) => e) || []
    message.lookupOnStart = object.lookupOnStart ?? false
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
