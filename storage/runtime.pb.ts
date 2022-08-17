/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'storage'

/** StorageInfo is information about an available storage method. */
export interface StorageInfo {
  /**
   * Isolated indicates that keys written to named stores are isolated from
   * other named stores from the same Storage source. In other words, each named
   * store is backed by a separate database. If false, each named store should
   * be separated with a key prefix (or similar).
   */
  isolated: boolean
  /**
   * Cache indicates this is cache storage where keys may be evicted. However,
   * cache storage is expected to be faster than non-cache storage.
   */
  cache: boolean
}

function createBaseStorageInfo(): StorageInfo {
  return { isolated: false, cache: false }
}

export const StorageInfo = {
  encode(
    message: StorageInfo,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.isolated === true) {
      writer.uint32(8).bool(message.isolated)
    }
    if (message.cache === true) {
      writer.uint32(16).bool(message.cache)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StorageInfo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseStorageInfo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.isolated = reader.bool()
          break
        case 2:
          message.cache = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<StorageInfo, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<StorageInfo | StorageInfo[]>
      | Iterable<StorageInfo | StorageInfo[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StorageInfo.encode(p).finish()]
        }
      } else {
        yield* [StorageInfo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, StorageInfo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<StorageInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StorageInfo.decode(p)]
        }
      } else {
        yield* [StorageInfo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): StorageInfo {
    return {
      isolated: isSet(object.isolated) ? Boolean(object.isolated) : false,
      cache: isSet(object.cache) ? Boolean(object.cache) : false,
    }
  },

  toJSON(message: StorageInfo): unknown {
    const obj: any = {}
    message.isolated !== undefined && (obj.isolated = message.isolated)
    message.cache !== undefined && (obj.cache = message.cache)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<StorageInfo>, I>>(
    object: I
  ): StorageInfo {
    const message = createBaseStorageInfo()
    message.isolated = object.isolated ?? false
    message.cache = object.cache ?? false
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
