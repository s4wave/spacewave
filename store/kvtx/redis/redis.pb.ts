/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'store.kvtx.redis'

/** ClientConfig configures a redis client. */
export interface ClientConfig {
  /** Url is the redis:// url to connect to. */
  url: string
}

function createBaseClientConfig(): ClientConfig {
  return { url: '' }
}

export const ClientConfig = {
  encode(
    message: ClientConfig,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.url !== '') {
      writer.uint32(10).string(message.url)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ClientConfig {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClientConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.url = reader.string()
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
  // Transform<ClientConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClientConfig | ClientConfig[]>
      | Iterable<ClientConfig | ClientConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ClientConfig.encode(p).finish()]
        }
      } else {
        yield* [ClientConfig.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClientConfig>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ClientConfig> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ClientConfig.decode(p)]
        }
      } else {
        yield* [ClientConfig.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ClientConfig {
    return { url: isSet(object.url) ? globalThis.String(object.url) : '' }
  },

  toJSON(message: ClientConfig): unknown {
    const obj: any = {}
    if (message.url !== '') {
      obj.url = message.url
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ClientConfig>, I>>(
    base?: I,
  ): ClientConfig {
    return ClientConfig.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ClientConfig>, I>>(
    object: I,
  ): ClientConfig {
    const message = createBaseClientConfig()
    message.url = object.url ?? ''
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
