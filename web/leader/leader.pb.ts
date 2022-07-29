/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bldr.web.leader'

/** ElectionEventType is the event type of the ElectionEvent. */
export enum ElectionEventType {
  /** ElectionEventType_UNKNOWN - ElectionEventType_UNKNOWN is the unknown type. */
  ElectionEventType_UNKNOWN = 0,
  /** ElectionEventType_LEADER_STEP_UP - ElectionEventType_LEADER_STEP_UP is sent when a leader is elected. */
  ElectionEventType_LEADER_STEP_UP = 1,
  /** ElectionEventType_LEADER_STEP_DOWN - ElectionEventType_LEADER_STEP_DOWN is sent when a leader steps down. */
  ElectionEventType_LEADER_STEP_DOWN = 2,
  /** ElectionEventType_ANNOUNCE - ElectionEventType_ANNOUNCE announces the presence of a new worker. */
  ElectionEventType_ANNOUNCE = 3,
  /** ElectionEventType_SHUTDOWN - ElectionEventType_SHUTDOWN announces the departure of a worker. */
  ElectionEventType_SHUTDOWN = 4,
  UNRECOGNIZED = -1,
}

export function electionEventTypeFromJSON(object: any): ElectionEventType {
  switch (object) {
    case 0:
    case 'ElectionEventType_UNKNOWN':
      return ElectionEventType.ElectionEventType_UNKNOWN
    case 1:
    case 'ElectionEventType_LEADER_STEP_UP':
      return ElectionEventType.ElectionEventType_LEADER_STEP_UP
    case 2:
    case 'ElectionEventType_LEADER_STEP_DOWN':
      return ElectionEventType.ElectionEventType_LEADER_STEP_DOWN
    case 3:
    case 'ElectionEventType_ANNOUNCE':
      return ElectionEventType.ElectionEventType_ANNOUNCE
    case 4:
    case 'ElectionEventType_SHUTDOWN':
      return ElectionEventType.ElectionEventType_SHUTDOWN
    case -1:
    case 'UNRECOGNIZED':
    default:
      return ElectionEventType.UNRECOGNIZED
  }
}

export function electionEventTypeToJSON(object: ElectionEventType): string {
  switch (object) {
    case ElectionEventType.ElectionEventType_UNKNOWN:
      return 'ElectionEventType_UNKNOWN'
    case ElectionEventType.ElectionEventType_LEADER_STEP_UP:
      return 'ElectionEventType_LEADER_STEP_UP'
    case ElectionEventType.ElectionEventType_LEADER_STEP_DOWN:
      return 'ElectionEventType_LEADER_STEP_DOWN'
    case ElectionEventType.ElectionEventType_ANNOUNCE:
      return 'ElectionEventType_ANNOUNCE'
    case ElectionEventType.ElectionEventType_SHUTDOWN:
      return 'ElectionEventType_SHUTDOWN'
    case ElectionEventType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** ElectionEvent is the message type of the Election BroadcastChannel. */
export interface ElectionEvent {
  /** EventType contains the election event type. */
  eventType: ElectionEventType
  /** WorkerId is the worker that sent the message. */
  workerId: string
}

function createBaseElectionEvent(): ElectionEvent {
  return { eventType: 0, workerId: '' }
}

export const ElectionEvent = {
  encode(
    message: ElectionEvent,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.eventType !== 0) {
      writer.uint32(8).int32(message.eventType)
    }
    if (message.workerId !== '') {
      writer.uint32(18).string(message.workerId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ElectionEvent {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseElectionEvent()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.eventType = reader.int32() as any
          break
        case 2:
          message.workerId = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ElectionEvent, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ElectionEvent | ElectionEvent[]>
      | Iterable<ElectionEvent | ElectionEvent[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ElectionEvent.encode(p).finish()]
        }
      } else {
        yield* [ElectionEvent.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ElectionEvent>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ElectionEvent> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ElectionEvent.decode(p)]
        }
      } else {
        yield* [ElectionEvent.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ElectionEvent {
    return {
      eventType: isSet(object.eventType)
        ? electionEventTypeFromJSON(object.eventType)
        : 0,
      workerId: isSet(object.workerId) ? String(object.workerId) : '',
    }
  },

  toJSON(message: ElectionEvent): unknown {
    const obj: any = {}
    message.eventType !== undefined &&
      (obj.eventType = electionEventTypeToJSON(message.eventType))
    message.workerId !== undefined && (obj.workerId = message.workerId)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ElectionEvent>, I>>(
    object: I
  ): ElectionEvent {
    const message = createBaseElectionEvent()
    message.eventType = object.eventType ?? 0
    message.workerId = object.workerId ?? ''
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
