/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.pkg'

/** WebPkgInfo is information about a WebPkg. */
export interface WebPkgInfo {
  /**
   * Id is the web package identifier.
   * Usually matches the npm package name.
   */
  id: string
  /**
   * Version is the web package version.
   * Usually matches the npm package version.
   * Note: this is sometimes (not always) semver format.
   */
  version: string
}

function createBaseWebPkgInfo(): WebPkgInfo {
  return { id: '', version: '' }
}

export const WebPkgInfo = {
  encode(
    message: WebPkgInfo,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.version !== '') {
      writer.uint32(18).string(message.version)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebPkgInfo {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebPkgInfo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.id = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.version = reader.string()
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
  // Transform<WebPkgInfo, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebPkgInfo | WebPkgInfo[]>
      | Iterable<WebPkgInfo | WebPkgInfo[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebPkgInfo.encode(p).finish()]
        }
      } else {
        yield* [WebPkgInfo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebPkgInfo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebPkgInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebPkgInfo.decode(p)]
        }
      } else {
        yield* [WebPkgInfo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): WebPkgInfo {
    return {
      id: isSet(object.id) ? String(object.id) : '',
      version: isSet(object.version) ? String(object.version) : '',
    }
  },

  toJSON(message: WebPkgInfo): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    if (message.version !== '') {
      obj.version = message.version
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WebPkgInfo>, I>>(base?: I): WebPkgInfo {
    return WebPkgInfo.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WebPkgInfo>, I>>(
    object: I,
  ): WebPkgInfo {
    const message = createBaseWebPkgInfo()
    message.id = object.id ?? ''
    message.version = object.version ?? ''
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
