/* eslint-disable */
import Long from 'long'
import { Config as Config1 } from '../../store/kvkey/kvkey.pb.js'
import { Config as Config2 } from '../controller/controller.pb.js'
import { Config as Config3 } from '../../store/kvtx/kv_tx.pb.js'
import { ObjectRef } from '../../bucket/bucket.pb.js'
import { Config as Config4 } from '../../block/transform/transform.pb.js'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'volume.block'

/** Config is the block graph backed hydra volume config. */
export interface Config {
  /** KvKeyOpts are key/value key constants. */
  kvKeyOpts: Config1 | undefined
  /** Verbose will log all operations to the logger for debugging. */
  verbose: boolean
  /** VolumeConfig is the volume controller config. */
  volumeConfig: Config2 | undefined
  /** StoreConfig is the store queue configuration for kvtx. */
  storeConfig: Config3 | undefined
  /**
   * NoGenerateKey indicates the controller should not generate a private key if
   * one is already present. Setting this to false will cause the system to
   * create a new private key if one is not present in the store at startup. If
   * no key is in the store at startup and this is true, an error will be
   * returned.
   */
  noGenerateKey: boolean
  /**
   * BucketId is the bucket id to attach to for reading/writing state.
   * If set, overrides the bucket id from init_head_ref and the state.
   * If unset, the bucket id is determined from head_ref.
   */
  bucketId: string
  /**
   * VolumeId is the volume id to attach to for writing DB state.
   * If unset, init_head_ref must be set, and the db will be read-only.
   */
  volumeId: string
  /**
   * ObjectStoreId is the hydra object store to open to store the HEAD ref.
   * If unset, init_head_ref must be set, and the db will be read-only.
   */
  objectStoreId: string
  /** ObjectStorePrefix is the prefix to use for all object store ops. */
  objectStorePrefix: string
  /**
   * ObjectStoreHeadKey is the key to use in the object store for HEAD ref.
   *
   * Defaults to "volume-head"
   */
  objectStoreHeadKey: string
  /**
   * InitHeadRef is the reference to the initial HEAD state of the database.
   * If the object store is empty, uses this reference to initialize it.
   * BucketId is overridden by BucketId field if it is set.
   */
  initHeadRef: ObjectRef | undefined
  /** StateTransformConf transforms the HEAD ref before storing it in storage. */
  stateTransformConf: Config4 | undefined
}

/**
 * HeadState contains the latest state of the volume.
 *
 * value of the object stored in object storage.
 */
export interface HeadState {
  /** HeadRef is the reference to the current HEAD state of the volume. */
  headRef: ObjectRef | undefined
}

function createBaseConfig(): Config {
  return {
    kvKeyOpts: undefined,
    verbose: false,
    volumeConfig: undefined,
    storeConfig: undefined,
    noGenerateKey: false,
    bucketId: '',
    volumeId: '',
    objectStoreId: '',
    objectStorePrefix: '',
    objectStoreHeadKey: '',
    initHeadRef: undefined,
    stateTransformConf: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(10).fork()).ldelim()
    }
    if (message.verbose === true) {
      writer.uint32(16).bool(message.verbose)
    }
    if (message.volumeConfig !== undefined) {
      Config2.encode(message.volumeConfig, writer.uint32(26).fork()).ldelim()
    }
    if (message.storeConfig !== undefined) {
      Config3.encode(message.storeConfig, writer.uint32(34).fork()).ldelim()
    }
    if (message.noGenerateKey === true) {
      writer.uint32(40).bool(message.noGenerateKey)
    }
    if (message.bucketId !== '') {
      writer.uint32(50).string(message.bucketId)
    }
    if (message.volumeId !== '') {
      writer.uint32(58).string(message.volumeId)
    }
    if (message.objectStoreId !== '') {
      writer.uint32(66).string(message.objectStoreId)
    }
    if (message.objectStorePrefix !== '') {
      writer.uint32(74).string(message.objectStorePrefix)
    }
    if (message.objectStoreHeadKey !== '') {
      writer.uint32(82).string(message.objectStoreHeadKey)
    }
    if (message.initHeadRef !== undefined) {
      ObjectRef.encode(message.initHeadRef, writer.uint32(90).fork()).ldelim()
    }
    if (message.stateTransformConf !== undefined) {
      Config4.encode(
        message.stateTransformConf,
        writer.uint32(98).fork()
      ).ldelim()
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
          message.kvKeyOpts = Config1.decode(reader, reader.uint32())
          break
        case 2:
          message.verbose = reader.bool()
          break
        case 3:
          message.volumeConfig = Config2.decode(reader, reader.uint32())
          break
        case 4:
          message.storeConfig = Config3.decode(reader, reader.uint32())
          break
        case 5:
          message.noGenerateKey = reader.bool()
          break
        case 6:
          message.bucketId = reader.string()
          break
        case 7:
          message.volumeId = reader.string()
          break
        case 8:
          message.objectStoreId = reader.string()
          break
        case 9:
          message.objectStorePrefix = reader.string()
          break
        case 10:
          message.objectStoreHeadKey = reader.string()
          break
        case 11:
          message.initHeadRef = ObjectRef.decode(reader, reader.uint32())
          break
        case 12:
          message.stateTransformConf = Config4.decode(reader, reader.uint32())
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
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
      volumeConfig: isSet(object.volumeConfig)
        ? Config2.fromJSON(object.volumeConfig)
        : undefined,
      storeConfig: isSet(object.storeConfig)
        ? Config3.fromJSON(object.storeConfig)
        : undefined,
      noGenerateKey: isSet(object.noGenerateKey)
        ? Boolean(object.noGenerateKey)
        : false,
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : '',
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : '',
      objectStoreId: isSet(object.objectStoreId)
        ? String(object.objectStoreId)
        : '',
      objectStorePrefix: isSet(object.objectStorePrefix)
        ? String(object.objectStorePrefix)
        : '',
      objectStoreHeadKey: isSet(object.objectStoreHeadKey)
        ? String(object.objectStoreHeadKey)
        : '',
      initHeadRef: isSet(object.initHeadRef)
        ? ObjectRef.fromJSON(object.initHeadRef)
        : undefined,
      stateTransformConf: isSet(object.stateTransformConf)
        ? Config4.fromJSON(object.stateTransformConf)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.kvKeyOpts !== undefined &&
      (obj.kvKeyOpts = message.kvKeyOpts
        ? Config1.toJSON(message.kvKeyOpts)
        : undefined)
    message.verbose !== undefined && (obj.verbose = message.verbose)
    message.volumeConfig !== undefined &&
      (obj.volumeConfig = message.volumeConfig
        ? Config2.toJSON(message.volumeConfig)
        : undefined)
    message.storeConfig !== undefined &&
      (obj.storeConfig = message.storeConfig
        ? Config3.toJSON(message.storeConfig)
        : undefined)
    message.noGenerateKey !== undefined &&
      (obj.noGenerateKey = message.noGenerateKey)
    message.bucketId !== undefined && (obj.bucketId = message.bucketId)
    message.volumeId !== undefined && (obj.volumeId = message.volumeId)
    message.objectStoreId !== undefined &&
      (obj.objectStoreId = message.objectStoreId)
    message.objectStorePrefix !== undefined &&
      (obj.objectStorePrefix = message.objectStorePrefix)
    message.objectStoreHeadKey !== undefined &&
      (obj.objectStoreHeadKey = message.objectStoreHeadKey)
    message.initHeadRef !== undefined &&
      (obj.initHeadRef = message.initHeadRef
        ? ObjectRef.toJSON(message.initHeadRef)
        : undefined)
    message.stateTransformConf !== undefined &&
      (obj.stateTransformConf = message.stateTransformConf
        ? Config4.toJSON(message.stateTransformConf)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.kvKeyOpts =
      object.kvKeyOpts !== undefined && object.kvKeyOpts !== null
        ? Config1.fromPartial(object.kvKeyOpts)
        : undefined
    message.verbose = object.verbose ?? false
    message.volumeConfig =
      object.volumeConfig !== undefined && object.volumeConfig !== null
        ? Config2.fromPartial(object.volumeConfig)
        : undefined
    message.storeConfig =
      object.storeConfig !== undefined && object.storeConfig !== null
        ? Config3.fromPartial(object.storeConfig)
        : undefined
    message.noGenerateKey = object.noGenerateKey ?? false
    message.bucketId = object.bucketId ?? ''
    message.volumeId = object.volumeId ?? ''
    message.objectStoreId = object.objectStoreId ?? ''
    message.objectStorePrefix = object.objectStorePrefix ?? ''
    message.objectStoreHeadKey = object.objectStoreHeadKey ?? ''
    message.initHeadRef =
      object.initHeadRef !== undefined && object.initHeadRef !== null
        ? ObjectRef.fromPartial(object.initHeadRef)
        : undefined
    message.stateTransformConf =
      object.stateTransformConf !== undefined &&
      object.stateTransformConf !== null
        ? Config4.fromPartial(object.stateTransformConf)
        : undefined
    return message
  },
}

function createBaseHeadState(): HeadState {
  return { headRef: undefined }
}

export const HeadState = {
  encode(
    message: HeadState,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.headRef !== undefined) {
      ObjectRef.encode(message.headRef, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HeadState {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHeadState()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.headRef = ObjectRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<HeadState, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<HeadState | HeadState[]>
      | Iterable<HeadState | HeadState[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HeadState.encode(p).finish()]
        }
      } else {
        yield* [HeadState.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HeadState>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<HeadState> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HeadState.decode(p)]
        }
      } else {
        yield* [HeadState.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): HeadState {
    return {
      headRef: isSet(object.headRef)
        ? ObjectRef.fromJSON(object.headRef)
        : undefined,
    }
  },

  toJSON(message: HeadState): unknown {
    const obj: any = {}
    message.headRef !== undefined &&
      (obj.headRef = message.headRef
        ? ObjectRef.toJSON(message.headRef)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<HeadState>, I>>(
    object: I
  ): HeadState {
    const message = createBaseHeadState()
    message.headRef =
      object.headRef !== undefined && object.headRef !== null
        ? ObjectRef.fromPartial(object.headRef)
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
