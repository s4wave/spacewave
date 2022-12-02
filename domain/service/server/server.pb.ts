/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'identity.domain.server'

/**
 * Config configures the identity server.
 * Forwards incoming requests to directives.
 */
export interface Config {
  /**
   * PeerIds are the list of peer IDs to listen on.
   * If empty, allows any incoming peer id w/ the protocol id.
   */
  peerIds: string[]
  /**
   * DomainIds is the list of domain IDs to service.
   * If empty, allows any domain ID.
   */
  domainIds: string[]
  /** RequestTimeout limits the amount of time a request can take. */
  requestTimeout: string
}

function createBaseConfig(): Config {
  return { peerIds: [], domainIds: [], requestTimeout: '' }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.peerIds) {
      writer.uint32(10).string(v!)
    }
    for (const v of message.domainIds) {
      writer.uint32(18).string(v!)
    }
    if (message.requestTimeout !== '') {
      writer.uint32(26).string(message.requestTimeout)
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
          message.peerIds.push(reader.string())
          break
        case 2:
          message.domainIds.push(reader.string())
          break
        case 3:
          message.requestTimeout = reader.string()
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
      peerIds: Array.isArray(object?.peerIds)
        ? object.peerIds.map((e: any) => String(e))
        : [],
      domainIds: Array.isArray(object?.domainIds)
        ? object.domainIds.map((e: any) => String(e))
        : [],
      requestTimeout: isSet(object.requestTimeout)
        ? String(object.requestTimeout)
        : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.peerIds) {
      obj.peerIds = message.peerIds.map((e) => e)
    } else {
      obj.peerIds = []
    }
    if (message.domainIds) {
      obj.domainIds = message.domainIds.map((e) => e)
    } else {
      obj.domainIds = []
    }
    message.requestTimeout !== undefined &&
      (obj.requestTimeout = message.requestTimeout)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.peerIds = object.peerIds?.map((e) => e) || []
    message.domainIds = object.domainIds?.map((e) => e) || []
    message.requestTimeout = object.requestTimeout ?? ''
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
