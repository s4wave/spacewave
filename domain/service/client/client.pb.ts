/* eslint-disable */
import Long from 'long'
import { DomainInfo } from '../../domain.pb.js'
import { Config as Config1 } from '@go/github.com/aperturerobotics/bifrost/stream/srpc/client/client.pb.js'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'identity.domain.client'

/** Config configures the identity client domain controller. */
export interface Config {
  /** DomainInfo is the identity domain information object. */
  domainInfo: DomainInfo | undefined
  /** ClientOpts are options passed to the client. */
  clientOpts: Config1 | undefined
  /**
   * PeerId is the peer id to use to sign requests.
   * Private key must be available.
   */
  peerId: string
}

function createBaseConfig(): Config {
  return { domainInfo: undefined, clientOpts: undefined, peerId: '' }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.domainInfo !== undefined) {
      DomainInfo.encode(message.domainInfo, writer.uint32(10).fork()).ldelim()
    }
    if (message.clientOpts !== undefined) {
      Config1.encode(message.clientOpts, writer.uint32(18).fork()).ldelim()
    }
    if (message.peerId !== '') {
      writer.uint32(26).string(message.peerId)
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
          message.domainInfo = DomainInfo.decode(reader, reader.uint32())
          break
        case 2:
          message.clientOpts = Config1.decode(reader, reader.uint32())
          break
        case 3:
          message.peerId = reader.string()
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
      domainInfo: isSet(object.domainInfo)
        ? DomainInfo.fromJSON(object.domainInfo)
        : undefined,
      clientOpts: isSet(object.clientOpts)
        ? Config1.fromJSON(object.clientOpts)
        : undefined,
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.domainInfo !== undefined &&
      (obj.domainInfo = message.domainInfo
        ? DomainInfo.toJSON(message.domainInfo)
        : undefined)
    message.clientOpts !== undefined &&
      (obj.clientOpts = message.clientOpts
        ? Config1.toJSON(message.clientOpts)
        : undefined)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.domainInfo =
      object.domainInfo !== undefined && object.domainInfo !== null
        ? DomainInfo.fromPartial(object.domainInfo)
        : undefined
    message.clientOpts =
      object.clientOpts !== undefined && object.clientOpts !== null
        ? Config1.fromPartial(object.clientOpts)
        : undefined
    message.peerId = object.peerId ?? ''
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
