/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'block.store.http.lookup'

/**
 * Config configures the http block lookup controller.
 * Serves LookupBlockFromNetwork directives by calling a http api.
 */
export interface Config {
  /** BucketId is the bucket id to serve lookups for. */
  bucketId: string
  /**
   * Url is the HTTP base URL to call the block lookup service.
   * E.x: https://myservice.local/block
   * Expects the block store http server to be served at the URL.
   * Calls will then be, for example:
   *  - GET /block/get/{block-ref}
   *  - GET /block/exists/{block-ref}
   *  - POST /block/put
   *  - DELETE /block/rm
   * Must be set and a valid http or https url.
   */
  url: string
  /** SkipNotFound skips returning a value if the block was not found. */
  skipNotFound: boolean
  /** Verbose enables verbose logging of the block store. */
  verbose: boolean
}

function createBaseConfig(): Config {
  return { bucketId: '', url: '', skipNotFound: false, verbose: false }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.bucketId !== '') {
      writer.uint32(10).string(message.bucketId)
    }
    if (message.url !== '') {
      writer.uint32(18).string(message.url)
    }
    if (message.skipNotFound === true) {
      writer.uint32(24).bool(message.skipNotFound)
    }
    if (message.verbose === true) {
      writer.uint32(32).bool(message.verbose)
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

          message.bucketId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.url = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.skipNotFound = reader.bool()
          continue
        case 4:
          if (tag !== 32) {
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
      | Iterable<Uint8Array | Uint8Array[]>,
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
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : '',
      url: isSet(object.url) ? String(object.url) : '',
      skipNotFound: isSet(object.skipNotFound)
        ? Boolean(object.skipNotFound)
        : false,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.bucketId !== '') {
      obj.bucketId = message.bucketId
    }
    if (message.url !== '') {
      obj.url = message.url
    }
    if (message.skipNotFound === true) {
      obj.skipNotFound = message.skipNotFound
    }
    if (message.verbose === true) {
      obj.verbose = message.verbose
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.bucketId = object.bucketId ?? ''
    message.url = object.url ?? ''
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
