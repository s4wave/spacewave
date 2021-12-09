/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

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

const baseStorageInfo: object = { isolated: false, cache: false }

export const StorageInfo = {
  encode(message: StorageInfo, writer: Writer = Writer.create()): Writer {
    if (message.isolated === true) {
      writer.uint32(8).bool(message.isolated)
    }
    if (message.cache === true) {
      writer.uint32(16).bool(message.cache)
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): StorageInfo {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseStorageInfo } as StorageInfo
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

  fromJSON(object: any): StorageInfo {
    const message = { ...baseStorageInfo } as StorageInfo
    if (object.isolated !== undefined && object.isolated !== null) {
      message.isolated = Boolean(object.isolated)
    } else {
      message.isolated = false
    }
    if (object.cache !== undefined && object.cache !== null) {
      message.cache = Boolean(object.cache)
    } else {
      message.cache = false
    }
    return message
  },

  toJSON(message: StorageInfo): unknown {
    const obj: any = {}
    message.isolated !== undefined && (obj.isolated = message.isolated)
    message.cache !== undefined && (obj.cache = message.cache)
    return obj
  },

  fromPartial(object: DeepPartial<StorageInfo>): StorageInfo {
    const message = { ...baseStorageInfo } as StorageInfo
    if (object.isolated !== undefined && object.isolated !== null) {
      message.isolated = object.isolated
    } else {
      message.isolated = false
    }
    if (object.cache !== undefined && object.cache !== null) {
      message.cache = object.cache
    } else {
      message.cache = false
    }
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
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

// If you get a compile-error about 'Constructor<Long> and ... have no overlap',
// add '--ts_proto_opt=esModuleInterop=true' as a flag when calling 'protoc'.
if (util.Long !== Long) {
  util.Long = Long as any
  configure()
}
