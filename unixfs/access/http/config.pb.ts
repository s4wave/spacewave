/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'unixfs.access.http'

/**
 * Config configures the HTTP UnixFS Access controller.
 *
 * If either match_path_prefixes or path_re matches, the URL will be handled.
 */
export interface Config {
  /**
   * UnixfsId is the identifier for the UnixFS on the bus.
   * The fs should be provided with the AccessUnixFS controller.
   */
  unixfsId: string
  /**
   * UnixfsPrefix is a path prefix to apply to paths in the FS.
   * This applies a chroot to the UnixFS.
   */
  unixfsPrefix: string
  /**
   * UnixfsHttpPrefix is a HTTP prefix to match & strip from requests.
   * Note: this is not related to match_path_prefixes.
   */
  unixfsHttpPrefix: string
  /**
   * NotFoundIfIdle returns 404 not found if the handler lookup becomes idle.
   * Lookup becomes idle if no handler is available for the URL.
   * If unset, waits until a handler is available.
   */
  notFoundIfIdle: boolean
  /**
   * MatchPathPrefixes is the list of URL path prefixes to match.
   * Can be empty to match all.
   */
  matchPathPrefixes: string[]
  /**
   * StripPathPrefix enables removing the matched path prefix from the URL path.
   * The first path prefix in match_path_prefixes to match will be removed.
   * If match_path_prefixes is empty, has no effect.
   */
  stripPathPrefix: boolean
  /**
   * PathRe is a url path regex to match URL paths with.
   * If unset, uses match_path_prefixes.
   */
  pathRe: string
}

function createBaseConfig(): Config {
  return {
    unixfsId: '',
    unixfsPrefix: '',
    unixfsHttpPrefix: '',
    notFoundIfIdle: false,
    matchPathPrefixes: [],
    stripPathPrefix: false,
    pathRe: '',
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsId !== '') {
      writer.uint32(10).string(message.unixfsId)
    }
    if (message.unixfsPrefix !== '') {
      writer.uint32(18).string(message.unixfsPrefix)
    }
    if (message.unixfsHttpPrefix !== '') {
      writer.uint32(26).string(message.unixfsHttpPrefix)
    }
    if (message.notFoundIfIdle !== false) {
      writer.uint32(32).bool(message.notFoundIfIdle)
    }
    for (const v of message.matchPathPrefixes) {
      writer.uint32(42).string(v!)
    }
    if (message.stripPathPrefix !== false) {
      writer.uint32(48).bool(message.stripPathPrefix)
    }
    if (message.pathRe !== '') {
      writer.uint32(58).string(message.pathRe)
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

          message.unixfsId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.unixfsPrefix = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.unixfsHttpPrefix = reader.string()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.notFoundIfIdle = reader.bool()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.matchPathPrefixes.push(reader.string())
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.stripPathPrefix = reader.bool()
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.pathRe = reader.string()
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
      unixfsId: isSet(object.unixfsId)
        ? globalThis.String(object.unixfsId)
        : '',
      unixfsPrefix: isSet(object.unixfsPrefix)
        ? globalThis.String(object.unixfsPrefix)
        : '',
      unixfsHttpPrefix: isSet(object.unixfsHttpPrefix)
        ? globalThis.String(object.unixfsHttpPrefix)
        : '',
      notFoundIfIdle: isSet(object.notFoundIfIdle)
        ? globalThis.Boolean(object.notFoundIfIdle)
        : false,
      matchPathPrefixes: globalThis.Array.isArray(object?.matchPathPrefixes)
        ? object.matchPathPrefixes.map((e: any) => globalThis.String(e))
        : [],
      stripPathPrefix: isSet(object.stripPathPrefix)
        ? globalThis.Boolean(object.stripPathPrefix)
        : false,
      pathRe: isSet(object.pathRe) ? globalThis.String(object.pathRe) : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.unixfsId !== '') {
      obj.unixfsId = message.unixfsId
    }
    if (message.unixfsPrefix !== '') {
      obj.unixfsPrefix = message.unixfsPrefix
    }
    if (message.unixfsHttpPrefix !== '') {
      obj.unixfsHttpPrefix = message.unixfsHttpPrefix
    }
    if (message.notFoundIfIdle !== false) {
      obj.notFoundIfIdle = message.notFoundIfIdle
    }
    if (message.matchPathPrefixes?.length) {
      obj.matchPathPrefixes = message.matchPathPrefixes
    }
    if (message.stripPathPrefix !== false) {
      obj.stripPathPrefix = message.stripPathPrefix
    }
    if (message.pathRe !== '') {
      obj.pathRe = message.pathRe
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.unixfsId = object.unixfsId ?? ''
    message.unixfsPrefix = object.unixfsPrefix ?? ''
    message.unixfsHttpPrefix = object.unixfsHttpPrefix ?? ''
    message.notFoundIfIdle = object.notFoundIfIdle ?? false
    message.matchPathPrefixes = object.matchPathPrefixes?.map((e) => e) || []
    message.stripPathPrefix = object.stripPathPrefix ?? false
    message.pathRe = object.pathRe ?? ''
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
