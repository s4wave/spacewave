/* eslint-disable */
import Long from 'long'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'world.mock'

/** MockObjectOp is a mock object operation. */
export interface MockObjectOp {
  /** NextMsg sets the next message on the mock object. */
  nextMsg: string
}

/** MockWorldOp is a mock world operation. */
export interface MockWorldOp {
  /** ObjectKey is the object key to apply to. */
  objectKey: string
  /** NextMsg sets the next message on the mock object. */
  nextMsg: string
}

function createBaseMockObjectOp(): MockObjectOp {
  return { nextMsg: '' }
}

export const MockObjectOp = {
  encode(
    message: MockObjectOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.nextMsg !== '') {
      writer.uint32(10).string(message.nextMsg)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MockObjectOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseMockObjectOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.nextMsg = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<MockObjectOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<MockObjectOp | MockObjectOp[]>
      | Iterable<MockObjectOp | MockObjectOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MockObjectOp.encode(p).finish()]
        }
      } else {
        yield* [MockObjectOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MockObjectOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<MockObjectOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MockObjectOp.decode(p)]
        }
      } else {
        yield* [MockObjectOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): MockObjectOp {
    return {
      nextMsg: isSet(object.nextMsg) ? String(object.nextMsg) : '',
    }
  },

  toJSON(message: MockObjectOp): unknown {
    const obj: any = {}
    message.nextMsg !== undefined && (obj.nextMsg = message.nextMsg)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<MockObjectOp>, I>>(
    object: I
  ): MockObjectOp {
    const message = createBaseMockObjectOp()
    message.nextMsg = object.nextMsg ?? ''
    return message
  },
}

function createBaseMockWorldOp(): MockWorldOp {
  return { objectKey: '', nextMsg: '' }
}

export const MockWorldOp = {
  encode(
    message: MockWorldOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.objectKey !== '') {
      writer.uint32(10).string(message.objectKey)
    }
    if (message.nextMsg !== '') {
      writer.uint32(18).string(message.nextMsg)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MockWorldOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseMockWorldOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string()
          break
        case 2:
          message.nextMsg = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<MockWorldOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<MockWorldOp | MockWorldOp[]>
      | Iterable<MockWorldOp | MockWorldOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MockWorldOp.encode(p).finish()]
        }
      } else {
        yield* [MockWorldOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MockWorldOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<MockWorldOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MockWorldOp.decode(p)]
        }
      } else {
        yield* [MockWorldOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): MockWorldOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : '',
      nextMsg: isSet(object.nextMsg) ? String(object.nextMsg) : '',
    }
  },

  toJSON(message: MockWorldOp): unknown {
    const obj: any = {}
    message.objectKey !== undefined && (obj.objectKey = message.objectKey)
    message.nextMsg !== undefined && (obj.nextMsg = message.nextMsg)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<MockWorldOp>, I>>(
    object: I
  ): MockWorldOp {
    const message = createBaseMockWorldOp()
    message.objectKey = object.objectKey ?? ''
    message.nextMsg = object.nextMsg ?? ''
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
