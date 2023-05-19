/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bitset'

/** BitSet is a block-backed BitSet representation. */
export interface BitSet {
  /** Set is the set of uint64 representing the bitset. */
  set: Long[]
  /** Len is the length of the bitset. */
  len: number
}

function createBaseBitSet(): BitSet {
  return { set: [], len: 0 }
}

export const BitSet = {
  encode(
    message: BitSet,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    writer.uint32(10).fork()
    for (const v of message.set) {
      writer.uint64(v)
    }
    writer.ldelim()
    if (message.len !== 0) {
      writer.uint32(16).uint32(message.len)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BitSet {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBitSet()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag === 8) {
            message.set.push(reader.uint64() as Long)

            continue
          }

          if (tag === 10) {
            const end2 = reader.uint32() + reader.pos
            while (reader.pos < end2) {
              message.set.push(reader.uint64() as Long)
            }

            continue
          }

          break
        case 2:
          if (tag !== 16) {
            break
          }

          message.len = reader.uint32()
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
  // Transform<BitSet, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BitSet | BitSet[]> | Iterable<BitSet | BitSet[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BitSet.encode(p).finish()]
        }
      } else {
        yield* [BitSet.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BitSet>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<BitSet> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BitSet.decode(p)]
        }
      } else {
        yield* [BitSet.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): BitSet {
    return {
      set: Array.isArray(object?.set)
        ? object.set.map((e: any) => Long.fromValue(e))
        : [],
      len: isSet(object.len) ? Number(object.len) : 0,
    }
  },

  toJSON(message: BitSet): unknown {
    const obj: any = {}
    if (message.set) {
      obj.set = message.set.map((e) => (e || Long.UZERO).toString())
    } else {
      obj.set = []
    }
    message.len !== undefined && (obj.len = Math.round(message.len))
    return obj
  },

  create<I extends Exact<DeepPartial<BitSet>, I>>(base?: I): BitSet {
    return BitSet.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<BitSet>, I>>(object: I): BitSet {
    const message = createBaseBitSet()
    message.set = object.set?.map((e) => Long.fromValue(e)) || []
    message.len = object.len ?? 0
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
