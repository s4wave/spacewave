/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../../block/block.pb.js'

export const protobufPackage = 'bucket.event'

/** EventType is the type of bucket reconciler event. */
export enum EventType {
  /** EventType_UNKNOWN - EventType_UNKNOWN is the unknown type. */
  EventType_UNKNOWN = 0,
  /** EventType_PUT_BLOCK - EventType_PUT_BLOCK is the put block event. */
  EventType_PUT_BLOCK = 1,
  /** EventType_RM_BLOCK - EventType_RM_BLOCK is the delete block event. */
  EventType_RM_BLOCK = 3,
  UNRECOGNIZED = -1,
}

export function eventTypeFromJSON(object: any): EventType {
  switch (object) {
    case 0:
    case 'EventType_UNKNOWN':
      return EventType.EventType_UNKNOWN
    case 1:
    case 'EventType_PUT_BLOCK':
      return EventType.EventType_PUT_BLOCK
    case 3:
    case 'EventType_RM_BLOCK':
      return EventType.EventType_RM_BLOCK
    case -1:
    case 'UNRECOGNIZED':
    default:
      return EventType.UNRECOGNIZED
  }
}

export function eventTypeToJSON(object: EventType): string {
  switch (object) {
    case EventType.EventType_UNKNOWN:
      return 'EventType_UNKNOWN'
    case EventType.EventType_PUT_BLOCK:
      return 'EventType_PUT_BLOCK'
    case EventType.EventType_RM_BLOCK:
      return 'EventType_RM_BLOCK'
    case EventType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Event is the container for the event data. */
export interface Event {
  /** EventType is the event type. */
  eventType: EventType
  /** PutBlock is the put event data. */
  putBlock: PutBlock | undefined
  /** RmBlock is the rm event data. */
  rmBlock: RmBlock | undefined
}

/** BlockCommon are common block properties. */
export interface BlockCommon {
  /**
   * BucketId is the bucket id.
   * May be unset.
   * Used for: PutBlock, RmBlock
   */
  bucketId: string
  /**
   * VolumeId is the volume id.
   * May be unset.
   * Used for: PutBlock, RmBlock
   */
  volumeId: string
  /**
   * BucketConfRev is the bucket config revision.
   * May be unset.
   * Used for: PutBlock, RmBlock
   */
  bucketConfRev: number
  /**
   * BlockRef is the block reference.
   * Used for: PutBlock, RmBlock
   */
  blockRef: BlockRef | undefined
}

/** PutBlock is the put block event. */
export interface PutBlock {
  /** BlockCommon contains the common block event params. */
  blockCommon: BlockCommon | undefined
}

/** RmBlock is the remoe block event. */
export interface RmBlock {
  /** BlockCommon contains the common block event params. */
  blockCommon: BlockCommon | undefined
}

function createBaseEvent(): Event {
  return { eventType: 0, putBlock: undefined, rmBlock: undefined }
}

export const Event = {
  encode(message: Event, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.eventType !== 0) {
      writer.uint32(8).int32(message.eventType)
    }
    if (message.putBlock !== undefined) {
      PutBlock.encode(message.putBlock, writer.uint32(18).fork()).ldelim()
    }
    if (message.rmBlock !== undefined) {
      RmBlock.encode(message.rmBlock, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Event {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEvent()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.eventType = reader.int32() as any
          break
        case 2:
          message.putBlock = PutBlock.decode(reader, reader.uint32())
          break
        case 4:
          message.rmBlock = RmBlock.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Event, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Event | Event[]> | Iterable<Event | Event[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Event.encode(p).finish()]
        }
      } else {
        yield* [Event.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Event>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Event> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Event.decode(p)]
        }
      } else {
        yield* [Event.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Event {
    return {
      eventType: isSet(object.eventType)
        ? eventTypeFromJSON(object.eventType)
        : 0,
      putBlock: isSet(object.putBlock)
        ? PutBlock.fromJSON(object.putBlock)
        : undefined,
      rmBlock: isSet(object.rmBlock)
        ? RmBlock.fromJSON(object.rmBlock)
        : undefined,
    }
  },

  toJSON(message: Event): unknown {
    const obj: any = {}
    message.eventType !== undefined &&
      (obj.eventType = eventTypeToJSON(message.eventType))
    message.putBlock !== undefined &&
      (obj.putBlock = message.putBlock
        ? PutBlock.toJSON(message.putBlock)
        : undefined)
    message.rmBlock !== undefined &&
      (obj.rmBlock = message.rmBlock
        ? RmBlock.toJSON(message.rmBlock)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Event>, I>>(object: I): Event {
    const message = createBaseEvent()
    message.eventType = object.eventType ?? 0
    message.putBlock =
      object.putBlock !== undefined && object.putBlock !== null
        ? PutBlock.fromPartial(object.putBlock)
        : undefined
    message.rmBlock =
      object.rmBlock !== undefined && object.rmBlock !== null
        ? RmBlock.fromPartial(object.rmBlock)
        : undefined
    return message
  },
}

function createBaseBlockCommon(): BlockCommon {
  return { bucketId: '', volumeId: '', bucketConfRev: 0, blockRef: undefined }
}

export const BlockCommon = {
  encode(
    message: BlockCommon,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.bucketId !== '') {
      writer.uint32(10).string(message.bucketId)
    }
    if (message.volumeId !== '') {
      writer.uint32(18).string(message.volumeId)
    }
    if (message.bucketConfRev !== 0) {
      writer.uint32(24).uint32(message.bucketConfRev)
    }
    if (message.blockRef !== undefined) {
      BlockRef.encode(message.blockRef, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BlockCommon {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBlockCommon()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.bucketId = reader.string()
          break
        case 2:
          message.volumeId = reader.string()
          break
        case 3:
          message.bucketConfRev = reader.uint32()
          break
        case 4:
          message.blockRef = BlockRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<BlockCommon, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<BlockCommon | BlockCommon[]>
      | Iterable<BlockCommon | BlockCommon[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BlockCommon.encode(p).finish()]
        }
      } else {
        yield* [BlockCommon.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BlockCommon>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<BlockCommon> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BlockCommon.decode(p)]
        }
      } else {
        yield* [BlockCommon.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): BlockCommon {
    return {
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : '',
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : '',
      bucketConfRev: isSet(object.bucketConfRev)
        ? Number(object.bucketConfRev)
        : 0,
      blockRef: isSet(object.blockRef)
        ? BlockRef.fromJSON(object.blockRef)
        : undefined,
    }
  },

  toJSON(message: BlockCommon): unknown {
    const obj: any = {}
    message.bucketId !== undefined && (obj.bucketId = message.bucketId)
    message.volumeId !== undefined && (obj.volumeId = message.volumeId)
    message.bucketConfRev !== undefined &&
      (obj.bucketConfRev = Math.round(message.bucketConfRev))
    message.blockRef !== undefined &&
      (obj.blockRef = message.blockRef
        ? BlockRef.toJSON(message.blockRef)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<BlockCommon>, I>>(
    object: I
  ): BlockCommon {
    const message = createBaseBlockCommon()
    message.bucketId = object.bucketId ?? ''
    message.volumeId = object.volumeId ?? ''
    message.bucketConfRev = object.bucketConfRev ?? 0
    message.blockRef =
      object.blockRef !== undefined && object.blockRef !== null
        ? BlockRef.fromPartial(object.blockRef)
        : undefined
    return message
  },
}

function createBasePutBlock(): PutBlock {
  return { blockCommon: undefined }
}

export const PutBlock = {
  encode(
    message: PutBlock,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.blockCommon !== undefined) {
      BlockCommon.encode(message.blockCommon, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PutBlock {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePutBlock()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.blockCommon = BlockCommon.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PutBlock, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PutBlock | PutBlock[]>
      | Iterable<PutBlock | PutBlock[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PutBlock.encode(p).finish()]
        }
      } else {
        yield* [PutBlock.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PutBlock>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<PutBlock> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PutBlock.decode(p)]
        }
      } else {
        yield* [PutBlock.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): PutBlock {
    return {
      blockCommon: isSet(object.blockCommon)
        ? BlockCommon.fromJSON(object.blockCommon)
        : undefined,
    }
  },

  toJSON(message: PutBlock): unknown {
    const obj: any = {}
    message.blockCommon !== undefined &&
      (obj.blockCommon = message.blockCommon
        ? BlockCommon.toJSON(message.blockCommon)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<PutBlock>, I>>(object: I): PutBlock {
    const message = createBasePutBlock()
    message.blockCommon =
      object.blockCommon !== undefined && object.blockCommon !== null
        ? BlockCommon.fromPartial(object.blockCommon)
        : undefined
    return message
  },
}

function createBaseRmBlock(): RmBlock {
  return { blockCommon: undefined }
}

export const RmBlock = {
  encode(
    message: RmBlock,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.blockCommon !== undefined) {
      BlockCommon.encode(message.blockCommon, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RmBlock {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRmBlock()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.blockCommon = BlockCommon.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RmBlock, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<RmBlock | RmBlock[]> | Iterable<RmBlock | RmBlock[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RmBlock.encode(p).finish()]
        }
      } else {
        yield* [RmBlock.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RmBlock>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<RmBlock> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RmBlock.decode(p)]
        }
      } else {
        yield* [RmBlock.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RmBlock {
    return {
      blockCommon: isSet(object.blockCommon)
        ? BlockCommon.fromJSON(object.blockCommon)
        : undefined,
    }
  },

  toJSON(message: RmBlock): unknown {
    const obj: any = {}
    message.blockCommon !== undefined &&
      (obj.blockCommon = message.blockCommon
        ? BlockCommon.toJSON(message.blockCommon)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<RmBlock>, I>>(object: I): RmBlock {
    const message = createBaseRmBlock()
    message.blockCommon =
      object.blockCommon !== undefined && object.blockCommon !== null
        ? BlockCommon.fromPartial(object.blockCommon)
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
