/* eslint-disable */
import { Backoff } from '@go/github.com/aperturerobotics/util/backoff/backoff.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../../block/block.pb.js'

export const protobufPackage = 'dex.http'

/** SyncMessageType is the set of sync message types */
export enum SyncMessageType {
  SyncMessageType_UNKNOWN = 0,
  /** SyncMessageType_START_XMIT - SyncMessageType_START_XMIT indicates start of block transmission. */
  SyncMessageType_START_XMIT = 1,
  /** SyncMessageType_CTNU_XMIT - SyncMessageType_CTNU_XMIT indicates continue block transmission. */
  SyncMessageType_CTNU_XMIT = 2,
  /** SyncMessageType_REFUSE_RX - SyncMessageType_REFUSE_RX indicates reception refusal. */
  SyncMessageType_REFUSE_RX = 3,
  UNRECOGNIZED = -1,
}

export function syncMessageTypeFromJSON(object: any): SyncMessageType {
  switch (object) {
    case 0:
    case 'SyncMessageType_UNKNOWN':
      return SyncMessageType.SyncMessageType_UNKNOWN
    case 1:
    case 'SyncMessageType_START_XMIT':
      return SyncMessageType.SyncMessageType_START_XMIT
    case 2:
    case 'SyncMessageType_CTNU_XMIT':
      return SyncMessageType.SyncMessageType_CTNU_XMIT
    case 3:
    case 'SyncMessageType_REFUSE_RX':
      return SyncMessageType.SyncMessageType_REFUSE_RX
    case -1:
    case 'UNRECOGNIZED':
    default:
      return SyncMessageType.UNRECOGNIZED
  }
}

export function syncMessageTypeToJSON(object: SyncMessageType): string {
  switch (object) {
    case SyncMessageType.SyncMessageType_UNKNOWN:
      return 'SyncMessageType_UNKNOWN'
    case SyncMessageType.SyncMessageType_START_XMIT:
      return 'SyncMessageType_START_XMIT'
    case SyncMessageType.SyncMessageType_CTNU_XMIT:
      return 'SyncMessageType_CTNU_XMIT'
    case SyncMessageType.SyncMessageType_REFUSE_RX:
      return 'SyncMessageType_REFUSE_RX'
    case SyncMessageType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Config configures the http data-exchange controller. */
export interface Config {
  /** BucketId is the bucket ID to serve / lookup blocks for. */
  bucketId: string
  /**
   * ListenPeerId is the peer id to use for listening for incoming streams.
   * Can be empty to disable serving content.
   */
  peerId: string
  /**
   * TransportId sets a transport ID constraint.
   * Can be empty.
   */
  transportId: Long
  /** SyncBackoff controls sync session backoff. */
  syncBackoff: Backoff | undefined
}

/** PubSubMessage is the root pub-sub message. */
export interface PubSubMessage {
  /**
   * WantRefs is the list of wanted blocks.
   * Blocks here have been added to the want list.
   */
  wantRefs: BlockRef[]
  /**
   * HaveRefs is the list of recently received blocks.
   * These should be removed from the want list.
   * This advertises the blocks to remote peers.
   */
  haveRefs: BlockRef[]
  /**
   * ClearRefs is the list of no longer wanted blocks.
   * These should be removed from the want list.
   */
  clearRefs: BlockRef[]
  /**
   * WantEmpty indicates the wantlist is now empty.
   * The clear_refs list will be empty if this is set.
   */
  wantEmpty: boolean
}

/** SyncMessage is the root sync session message. */
export interface SyncMessage {
  /** MessageType is the message type. */
  messageType: SyncMessageType
  /**
   * Ref is the block reference if relevant.
   * Used for START_XMIT, REFUSE_RX
   */
  ref: BlockRef | undefined
  /**
   * Chunk is the data chunk.
   * Stream is always ordered - therefore, we don't need to send index.
   * Used for START_XMIT, CTNU_XMIT
   */
  chunk: Uint8Array
  /**
   * Complete indicates this is the last block to transmit in the sequence.
   * Used for START_XMIT, CTNU_XMIT
   */
  complete: boolean
  /**
   * BlockSize is the size of the block.
   * Used for START_XMIT
   */
  blockSize: number
}

function createBaseConfig(): Config {
  return {
    bucketId: '',
    peerId: '',
    transportId: Long.UZERO,
    syncBackoff: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.bucketId !== '') {
      writer.uint32(10).string(message.bucketId)
    }
    if (message.peerId !== '') {
      writer.uint32(18).string(message.peerId)
    }
    if (!message.transportId.isZero()) {
      writer.uint32(32).uint64(message.transportId)
    }
    if (message.syncBackoff !== undefined) {
      Backoff.encode(message.syncBackoff, writer.uint32(42).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break
          }

          message.bucketId = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.peerId = reader.string()
          continue
        case 4:
          if (tag != 32) {
            break
          }

          message.transportId = reader.uint64() as Long
          continue
        case 5:
          if (tag != 42) {
            break
          }

          message.syncBackoff = Backoff.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) == 4 || tag == 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
      transportId: isSet(object.transportId)
        ? Long.fromValue(object.transportId)
        : Long.UZERO,
      syncBackoff: isSet(object.syncBackoff)
        ? Backoff.fromJSON(object.syncBackoff)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.bucketId !== undefined && (obj.bucketId = message.bucketId)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    message.transportId !== undefined &&
      (obj.transportId = (message.transportId || Long.UZERO).toString())
    message.syncBackoff !== undefined &&
      (obj.syncBackoff = message.syncBackoff
        ? Backoff.toJSON(message.syncBackoff)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.bucketId = object.bucketId ?? ''
    message.peerId = object.peerId ?? ''
    message.transportId =
      object.transportId !== undefined && object.transportId !== null
        ? Long.fromValue(object.transportId)
        : Long.UZERO
    message.syncBackoff =
      object.syncBackoff !== undefined && object.syncBackoff !== null
        ? Backoff.fromPartial(object.syncBackoff)
        : undefined
    return message
  },
}

function createBasePubSubMessage(): PubSubMessage {
  return { wantRefs: [], haveRefs: [], clearRefs: [], wantEmpty: false }
}

export const PubSubMessage = {
  encode(
    message: PubSubMessage,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.wantRefs) {
      BlockRef.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    for (const v of message.haveRefs) {
      BlockRef.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    for (const v of message.clearRefs) {
      BlockRef.encode(v!, writer.uint32(26).fork()).ldelim()
    }
    if (message.wantEmpty === true) {
      writer.uint32(32).bool(message.wantEmpty)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PubSubMessage {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBasePubSubMessage()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break
          }

          message.wantRefs.push(BlockRef.decode(reader, reader.uint32()))
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.haveRefs.push(BlockRef.decode(reader, reader.uint32()))
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.clearRefs.push(BlockRef.decode(reader, reader.uint32()))
          continue
        case 4:
          if (tag != 32) {
            break
          }

          message.wantEmpty = reader.bool()
          continue
      }
      if ((tag & 7) == 4 || tag == 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PubSubMessage, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PubSubMessage | PubSubMessage[]>
      | Iterable<PubSubMessage | PubSubMessage[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PubSubMessage.encode(p).finish()]
        }
      } else {
        yield* [PubSubMessage.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PubSubMessage>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<PubSubMessage> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PubSubMessage.decode(p)]
        }
      } else {
        yield* [PubSubMessage.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): PubSubMessage {
    return {
      wantRefs: Array.isArray(object?.wantRefs)
        ? object.wantRefs.map((e: any) => BlockRef.fromJSON(e))
        : [],
      haveRefs: Array.isArray(object?.haveRefs)
        ? object.haveRefs.map((e: any) => BlockRef.fromJSON(e))
        : [],
      clearRefs: Array.isArray(object?.clearRefs)
        ? object.clearRefs.map((e: any) => BlockRef.fromJSON(e))
        : [],
      wantEmpty: isSet(object.wantEmpty) ? Boolean(object.wantEmpty) : false,
    }
  },

  toJSON(message: PubSubMessage): unknown {
    const obj: any = {}
    if (message.wantRefs) {
      obj.wantRefs = message.wantRefs.map((e) =>
        e ? BlockRef.toJSON(e) : undefined
      )
    } else {
      obj.wantRefs = []
    }
    if (message.haveRefs) {
      obj.haveRefs = message.haveRefs.map((e) =>
        e ? BlockRef.toJSON(e) : undefined
      )
    } else {
      obj.haveRefs = []
    }
    if (message.clearRefs) {
      obj.clearRefs = message.clearRefs.map((e) =>
        e ? BlockRef.toJSON(e) : undefined
      )
    } else {
      obj.clearRefs = []
    }
    message.wantEmpty !== undefined && (obj.wantEmpty = message.wantEmpty)
    return obj
  },

  create<I extends Exact<DeepPartial<PubSubMessage>, I>>(
    base?: I
  ): PubSubMessage {
    return PubSubMessage.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<PubSubMessage>, I>>(
    object: I
  ): PubSubMessage {
    const message = createBasePubSubMessage()
    message.wantRefs =
      object.wantRefs?.map((e) => BlockRef.fromPartial(e)) || []
    message.haveRefs =
      object.haveRefs?.map((e) => BlockRef.fromPartial(e)) || []
    message.clearRefs =
      object.clearRefs?.map((e) => BlockRef.fromPartial(e)) || []
    message.wantEmpty = object.wantEmpty ?? false
    return message
  },
}

function createBaseSyncMessage(): SyncMessage {
  return {
    messageType: 0,
    ref: undefined,
    chunk: new Uint8Array(),
    complete: false,
    blockSize: 0,
  }
}

export const SyncMessage = {
  encode(
    message: SyncMessage,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.messageType !== 0) {
      writer.uint32(8).int32(message.messageType)
    }
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(18).fork()).ldelim()
    }
    if (message.chunk.length !== 0) {
      writer.uint32(26).bytes(message.chunk)
    }
    if (message.complete === true) {
      writer.uint32(32).bool(message.complete)
    }
    if (message.blockSize !== 0) {
      writer.uint32(40).uint32(message.blockSize)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): SyncMessage {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseSyncMessage()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 8) {
            break
          }

          message.messageType = reader.int32() as any
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.ref = BlockRef.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.chunk = reader.bytes()
          continue
        case 4:
          if (tag != 32) {
            break
          }

          message.complete = reader.bool()
          continue
        case 5:
          if (tag != 40) {
            break
          }

          message.blockSize = reader.uint32()
          continue
      }
      if ((tag & 7) == 4 || tag == 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<SyncMessage, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<SyncMessage | SyncMessage[]>
      | Iterable<SyncMessage | SyncMessage[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SyncMessage.encode(p).finish()]
        }
      } else {
        yield* [SyncMessage.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SyncMessage>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<SyncMessage> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SyncMessage.decode(p)]
        }
      } else {
        yield* [SyncMessage.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): SyncMessage {
    return {
      messageType: isSet(object.messageType)
        ? syncMessageTypeFromJSON(object.messageType)
        : 0,
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
      chunk: isSet(object.chunk)
        ? bytesFromBase64(object.chunk)
        : new Uint8Array(),
      complete: isSet(object.complete) ? Boolean(object.complete) : false,
      blockSize: isSet(object.blockSize) ? Number(object.blockSize) : 0,
    }
  },

  toJSON(message: SyncMessage): unknown {
    const obj: any = {}
    message.messageType !== undefined &&
      (obj.messageType = syncMessageTypeToJSON(message.messageType))
    message.ref !== undefined &&
      (obj.ref = message.ref ? BlockRef.toJSON(message.ref) : undefined)
    message.chunk !== undefined &&
      (obj.chunk = base64FromBytes(
        message.chunk !== undefined ? message.chunk : new Uint8Array()
      ))
    message.complete !== undefined && (obj.complete = message.complete)
    message.blockSize !== undefined &&
      (obj.blockSize = Math.round(message.blockSize))
    return obj
  },

  create<I extends Exact<DeepPartial<SyncMessage>, I>>(base?: I): SyncMessage {
    return SyncMessage.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<SyncMessage>, I>>(
    object: I
  ): SyncMessage {
    const message = createBaseSyncMessage()
    message.messageType = object.messageType ?? 0
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
        : undefined
    message.chunk = object.chunk ?? new Uint8Array()
    message.complete = object.complete ?? false
    message.blockSize = object.blockSize ?? 0
    return message
  },
}

declare var self: any | undefined
declare var window: any | undefined
declare var global: any | undefined
var tsProtoGlobalThis: any = (() => {
  if (typeof globalThis !== 'undefined') {
    return globalThis
  }
  if (typeof self !== 'undefined') {
    return self
  }
  if (typeof window !== 'undefined') {
    return window
  }
  if (typeof global !== 'undefined') {
    return global
  }
  throw 'Unable to locate global object'
})()

function bytesFromBase64(b64: string): Uint8Array {
  if (tsProtoGlobalThis.Buffer) {
    return Uint8Array.from(tsProtoGlobalThis.Buffer.from(b64, 'base64'))
  } else {
    const bin = tsProtoGlobalThis.atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i)
    }
    return arr
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (tsProtoGlobalThis.Buffer) {
    return tsProtoGlobalThis.Buffer.from(arr).toString('base64')
  } else {
    const bin: string[] = []
    arr.forEach((byte) => {
      bin.push(String.fromCharCode(byte))
    })
    return tsProtoGlobalThis.btoa(bin.join(''))
  }
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
