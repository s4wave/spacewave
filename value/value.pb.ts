/* eslint-disable */
import Long from 'long'
import { BlockRef } from '@go/github.com/aperturerobotics/hydra/block/block.pb.js'
import { ObjectRef } from '@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'forge.value'

/** ValueType is the set of possible value types. */
export enum ValueType {
  /** ValueType_UNKNOWN - ValueType_UNKNOWN is the unknown value type. */
  ValueType_UNKNOWN = 0,
  /** ValueType_BLOCK_REF - ValueType_BLOCK_REF is a block reference in the same bucket. */
  ValueType_BLOCK_REF = 1,
  /** ValueType_BUCKET_REF - ValueType_BUCKET_REF is a cross-bucket block reference w/ transform config. */
  ValueType_BUCKET_REF = 2,
  UNRECOGNIZED = -1,
}

export function valueTypeFromJSON(object: any): ValueType {
  switch (object) {
    case 0:
    case 'ValueType_UNKNOWN':
      return ValueType.ValueType_UNKNOWN
    case 1:
    case 'ValueType_BLOCK_REF':
      return ValueType.ValueType_BLOCK_REF
    case 2:
    case 'ValueType_BUCKET_REF':
      return ValueType.ValueType_BUCKET_REF
    case -1:
    case 'UNRECOGNIZED':
    default:
      return ValueType.UNRECOGNIZED
  }
}

export function valueTypeToJSON(object: ValueType): string {
  switch (object) {
    case ValueType.ValueType_UNKNOWN:
      return 'ValueType_UNKNOWN'
    case ValueType.ValueType_BLOCK_REF:
      return 'ValueType_BLOCK_REF'
    case ValueType.ValueType_BUCKET_REF:
      return 'ValueType_BUCKET_REF'
    case ValueType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Value contains a input/output value. */
export interface Value {
  /**
   * Name is the unique name of the value in the parent set.
   * Note: for in-line values, this may be empty.
   */
  name: string
  /** ValueType is the type of value. */
  valueType: ValueType
  /**
   * BlockRef is the value for the block ref type.
   * ValueType: BLOCK_REF
   */
  blockRef: BlockRef | undefined
  /**
   * BucketRef is a cross-bucket block reference w/ transform config.
   * Note: the target block DAG may be copied into other buckets.
   * ValueType: BUCKET_REF
   */
  bucketRef: ObjectRef | undefined
}

/** Result is information about a step in COMPLETE state. */
export interface Result {
  /** Success indicates the pass succeeded, if false, it failed. */
  success: boolean
  /** FailError contains the error if the pass failed. */
  failError: string
  /** Canceled is set if the step was canceled. */
  canceled: boolean
}

function createBaseValue(): Value {
  return { name: '', valueType: 0, blockRef: undefined, bucketRef: undefined }
}

export const Value = {
  encode(message: Value, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.valueType !== 0) {
      writer.uint32(16).int32(message.valueType)
    }
    if (message.blockRef !== undefined) {
      BlockRef.encode(message.blockRef, writer.uint32(26).fork()).ldelim()
    }
    if (message.bucketRef !== undefined) {
      ObjectRef.encode(message.bucketRef, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Value {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseValue()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string()
          break
        case 2:
          message.valueType = reader.int32() as any
          break
        case 3:
          message.blockRef = BlockRef.decode(reader, reader.uint32())
          break
        case 4:
          message.bucketRef = ObjectRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Value, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Value | Value[]> | Iterable<Value | Value[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Value.encode(p).finish()]
        }
      } else {
        yield* [Value.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Value>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Value> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Value.decode(p)]
        }
      } else {
        yield* [Value.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Value {
    return {
      name: isSet(object.name) ? String(object.name) : '',
      valueType: isSet(object.valueType)
        ? valueTypeFromJSON(object.valueType)
        : 0,
      blockRef: isSet(object.blockRef)
        ? BlockRef.fromJSON(object.blockRef)
        : undefined,
      bucketRef: isSet(object.bucketRef)
        ? ObjectRef.fromJSON(object.bucketRef)
        : undefined,
    }
  },

  toJSON(message: Value): unknown {
    const obj: any = {}
    message.name !== undefined && (obj.name = message.name)
    message.valueType !== undefined &&
      (obj.valueType = valueTypeToJSON(message.valueType))
    message.blockRef !== undefined &&
      (obj.blockRef = message.blockRef
        ? BlockRef.toJSON(message.blockRef)
        : undefined)
    message.bucketRef !== undefined &&
      (obj.bucketRef = message.bucketRef
        ? ObjectRef.toJSON(message.bucketRef)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Value>, I>>(object: I): Value {
    const message = createBaseValue()
    message.name = object.name ?? ''
    message.valueType = object.valueType ?? 0
    message.blockRef =
      object.blockRef !== undefined && object.blockRef !== null
        ? BlockRef.fromPartial(object.blockRef)
        : undefined
    message.bucketRef =
      object.bucketRef !== undefined && object.bucketRef !== null
        ? ObjectRef.fromPartial(object.bucketRef)
        : undefined
    return message
  },
}

function createBaseResult(): Result {
  return { success: false, failError: '', canceled: false }
}

export const Result = {
  encode(
    message: Result,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.success === true) {
      writer.uint32(8).bool(message.success)
    }
    if (message.failError !== '') {
      writer.uint32(18).string(message.failError)
    }
    if (message.canceled === true) {
      writer.uint32(24).bool(message.canceled)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Result {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseResult()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.success = reader.bool()
          break
        case 2:
          message.failError = reader.string()
          break
        case 3:
          message.canceled = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Result, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Result | Result[]> | Iterable<Result | Result[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Result.encode(p).finish()]
        }
      } else {
        yield* [Result.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Result>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Result> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Result.decode(p)]
        }
      } else {
        yield* [Result.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Result {
    return {
      success: isSet(object.success) ? Boolean(object.success) : false,
      failError: isSet(object.failError) ? String(object.failError) : '',
      canceled: isSet(object.canceled) ? Boolean(object.canceled) : false,
    }
  },

  toJSON(message: Result): unknown {
    const obj: any = {}
    message.success !== undefined && (obj.success = message.success)
    message.failError !== undefined && (obj.failError = message.failError)
    message.canceled !== undefined && (obj.canceled = message.canceled)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Result>, I>>(object: I): Result {
    const message = createBaseResult()
    message.success = object.success ?? false
    message.failError = object.failError ?? ''
    message.canceled = object.canceled ?? false
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
