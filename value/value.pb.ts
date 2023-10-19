/* eslint-disable */
import { BlockRef } from '@go/github.com/aperturerobotics/hydra/block/block.pb.js'
import { ObjectRef } from '@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js'
import Long from 'long'
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
  /** ValueType_WORLD_OBJECT_SNAPSHOT - ValueType_WORLD_OBJECT_SNAPSHOT is a snapshot of a WorldObject state. */
  ValueType_WORLD_OBJECT_SNAPSHOT = 3,
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
    case 3:
    case 'ValueType_WORLD_OBJECT_SNAPSHOT':
      return ValueType.ValueType_WORLD_OBJECT_SNAPSHOT
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
    case ValueType.ValueType_WORLD_OBJECT_SNAPSHOT:
      return 'ValueType_WORLD_OBJECT_SNAPSHOT'
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
  /**
   * ValueType is the type of value.
   * 0: indicates empty value.
   */
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
  /**
   * WorldObjectSnapshot is the snapshot of the world object.
   * ValueType: WORLD_OBJECT_SNAPSHOT.
   */
  worldObjectSnapshot: WorldObjectSnapshot | undefined
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

/** WorldObjectSnapshot is a snapshot of a WorldObject state. */
export interface WorldObjectSnapshot {
  /** Key is the unique Object key. */
  key: string
  /**
   * RootRef is the block ref to the root of the object structure.
   * Note: Object type is not stored. Type data is stored in the Graph, inline, or not at all.
   */
  rootRef: ObjectRef | undefined
  /**
   * Rev is the rev nonce of the object.
   * Incremented when a transaction is applied to the object.
   * Incremented when root_ref is changed (SetRootRef).
   * Incremented when adding or removing a graph quad referencing Object.
   */
  rev: Long
  /**
   * ObjectType is the object type determined by the world_types system.
   * May be empty if the object has no <type> defined.
   */
  objectType: string
  /**
   * ObjectParent is the object key of the parent of this object.
   * May be empty if the object has no <parent> defined.
   */
  objectParent: string
}

function createBaseValue(): Value {
  return {
    name: '',
    valueType: 0,
    blockRef: undefined,
    bucketRef: undefined,
    worldObjectSnapshot: undefined,
  }
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
    if (message.worldObjectSnapshot !== undefined) {
      WorldObjectSnapshot.encode(
        message.worldObjectSnapshot,
        writer.uint32(42).fork(),
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Value {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseValue()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.name = reader.string()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.valueType = reader.int32() as any
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.blockRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.bucketRef = ObjectRef.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.worldObjectSnapshot = WorldObjectSnapshot.decode(
            reader,
            reader.uint32(),
          )
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
  // Transform<Value, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Value | Value[]> | Iterable<Value | Value[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Value.encode(p).finish()]
        }
      } else {
        yield* [Value.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Value>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Value> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Value.decode(p)]
        }
      } else {
        yield* [Value.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Value {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      valueType: isSet(object.valueType)
        ? valueTypeFromJSON(object.valueType)
        : 0,
      blockRef: isSet(object.blockRef)
        ? BlockRef.fromJSON(object.blockRef)
        : undefined,
      bucketRef: isSet(object.bucketRef)
        ? ObjectRef.fromJSON(object.bucketRef)
        : undefined,
      worldObjectSnapshot: isSet(object.worldObjectSnapshot)
        ? WorldObjectSnapshot.fromJSON(object.worldObjectSnapshot)
        : undefined,
    }
  },

  toJSON(message: Value): unknown {
    const obj: any = {}
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.valueType !== 0) {
      obj.valueType = valueTypeToJSON(message.valueType)
    }
    if (message.blockRef !== undefined) {
      obj.blockRef = BlockRef.toJSON(message.blockRef)
    }
    if (message.bucketRef !== undefined) {
      obj.bucketRef = ObjectRef.toJSON(message.bucketRef)
    }
    if (message.worldObjectSnapshot !== undefined) {
      obj.worldObjectSnapshot = WorldObjectSnapshot.toJSON(
        message.worldObjectSnapshot,
      )
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Value>, I>>(base?: I): Value {
    return Value.fromPartial(base ?? ({} as any))
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
    message.worldObjectSnapshot =
      object.worldObjectSnapshot !== undefined &&
      object.worldObjectSnapshot !== null
        ? WorldObjectSnapshot.fromPartial(object.worldObjectSnapshot)
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
    writer: _m0.Writer = _m0.Writer.create(),
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
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseResult()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.success = reader.bool()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.failError = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.canceled = reader.bool()
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
  // Transform<Result, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Result | Result[]> | Iterable<Result | Result[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Result.encode(p).finish()]
        }
      } else {
        yield* [Result.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Result>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Result> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Result.decode(p)]
        }
      } else {
        yield* [Result.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Result {
    return {
      success: isSet(object.success)
        ? globalThis.Boolean(object.success)
        : false,
      failError: isSet(object.failError)
        ? globalThis.String(object.failError)
        : '',
      canceled: isSet(object.canceled)
        ? globalThis.Boolean(object.canceled)
        : false,
    }
  },

  toJSON(message: Result): unknown {
    const obj: any = {}
    if (message.success === true) {
      obj.success = message.success
    }
    if (message.failError !== '') {
      obj.failError = message.failError
    }
    if (message.canceled === true) {
      obj.canceled = message.canceled
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Result>, I>>(base?: I): Result {
    return Result.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Result>, I>>(object: I): Result {
    const message = createBaseResult()
    message.success = object.success ?? false
    message.failError = object.failError ?? ''
    message.canceled = object.canceled ?? false
    return message
  },
}

function createBaseWorldObjectSnapshot(): WorldObjectSnapshot {
  return {
    key: '',
    rootRef: undefined,
    rev: Long.UZERO,
    objectType: '',
    objectParent: '',
  }
}

export const WorldObjectSnapshot = {
  encode(
    message: WorldObjectSnapshot,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== '') {
      writer.uint32(10).string(message.key)
    }
    if (message.rootRef !== undefined) {
      ObjectRef.encode(message.rootRef, writer.uint32(18).fork()).ldelim()
    }
    if (!message.rev.isZero()) {
      writer.uint32(24).uint64(message.rev)
    }
    if (message.objectType !== '') {
      writer.uint32(34).string(message.objectType)
    }
    if (message.objectParent !== '') {
      writer.uint32(42).string(message.objectParent)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WorldObjectSnapshot {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWorldObjectSnapshot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.key = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.rootRef = ObjectRef.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.rev = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.objectType = reader.string()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.objectParent = reader.string()
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
  // Transform<WorldObjectSnapshot, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WorldObjectSnapshot | WorldObjectSnapshot[]>
      | Iterable<WorldObjectSnapshot | WorldObjectSnapshot[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WorldObjectSnapshot.encode(p).finish()]
        }
      } else {
        yield* [WorldObjectSnapshot.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WorldObjectSnapshot>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WorldObjectSnapshot> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WorldObjectSnapshot.decode(p)]
        }
      } else {
        yield* [WorldObjectSnapshot.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WorldObjectSnapshot {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : '',
      rootRef: isSet(object.rootRef)
        ? ObjectRef.fromJSON(object.rootRef)
        : undefined,
      rev: isSet(object.rev) ? Long.fromValue(object.rev) : Long.UZERO,
      objectType: isSet(object.objectType)
        ? globalThis.String(object.objectType)
        : '',
      objectParent: isSet(object.objectParent)
        ? globalThis.String(object.objectParent)
        : '',
    }
  },

  toJSON(message: WorldObjectSnapshot): unknown {
    const obj: any = {}
    if (message.key !== '') {
      obj.key = message.key
    }
    if (message.rootRef !== undefined) {
      obj.rootRef = ObjectRef.toJSON(message.rootRef)
    }
    if (!message.rev.isZero()) {
      obj.rev = (message.rev || Long.UZERO).toString()
    }
    if (message.objectType !== '') {
      obj.objectType = message.objectType
    }
    if (message.objectParent !== '') {
      obj.objectParent = message.objectParent
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WorldObjectSnapshot>, I>>(
    base?: I,
  ): WorldObjectSnapshot {
    return WorldObjectSnapshot.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WorldObjectSnapshot>, I>>(
    object: I,
  ): WorldObjectSnapshot {
    const message = createBaseWorldObjectSnapshot()
    message.key = object.key ?? ''
    message.rootRef =
      object.rootRef !== undefined && object.rootRef !== null
        ? ObjectRef.fromPartial(object.rootRef)
        : undefined
    message.rev =
      object.rev !== undefined && object.rev !== null
        ? Long.fromValue(object.rev)
        : Long.UZERO
    message.objectType = object.objectType ?? ''
    message.objectParent = object.objectParent ?? ''
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
