/* eslint-disable */
import { Timestamp } from '@go/github.com/aperturerobotics/timestamp/timestamp.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { UnixfsRef } from '../unixfs.pb.js'

export const protobufPackage = 'unixfs.world.access'

/**
 * Config configures the world-backed UnixFS access controller.
 * Resolves AccessUnixFS requests with a Hydra World UnixFS.
 */
export interface Config {
  /** FsId is the filesystem ID to expose on the bus. */
  fsId: string
  /** EngineId is the world engine ID to access. */
  engineId: string
  /**
   * PeerId is the peer id to use for transactions.
   * If unset, the filesystem will be read-only.
   */
  peerId: string
  /** FsRef is the reference to the filesystem. */
  fsRef: UnixfsRef | undefined
  /** MkdirPath creates the path within the FS if it doesn't exist. */
  mkdirPath: boolean
  /** DisableWatchChanges disables watching for changes in the FS. */
  disableWatchChanges: boolean
  /**
   * Timestamp sets a constant timestamp for write operations.
   * If unset, uses time.Now()
   */
  timestamp: Timestamp | undefined
}

function createBaseConfig(): Config {
  return {
    fsId: '',
    engineId: '',
    peerId: '',
    fsRef: undefined,
    mkdirPath: false,
    disableWatchChanges: false,
    timestamp: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.fsId !== '') {
      writer.uint32(10).string(message.fsId)
    }
    if (message.engineId !== '') {
      writer.uint32(18).string(message.engineId)
    }
    if (message.peerId !== '') {
      writer.uint32(26).string(message.peerId)
    }
    if (message.fsRef !== undefined) {
      UnixfsRef.encode(message.fsRef, writer.uint32(34).fork()).ldelim()
    }
    if (message.mkdirPath === true) {
      writer.uint32(40).bool(message.mkdirPath)
    }
    if (message.disableWatchChanges === true) {
      writer.uint32(48).bool(message.disableWatchChanges)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(58).fork()).ldelim()
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

          message.fsId = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.engineId = reader.string()
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.peerId = reader.string()
          continue
        case 4:
          if (tag != 34) {
            break
          }

          message.fsRef = UnixfsRef.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag != 40) {
            break
          }

          message.mkdirPath = reader.bool()
          continue
        case 6:
          if (tag != 48) {
            break
          }

          message.disableWatchChanges = reader.bool()
          continue
        case 7:
          if (tag != 58) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
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
      fsId: isSet(object.fsId) ? String(object.fsId) : '',
      engineId: isSet(object.engineId) ? String(object.engineId) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
      fsRef: isSet(object.fsRef) ? UnixfsRef.fromJSON(object.fsRef) : undefined,
      mkdirPath: isSet(object.mkdirPath) ? Boolean(object.mkdirPath) : false,
      disableWatchChanges: isSet(object.disableWatchChanges)
        ? Boolean(object.disableWatchChanges)
        : false,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.fsId !== undefined && (obj.fsId = message.fsId)
    message.engineId !== undefined && (obj.engineId = message.engineId)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    message.fsRef !== undefined &&
      (obj.fsRef = message.fsRef ? UnixfsRef.toJSON(message.fsRef) : undefined)
    message.mkdirPath !== undefined && (obj.mkdirPath = message.mkdirPath)
    message.disableWatchChanges !== undefined &&
      (obj.disableWatchChanges = message.disableWatchChanges)
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp
        ? Timestamp.toJSON(message.timestamp)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.fsId = object.fsId ?? ''
    message.engineId = object.engineId ?? ''
    message.peerId = object.peerId ?? ''
    message.fsRef =
      object.fsRef !== undefined && object.fsRef !== null
        ? UnixfsRef.fromPartial(object.fsRef)
        : undefined
    message.mkdirPath = object.mkdirPath ?? false
    message.disableWatchChanges = object.disableWatchChanges ?? false
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
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
