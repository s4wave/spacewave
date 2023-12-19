/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { ObjectRef } from '../bucket.pb.js'

export const protobufPackage = 'bucket.mock'

/** Root is the root of the mock structure. */
export interface Root {
  /** ExamplePtr is an example reference to a block mock Root. */
  examplePtr: ObjectRef | undefined
}

function createBaseRoot(): Root {
  return { examplePtr: undefined }
}

export const Root = {
  encode(message: Root, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.examplePtr !== undefined) {
      ObjectRef.encode(message.examplePtr, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Root {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.examplePtr = ObjectRef.decode(reader, reader.uint32())
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
  // Transform<Root, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Root | Root[]> | Iterable<Root | Root[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Root.encode(p).finish()]
        }
      } else {
        yield* [Root.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Root>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Root> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Root.decode(p)]
        }
      } else {
        yield* [Root.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Root {
    return {
      examplePtr: isSet(object.examplePtr)
        ? ObjectRef.fromJSON(object.examplePtr)
        : undefined,
    }
  },

  toJSON(message: Root): unknown {
    const obj: any = {}
    if (message.examplePtr !== undefined) {
      obj.examplePtr = ObjectRef.toJSON(message.examplePtr)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Root>, I>>(base?: I): Root {
    return Root.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Root>, I>>(object: I): Root {
    const message = createBaseRoot()
    message.examplePtr =
      object.examplePtr !== undefined && object.examplePtr !== null
        ? ObjectRef.fromPartial(object.examplePtr)
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
