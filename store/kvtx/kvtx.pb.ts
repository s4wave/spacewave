/* eslint-disable */
import {
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
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
  /**
   * HashType is the hash type to use for block refs.
   * If unset (0 value) will use default for Hydra (BLAKE3).
   */
  hashType: HashType
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
  return { mqueueConfig: undefined, hashType: 0 }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.mqueueConfig !== undefined) {
      Config1.encode(message.mqueueConfig, writer.uint32(10).fork()).ldelim()
    }
    if (message.hashType !== 0) {
      writer.uint32(16).int32(message.hashType)
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
          if (tag !== 10) {
            break
          }

          message.mqueueConfig = Config1.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.hashType = reader.int32() as any
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
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      mqueueConfig: isSet(object.mqueueConfig)
        ? Config1.fromJSON(object.mqueueConfig)
        : undefined,
      hashType: isSet(object.hashType) ? hashTypeFromJSON(object.hashType) : 0,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.mqueueConfig !== undefined) {
      obj.mqueueConfig = Config1.toJSON(message.mqueueConfig)
    }
    if (message.hashType !== 0) {
      obj.hashType = hashTypeToJSON(message.hashType)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.mqueueConfig =
      object.mqueueConfig !== undefined && object.mqueueConfig !== null
        ? Config1.fromPartial(object.mqueueConfig)
        : undefined
    message.hashType = object.hashType ?? 0
    return message
  },
}

function createBaseMqueueMeta(): MqueueMeta {
  return { id: new Uint8Array(0) }
}

export const MqueueMeta = {
  encode(
    message: MqueueMeta,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id.length !== 0) {
      writer.uint32(10).bytes(message.id)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MqueueMeta {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseMqueueMeta()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.id = reader.bytes()
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
  // Transform<MqueueMeta, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<MqueueMeta | MqueueMeta[]>
      | Iterable<MqueueMeta | MqueueMeta[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [MqueueMeta.encode(p).finish()]
        }
      } else {
        yield* [MqueueMeta.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MqueueMeta>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<MqueueMeta> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [MqueueMeta.decode(p)]
        }
      } else {
        yield* [MqueueMeta.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): MqueueMeta {
    return {
      id: isSet(object.id) ? bytesFromBase64(object.id) : new Uint8Array(0),
    }
  },

  toJSON(message: MqueueMeta): unknown {
    const obj: any = {}
    if (message.id.length !== 0) {
      obj.id = base64FromBytes(message.id)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<MqueueMeta>, I>>(base?: I): MqueueMeta {
    return MqueueMeta.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<MqueueMeta>, I>>(
    object: I,
  ): MqueueMeta {
    const message = createBaseMqueueMeta()
    message.id = object.id ?? new Uint8Array(0)
    return message
  },
}

function createBaseBucketReconcilerMqueueId(): BucketReconcilerMqueueId {
  return { bucketId: '', reconcilerId: '' }
}

export const BucketReconcilerMqueueId = {
  encode(
    message: BucketReconcilerMqueueId,
    writer: _m0.Writer = _m0.Writer.create(),
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
    length?: number,
  ): BucketReconcilerMqueueId {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBucketReconcilerMqueueId()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.bucketId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.reconcilerId = reader.string()
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
  // Transform<BucketReconcilerMqueueId, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<BucketReconcilerMqueueId | BucketReconcilerMqueueId[]>
      | Iterable<BucketReconcilerMqueueId | BucketReconcilerMqueueId[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [BucketReconcilerMqueueId.encode(p).finish()]
        }
      } else {
        yield* [BucketReconcilerMqueueId.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BucketReconcilerMqueueId>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BucketReconcilerMqueueId> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [BucketReconcilerMqueueId.decode(p)]
        }
      } else {
        yield* [BucketReconcilerMqueueId.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): BucketReconcilerMqueueId {
    return {
      bucketId: isSet(object.bucketId)
        ? globalThis.String(object.bucketId)
        : '',
      reconcilerId: isSet(object.reconcilerId)
        ? globalThis.String(object.reconcilerId)
        : '',
    }
  },

  toJSON(message: BucketReconcilerMqueueId): unknown {
    const obj: any = {}
    if (message.bucketId !== '') {
      obj.bucketId = message.bucketId
    }
    if (message.reconcilerId !== '') {
      obj.reconcilerId = message.reconcilerId
    }
    return obj
  },

  create<I extends Exact<DeepPartial<BucketReconcilerMqueueId>, I>>(
    base?: I,
  ): BucketReconcilerMqueueId {
    return BucketReconcilerMqueueId.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<BucketReconcilerMqueueId>, I>>(
    object: I,
  ): BucketReconcilerMqueueId {
    const message = createBaseBucketReconcilerMqueueId()
    message.bucketId = object.bucketId ?? ''
    message.reconcilerId = object.reconcilerId ?? ''
    return message
  },
}

function bytesFromBase64(b64: string): Uint8Array {
  if (globalThis.Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, 'base64'))
  } else {
    const bin = globalThis.atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i)
    }
    return arr
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (globalThis.Buffer) {
    return globalThis.Buffer.from(arr).toString('base64')
  } else {
    const bin: string[] = []
    arr.forEach((byte) => {
      bin.push(globalThis.String.fromCharCode(byte))
    })
    return globalThis.btoa(bin.join(''))
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
