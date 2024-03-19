/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { ObjectRef } from '../../bucket/bucket.pb.js'
import { Config as Config1 } from '../../store/kvkey/kvkey.pb.js'
import { Config as Config3 } from '../../store/kvtx/kvtx.pb.js'
import { Config as Config2 } from '../controller/controller.pb.js'

export const protobufPackage = 'volume.world'

/** Config is the World Object block-graph backed hydra volume config. */
export interface Config {
  /** KvKeyOpts are key/value key constants. */
  kvKeyOpts: Config1 | undefined
  /** Verbose will log all operations to the logger for debugging. */
  verbose: boolean
  /** VolumeConfig is the volume controller config. */
  volumeConfig: Config2 | undefined
  /** StoreConfig is the store configuration for kvtx. */
  storeConfig: Config3 | undefined
  /**
   * NoGenerateKey indicates the controller should not generate a private key if
   * one is not already present. Setting this to false will cause the system to
   * create a new private key if one is not present in the store at startup. If
   * no key is in the store at startup and this is true, returns an error.
   */
  noGenerateKey: boolean
  /**
   * NoWriteKey indicates the controller should not write a private key to
   * storage if it generates one. This results in an ephemeral volume peer
   * identity if there is no key present in the store already.
   *
   * Has no effect if the store has a peer private key.
   */
  noWriteKey: boolean
  /**
   * InitHeadRef is the reference to the initial HEAD state of the volume.
   * If the object does not exist, uses this reference to initialize it.
   */
  initHeadRef: ObjectRef | undefined
  /** EngineId is the world engine to attach to for reading/writing state. */
  engineId: string
  /**
   * ObjectKey is the BlockVolume object to attach to.
   * If not exists, waits for it to exist.
   */
  objectKey: string
  /**
   * BucketId is the bucket id to attach to for reading/writing state.
   * If set, overrides the bucket id from init_head_ref and the state.
   * If unset, the bucket id is determined from head_ref.
   */
  bucketId: string
  /**
   * VolumeId is the volume id to attach to for reading/writing data.
   * If unset, init_head_ref must be set, and the volume will be read-only.
   */
  volumeId: string
}

function createBaseConfig(): Config {
  return {
    kvKeyOpts: undefined,
    verbose: false,
    volumeConfig: undefined,
    storeConfig: undefined,
    noGenerateKey: false,
    noWriteKey: false,
    initHeadRef: undefined,
    engineId: '',
    objectKey: '',
    bucketId: '',
    volumeId: '',
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(10).fork()).ldelim()
    }
    if (message.verbose !== false) {
      writer.uint32(16).bool(message.verbose)
    }
    if (message.volumeConfig !== undefined) {
      Config2.encode(message.volumeConfig, writer.uint32(26).fork()).ldelim()
    }
    if (message.storeConfig !== undefined) {
      Config3.encode(message.storeConfig, writer.uint32(34).fork()).ldelim()
    }
    if (message.noGenerateKey !== false) {
      writer.uint32(40).bool(message.noGenerateKey)
    }
    if (message.noWriteKey !== false) {
      writer.uint32(88).bool(message.noWriteKey)
    }
    if (message.initHeadRef !== undefined) {
      ObjectRef.encode(message.initHeadRef, writer.uint32(50).fork()).ldelim()
    }
    if (message.engineId !== '') {
      writer.uint32(58).string(message.engineId)
    }
    if (message.objectKey !== '') {
      writer.uint32(66).string(message.objectKey)
    }
    if (message.bucketId !== '') {
      writer.uint32(74).string(message.bucketId)
    }
    if (message.volumeId !== '') {
      writer.uint32(82).string(message.volumeId)
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

          message.kvKeyOpts = Config1.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.verbose = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.volumeConfig = Config2.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.storeConfig = Config3.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.noGenerateKey = reader.bool()
          continue
        case 11:
          if (tag !== 88) {
            break
          }

          message.noWriteKey = reader.bool()
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.initHeadRef = ObjectRef.decode(reader, reader.uint32())
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.engineId = reader.string()
          continue
        case 8:
          if (tag !== 66) {
            break
          }

          message.objectKey = reader.string()
          continue
        case 9:
          if (tag !== 74) {
            break
          }

          message.bucketId = reader.string()
          continue
        case 10:
          if (tag !== 82) {
            break
          }

          message.volumeId = reader.string()
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
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      verbose: isSet(object.verbose)
        ? globalThis.Boolean(object.verbose)
        : false,
      volumeConfig: isSet(object.volumeConfig)
        ? Config2.fromJSON(object.volumeConfig)
        : undefined,
      storeConfig: isSet(object.storeConfig)
        ? Config3.fromJSON(object.storeConfig)
        : undefined,
      noGenerateKey: isSet(object.noGenerateKey)
        ? globalThis.Boolean(object.noGenerateKey)
        : false,
      noWriteKey: isSet(object.noWriteKey)
        ? globalThis.Boolean(object.noWriteKey)
        : false,
      initHeadRef: isSet(object.initHeadRef)
        ? ObjectRef.fromJSON(object.initHeadRef)
        : undefined,
      engineId: isSet(object.engineId)
        ? globalThis.String(object.engineId)
        : '',
      objectKey: isSet(object.objectKey)
        ? globalThis.String(object.objectKey)
        : '',
      bucketId: isSet(object.bucketId)
        ? globalThis.String(object.bucketId)
        : '',
      volumeId: isSet(object.volumeId)
        ? globalThis.String(object.volumeId)
        : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.kvKeyOpts !== undefined) {
      obj.kvKeyOpts = Config1.toJSON(message.kvKeyOpts)
    }
    if (message.verbose !== false) {
      obj.verbose = message.verbose
    }
    if (message.volumeConfig !== undefined) {
      obj.volumeConfig = Config2.toJSON(message.volumeConfig)
    }
    if (message.storeConfig !== undefined) {
      obj.storeConfig = Config3.toJSON(message.storeConfig)
    }
    if (message.noGenerateKey !== false) {
      obj.noGenerateKey = message.noGenerateKey
    }
    if (message.noWriteKey !== false) {
      obj.noWriteKey = message.noWriteKey
    }
    if (message.initHeadRef !== undefined) {
      obj.initHeadRef = ObjectRef.toJSON(message.initHeadRef)
    }
    if (message.engineId !== '') {
      obj.engineId = message.engineId
    }
    if (message.objectKey !== '') {
      obj.objectKey = message.objectKey
    }
    if (message.bucketId !== '') {
      obj.bucketId = message.bucketId
    }
    if (message.volumeId !== '') {
      obj.volumeId = message.volumeId
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
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
    message.noWriteKey = object.noWriteKey ?? false
    message.initHeadRef =
      object.initHeadRef !== undefined && object.initHeadRef !== null
        ? ObjectRef.fromPartial(object.initHeadRef)
        : undefined
    message.engineId = object.engineId ?? ''
    message.objectKey = object.objectKey ?? ''
    message.bucketId = object.bucketId ?? ''
    message.volumeId = object.volumeId ?? ''
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
