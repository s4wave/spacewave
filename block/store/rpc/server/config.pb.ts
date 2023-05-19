/* eslint-disable */
import {
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'block.store.rpc.server'

/** Config configures the block store rpc server. */
export interface Config {
  /** BucketId is the bucket id to lookup on the bus. */
  bucketId: string
  /**
   * VolumeId is the volume id to read/write from.
   * If unset, uses the BucketLookup API to lookup blocks.
   * Can be empty.
   */
  volumeId: string
  /** Write enables the write api endpoints. */
  write: boolean
  /**
   * ServiceId is the service id to serve LookupRpcService requests.
   * Cannot be empty.
   */
  serviceId: string
  /**
   * ServerIdRe is the regex of server IDs to accept for LookupRpcService.
   * If empty, will accept any.
   */
  serverIdRe: string
  /**
   * ForceHashType forces writing the given hash type to the store.
   * If unset, accepts any hash type the underlying bucket accepts.
   */
  forceHashType: HashType
}

function createBaseConfig(): Config {
  return {
    bucketId: '',
    volumeId: '',
    write: false,
    serviceId: '',
    serverIdRe: '',
    forceHashType: 0,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.bucketId !== '') {
      writer.uint32(10).string(message.bucketId)
    }
    if (message.volumeId !== '') {
      writer.uint32(18).string(message.volumeId)
    }
    if (message.write === true) {
      writer.uint32(24).bool(message.write)
    }
    if (message.serviceId !== '') {
      writer.uint32(34).string(message.serviceId)
    }
    if (message.serverIdRe !== '') {
      writer.uint32(42).string(message.serverIdRe)
    }
    if (message.forceHashType !== 0) {
      writer.uint32(48).int32(message.forceHashType)
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

          message.bucketId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.volumeId = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.write = reader.bool()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.serviceId = reader.string()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.serverIdRe = reader.string()
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.forceHashType = reader.int32() as any
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
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : '',
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : '',
      write: isSet(object.write) ? Boolean(object.write) : false,
      serviceId: isSet(object.serviceId) ? String(object.serviceId) : '',
      serverIdRe: isSet(object.serverIdRe) ? String(object.serverIdRe) : '',
      forceHashType: isSet(object.forceHashType)
        ? hashTypeFromJSON(object.forceHashType)
        : 0,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.bucketId !== undefined && (obj.bucketId = message.bucketId)
    message.volumeId !== undefined && (obj.volumeId = message.volumeId)
    message.write !== undefined && (obj.write = message.write)
    message.serviceId !== undefined && (obj.serviceId = message.serviceId)
    message.serverIdRe !== undefined && (obj.serverIdRe = message.serverIdRe)
    message.forceHashType !== undefined &&
      (obj.forceHashType = hashTypeToJSON(message.forceHashType))
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.bucketId = object.bucketId ?? ''
    message.volumeId = object.volumeId ?? ''
    message.write = object.write ?? false
    message.serviceId = object.serviceId ?? ''
    message.serverIdRe = object.serverIdRe ?? ''
    message.forceHashType = object.forceHashType ?? 0
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
