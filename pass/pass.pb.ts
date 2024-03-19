/* eslint-disable */
import { BlockRef } from '@go/github.com/aperturerobotics/hydra/block/block.pb.js'
import { Timestamp } from '@go/github.com/aperturerobotics/timestamp/timestamp.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import {
  State as State1,
  stateFromJSON as stateFromJSON2,
  stateToJSON as stateToJSON3,
} from '../execution/execution.pb.js'
import { ValueSet } from '../target/target.pb.js'
import { Result } from '../value/value.pb.js'

export const protobufPackage = 'forge.pass'

/** State contains the possible Pass states. */
export enum State {
  /** PassState_UNKNOWN - PassState_UNKNOWN is the unknown type. */
  PassState_UNKNOWN = 0,
  /**
   * PassState_PENDING - PassState_PENDING is the state when the pass is not yet running.
   * Transitions to RUNNING when the pass is promoted to running.
   */
  PassState_PENDING = 1,
  /**
   * PassState_RUNNING - PassState_RUNNING is the state when the executions are running.
   * ExecStates can be added, removed, updated during this state as needed.
   */
  PassState_RUNNING = 2,
  /**
   * PassState_CHECKING - PassState_CHECKING is the state when all exec states are completed.
   * If multiple executions were scheduled, they are checked for agreement.
   * Transition to COMPLETE state on failure (disageement) or success (validation).
   */
  PassState_CHECKING = 3,
  /**
   * PassState_COMPLETE - PassState_COMPLETE is the normal terminal state of the pass.
   * This includes both success and failure termination states.
   */
  PassState_COMPLETE = 4,
  UNRECOGNIZED = -1,
}

export function stateFromJSON(object: any): State {
  switch (object) {
    case 0:
    case 'PassState_UNKNOWN':
      return State.PassState_UNKNOWN
    case 1:
    case 'PassState_PENDING':
      return State.PassState_PENDING
    case 2:
    case 'PassState_RUNNING':
      return State.PassState_RUNNING
    case 3:
    case 'PassState_CHECKING':
      return State.PassState_CHECKING
    case 4:
    case 'PassState_COMPLETE':
      return State.PassState_COMPLETE
    case -1:
    case 'UNRECOGNIZED':
    default:
      return State.UNRECOGNIZED
  }
}

export function stateToJSON(object: State): string {
  switch (object) {
    case State.PassState_UNKNOWN:
      return 'PassState_UNKNOWN'
    case State.PassState_PENDING:
      return 'PassState_PENDING'
    case State.PassState_RUNNING:
      return 'PassState_RUNNING'
    case State.PassState_CHECKING:
      return 'PassState_CHECKING'
    case State.PassState_COMPLETE:
      return 'PassState_COMPLETE'
    case State.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * Pass contains state for a Task pass.
 * Contains a pointer to the Target used for the pass.
 * Contains snapshots of the execution instance states.
 * Execution instances can be added / removed.
 *
 * The Pass is complete when len(exec_states) == replicas and all exec states
 * are in the completed (terminal) state, or when a fatal error occurs.
 *
 * If the Target object changes (in the world) or inputs change, a new Pass
 * should be created (these fields are immutable).
 *
 * World graph links:
 *  - parent: usually a Task which created the Pass
 *  - forge/pass-execution: all active execution instances for the pass
 * Incoming graph links:
 *  - parent: from the Execution for the pass.
 */
export interface Pass {
  /** PassState is the current state of the pass. */
  passState: State
  /**
   * PeerId is the Pass controller peer ID.
   * Usually the peer ID of the Cluster controller managing this Pass.
   * Can be empty.
   */
  peerId: string
  /** TargetRef is the block reference to the Target for the pass. */
  targetRef: BlockRef | undefined
  /**
   * ValueSet is the set of inputs and outputs used in the pass.
   * The input set is resolved before creating the Pass object.
   * The inputs are copied to the Pass objects.
   * The output set is updated when transitioning from CHECKING -> COMPLETE.
   */
  valueSet: ValueSet | undefined
  /** Result is information about the outcome of a completed Pass. */
  result: Result | undefined
  /**
   * Replicas is the configured number of executions for the Pass.
   *
   * len(replicas) must match len(exec_states) to complete
   */
  replicas: number
  /** PassNonce is the nonce of the pass set by the pass creator. */
  passNonce: Long
  /**
   * ExecStates contains the most recent snapshot of the execution states.
   * Updated when:
   *  - PENDING to RUNNING: contains initial execution states (PENDING)
   *  - RUNNING: can add and remove execution states as needed.
   *  - RUNNING to CHECKING or COMPLETE: contains final states (COMPLETE)
   *  - Any to PENDING: cleared (set to len=0).
   */
  execStates: ExecState[]
  /**
   * Timestamp is the time the Pass was created.
   * Used as a reference timestamp to make all ops deterministic.
   * For example: all unixfs timestamps will be set to this value.
   * Must be set.
   */
  timestamp: Timestamp | undefined
}

/** ExecState contains the previous snapshot of an execution state. */
export interface ExecState {
  /**
   * ObjectKey is the object key of the execution instance.
   * Must exist before adding to Pass state.
   * Graph quad also exists: <pass> <pass/execution> <object_key>
   */
  objectKey: string
  /** ExecutionState is the current state of the execution. */
  executionState: State1
  /**
   * PeerId is the identifier of the peer assigned to the execution.
   * Can be empty.
   */
  peerId: string
  /** Timestamp is the time the parent object (usually Pass) was created. */
  timestamp: Timestamp | undefined
  /**
   * ValueSet is the set of inputs and outputs used in the execution.
   * Outputs are updated when the execution reaches COMPLETE state.
   */
  valueSet: ValueSet | undefined
  /** Result is information about the outcome of the execution. */
  result: Result | undefined
}

function createBasePass(): Pass {
  return {
    passState: 0,
    peerId: '',
    targetRef: undefined,
    valueSet: undefined,
    result: undefined,
    replicas: 0,
    passNonce: Long.UZERO,
    execStates: [],
    timestamp: undefined,
  }
}

export const Pass = {
  encode(message: Pass, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.passState !== 0) {
      writer.uint32(8).int32(message.passState)
    }
    if (message.peerId !== '') {
      writer.uint32(18).string(message.peerId)
    }
    if (message.targetRef !== undefined) {
      BlockRef.encode(message.targetRef, writer.uint32(26).fork()).ldelim()
    }
    if (message.valueSet !== undefined) {
      ValueSet.encode(message.valueSet, writer.uint32(34).fork()).ldelim()
    }
    if (message.result !== undefined) {
      Result.encode(message.result, writer.uint32(42).fork()).ldelim()
    }
    if (message.replicas !== 0) {
      writer.uint32(48).uint32(message.replicas)
    }
    if (!message.passNonce.equals(Long.UZERO)) {
      writer.uint32(56).uint64(message.passNonce)
    }
    for (const v of message.execStates) {
      ExecState.encode(v!, writer.uint32(66).fork()).ldelim()
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(74).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Pass {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePass()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.passState = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.peerId = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.targetRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.valueSet = ValueSet.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.result = Result.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.replicas = reader.uint32()
          continue
        case 7:
          if (tag !== 56) {
            break
          }

          message.passNonce = reader.uint64() as Long
          continue
        case 8:
          if (tag !== 66) {
            break
          }

          message.execStates.push(ExecState.decode(reader, reader.uint32()))
          continue
        case 9:
          if (tag !== 74) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
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
  // Transform<Pass, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Pass | Pass[]> | Iterable<Pass | Pass[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Pass.encode(p).finish()]
        }
      } else {
        yield* [Pass.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Pass>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Pass> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Pass.decode(p)]
        }
      } else {
        yield* [Pass.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Pass {
    return {
      passState: isSet(object.passState) ? stateFromJSON(object.passState) : 0,
      peerId: isSet(object.peerId) ? globalThis.String(object.peerId) : '',
      targetRef: isSet(object.targetRef)
        ? BlockRef.fromJSON(object.targetRef)
        : undefined,
      valueSet: isSet(object.valueSet)
        ? ValueSet.fromJSON(object.valueSet)
        : undefined,
      result: isSet(object.result) ? Result.fromJSON(object.result) : undefined,
      replicas: isSet(object.replicas) ? globalThis.Number(object.replicas) : 0,
      passNonce: isSet(object.passNonce)
        ? Long.fromValue(object.passNonce)
        : Long.UZERO,
      execStates: globalThis.Array.isArray(object?.execStates)
        ? object.execStates.map((e: any) => ExecState.fromJSON(e))
        : [],
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: Pass): unknown {
    const obj: any = {}
    if (message.passState !== 0) {
      obj.passState = stateToJSON(message.passState)
    }
    if (message.peerId !== '') {
      obj.peerId = message.peerId
    }
    if (message.targetRef !== undefined) {
      obj.targetRef = BlockRef.toJSON(message.targetRef)
    }
    if (message.valueSet !== undefined) {
      obj.valueSet = ValueSet.toJSON(message.valueSet)
    }
    if (message.result !== undefined) {
      obj.result = Result.toJSON(message.result)
    }
    if (message.replicas !== 0) {
      obj.replicas = Math.round(message.replicas)
    }
    if (!message.passNonce.equals(Long.UZERO)) {
      obj.passNonce = (message.passNonce || Long.UZERO).toString()
    }
    if (message.execStates?.length) {
      obj.execStates = message.execStates.map((e) => ExecState.toJSON(e))
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Pass>, I>>(base?: I): Pass {
    return Pass.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Pass>, I>>(object: I): Pass {
    const message = createBasePass()
    message.passState = object.passState ?? 0
    message.peerId = object.peerId ?? ''
    message.targetRef =
      object.targetRef !== undefined && object.targetRef !== null
        ? BlockRef.fromPartial(object.targetRef)
        : undefined
    message.valueSet =
      object.valueSet !== undefined && object.valueSet !== null
        ? ValueSet.fromPartial(object.valueSet)
        : undefined
    message.result =
      object.result !== undefined && object.result !== null
        ? Result.fromPartial(object.result)
        : undefined
    message.replicas = object.replicas ?? 0
    message.passNonce =
      object.passNonce !== undefined && object.passNonce !== null
        ? Long.fromValue(object.passNonce)
        : Long.UZERO
    message.execStates =
      object.execStates?.map((e) => ExecState.fromPartial(e)) || []
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseExecState(): ExecState {
  return {
    objectKey: '',
    executionState: 0,
    peerId: '',
    timestamp: undefined,
    valueSet: undefined,
    result: undefined,
  }
}

export const ExecState = {
  encode(
    message: ExecState,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.objectKey !== '') {
      writer.uint32(10).string(message.objectKey)
    }
    if (message.executionState !== 0) {
      writer.uint32(16).int32(message.executionState)
    }
    if (message.peerId !== '') {
      writer.uint32(26).string(message.peerId)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim()
    }
    if (message.valueSet !== undefined) {
      ValueSet.encode(message.valueSet, writer.uint32(42).fork()).ldelim()
    }
    if (message.result !== undefined) {
      Result.encode(message.result, writer.uint32(50).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ExecState {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseExecState()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.objectKey = reader.string()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.executionState = reader.int32() as any
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.peerId = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.valueSet = ValueSet.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag !== 50) {
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
  // Transform<ExecState, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ExecState | ExecState[]>
      | Iterable<ExecState | ExecState[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ExecState.encode(p).finish()]
        }
      } else {
        yield* [ExecState.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ExecState>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ExecState> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ExecState.decode(p)]
        }
      } else {
        yield* [ExecState.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ExecState {
    return {
      objectKey: isSet(object.objectKey)
        ? globalThis.String(object.objectKey)
        : '',
      executionState: isSet(object.executionState)
        ? stateFromJSON2(object.executionState)
        : 0,
      peerId: isSet(object.peerId) ? globalThis.String(object.peerId) : '',
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
      valueSet: isSet(object.valueSet)
        ? ValueSet.fromJSON(object.valueSet)
        : undefined,
      result: isSet(object.result) ? Result.fromJSON(object.result) : undefined,
    }
  },

  toJSON(message: ExecState): unknown {
    const obj: any = {}
    if (message.objectKey !== '') {
      obj.objectKey = message.objectKey
    }
    if (message.executionState !== 0) {
      obj.executionState = stateToJSON3(message.executionState)
    }
    if (message.peerId !== '') {
      obj.peerId = message.peerId
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    if (message.valueSet !== undefined) {
      obj.valueSet = ValueSet.toJSON(message.valueSet)
    }
    if (message.result !== undefined) {
      obj.result = Result.toJSON(message.result)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ExecState>, I>>(base?: I): ExecState {
    return ExecState.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ExecState>, I>>(
    object: I,
  ): ExecState {
    const message = createBaseExecState()
    message.objectKey = object.objectKey ?? ''
    message.executionState = object.executionState ?? 0
    message.peerId = object.peerId ?? ''
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    message.valueSet =
      object.valueSet !== undefined && object.valueSet !== null
        ? ValueSet.fromPartial(object.valueSet)
        : undefined
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
