/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { ValueSet } from '../../target/target.pb.js'
import { Result } from '../../value/value.pb.js'

export const protobufPackage = 'task.tx'

/** TxType indicates the kind of transaction. */
export enum TxType {
  TxType_INVALID = 0,
  /**
   * TxType_UPDATE_INPUTS - TxType_UPDATE_INPUTS updates the ValueSet inputs and target.
   * If the values are identical: does nothing.
   * If changed: transitions to PENDING state and cancels any ongoing Pass.
   */
  TxType_UPDATE_INPUTS = 1,
  /**
   * TxType_START - TxType_START marks the pass as running.
   * Transitions to state RUNNING from PENDING.
   */
  TxType_START = 2,
  /**
   * TxType_UPDATE_WITH_PASS_STATE - TxType_UPDATE_WITH_PASS_STATE updates the state of the Task based the Pass.
   * If none or missing: can transition to PENDING.
   * If failed: can transition to COMPLETE or RETRY.
   * If success or complete: can transition to CHECKING state.
   */
  TxType_UPDATE_WITH_PASS_STATE = 3,
  /**
   * TxType_COMPLETE - TxType_COMPLETE sets the result of the Task.
   * If failed, can transition from any state.
   * If success, must transition from CHECKING state.
   * If success, all Execution states must be Successful.
   */
  TxType_COMPLETE = 4,
  UNRECOGNIZED = -1,
}

export function txTypeFromJSON(object: any): TxType {
  switch (object) {
    case 0:
    case 'TxType_INVALID':
      return TxType.TxType_INVALID
    case 1:
    case 'TxType_UPDATE_INPUTS':
      return TxType.TxType_UPDATE_INPUTS
    case 2:
    case 'TxType_START':
      return TxType.TxType_START
    case 3:
    case 'TxType_UPDATE_WITH_PASS_STATE':
      return TxType.TxType_UPDATE_WITH_PASS_STATE
    case 4:
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
    case TxType.TxType_UPDATE_INPUTS:
      return 'TxType_UPDATE_INPUTS'
    case TxType.TxType_START:
      return 'TxType_START'
    case TxType.TxType_UPDATE_WITH_PASS_STATE:
      return 'TxType_UPDATE_WITH_PASS_STATE'
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
   * TaskObjectKey is the Task object ID this is associated with.
   * The Task object must already exist.
   */
  taskObjectKey: string
  /**
   * TxUpdateInputs updates the Task with the latest Target and Inputs.
   * TxType_UPDATE_INPUTS
   */
  txUpdateInputs: TxUpdateInputs | undefined
  /**
   * TxStart contains the start transaction tx.
   * TxType_START
   */
  txStart: TxStart | undefined
  /**
   * TxUpdatePassState contains the update pass state tx.
   * TxType_UPDATE_WITH_PASS_STATE
   */
  txUpdateWithPassState: TxUpdateWithPassState | undefined
  /**
   * TxComplete contains the complete tx.
   * TxType_COMPLETE
   */
  txComplete: TxComplete | undefined
}

/**
 * TxUpdateInputs updates the Task with the latest Target and Inputs.
 * If the value is identical: does nothing.
 * If changed: transitions to PENDING state and cancels any ongoing Pass.
 *
 * TxType: TxType_UPDATE_INPUTS
 */
export interface TxUpdateInputs {
  /** UpdateTarget indicates to update the TargetRef if necessary. */
  updateTarget: boolean
  /** ResetInputs indicates to clear the existing inputs before setting. */
  resetInputs: boolean
  /**
   * ValueSet is the set of inputs to set for the task.
   * Outputs must be empty.
   * Any Value with Type=UNKNOWN (0) will be deleted.
   */
  valueSet: ValueSet | undefined
}

/**
 * TxStart starts the execution of the Task by creating a Pass.
 * Transitions from PENDING, RETRY, or COMPLETE to RUNNING.
 * Cancels any existing Pass.
 *
 * TxType: TxType_START
 */
export interface TxStart {
  /** AssignSelf assigns the sender of the tx to the new Pass. */
  assignSelf: boolean
}

/**
 * TxUpdateWithPassState updates the state of the Task based on the current Pass.
 * If none or not found: transitions to PENDING.
 * If complete or failed: transitions to CHECKING state.
 * TxType: TxType_UPDATE_WITH_PASS_STATE
 */
export interface TxUpdateWithPassState {}

/**
 * TxComplete completes the execution by setting the result.
 * If failed, may transition from any state.
 * If success, must be in the CHECKING state.
 * If success, the most recent Pass must be in COMPLETE state and not failed.
 * TxType: TxType_COMPLETE
 */
export interface TxComplete {
  /** Result is information about the outcome of a completed pass. */
  result: Result | undefined
  /**
   * ValueSet is the set of outputs from the task.
   * Inputs must be empty.
   * Must match the outputs calculated from the Pass and Execution objects.
   * Must be empty if the result is not success.
   */
  valueSet: ValueSet | undefined
}

function createBaseTx(): Tx {
  return {
    txType: 0,
    taskObjectKey: '',
    txUpdateInputs: undefined,
    txStart: undefined,
    txUpdateWithPassState: undefined,
    txComplete: undefined,
  }
}

export const Tx = {
  encode(message: Tx, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.txType !== 0) {
      writer.uint32(8).int32(message.txType)
    }
    if (message.taskObjectKey !== '') {
      writer.uint32(18).string(message.taskObjectKey)
    }
    if (message.txUpdateInputs !== undefined) {
      TxUpdateInputs.encode(
        message.txUpdateInputs,
        writer.uint32(26).fork(),
      ).ldelim()
    }
    if (message.txStart !== undefined) {
      TxStart.encode(message.txStart, writer.uint32(34).fork()).ldelim()
    }
    if (message.txUpdateWithPassState !== undefined) {
      TxUpdateWithPassState.encode(
        message.txUpdateWithPassState,
        writer.uint32(42).fork(),
      ).ldelim()
    }
    if (message.txComplete !== undefined) {
      TxComplete.encode(message.txComplete, writer.uint32(50).fork()).ldelim()
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

          message.taskObjectKey = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.txUpdateInputs = TxUpdateInputs.decode(
            reader,
            reader.uint32(),
          )
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.txStart = TxStart.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.txUpdateWithPassState = TxUpdateWithPassState.decode(
            reader,
            reader.uint32(),
          )
          continue
        case 6:
          if (tag !== 50) {
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
      taskObjectKey: isSet(object.taskObjectKey)
        ? String(object.taskObjectKey)
        : '',
      txUpdateInputs: isSet(object.txUpdateInputs)
        ? TxUpdateInputs.fromJSON(object.txUpdateInputs)
        : undefined,
      txStart: isSet(object.txStart)
        ? TxStart.fromJSON(object.txStart)
        : undefined,
      txUpdateWithPassState: isSet(object.txUpdateWithPassState)
        ? TxUpdateWithPassState.fromJSON(object.txUpdateWithPassState)
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
    if (message.taskObjectKey !== '') {
      obj.taskObjectKey = message.taskObjectKey
    }
    if (message.txUpdateInputs !== undefined) {
      obj.txUpdateInputs = TxUpdateInputs.toJSON(message.txUpdateInputs)
    }
    if (message.txStart !== undefined) {
      obj.txStart = TxStart.toJSON(message.txStart)
    }
    if (message.txUpdateWithPassState !== undefined) {
      obj.txUpdateWithPassState = TxUpdateWithPassState.toJSON(
        message.txUpdateWithPassState,
      )
    }
    if (message.txComplete !== undefined) {
      obj.txComplete = TxComplete.toJSON(message.txComplete)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Tx>, I>>(base?: I): Tx {
    return Tx.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Tx>, I>>(object: I): Tx {
    const message = createBaseTx()
    message.txType = object.txType ?? 0
    message.taskObjectKey = object.taskObjectKey ?? ''
    message.txUpdateInputs =
      object.txUpdateInputs !== undefined && object.txUpdateInputs !== null
        ? TxUpdateInputs.fromPartial(object.txUpdateInputs)
        : undefined
    message.txStart =
      object.txStart !== undefined && object.txStart !== null
        ? TxStart.fromPartial(object.txStart)
        : undefined
    message.txUpdateWithPassState =
      object.txUpdateWithPassState !== undefined &&
      object.txUpdateWithPassState !== null
        ? TxUpdateWithPassState.fromPartial(object.txUpdateWithPassState)
        : undefined
    message.txComplete =
      object.txComplete !== undefined && object.txComplete !== null
        ? TxComplete.fromPartial(object.txComplete)
        : undefined
    return message
  },
}

function createBaseTxUpdateInputs(): TxUpdateInputs {
  return { updateTarget: false, resetInputs: false, valueSet: undefined }
}

export const TxUpdateInputs = {
  encode(
    message: TxUpdateInputs,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.updateTarget === true) {
      writer.uint32(8).bool(message.updateTarget)
    }
    if (message.resetInputs === true) {
      writer.uint32(16).bool(message.resetInputs)
    }
    if (message.valueSet !== undefined) {
      ValueSet.encode(message.valueSet, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxUpdateInputs {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxUpdateInputs()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.updateTarget = reader.bool()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.resetInputs = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.valueSet = ValueSet.decode(reader, reader.uint32())
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
  // Transform<TxUpdateInputs, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxUpdateInputs | TxUpdateInputs[]>
      | Iterable<TxUpdateInputs | TxUpdateInputs[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxUpdateInputs.encode(p).finish()]
        }
      } else {
        yield* [TxUpdateInputs.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxUpdateInputs>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxUpdateInputs> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxUpdateInputs.decode(p)]
        }
      } else {
        yield* [TxUpdateInputs.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxUpdateInputs {
    return {
      updateTarget: isSet(object.updateTarget)
        ? Boolean(object.updateTarget)
        : false,
      resetInputs: isSet(object.resetInputs)
        ? Boolean(object.resetInputs)
        : false,
      valueSet: isSet(object.valueSet)
        ? ValueSet.fromJSON(object.valueSet)
        : undefined,
    }
  },

  toJSON(message: TxUpdateInputs): unknown {
    const obj: any = {}
    if (message.updateTarget === true) {
      obj.updateTarget = message.updateTarget
    }
    if (message.resetInputs === true) {
      obj.resetInputs = message.resetInputs
    }
    if (message.valueSet !== undefined) {
      obj.valueSet = ValueSet.toJSON(message.valueSet)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TxUpdateInputs>, I>>(
    base?: I,
  ): TxUpdateInputs {
    return TxUpdateInputs.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TxUpdateInputs>, I>>(
    object: I,
  ): TxUpdateInputs {
    const message = createBaseTxUpdateInputs()
    message.updateTarget = object.updateTarget ?? false
    message.resetInputs = object.resetInputs ?? false
    message.valueSet =
      object.valueSet !== undefined && object.valueSet !== null
        ? ValueSet.fromPartial(object.valueSet)
        : undefined
    return message
  },
}

function createBaseTxStart(): TxStart {
  return { assignSelf: false }
}

export const TxStart = {
  encode(
    message: TxStart,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.assignSelf === true) {
      writer.uint32(8).bool(message.assignSelf)
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
          if (tag !== 8) {
            break
          }

          message.assignSelf = reader.bool()
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
    return {
      assignSelf: isSet(object.assignSelf) ? Boolean(object.assignSelf) : false,
    }
  },

  toJSON(message: TxStart): unknown {
    const obj: any = {}
    if (message.assignSelf === true) {
      obj.assignSelf = message.assignSelf
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TxStart>, I>>(base?: I): TxStart {
    return TxStart.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TxStart>, I>>(object: I): TxStart {
    const message = createBaseTxStart()
    message.assignSelf = object.assignSelf ?? false
    return message
  },
}

function createBaseTxUpdateWithPassState(): TxUpdateWithPassState {
  return {}
}

export const TxUpdateWithPassState = {
  encode(
    _: TxUpdateWithPassState,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): TxUpdateWithPassState {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxUpdateWithPassState()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TxUpdateWithPassState, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxUpdateWithPassState | TxUpdateWithPassState[]>
      | Iterable<TxUpdateWithPassState | TxUpdateWithPassState[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxUpdateWithPassState.encode(p).finish()]
        }
      } else {
        yield* [TxUpdateWithPassState.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxUpdateWithPassState>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxUpdateWithPassState> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxUpdateWithPassState.decode(p)]
        }
      } else {
        yield* [TxUpdateWithPassState.decode(pkt)]
      }
    }
  },

  fromJSON(_: any): TxUpdateWithPassState {
    return {}
  },

  toJSON(_: TxUpdateWithPassState): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<TxUpdateWithPassState>, I>>(
    base?: I,
  ): TxUpdateWithPassState {
    return TxUpdateWithPassState.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TxUpdateWithPassState>, I>>(
    _: I,
  ): TxUpdateWithPassState {
    const message = createBaseTxUpdateWithPassState()
    return message
  },
}

function createBaseTxComplete(): TxComplete {
  return { result: undefined, valueSet: undefined }
}

export const TxComplete = {
  encode(
    message: TxComplete,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.result !== undefined) {
      Result.encode(message.result, writer.uint32(10).fork()).ldelim()
    }
    if (message.valueSet !== undefined) {
      ValueSet.encode(message.valueSet, writer.uint32(18).fork()).ldelim()
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
        case 2:
          if (tag !== 18) {
            break
          }

          message.valueSet = ValueSet.decode(reader, reader.uint32())
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
      valueSet: isSet(object.valueSet)
        ? ValueSet.fromJSON(object.valueSet)
        : undefined,
    }
  },

  toJSON(message: TxComplete): unknown {
    const obj: any = {}
    if (message.result !== undefined) {
      obj.result = Result.toJSON(message.result)
    }
    if (message.valueSet !== undefined) {
      obj.valueSet = ValueSet.toJSON(message.valueSet)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TxComplete>, I>>(base?: I): TxComplete {
    return TxComplete.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TxComplete>, I>>(
    object: I,
  ): TxComplete {
    const message = createBaseTxComplete()
    message.result =
      object.result !== undefined && object.result !== null
        ? Result.fromPartial(object.result)
        : undefined
    message.valueSet =
      object.valueSet !== undefined && object.valueSet !== null
        ? ValueSet.fromPartial(object.valueSet)
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
