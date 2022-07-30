/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

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
}

function createBaseConfig(): Config {
  return { disableEventBlockRm: false, volumeIdAlias: [] }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.disableEventBlockRm === true) {
      writer.uint32(8).bool(message.disableEventBlockRm)
    }
    for (const v of message.volumeIdAlias) {
      writer.uint32(18).string(v!)
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
          message.disableEventBlockRm = reader.bool()
          break
        case 2:
          message.volumeIdAlias.push(reader.string())
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
      disableEventBlockRm: isSet(object.disableEventBlockRm)
        ? Boolean(object.disableEventBlockRm)
        : false,
      volumeIdAlias: Array.isArray(object?.volumeIdAlias)
        ? object.volumeIdAlias.map((e: any) => String(e))
        : [],
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.disableEventBlockRm !== undefined &&
      (obj.disableEventBlockRm = message.disableEventBlockRm)
    if (message.volumeIdAlias) {
      obj.volumeIdAlias = message.volumeIdAlias.map((e) => e)
    } else {
      obj.volumeIdAlias = []
    }
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.disableEventBlockRm = object.disableEventBlockRm ?? false
    message.volumeIdAlias = object.volumeIdAlias?.map((e) => e) || []
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
