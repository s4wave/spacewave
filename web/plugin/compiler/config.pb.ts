/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bldr.web.plugin.compiler'

/** Config configures the web plugin builder. */
export interface Config {
  /** ProjectId overrides the project id set in the project config. */
  projectId: string
  /**
   * DelveAddr is the address to listen for Delve remote connections.
   * If the build mode is dev and this is set, uses delve to run the plugin.
   * Ignored if build mode is not dev or build platform is not "native".
   * Special value: "wait" - waits for plugin entrypoint to be run manually.
   */
  delveAddr: string
  /**
   * ElectronPkg is the name and version of the npm package to use for electron.
   * If unset, defaults to the version in package.json.
   * If not found, defaults to electron@latest.
   * Ignored if build platform is not "native".
   */
  electronPkg: string
}

function createBaseConfig(): Config {
  return { projectId: '', delveAddr: '', electronPkg: '' }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.projectId !== '') {
      writer.uint32(10).string(message.projectId)
    }
    if (message.delveAddr !== '') {
      writer.uint32(18).string(message.delveAddr)
    }
    if (message.electronPkg !== '') {
      writer.uint32(26).string(message.electronPkg)
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

          message.projectId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.delveAddr = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.electronPkg = reader.string()
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
      projectId: isSet(object.projectId)
        ? globalThis.String(object.projectId)
        : '',
      delveAddr: isSet(object.delveAddr)
        ? globalThis.String(object.delveAddr)
        : '',
      electronPkg: isSet(object.electronPkg)
        ? globalThis.String(object.electronPkg)
        : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.projectId !== '') {
      obj.projectId = message.projectId
    }
    if (message.delveAddr !== '') {
      obj.delveAddr = message.delveAddr
    }
    if (message.electronPkg !== '') {
      obj.electronPkg = message.electronPkg
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.projectId = object.projectId ?? ''
    message.delveAddr = object.delveAddr ?? ''
    message.electronPkg = object.electronPkg ?? ''
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
