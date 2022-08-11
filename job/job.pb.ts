/* eslint-disable */
import Long from 'long'
import { Result } from '../value/value.pb.js'
import { Timestamp } from '../../timestamp/timestamp.pb.js'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'forge.job'

/** State contains the possible Job states. */
export enum State {
  /** JobState_UNKNOWN - JobState_UNKNOWN is the unknown type. */
  JobState_UNKNOWN = 0,
  /**
   * JobState_PENDING - JobState_PENDING indicates the job is queued for execution.
   * Transitions to RUNNING state when the scheduler starts the job.
   */
  JobState_PENDING = 1,
  /**
   * JobState_RUNNING - JobState_RUNNING is the state when the job is underway.
   *
   * If the job is returned to PENDING, all ongoing Pass are canceled, and
   * replaced with new Pass in PENDING state.
   */
  JobState_RUNNING = 2,
  /**
   * JobState_COMPLETE - JobState_COMPLETE is the normal terminal state of the job.
   * This includes both success and failure termination states.
   */
  JobState_COMPLETE = 3,
  UNRECOGNIZED = -1,
}

export function stateFromJSON(object: any): State {
  switch (object) {
    case 0:
    case 'JobState_UNKNOWN':
      return State.JobState_UNKNOWN
    case 1:
    case 'JobState_PENDING':
      return State.JobState_PENDING
    case 2:
    case 'JobState_RUNNING':
      return State.JobState_RUNNING
    case 3:
    case 'JobState_COMPLETE':
      return State.JobState_COMPLETE
    case -1:
    case 'UNRECOGNIZED':
    default:
      return State.UNRECOGNIZED
  }
}

export function stateToJSON(object: State): string {
  switch (object) {
    case State.JobState_UNKNOWN:
      return 'JobState_UNKNOWN'
    case State.JobState_PENDING:
      return 'JobState_PENDING'
    case State.JobState_RUNNING:
      return 'JobState_RUNNING'
    case State.JobState_COMPLETE:
      return 'JobState_COMPLETE'
    case State.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * Job contains state for running a set of Tasks.
 *
 * The Job is complete when all Task are in the completed (terminal) state, or
 * when a fatal error occurs.
 *
 * World graph links:
 *  - parent: usually the Cluster which created the Job.
 *  - forge/job-task: all active Task instances for the Job.
 * Incoming links:
 *  - parent: any Tasks created specifically for the Job
 */
export interface Job {
  /** JobState is the current state of the job. */
  jobState: State
  /** Result is information about the outcome of a completed Job. */
  result: Result | undefined
  /**
   * Timestamp is the time the Job was created.
   * Used as a reference timestamp to make all ops deterministic.
   * For example: all unixfs timestamps will be set to this value.
   * Must be set.
   */
  timestamp: Timestamp | undefined
}

function createBaseJob(): Job {
  return { jobState: 0, result: undefined, timestamp: undefined }
}

export const Job = {
  encode(message: Job, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.jobState !== 0) {
      writer.uint32(8).int32(message.jobState)
    }
    if (message.result !== undefined) {
      Result.encode(message.result, writer.uint32(18).fork()).ldelim()
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Job {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseJob()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.jobState = reader.int32() as any
          break
        case 2:
          message.result = Result.decode(reader, reader.uint32())
          break
        case 3:
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
  // Transform<Job, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Job | Job[]> | Iterable<Job | Job[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Job.encode(p).finish()]
        }
      } else {
        yield* [Job.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Job>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Job> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Job.decode(p)]
        }
      } else {
        yield* [Job.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Job {
    return {
      jobState: isSet(object.jobState) ? stateFromJSON(object.jobState) : 0,
      result: isSet(object.result) ? Result.fromJSON(object.result) : undefined,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: Job): unknown {
    const obj: any = {}
    message.jobState !== undefined &&
      (obj.jobState = stateToJSON(message.jobState))
    message.result !== undefined &&
      (obj.result = message.result ? Result.toJSON(message.result) : undefined)
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp
        ? Timestamp.toJSON(message.timestamp)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Job>, I>>(object: I): Job {
    const message = createBaseJob()
    message.jobState = object.jobState ?? 0
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
