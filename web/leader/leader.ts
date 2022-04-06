/* eslint-disable */
import * as Long from 'long'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'bldr.web.leader'

/** ElectionEventType is the event type of the ElectionEvent. */
export enum ElectionEventType {
  /** ElectionEventType_UNKNOWN - ElectionEventType_UNKNOWN is the unknown type. */
  ElectionEventType_UNKNOWN = 0,
  /** ElectionEventType_LEADER_ELECTED - ElectionEventType_LEADER_ELECTED is sent when a leader is elected. */
  ElectionEventType_LEADER_ELECTED = 1,
  /** ElectionEventType_LEADER_STEP_DOWN - ElectionEventType_LEADER_STEP_DOWN is sent when a leader steps down. */
  ElectionEventType_LEADER_STEP_DOWN = 2,
  UNRECOGNIZED = -1,
}

export function electionEventTypeFromJSON(object: any): ElectionEventType {
  switch (object) {
    case 0:
    case 'ElectionEventType_UNKNOWN':
      return ElectionEventType.ElectionEventType_UNKNOWN
    case 1:
    case 'ElectionEventType_LEADER_ELECTED':
      return ElectionEventType.ElectionEventType_LEADER_ELECTED
    case 2:
    case 'ElectionEventType_LEADER_STEP_DOWN':
      return ElectionEventType.ElectionEventType_LEADER_STEP_DOWN
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
    case ElectionEventType.ElectionEventType_LEADER_ELECTED:
      return 'ElectionEventType_LEADER_ELECTED'
    case ElectionEventType.ElectionEventType_LEADER_STEP_DOWN:
      return 'ElectionEventType_LEADER_STEP_DOWN'
    default:
      return 'UNKNOWN'
  }
}

/** ElectionEvent is the message type of the Election BroadcastChannel. */
export interface ElectionEvent {
  /** EventType contains the election event type. */
  eventType: ElectionEventType
  /**
   * LeaderId contains the leader identifier parameter.
   * ElectionEventType_LEADER_ELECTED
   * ElectionEventType_LEADER_STEP_DOWN
   */
  leaderId: string
}

function createBaseElectionEvent(): ElectionEvent {
  return { eventType: 0, leaderId: '' }
}

export const ElectionEvent = {
  encode(
    message: ElectionEvent,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.eventType !== 0) {
      writer.uint32(8).int32(message.eventType)
    }
    if (message.leaderId !== '') {
      writer.uint32(18).string(message.leaderId)
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
          message.leaderId = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): ElectionEvent {
    return {
      eventType: isSet(object.eventType)
        ? electionEventTypeFromJSON(object.eventType)
        : 0,
      leaderId: isSet(object.leaderId) ? String(object.leaderId) : '',
    }
  },

  toJSON(message: ElectionEvent): unknown {
    const obj: any = {}
    message.eventType !== undefined &&
      (obj.eventType = electionEventTypeToJSON(message.eventType))
    message.leaderId !== undefined && (obj.leaderId = message.leaderId)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ElectionEvent>, I>>(
    object: I
  ): ElectionEvent {
    const message = createBaseElectionEvent()
    message.eventType = object.eventType ?? 0
    message.leaderId = object.leaderId ?? ''
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
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
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

// If you get a compile-error about 'Constructor<Long> and ... have no overlap',
// add '--ts_proto_opt=esModuleInterop=true' as a flag when calling 'protoc'.
if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
