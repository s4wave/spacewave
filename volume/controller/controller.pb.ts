/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import {
  BlockStoreMode,
  blockStoreModeFromJSON,
  blockStoreModeToJSON,
} from '../../block/store/store.pb.js'

export const protobufPackage = 'volume.controller'

/** Config configures the generic volume controller. */
export interface Config {
  /**
   * DisableEventBlockRm disables the block removed event.
   *
   * Optimization: skips exists() and mqueue write() on delete.
   */
  disableEventBlockRm: boolean
  /** VolumeIdAlias matches LookupVolume calls for the given ids. */
  volumeIdAlias: string[]
  /** DisableReconcilerQueues disables waking filled reconciler queues. */
  disableReconcilerQueues: boolean
  /** DisablePeer disables loading the peer controller from the volume. */
  disablePeer: boolean
  /**
   * BlockStoreId configures using a separate block store for blocks.
   * uses LookupBlockStore to lookup the block store on the bus.
   */
  blockStoreId: string
  /**
   * BlockStoreMode indicates the mode to use for the block store.
   * The volume is the lower, the block store is the upper.
   * Does nothing if block_store_id is empty.
   */
  blockStoreMode: BlockStoreMode
}

function createBaseConfig(): Config {
  return {
    disableEventBlockRm: false,
    volumeIdAlias: [],
    disableReconcilerQueues: false,
    disablePeer: false,
    blockStoreId: '',
    blockStoreMode: 0,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.disableEventBlockRm === true) {
      writer.uint32(8).bool(message.disableEventBlockRm)
    }
    for (const v of message.volumeIdAlias) {
      writer.uint32(18).string(v!)
    }
    if (message.disableReconcilerQueues === true) {
      writer.uint32(24).bool(message.disableReconcilerQueues)
    }
    if (message.disablePeer === true) {
      writer.uint32(32).bool(message.disablePeer)
    }
    if (message.blockStoreId !== '') {
      writer.uint32(42).string(message.blockStoreId)
    }
    if (message.blockStoreMode !== 0) {
      writer.uint32(48).int32(message.blockStoreMode)
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
          if (tag !== 8) {
            break
          }

          message.disableEventBlockRm = reader.bool()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.volumeIdAlias.push(reader.string())
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.disableReconcilerQueues = reader.bool()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.disablePeer = reader.bool()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.blockStoreId = reader.string()
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.blockStoreMode = reader.int32() as any
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
      disableEventBlockRm: isSet(object.disableEventBlockRm)
        ? globalThis.Boolean(object.disableEventBlockRm)
        : false,
      volumeIdAlias: globalThis.Array.isArray(object?.volumeIdAlias)
        ? object.volumeIdAlias.map((e: any) => globalThis.String(e))
        : [],
      disableReconcilerQueues: isSet(object.disableReconcilerQueues)
        ? globalThis.Boolean(object.disableReconcilerQueues)
        : false,
      disablePeer: isSet(object.disablePeer)
        ? globalThis.Boolean(object.disablePeer)
        : false,
      blockStoreId: isSet(object.blockStoreId)
        ? globalThis.String(object.blockStoreId)
        : '',
      blockStoreMode: isSet(object.blockStoreMode)
        ? blockStoreModeFromJSON(object.blockStoreMode)
        : 0,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.disableEventBlockRm === true) {
      obj.disableEventBlockRm = message.disableEventBlockRm
    }
    if (message.volumeIdAlias?.length) {
      obj.volumeIdAlias = message.volumeIdAlias
    }
    if (message.disableReconcilerQueues === true) {
      obj.disableReconcilerQueues = message.disableReconcilerQueues
    }
    if (message.disablePeer === true) {
      obj.disablePeer = message.disablePeer
    }
    if (message.blockStoreId !== '') {
      obj.blockStoreId = message.blockStoreId
    }
    if (message.blockStoreMode !== 0) {
      obj.blockStoreMode = blockStoreModeToJSON(message.blockStoreMode)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.disableEventBlockRm = object.disableEventBlockRm ?? false
    message.volumeIdAlias = object.volumeIdAlias?.map((e) => e) || []
    message.disableReconcilerQueues = object.disableReconcilerQueues ?? false
    message.disablePeer = object.disablePeer ?? false
    message.blockStoreId = object.blockStoreId ?? ''
    message.blockStoreMode = object.blockStoreMode ?? 0
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
