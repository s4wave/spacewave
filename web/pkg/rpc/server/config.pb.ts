/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.pkg.rpc.server'

/**
 * Config configures the web pkg rpc server.
 * Provides the AccessWebPkg RPC service.
 * Resolves LookupRpcService.
 * Provides one or more services according to the pkg ids:
 *  - web.pkg.rpc.AccessWebPkg/@aperturerobotics/util/ -> LookupWebPkg<@aperturerobotics/util>
 *  - web.pkg.rpc.AccessWebPkg/react -> LookupWebPkg<react>
 */
export interface Config {
  /**
   * ServiceIdPrefix is the service id prefix to listen on.
   * If empty, defaults to web.pkg.rpc.AccessWebPkg.
   */
  serviceIdPrefix: string
  /**
   * WebPkgIdRe is a regex string to match web pkgs IDs.
   * Set to '.*' or empty to match all web pkgs ids.
   */
  webPkgIdRe: string
  /**
   * WebPkgIdPrefixes is a list of web pkg id prefixes to match.
   * If the value is in this list, overrides web_pkg_id_re.
   * Set to '.*' or empty to match all web pkgs ids.
   */
  webPkgIdPrefixes: string[]
  /**
   * WebPkgIdList is a list of web pkg IDs to resolve.
   * If the value is in this list, overrides web_pkg_id_re and web_pkg_id_prefixes.
   * Ignored if empty.
   */
  webPkgIdList: string[]
  /**
   * ReleaseDelay is a delay duration to wait before releasing a unreferenced web pkg.
   * If empty string, defaults to 1s (1 second).
   */
  releaseDelay: string
}

function createBaseConfig(): Config {
  return {
    serviceIdPrefix: '',
    webPkgIdRe: '',
    webPkgIdPrefixes: [],
    webPkgIdList: [],
    releaseDelay: '',
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.serviceIdPrefix !== '') {
      writer.uint32(10).string(message.serviceIdPrefix)
    }
    if (message.webPkgIdRe !== '') {
      writer.uint32(18).string(message.webPkgIdRe)
    }
    for (const v of message.webPkgIdPrefixes) {
      writer.uint32(26).string(v!)
    }
    for (const v of message.webPkgIdList) {
      writer.uint32(34).string(v!)
    }
    if (message.releaseDelay !== '') {
      writer.uint32(42).string(message.releaseDelay)
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

          message.serviceIdPrefix = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.webPkgIdRe = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.webPkgIdPrefixes.push(reader.string())
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.webPkgIdList.push(reader.string())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.releaseDelay = reader.string()
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
      serviceIdPrefix: isSet(object.serviceIdPrefix)
        ? globalThis.String(object.serviceIdPrefix)
        : '',
      webPkgIdRe: isSet(object.webPkgIdRe)
        ? globalThis.String(object.webPkgIdRe)
        : '',
      webPkgIdPrefixes: globalThis.Array.isArray(object?.webPkgIdPrefixes)
        ? object.webPkgIdPrefixes.map((e: any) => globalThis.String(e))
        : [],
      webPkgIdList: globalThis.Array.isArray(object?.webPkgIdList)
        ? object.webPkgIdList.map((e: any) => globalThis.String(e))
        : [],
      releaseDelay: isSet(object.releaseDelay)
        ? globalThis.String(object.releaseDelay)
        : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.serviceIdPrefix !== '') {
      obj.serviceIdPrefix = message.serviceIdPrefix
    }
    if (message.webPkgIdRe !== '') {
      obj.webPkgIdRe = message.webPkgIdRe
    }
    if (message.webPkgIdPrefixes?.length) {
      obj.webPkgIdPrefixes = message.webPkgIdPrefixes
    }
    if (message.webPkgIdList?.length) {
      obj.webPkgIdList = message.webPkgIdList
    }
    if (message.releaseDelay !== '') {
      obj.releaseDelay = message.releaseDelay
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.serviceIdPrefix = object.serviceIdPrefix ?? ''
    message.webPkgIdRe = object.webPkgIdRe ?? ''
    message.webPkgIdPrefixes = object.webPkgIdPrefixes?.map((e) => e) || []
    message.webPkgIdList = object.webPkgIdList?.map((e) => e) || []
    message.releaseDelay = object.releaseDelay ?? ''
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
