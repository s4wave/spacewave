/* eslint-disable */
import Long from 'long'
import { BlockRef } from '@go/github.com/aperturerobotics/hydra/block/block.pb.js'
import { ValueSet } from '../target/target.pb.js'
import { Result } from '../value/value.pb.js'
import { Timestamp } from '@go/github.com/aperturerobotics/timestamp/timestamp.pb.js'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'forge.task'

/** State contains the possible Task states. */
export enum State {
  /** TaskState_UNKNOWN - TaskState_UNKNOWN is the unknown type. */
  TaskState_UNKNOWN = 0,
  /**
   * TaskState_PENDING - TaskState_PENDING is the state when waiting for target & inputs to be resolved.
   * If the <task/target> link or other inputs are not set, remains in PENDING state.
   * Transitions to RUNNING state when inputs are resolved and Pass is created.
   * Transitions to PENDING when the target or input values are updated.
   */
  TaskState_PENDING = 1,
  /**
   * TaskState_RUNNING - TaskState_RUNNING is the state when a Pass is assigned and currently executing.
   * If the linked Target does not match the linked pass, cancel the pass.
   * If the inputs do not match the linked Pass inputs, cancel the pass.
   * If the pass becomes canceled, return to PENDING state.
   */
  TaskState_RUNNING = 2,
  /**
   * TaskState_CHECKING - TaskState_CHECKING is the state when the Pass has completed.
   * The Task controller will then check the Pass results.
   * Transition to COMPLETE state on failure (invalid) or success (validation).
   */
  TaskState_CHECKING = 3,
  /**
   * TaskState_COMPLETE - TaskState_COMPLETE is the terminal state of the task.
   * This includes both success and failure termination states.
   */
  TaskState_COMPLETE = 4,
  /**
   * TaskState_RETRY - TaskState_RETRY wait for a change in Inputs before retrying with a new Pass.
   * When the assigned Pass is deleted or Inputs are different from latest resolved,
   * remove the <task/pass> graph link and transition to PENDING.
   */
  TaskState_RETRY = 5,
  UNRECOGNIZED = -1,
}

export function stateFromJSON(object: any): State {
  switch (object) {
    case 0:
    case 'TaskState_UNKNOWN':
      return State.TaskState_UNKNOWN
    case 1:
    case 'TaskState_PENDING':
      return State.TaskState_PENDING
    case 2:
    case 'TaskState_RUNNING':
      return State.TaskState_RUNNING
    case 3:
    case 'TaskState_CHECKING':
      return State.TaskState_CHECKING
    case 4:
    case 'TaskState_COMPLETE':
      return State.TaskState_COMPLETE
    case 5:
    case 'TaskState_RETRY':
      return State.TaskState_RETRY
    case -1:
    case 'UNRECOGNIZED':
    default:
      return State.UNRECOGNIZED
  }
}

export function stateToJSON(object: State): string {
  switch (object) {
    case State.TaskState_UNKNOWN:
      return 'TaskState_UNKNOWN'
    case State.TaskState_PENDING:
      return 'TaskState_PENDING'
    case State.TaskState_RUNNING:
      return 'TaskState_RUNNING'
    case State.TaskState_CHECKING:
      return 'TaskState_CHECKING'
    case State.TaskState_COMPLETE:
      return 'TaskState_COMPLETE'
    case State.TaskState_RETRY:
      return 'TaskState_RETRY'
    case State.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * Task contains state for running a Target.
 *
 * World graph links:
 *  - parent: usually the Job that created the Target.
 *  - forge/task-pass: all Pass for the Task
 *  - forge/task-target: current active Target of the Task, max 1
 * Incoming graph links:
 *  - parent: from the Pass.
 */
export interface Task {
  /** TaskState is the current state of the task. */
  taskState: State
  /**
   * Name is the human readable Task name.
   * Example: "my-task-1"
   * Must be a valid DNS label as defined in RFC 1123.
   */
  name: string
  /**
   * PeerId is the Task controller peer ID.
   * Usually the peer ID of the Cluster controller managing this Task.
   * Can be empty.
   */
  peerId: string
  /**
   * Replicas is the configured number of replicas for the created Pass.
   * Cannot be zero.
   * Transitions the Task to PENDING when different from latest Pass.
   */
  replicas: number
  /**
   * PassNonce is the most recent pass index.
   * Incremented when a new pass is added.
   * Can be initially zero when no Pass exists.
   * Transitions the Task to PENDING when different from latest Pass.
   */
  passNonce: Long
  /**
   * TargetRef is the block reference to the Target for the Task.
   * Resolved & copied when transitioning to/from PENDING state.
   * Can be initially empty.
   * Transitions the Task to PENDING when different from latest Pass.
   */
  targetRef: BlockRef | undefined
  /**
   * ValueSet is the set of inputs and outputs for the Task.
   * The input set is resolved when transitioning to/from PENDING state.
   * The output set is updated when transitioning from CHECKING -> COMPLETE.
   * Can be initially empty.
   * Transitions the Task to PENDING when different from latest Pass.
   */
  valueSet: ValueSet | undefined
  /** Result is information about the outcome of a completed Pass. */
  result: Result | undefined
  /**
   * Timestamp is the time the Task was created.
   * Used as a reference timestamp to make all ops deterministic.
   * For example: all unixfs timestamps will be set to this value.
   * Must be set.
   */
  timestamp: Timestamp | undefined
}

function createBaseTask(): Task {
  return {
    taskState: 0,
    name: '',
    peerId: '',
    replicas: 0,
    passNonce: Long.UZERO,
    targetRef: undefined,
    valueSet: undefined,
    result: undefined,
    timestamp: undefined,
  }
}

export const Task = {
  encode(message: Task, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.taskState !== 0) {
      writer.uint32(8).int32(message.taskState)
    }
    if (message.name !== '') {
      writer.uint32(18).string(message.name)
    }
    if (message.peerId !== '') {
      writer.uint32(26).string(message.peerId)
    }
    if (message.replicas !== 0) {
      writer.uint32(40).uint32(message.replicas)
    }
    if (!message.passNonce.isZero()) {
      writer.uint32(48).uint64(message.passNonce)
    }
    if (message.targetRef !== undefined) {
      BlockRef.encode(message.targetRef, writer.uint32(58).fork()).ldelim()
    }
    if (message.valueSet !== undefined) {
      ValueSet.encode(message.valueSet, writer.uint32(66).fork()).ldelim()
    }
    if (message.result !== undefined) {
      Result.encode(message.result, writer.uint32(74).fork()).ldelim()
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(82).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Task {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTask()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.taskState = reader.int32() as any
          break
        case 2:
          message.name = reader.string()
          break
        case 3:
          message.peerId = reader.string()
          break
        case 5:
          message.replicas = reader.uint32()
          break
        case 6:
          message.passNonce = reader.uint64() as Long
          break
        case 7:
          message.targetRef = BlockRef.decode(reader, reader.uint32())
          break
        case 8:
          message.valueSet = ValueSet.decode(reader, reader.uint32())
          break
        case 9:
          message.result = Result.decode(reader, reader.uint32())
          break
        case 10:
          message.timestamp = Timestamp.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Task, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Task | Task[]> | Iterable<Task | Task[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Task.encode(p).finish()]
        }
      } else {
        yield* [Task.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Task>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Task> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Task.decode(p)]
        }
      } else {
        yield* [Task.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Task {
    return {
      taskState: isSet(object.taskState) ? stateFromJSON(object.taskState) : 0,
      name: isSet(object.name) ? String(object.name) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
      replicas: isSet(object.replicas) ? Number(object.replicas) : 0,
      passNonce: isSet(object.passNonce)
        ? Long.fromValue(object.passNonce)
        : Long.UZERO,
      targetRef: isSet(object.targetRef)
        ? BlockRef.fromJSON(object.targetRef)
        : undefined,
      valueSet: isSet(object.valueSet)
        ? ValueSet.fromJSON(object.valueSet)
        : undefined,
      result: isSet(object.result) ? Result.fromJSON(object.result) : undefined,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: Task): unknown {
    const obj: any = {}
    message.taskState !== undefined &&
      (obj.taskState = stateToJSON(message.taskState))
    message.name !== undefined && (obj.name = message.name)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    message.replicas !== undefined &&
      (obj.replicas = Math.round(message.replicas))
    message.passNonce !== undefined &&
      (obj.passNonce = (message.passNonce || Long.UZERO).toString())
    message.targetRef !== undefined &&
      (obj.targetRef = message.targetRef
        ? BlockRef.toJSON(message.targetRef)
        : undefined)
    message.valueSet !== undefined &&
      (obj.valueSet = message.valueSet
        ? ValueSet.toJSON(message.valueSet)
        : undefined)
    message.result !== undefined &&
      (obj.result = message.result ? Result.toJSON(message.result) : undefined)
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp
        ? Timestamp.toJSON(message.timestamp)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Task>, I>>(object: I): Task {
    const message = createBaseTask()
    message.taskState = object.taskState ?? 0
    message.name = object.name ?? ''
    message.peerId = object.peerId ?? ''
    message.replicas = object.replicas ?? 0
    message.passNonce =
      object.passNonce !== undefined && object.passNonce !== null
        ? Long.fromValue(object.passNonce)
        : Long.UZERO
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
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
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
