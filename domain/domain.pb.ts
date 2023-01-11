/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'identity.domain'

/** DomainInfo contains information about the domain to show the user. */
export interface DomainInfo {
  /**
   * DomainId is the domain identifier.
   * (not human readable)
   */
  domainId: string
  /** Name is the domain name to render (title). */
  name: string
  /** Description is the short description to show. */
  description: string
}

function createBaseDomainInfo(): DomainInfo {
  return { domainId: '', name: '', description: '' }
}

export const DomainInfo = {
  encode(
    message: DomainInfo,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.domainId !== '') {
      writer.uint32(10).string(message.domainId)
    }
    if (message.name !== '') {
      writer.uint32(18).string(message.name)
    }
    if (message.description !== '') {
      writer.uint32(26).string(message.description)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DomainInfo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDomainInfo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.domainId = reader.string()
          break
        case 2:
          message.name = reader.string()
          break
        case 3:
          message.description = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<DomainInfo, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DomainInfo | DomainInfo[]>
      | Iterable<DomainInfo | DomainInfo[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DomainInfo.encode(p).finish()]
        }
      } else {
        yield* [DomainInfo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DomainInfo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<DomainInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DomainInfo.decode(p)]
        }
      } else {
        yield* [DomainInfo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): DomainInfo {
    return {
      domainId: isSet(object.domainId) ? String(object.domainId) : '',
      name: isSet(object.name) ? String(object.name) : '',
      description: isSet(object.description) ? String(object.description) : '',
    }
  },

  toJSON(message: DomainInfo): unknown {
    const obj: any = {}
    message.domainId !== undefined && (obj.domainId = message.domainId)
    message.name !== undefined && (obj.name = message.name)
    message.description !== undefined && (obj.description = message.description)
    return obj
  },

  create<I extends Exact<DeepPartial<DomainInfo>, I>>(base?: I): DomainInfo {
    return DomainInfo.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<DomainInfo>, I>>(
    object: I
  ): DomainInfo {
    const message = createBaseDomainInfo()
    message.domainId = object.domainId ?? ''
    message.name = object.name ?? ''
    message.description = object.description ?? ''
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
