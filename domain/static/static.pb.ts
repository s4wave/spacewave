/* eslint-disable */
import Long from 'long'
import { Entity } from '../../identity.pb.js'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'identity.domain.static'

/** Config is the static identity provider config. */
export interface Config {
  /**
   * Domains is the list of domains to service lookups for.
   * If empty services all domains.
   */
  domains: string[]
  /** Entities is the set of entities to make available on the domain. */
  entities: Entity[]
  /** SilentNotFound indicates not found will not satistfy the lookup. */
  silentNotFound: boolean
}

function createBaseConfig(): Config {
  return { domains: [], entities: [], silentNotFound: false }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.domains) {
      writer.uint32(10).string(v!)
    }
    for (const v of message.entities) {
      Entity.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    if (message.silentNotFound === true) {
      writer.uint32(24).bool(message.silentNotFound)
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
          message.domains.push(reader.string())
          break
        case 2:
          message.entities.push(Entity.decode(reader, reader.uint32()))
          break
        case 3:
          message.silentNotFound = reader.bool()
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
      domains: Array.isArray(object?.domains)
        ? object.domains.map((e: any) => String(e))
        : [],
      entities: Array.isArray(object?.entities)
        ? object.entities.map((e: any) => Entity.fromJSON(e))
        : [],
      silentNotFound: isSet(object.silentNotFound)
        ? Boolean(object.silentNotFound)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.domains) {
      obj.domains = message.domains.map((e) => e)
    } else {
      obj.domains = []
    }
    if (message.entities) {
      obj.entities = message.entities.map((e) =>
        e ? Entity.toJSON(e) : undefined
      )
    } else {
      obj.entities = []
    }
    message.silentNotFound !== undefined &&
      (obj.silentNotFound = message.silentNotFound)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.domains = object.domains?.map((e) => e) || []
    message.entities = object.entities?.map((e) => Entity.fromPartial(e)) || []
    message.silentNotFound = object.silentNotFound ?? false
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
