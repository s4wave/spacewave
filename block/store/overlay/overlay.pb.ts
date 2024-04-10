/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import {
  OverlayMode,
  overlayModeFromJSON,
  overlayModeToJSON,
  PutOpts,
} from '../../block.pb.js'

export const protobufPackage = 'block.store.overlay'

/** Config configures the overlay block store controller. */
export interface Config {
  /** BlockStoreId is the block store id to use on the bus. */
  blockStoreId: string
  /** LowerBlockStoreId is the identifier of the "lower" block store. */
  lowerBlockStoreId: string
  /** UpperBlockStoreId is the identifier of the "upper" block store. */
  upperBlockStoreId: string
  /** OverlayMode indicates the mode to use for the block store. */
  overlayMode: OverlayMode
  /**
   * WritebackTimeoutDur is the timeout for writing back blocks.
   * If overlay_mode does not enable writeback, this is N/A.
   * Example: 30s
   */
  writebackTimeoutDur: string
  /**
   * WritebackPutOpts are the base put options for writing back blocks.
   * If overlay_mode does not enable writeback, this is N/A.
   */
  writebackPutOpts: PutOpts | undefined
  /** BucketIds is a list of bucket ids to serve LookupBlockFromNetwork directives. */
  bucketIds: string[]
  /** SkipNotFound skips returning a value if the block was not found. */
  skipNotFound: boolean
  /** Verbose enables verbose logging of the block store. */
  verbose: boolean
}

function createBaseConfig(): Config {
  return {
    blockStoreId: '',
    lowerBlockStoreId: '',
    upperBlockStoreId: '',
    overlayMode: 0,
    writebackTimeoutDur: '',
    writebackPutOpts: undefined,
    bucketIds: [],
    skipNotFound: false,
    verbose: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.blockStoreId !== '') {
      writer.uint32(10).string(message.blockStoreId)
    }
    if (message.lowerBlockStoreId !== '') {
      writer.uint32(18).string(message.lowerBlockStoreId)
    }
    if (message.upperBlockStoreId !== '') {
      writer.uint32(26).string(message.upperBlockStoreId)
    }
    if (message.overlayMode !== 0) {
      writer.uint32(32).int32(message.overlayMode)
    }
    if (message.writebackTimeoutDur !== '') {
      writer.uint32(42).string(message.writebackTimeoutDur)
    }
    if (message.writebackPutOpts !== undefined) {
      PutOpts.encode(
        message.writebackPutOpts,
        writer.uint32(50).fork(),
      ).ldelim()
    }
    for (const v of message.bucketIds) {
      writer.uint32(58).string(v!)
    }
    if (message.skipNotFound !== false) {
      writer.uint32(64).bool(message.skipNotFound)
    }
    if (message.verbose !== false) {
      writer.uint32(72).bool(message.verbose)
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

          message.blockStoreId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.lowerBlockStoreId = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.upperBlockStoreId = reader.string()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.overlayMode = reader.int32() as any
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.writebackTimeoutDur = reader.string()
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.writebackPutOpts = PutOpts.decode(reader, reader.uint32())
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.bucketIds.push(reader.string())
          continue
        case 8:
          if (tag !== 64) {
            break
          }

          message.skipNotFound = reader.bool()
          continue
        case 9:
          if (tag !== 72) {
            break
          }

          message.verbose = reader.bool()
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
      blockStoreId: isSet(object.blockStoreId)
        ? globalThis.String(object.blockStoreId)
        : '',
      lowerBlockStoreId: isSet(object.lowerBlockStoreId)
        ? globalThis.String(object.lowerBlockStoreId)
        : '',
      upperBlockStoreId: isSet(object.upperBlockStoreId)
        ? globalThis.String(object.upperBlockStoreId)
        : '',
      overlayMode: isSet(object.overlayMode)
        ? overlayModeFromJSON(object.overlayMode)
        : 0,
      writebackTimeoutDur: isSet(object.writebackTimeoutDur)
        ? globalThis.String(object.writebackTimeoutDur)
        : '',
      writebackPutOpts: isSet(object.writebackPutOpts)
        ? PutOpts.fromJSON(object.writebackPutOpts)
        : undefined,
      bucketIds: globalThis.Array.isArray(object?.bucketIds)
        ? object.bucketIds.map((e: any) => globalThis.String(e))
        : [],
      skipNotFound: isSet(object.skipNotFound)
        ? globalThis.Boolean(object.skipNotFound)
        : false,
      verbose: isSet(object.verbose)
        ? globalThis.Boolean(object.verbose)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.blockStoreId !== '') {
      obj.blockStoreId = message.blockStoreId
    }
    if (message.lowerBlockStoreId !== '') {
      obj.lowerBlockStoreId = message.lowerBlockStoreId
    }
    if (message.upperBlockStoreId !== '') {
      obj.upperBlockStoreId = message.upperBlockStoreId
    }
    if (message.overlayMode !== 0) {
      obj.overlayMode = overlayModeToJSON(message.overlayMode)
    }
    if (message.writebackTimeoutDur !== '') {
      obj.writebackTimeoutDur = message.writebackTimeoutDur
    }
    if (message.writebackPutOpts !== undefined) {
      obj.writebackPutOpts = PutOpts.toJSON(message.writebackPutOpts)
    }
    if (message.bucketIds?.length) {
      obj.bucketIds = message.bucketIds
    }
    if (message.skipNotFound !== false) {
      obj.skipNotFound = message.skipNotFound
    }
    if (message.verbose !== false) {
      obj.verbose = message.verbose
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.blockStoreId = object.blockStoreId ?? ''
    message.lowerBlockStoreId = object.lowerBlockStoreId ?? ''
    message.upperBlockStoreId = object.upperBlockStoreId ?? ''
    message.overlayMode = object.overlayMode ?? 0
    message.writebackTimeoutDur = object.writebackTimeoutDur ?? ''
    message.writebackPutOpts =
      object.writebackPutOpts !== undefined && object.writebackPutOpts !== null
        ? PutOpts.fromPartial(object.writebackPutOpts)
        : undefined
    message.bucketIds = object.bucketIds?.map((e) => e) || []
    message.skipNotFound = object.skipNotFound ?? false
    message.verbose = object.verbose ?? false
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
