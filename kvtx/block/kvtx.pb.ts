/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../../block/block.pb.js'

export const protobufPackage = 'kvtx.block'

/** KVImplType is the key/value store implementation enum. */
export enum KVImplType {
  /** KV_IMPL_TYPE_UNKNOWN - KEY_VALUE_IMPL_UNKNOWN is the zero value. */
  KV_IMPL_TYPE_UNKNOWN = 0,
  /** KV_IMPL_TYPE_IAVL - KEY_VALUE_IMPL_IAVL is the immutable avl tree value. */
  KV_IMPL_TYPE_IAVL = 1,
  UNRECOGNIZED = -1,
}

export function kVImplTypeFromJSON(object: any): KVImplType {
  switch (object) {
    case 0:
    case 'KV_IMPL_TYPE_UNKNOWN':
      return KVImplType.KV_IMPL_TYPE_UNKNOWN
    case 1:
    case 'KV_IMPL_TYPE_IAVL':
      return KVImplType.KV_IMPL_TYPE_IAVL
    case -1:
    case 'UNRECOGNIZED':
    default:
      return KVImplType.UNRECOGNIZED
  }
}

export function kVImplTypeToJSON(object: KVImplType): string {
  switch (object) {
    case KVImplType.KV_IMPL_TYPE_UNKNOWN:
      return 'KV_IMPL_TYPE_UNKNOWN'
    case KVImplType.KV_IMPL_TYPE_IAVL:
      return 'KV_IMPL_TYPE_IAVL'
    case KVImplType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * KeyValueStore is the root of a Key-Value Transaction store.
 * Allows for multiple options of underlying block structures.
 */
export interface KeyValueStore {
  /** ImplType is the key value implementation type. */
  implType: KVImplType
  /**
   * IavlRoot is the root node for the iavl tree.
   * KV_IMPL_TYPE_IAVL
   */
  iavlRoot: BlockRef | undefined
}

function createBaseKeyValueStore(): KeyValueStore {
  return { implType: 0, iavlRoot: undefined }
}

export const KeyValueStore = {
  encode(
    message: KeyValueStore,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.implType !== 0) {
      writer.uint32(8).int32(message.implType)
    }
    if (message.iavlRoot !== undefined) {
      BlockRef.encode(message.iavlRoot, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KeyValueStore {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKeyValueStore()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 8) {
            break
          }

          message.implType = reader.int32() as any
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.iavlRoot = BlockRef.decode(reader, reader.uint32())
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
  // Transform<KeyValueStore, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KeyValueStore | KeyValueStore[]>
      | Iterable<KeyValueStore | KeyValueStore[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyValueStore.encode(p).finish()]
        }
      } else {
        yield* [KeyValueStore.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KeyValueStore>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KeyValueStore> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyValueStore.decode(p)]
        }
      } else {
        yield* [KeyValueStore.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KeyValueStore {
    return {
      implType: isSet(object.implType)
        ? kVImplTypeFromJSON(object.implType)
        : 0,
      iavlRoot: isSet(object.iavlRoot)
        ? BlockRef.fromJSON(object.iavlRoot)
        : undefined,
    }
  },

  toJSON(message: KeyValueStore): unknown {
    const obj: any = {}
    message.implType !== undefined &&
      (obj.implType = kVImplTypeToJSON(message.implType))
    message.iavlRoot !== undefined &&
      (obj.iavlRoot = message.iavlRoot
        ? BlockRef.toJSON(message.iavlRoot)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<KeyValueStore>, I>>(
    base?: I
  ): KeyValueStore {
    return KeyValueStore.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<KeyValueStore>, I>>(
    object: I
  ): KeyValueStore {
    const message = createBaseKeyValueStore()
    message.implType = object.implType ?? 0
    message.iavlRoot =
      object.iavlRoot !== undefined && object.iavlRoot !== null
        ? BlockRef.fromPartial(object.iavlRoot)
        : undefined
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
