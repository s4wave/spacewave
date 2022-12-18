/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../kvtx/mqueue/mqueue.pb.js'

export const protobufPackage = 'store.kvtx'

/** Config is the configuration for the kvtx store. */
export interface Config {
  /**
   * MqueueConfig is the kvtx mqueue configuration.
   * Note: some stores override the mqueue implementation.
   */
  mqueueConfig: Config1 | undefined
}

/** MqueueMeta contains message queue metadata. */
export interface MqueueMeta {
  /** Id is the message queue id. */
  id: Uint8Array
}

/**
 * BucketReconcilerMqueueId is the message queue identifier.
 *
 * Encoded -> b58.
 */
export interface BucketReconcilerMqueueId {
  bucketId: string
  reconcilerId: string
}

function createBaseConfig(): Config {
  return { mqueueConfig: undefined }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.mqueueConfig !== undefined) {
      Config1.encode(message.mqueueConfig, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.mqueueConfig = Config1.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
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
      mqueueConfig: isSet(object.mqueueConfig)
        ? Config1.fromJSON(object.mqueueConfig)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.mqueueConfig !== undefined &&
      (obj.mqueueConfig = message.mqueueConfig
        ? Config1.toJSON(message.mqueueConfig)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.mqueueConfig =
      object.mqueueConfig !== undefined && object.mqueueConfig !== null
        ? Config1.fromPartial(object.mqueueConfig)
        : undefined
    return message
  },
}

function createBaseMqueueMeta(): MqueueMeta {
  return { id: new Uint8Array() }
}

export const MqueueMeta = {
  encode(
    message: MqueueMeta,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.id.length !== 0) {
      writer.uint32(10).bytes(message.id)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MqueueMeta {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseMqueueMeta()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.id = reader.bytes()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<MqueueMeta, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<MqueueMeta | MqueueMeta[]>
      | Iterable<MqueueMeta | MqueueMeta[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MqueueMeta.encode(p).finish()]
        }
      } else {
        yield* [MqueueMeta.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MqueueMeta>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<MqueueMeta> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MqueueMeta.decode(p)]
        }
      } else {
        yield* [MqueueMeta.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): MqueueMeta {
    return {
      id: isSet(object.id) ? bytesFromBase64(object.id) : new Uint8Array(),
    }
  },

  toJSON(message: MqueueMeta): unknown {
    const obj: any = {}
    message.id !== undefined &&
      (obj.id = base64FromBytes(
        message.id !== undefined ? message.id : new Uint8Array()
      ))
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<MqueueMeta>, I>>(
    object: I
  ): MqueueMeta {
    const message = createBaseMqueueMeta()
    message.id = object.id ?? new Uint8Array()
    return message
  },
}

function createBaseBucketReconcilerMqueueId(): BucketReconcilerMqueueId {
  return { bucketId: '', reconcilerId: '' }
}

export const BucketReconcilerMqueueId = {
  encode(
    message: BucketReconcilerMqueueId,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.bucketId !== '') {
      writer.uint32(10).string(message.bucketId)
    }
    if (message.reconcilerId !== '') {
      writer.uint32(18).string(message.reconcilerId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): BucketReconcilerMqueueId {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBucketReconcilerMqueueId()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.bucketId = reader.string()
          break
        case 2:
          message.reconcilerId = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<BucketReconcilerMqueueId, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<BucketReconcilerMqueueId | BucketReconcilerMqueueId[]>
      | Iterable<BucketReconcilerMqueueId | BucketReconcilerMqueueId[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketReconcilerMqueueId.encode(p).finish()]
        }
      } else {
        yield* [BucketReconcilerMqueueId.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BucketReconcilerMqueueId>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<BucketReconcilerMqueueId> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketReconcilerMqueueId.decode(p)]
        }
      } else {
        yield* [BucketReconcilerMqueueId.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): BucketReconcilerMqueueId {
    return {
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : '',
      reconcilerId: isSet(object.reconcilerId)
        ? String(object.reconcilerId)
        : '',
    }
  },

  toJSON(message: BucketReconcilerMqueueId): unknown {
    const obj: any = {}
    message.bucketId !== undefined && (obj.bucketId = message.bucketId)
    message.reconcilerId !== undefined &&
      (obj.reconcilerId = message.reconcilerId)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<BucketReconcilerMqueueId>, I>>(
    object: I
  ): BucketReconcilerMqueueId {
    const message = createBaseBucketReconcilerMqueueId()
    message.bucketId = object.bucketId ?? ''
    message.reconcilerId = object.reconcilerId ?? ''
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
