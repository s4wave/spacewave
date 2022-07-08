/* eslint-disable */
import Long from 'long'
import { Timestamp } from '../../vendor/github.com/aperturerobotics/timestamp/timestamp.pb.js'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'kvtx.genji'

/** StoreMeta contains metadata about a store in the db. */
export interface StoreMeta {
  /** CreatedTs is the timestamp store was created. */
  createdTs: Timestamp | undefined
}

function createBaseStoreMeta(): StoreMeta {
  return { createdTs: undefined }
}

export const StoreMeta = {
  encode(
    message: StoreMeta,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.createdTs !== undefined) {
      Timestamp.encode(message.createdTs, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StoreMeta {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseStoreMeta()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.createdTs = Timestamp.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<StoreMeta, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<StoreMeta | StoreMeta[]>
      | Iterable<StoreMeta | StoreMeta[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StoreMeta.encode(p).finish()]
        }
      } else {
        yield* [StoreMeta.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, StoreMeta>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<StoreMeta> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [StoreMeta.decode(p)]
        }
      } else {
        yield* [StoreMeta.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): StoreMeta {
    return {
      createdTs: isSet(object.createdTs)
        ? Timestamp.fromJSON(object.createdTs)
        : undefined,
    }
  },

  toJSON(message: StoreMeta): unknown {
    const obj: any = {}
    message.createdTs !== undefined &&
      (obj.createdTs = message.createdTs
        ? Timestamp.toJSON(message.createdTs)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<StoreMeta>, I>>(
    object: I
  ): StoreMeta {
    const message = createBaseStoreMeta()
    message.createdTs =
      object.createdTs !== undefined && object.createdTs !== null
        ? Timestamp.fromPartial(object.createdTs)
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
