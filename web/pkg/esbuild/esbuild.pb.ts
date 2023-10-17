/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.pkg.esbuild'

/** WebPkgRef contains information about references to a web pkg. */
export interface WebPkgRef {
  /** WebPkgId is the web pkg identifier. */
  webPkgId: string
  /** WebPkgRoot is the path to the web pkg root dir relative to project root. */
  webPkgRoot: string
  /** Imports is the list of paths that were imported from the web pkg. */
  imports: string[]
  /**
   * CrossRefs is the list of other web pkgs that this pkg imports.
   * NOTE: this is not filled unless ResolveWebPkgRefsEsbuild is called.
   */
  crossRefs: string[]
}

function createBaseWebPkgRef(): WebPkgRef {
  return { webPkgId: '', webPkgRoot: '', imports: [], crossRefs: [] }
}

export const WebPkgRef = {
  encode(
    message: WebPkgRef,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.webPkgId !== '') {
      writer.uint32(10).string(message.webPkgId)
    }
    if (message.webPkgRoot !== '') {
      writer.uint32(18).string(message.webPkgRoot)
    }
    for (const v of message.imports) {
      writer.uint32(26).string(v!)
    }
    for (const v of message.crossRefs) {
      writer.uint32(34).string(v!)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebPkgRef {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebPkgRef()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.webPkgId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.webPkgRoot = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.imports.push(reader.string())
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.crossRefs.push(reader.string())
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
  // Transform<WebPkgRef, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebPkgRef | WebPkgRef[]>
      | Iterable<WebPkgRef | WebPkgRef[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WebPkgRef.encode(p).finish()]
        }
      } else {
        yield* [WebPkgRef.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebPkgRef>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebPkgRef> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WebPkgRef.decode(p)]
        }
      } else {
        yield* [WebPkgRef.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WebPkgRef {
    return {
      webPkgId: isSet(object.webPkgId)
        ? globalThis.String(object.webPkgId)
        : '',
      webPkgRoot: isSet(object.webPkgRoot)
        ? globalThis.String(object.webPkgRoot)
        : '',
      imports: globalThis.Array.isArray(object?.imports)
        ? object.imports.map((e: any) => globalThis.String(e))
        : [],
      crossRefs: globalThis.Array.isArray(object?.crossRefs)
        ? object.crossRefs.map((e: any) => globalThis.String(e))
        : [],
    }
  },

  toJSON(message: WebPkgRef): unknown {
    const obj: any = {}
    if (message.webPkgId !== '') {
      obj.webPkgId = message.webPkgId
    }
    if (message.webPkgRoot !== '') {
      obj.webPkgRoot = message.webPkgRoot
    }
    if (message.imports?.length) {
      obj.imports = message.imports
    }
    if (message.crossRefs?.length) {
      obj.crossRefs = message.crossRefs
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WebPkgRef>, I>>(base?: I): WebPkgRef {
    return WebPkgRef.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WebPkgRef>, I>>(
    object: I,
  ): WebPkgRef {
    const message = createBaseWebPkgRef()
    message.webPkgId = object.webPkgId ?? ''
    message.webPkgRoot = object.webPkgRoot ?? ''
    message.imports = object.imports?.map((e) => e) || []
    message.crossRefs = object.crossRefs?.map((e) => e) || []
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
