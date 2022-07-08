/* eslint-disable */
import Long from 'long'
import { Quad } from '../quad/quad.pb.js'
import { BloomFilter } from '../bloom/bloom.pb.js'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'filters'

/**
 * KeyFilters contains fields used to determine if a key might be in a set.
 * False-negative rate 0%, false-positive rate variable.
 */
export interface KeyFilters {
  /**
   * KeyPrefix is the common prefix affected by all included operations.
   * Empty if the operation affected keys without a common prefix.
   * Ignore this field if it is empty.
   * If a Graph transaction, this will be empty.
   */
  keyPrefix: string
  /**
   * QuadPrefix contains prefixes affected by selected graph quads.
   * Ignore this field if it is empty.
   * If a Object transaction, this will be empty.
   */
  quadPrefix: Quad | undefined
  /**
   * KeyBloom is a bloom filter with all included keys.
   * Also includes subject, obj fields of the quad.
   * Capacity is min 512 (300bytes), max 500k (300KiB) at 10% FP rate.
   * False-negative rate 0%, false-positive rate variable.
   * If change_batch.prev_ref is empty, this field will also be empty.
   */
  keyBloom: BloomFilter | undefined
}

function createBaseKeyFilters(): KeyFilters {
  return { keyPrefix: '', quadPrefix: undefined, keyBloom: undefined }
}

export const KeyFilters = {
  encode(
    message: KeyFilters,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.keyPrefix !== '') {
      writer.uint32(10).string(message.keyPrefix)
    }
    if (message.quadPrefix !== undefined) {
      Quad.encode(message.quadPrefix, writer.uint32(18).fork()).ldelim()
    }
    if (message.keyBloom !== undefined) {
      BloomFilter.encode(message.keyBloom, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KeyFilters {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKeyFilters()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.keyPrefix = reader.string()
          break
        case 2:
          message.quadPrefix = Quad.decode(reader, reader.uint32())
          break
        case 3:
          message.keyBloom = BloomFilter.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KeyFilters, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KeyFilters | KeyFilters[]>
      | Iterable<KeyFilters | KeyFilters[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyFilters.encode(p).finish()]
        }
      } else {
        yield* [KeyFilters.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KeyFilters>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KeyFilters> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeyFilters.decode(p)]
        }
      } else {
        yield* [KeyFilters.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KeyFilters {
    return {
      keyPrefix: isSet(object.keyPrefix) ? String(object.keyPrefix) : '',
      quadPrefix: isSet(object.quadPrefix)
        ? Quad.fromJSON(object.quadPrefix)
        : undefined,
      keyBloom: isSet(object.keyBloom)
        ? BloomFilter.fromJSON(object.keyBloom)
        : undefined,
    }
  },

  toJSON(message: KeyFilters): unknown {
    const obj: any = {}
    message.keyPrefix !== undefined && (obj.keyPrefix = message.keyPrefix)
    message.quadPrefix !== undefined &&
      (obj.quadPrefix = message.quadPrefix
        ? Quad.toJSON(message.quadPrefix)
        : undefined)
    message.keyBloom !== undefined &&
      (obj.keyBloom = message.keyBloom
        ? BloomFilter.toJSON(message.keyBloom)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<KeyFilters>, I>>(
    object: I
  ): KeyFilters {
    const message = createBaseKeyFilters()
    message.keyPrefix = object.keyPrefix ?? ''
    message.quadPrefix =
      object.quadPrefix !== undefined && object.quadPrefix !== null
        ? Quad.fromPartial(object.quadPrefix)
        : undefined
    message.keyBloom =
      object.keyBloom !== undefined && object.keyBloom !== null
        ? BloomFilter.fromPartial(object.keyBloom)
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
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & Record<
        Exclude<keyof I, KeysOfUnion<P>>,
        never
      >

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
