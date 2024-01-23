/* eslint-disable */
import { Config as Config1 } from '@go/github.com/aperturerobotics/bifrost/stream/srpc/server/server.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'identity.domain.server'

/**
 * Config configures the identity server.
 * Forwards incoming requests to directives.
 */
export interface Config {
  /**
   * Server configures the peer ids and protocol ids to listen on.
   * If the protocol IDs list is empty, uses the default protocol id.
   */
  server: Config1 | undefined
  /**
   * DomainIds is the list of domain IDs to service.
   * If empty, allows any domain ID.
   */
  domainIds: string[]
  /** RequestTimeout limits the amount of time a request can take. */
  requestTimeout: string
}

function createBaseConfig(): Config {
  return { server: undefined, domainIds: [], requestTimeout: '' }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.server !== undefined) {
      Config1.encode(message.server, writer.uint32(10).fork()).ldelim()
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

          message.server = Config1.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.domainIds.push(reader.string())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.requestTimeout = reader.string()
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
      server: isSet(object.server)
        ? Config1.fromJSON(object.server)
        : undefined,
      domainIds: globalThis.Array.isArray(object?.domainIds)
        ? object.domainIds.map((e: any) => globalThis.String(e))
        : [],
      requestTimeout: isSet(object.requestTimeout)
        ? globalThis.String(object.requestTimeout)
        : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.server !== undefined) {
      obj.server = Config1.toJSON(message.server)
    }
    if (message.domainIds?.length) {
      obj.domainIds = message.domainIds
    }
    if (message.requestTimeout !== '') {
      obj.requestTimeout = message.requestTimeout
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.server =
      object.server !== undefined && object.server !== null
        ? Config1.fromPartial(object.server)
        : undefined
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
