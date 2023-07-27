/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Result, Value } from '../../value/value.pb.js'

export const protobufPackage = 'execution.tx'

/** TxType indicates the kind of transaction. */
export enum TxType {
  TxType_INVALID = 0,
  /** TxType_START - TxType_START starts execution of the transaction. */
  TxType_START = 1,
  /** TxType_SET_OUTPUTS - TxType_SET_OUTPUTS changes the value of an output. */
  TxType_SET_OUTPUTS = 2,
  /** TxType_COMPLETE - TxType_COMPLETE sets the result of the execution. */
  TxType_COMPLETE = 3,
  UNRECOGNIZED = -1,
}

export function txTypeFromJSON(object: any): TxType {
  switch (object) {
    case 0:
    case 'TxType_INVALID':
      return TxType.TxType_INVALID
    case 1:
    case 'TxType_START':
      return TxType.TxType_START
    case 2:
    case 'TxType_SET_OUTPUTS':
      return TxType.TxType_SET_OUTPUTS
    case 3:
    case 'TxType_COMPLETE':
      return TxType.TxType_COMPLETE
    case -1:
    case 'UNRECOGNIZED':
    default:
      return TxType.UNRECOGNIZED
  }
}

export function txTypeToJSON(object: TxType): string {
  switch (object) {
    case TxType.TxType_INVALID:
      return 'TxType_INVALID'
    case TxType.TxType_START:
      return 'TxType_START'
    case TxType.TxType_SET_OUTPUTS:
      return 'TxType_SET_OUTPUTS'
    case TxType.TxType_COMPLETE:
      return 'TxType_COMPLETE'
    case TxType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Tx is the on-the-wire representation of a transaction. */
export interface Tx {
  /** TxType is the kind of transaction this is. */
  txType: TxType
  /**
   * TxStart contains the start transaction tx.
   * TxType_INVALID
   */
  txStart: TxStart | undefined
  /**
   * TxSetOutputs contains the set outputs tx.
   * TxType_SET_OUTPUTS
   */
  txSetOutputs: TxSetOutputs | undefined
  /**
   * TxComplete contains the complete tx.
   * TxType_COMPLETE
   */
  txComplete: TxComplete | undefined
}

/**
 * TxStart starts the execution with a peer id.
 * Execution must be in the PENDING state.
 * TxType: TxType_START
 */
export interface TxStart {
  /**
   * PeerId is the peer identifier to set as the executor.
   * Must be the same as the sender of the transaction.
   * Must match peer_id on the Execution if it's not empty.
   */
  peerId: string
}

/**
 * TxSetOutputs updates the value of one or more execution outputs.
 * Execution must be in the RUNNING state.
 * Sender must be the peer_id specified on the Execution.
 * TxType: TxType_SET_OUTPUTS
 */
export interface TxSetOutputs {
  /** Outputs is the set of values to set. */
  outputs: Value[]
  /** ClearOld indicates to clear all old output values before setting. */
  clearOld: boolean
}

/**
 * TxComplete completes the execution by setting the result.
 * Execution must be in the RUNNING state.
 * Sender must be the peer_id specified on the Execution.
 * TxType: TxType_COMPLETE
 */
export interface TxComplete {
  /** Result is information about the outcome of a completed execution. */
  result: Result | undefined
}

function createBaseTx(): Tx {
  return {
    txType: 0,
    txStart: undefined,
    txSetOutputs: undefined,
    txComplete: undefined,
  }
}

export const Tx = {
  encode(message: Tx, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.txType !== 0) {
      writer.uint32(8).int32(message.txType)
    }
    if (message.txStart !== undefined) {
      TxStart.encode(message.txStart, writer.uint32(18).fork()).ldelim()
    }
    if (message.txSetOutputs !== undefined) {
      TxSetOutputs.encode(
        message.txSetOutputs,
        writer.uint32(26).fork(),
      ).ldelim()
    }
    if (message.txComplete !== undefined) {
      TxComplete.encode(message.txComplete, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Tx {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTx()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.txType = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.txStart = TxStart.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.txSetOutputs = TxSetOutputs.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.txComplete = TxComplete.decode(reader, reader.uint32())
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
  // Transform<Tx, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Tx | Tx[]> | Iterable<Tx | Tx[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Tx.encode(p).finish()]
        }
      } else {
        yield* [Tx.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Tx>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Tx> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Tx.decode(p)]
        }
      } else {
        yield* [Tx.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Tx {
    return {
      txType: isSet(object.txType) ? txTypeFromJSON(object.txType) : 0,
      txStart: isSet(object.txStart)
        ? TxStart.fromJSON(object.txStart)
        : undefined,
      txSetOutputs: isSet(object.txSetOutputs)
        ? TxSetOutputs.fromJSON(object.txSetOutputs)
        : undefined,
      txComplete: isSet(object.txComplete)
        ? TxComplete.fromJSON(object.txComplete)
        : undefined,
    }
  },

  toJSON(message: Tx): unknown {
    const obj: any = {}
    if (message.txType !== 0) {
      obj.txType = txTypeToJSON(message.txType)
    }
    if (message.txStart !== undefined) {
      obj.txStart = TxStart.toJSON(message.txStart)
    }
    if (message.txSetOutputs !== undefined) {
      obj.txSetOutputs = TxSetOutputs.toJSON(message.txSetOutputs)
    }
    if (message.txComplete !== undefined) {
      obj.txComplete = TxComplete.toJSON(message.txComplete)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Tx>, I>>(base?: I): Tx {
    return Tx.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Tx>, I>>(object: I): Tx {
    const message = createBaseTx()
    message.txType = object.txType ?? 0
    message.txStart =
      object.txStart !== undefined && object.txStart !== null
        ? TxStart.fromPartial(object.txStart)
        : undefined
    message.txSetOutputs =
      object.txSetOutputs !== undefined && object.txSetOutputs !== null
        ? TxSetOutputs.fromPartial(object.txSetOutputs)
        : undefined
    message.txComplete =
      object.txComplete !== undefined && object.txComplete !== null
        ? TxComplete.fromPartial(object.txComplete)
        : undefined
    return message
  },
}

function createBaseTxStart(): TxStart {
  return { peerId: '' }
}

export const TxStart = {
  encode(
    message: TxStart,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.peerId !== '') {
      writer.uint32(10).string(message.peerId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxStart {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxStart()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.peerId = reader.string()
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
  // Transform<TxStart, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<TxStart | TxStart[]> | Iterable<TxStart | TxStart[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxStart.encode(p).finish()]
        }
      } else {
        yield* [TxStart.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxStart>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxStart> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxStart.decode(p)]
        }
      } else {
        yield* [TxStart.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxStart {
    return { peerId: isSet(object.peerId) ? String(object.peerId) : '' }
  },

  toJSON(message: TxStart): unknown {
    const obj: any = {}
    if (message.peerId !== '') {
      obj.peerId = message.peerId
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TxStart>, I>>(base?: I): TxStart {
    return TxStart.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxStart>, I>>(object: I): TxStart {
    const message = createBaseTxStart()
    message.peerId = object.peerId ?? ''
    return message
  },
}

function createBaseTxSetOutputs(): TxSetOutputs {
  return { outputs: [], clearOld: false }
}

export const TxSetOutputs = {
  encode(
    message: TxSetOutputs,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.outputs) {
      Value.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    if (message.clearOld === true) {
      writer.uint32(16).bool(message.clearOld)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxSetOutputs {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxSetOutputs()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.outputs.push(Value.decode(reader, reader.uint32()))
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.clearOld = reader.bool()
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
  // Transform<TxSetOutputs, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxSetOutputs | TxSetOutputs[]>
      | Iterable<TxSetOutputs | TxSetOutputs[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxSetOutputs.encode(p).finish()]
        }
      } else {
        yield* [TxSetOutputs.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxSetOutputs>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxSetOutputs> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxSetOutputs.decode(p)]
        }
      } else {
        yield* [TxSetOutputs.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxSetOutputs {
    return {
      outputs: Array.isArray(object?.outputs)
        ? object.outputs.map((e: any) => Value.fromJSON(e))
        : [],
      clearOld: isSet(object.clearOld) ? Boolean(object.clearOld) : false,
    }
  },

  toJSON(message: TxSetOutputs): unknown {
    const obj: any = {}
    if (message.outputs?.length) {
      obj.outputs = message.outputs.map((e) => Value.toJSON(e))
    }
    if (message.clearOld === true) {
      obj.clearOld = message.clearOld
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TxSetOutputs>, I>>(
    base?: I,
  ): TxSetOutputs {
    return TxSetOutputs.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxSetOutputs>, I>>(
    object: I,
  ): TxSetOutputs {
    const message = createBaseTxSetOutputs()
    message.outputs = object.outputs?.map((e) => Value.fromPartial(e)) || []
    message.clearOld = object.clearOld ?? false
    return message
  },
}

function createBaseTxComplete(): TxComplete {
  return { result: undefined }
}

export const TxComplete = {
  encode(
    message: TxComplete,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.result !== undefined) {
      Result.encode(message.result, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxComplete {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxComplete()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.result = Result.decode(reader, reader.uint32())
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
  // Transform<TxComplete, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxComplete | TxComplete[]>
      | Iterable<TxComplete | TxComplete[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxComplete.encode(p).finish()]
        }
      } else {
        yield* [TxComplete.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxComplete>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxComplete> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxComplete.decode(p)]
        }
      } else {
        yield* [TxComplete.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxComplete {
    return {
      result: isSet(object.result) ? Result.fromJSON(object.result) : undefined,
    }
  },

  toJSON(message: TxComplete): unknown {
    const obj: any = {}
    if (message.result !== undefined) {
      obj.result = Result.toJSON(message.result)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TxComplete>, I>>(base?: I): TxComplete {
    return TxComplete.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxComplete>, I>>(
    object: I,
  ): TxComplete {
    const message = createBaseTxComplete()
    message.result =
      object.result !== undefined && object.result !== null
        ? Result.fromPartial(object.result)
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
