/* eslint-disable */
import Long from 'long'
import { ObjectRef } from '../../../bucket/bucket.pb.js'
import { Config as Config1 } from '../../../block/transform/transform.pb.js'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'world.block.engine'

/**
 * Config configures a World Graph engine bound to a block graph.
 * Builds a bucket handle using the given bucket ID.
 * Stores the HEAD reference in an object store.
 */
export interface Config {
  /**
   * EngineId is the identifier used to look up the world on the bus.
   * Used to match & resolve WorldEngine directives.
   * If empty, LookupWorldEngine will not be processed.
   */
  engineId: string
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
   * Defaults to "world-head"
   */
  objectStoreHeadKey: string
  /**
   * InitHeadRef is the reference to the initial HEAD state of the database.
   * If the object store is empty, uses this reference to initialize it.
   * BucketId is overridden by BucketId field if it is set.
   */
  initHeadRef: ObjectRef | undefined
  /** StateTransformConf transforms the HEAD ref before storing it in storage. */
  stateTransformConf: Config1 | undefined
  /**
   * DisableLookup disables looking up anything on the bus via directives.
   * Implies both DisableApplyWorldOp and DisableApplyObjectOp.
   */
  disableLookup: boolean
  /** DisableApplyWorldOp disables calling the ApplyWorldOp directive. */
  disableApplyWorldOp: boolean
  /** DisableApplyObjectOp directive. */
  disableApplyObjectOp: boolean
  /** Verbose logs all operation results as debug messages. */
  verbose: boolean
}

/** HeadState contains the head state in the object storage. */
export interface HeadState {
  /** HeadRef is the reference to the current HEAD state of the database. */
  headRef: ObjectRef | undefined
}

function createBaseConfig(): Config {
  return {
    engineId: '',
    bucketId: '',
    volumeId: '',
    objectStoreId: '',
    objectStorePrefix: '',
    objectStoreHeadKey: '',
    initHeadRef: undefined,
    stateTransformConf: undefined,
    disableLookup: false,
    disableApplyWorldOp: false,
    disableApplyObjectOp: false,
    verbose: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.engineId !== '') {
      writer.uint32(10).string(message.engineId)
    }
    if (message.bucketId !== '') {
      writer.uint32(18).string(message.bucketId)
    }
    if (message.volumeId !== '') {
      writer.uint32(26).string(message.volumeId)
    }
    if (message.objectStoreId !== '') {
      writer.uint32(34).string(message.objectStoreId)
    }
    if (message.objectStorePrefix !== '') {
      writer.uint32(42).string(message.objectStorePrefix)
    }
    if (message.objectStoreHeadKey !== '') {
      writer.uint32(50).string(message.objectStoreHeadKey)
    }
    if (message.initHeadRef !== undefined) {
      ObjectRef.encode(message.initHeadRef, writer.uint32(58).fork()).ldelim()
    }
    if (message.stateTransformConf !== undefined) {
      Config1.encode(
        message.stateTransformConf,
        writer.uint32(90).fork()
      ).ldelim()
    }
    if (message.disableLookup === true) {
      writer.uint32(64).bool(message.disableLookup)
    }
    if (message.disableApplyWorldOp === true) {
      writer.uint32(72).bool(message.disableApplyWorldOp)
    }
    if (message.disableApplyObjectOp === true) {
      writer.uint32(80).bool(message.disableApplyObjectOp)
    }
    if (message.verbose === true) {
      writer.uint32(96).bool(message.verbose)
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
          message.engineId = reader.string()
          break
        case 2:
          message.bucketId = reader.string()
          break
        case 3:
          message.volumeId = reader.string()
          break
        case 4:
          message.objectStoreId = reader.string()
          break
        case 5:
          message.objectStorePrefix = reader.string()
          break
        case 6:
          message.objectStoreHeadKey = reader.string()
          break
        case 7:
          message.initHeadRef = ObjectRef.decode(reader, reader.uint32())
          break
        case 11:
          message.stateTransformConf = Config1.decode(reader, reader.uint32())
          break
        case 8:
          message.disableLookup = reader.bool()
          break
        case 9:
          message.disableApplyWorldOp = reader.bool()
          break
        case 10:
          message.disableApplyObjectOp = reader.bool()
          break
        case 12:
          message.verbose = reader.bool()
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
      engineId: isSet(object.engineId) ? String(object.engineId) : '',
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
        ? Config1.fromJSON(object.stateTransformConf)
        : undefined,
      disableLookup: isSet(object.disableLookup)
        ? Boolean(object.disableLookup)
        : false,
      disableApplyWorldOp: isSet(object.disableApplyWorldOp)
        ? Boolean(object.disableApplyWorldOp)
        : false,
      disableApplyObjectOp: isSet(object.disableApplyObjectOp)
        ? Boolean(object.disableApplyObjectOp)
        : false,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.engineId !== undefined && (obj.engineId = message.engineId)
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
        ? Config1.toJSON(message.stateTransformConf)
        : undefined)
    message.disableLookup !== undefined &&
      (obj.disableLookup = message.disableLookup)
    message.disableApplyWorldOp !== undefined &&
      (obj.disableApplyWorldOp = message.disableApplyWorldOp)
    message.disableApplyObjectOp !== undefined &&
      (obj.disableApplyObjectOp = message.disableApplyObjectOp)
    message.verbose !== undefined && (obj.verbose = message.verbose)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.engineId = object.engineId ?? ''
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
        ? Config1.fromPartial(object.stateTransformConf)
        : undefined
    message.disableLookup = object.disableLookup ?? false
    message.disableApplyWorldOp = object.disableApplyWorldOp ?? false
    message.disableApplyObjectOp = object.disableApplyObjectOp ?? false
    message.verbose = object.verbose ?? false
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
