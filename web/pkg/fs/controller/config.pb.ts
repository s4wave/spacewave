/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.pkg.fs.controller'

/**
 * Config configures the web pkg fs controller.
 * Looks up a UnixFS using AccessUnixFS.
 * Accesses a sub-directory of that UnixFS as a static web pkg FS.
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
   * NotFoundIfIdle returns 404 not found if the FS lookup becomes idle.
   * Lookup becomes idle if no FS is available for the URL.
   * If unset, waits until the FS is available.
   */
  notFoundIfIdle: boolean
  /**
   * WebPkgIdList is a list of web pkg IDs to resolve.
   * Ignored if empty.
   */
  webPkgIdList: string[]
}

function createBaseConfig(): Config {
  return {
    unixfsId: '',
    unixfsPrefix: '',
    notFoundIfIdle: false,
    webPkgIdList: [],
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
    if (message.notFoundIfIdle === true) {
      writer.uint32(24).bool(message.notFoundIfIdle)
    }
    for (const v of message.webPkgIdList) {
      writer.uint32(34).string(v!)
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
          if (tag !== 24) {
            break
          }

          message.notFoundIfIdle = reader.bool()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.webPkgIdList.push(reader.string())
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
      notFoundIfIdle: isSet(object.notFoundIfIdle)
        ? globalThis.Boolean(object.notFoundIfIdle)
        : false,
      webPkgIdList: globalThis.Array.isArray(object?.webPkgIdList)
        ? object.webPkgIdList.map((e: any) => globalThis.String(e))
        : [],
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
    if (message.notFoundIfIdle === true) {
      obj.notFoundIfIdle = message.notFoundIfIdle
    }
    if (message.webPkgIdList?.length) {
      obj.webPkgIdList = message.webPkgIdList
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
    message.notFoundIfIdle = object.notFoundIfIdle ?? false
    message.webPkgIdList = object.webPkgIdList?.map((e) => e) || []
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
