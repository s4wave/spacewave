/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'electron'

/** Config is the configuration for the electron runtime. */
export interface Config {
  /** ElectronPath is the path to the electron binary. */
  electronPath: string
  /**
   * WorkdirPath is the path to the working directory to use.
   * If unset, defaults to the current working directory of the process.
   */
  workdirPath: string
  /**
   * RendererPath is the path to the renderer bundle.
   * Must be one of the accepted Electron path types.
   * Ex: http://, file://, path to directory, path to index.js
   * Relative paths must be relative to workdir_path.
   */
  rendererPath: string
  /**
   * WebRuntimeId is the value to use for the runtime uuid.
   * Used for the Unix pipe paths and for the BroadcastChannel ids.
   * Should be unique against other running Electron instances.
   */
  webRuntimeId: string
  /** ElectronFlags are additional flags to pass to electron. */
  electronFlags: string[]
}

function createBaseConfig(): Config {
  return {
    electronPath: '',
    workdirPath: '',
    rendererPath: '',
    webRuntimeId: '',
    electronFlags: [],
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.electronPath !== '') {
      writer.uint32(10).string(message.electronPath)
    }
    if (message.workdirPath !== '') {
      writer.uint32(42).string(message.workdirPath)
    }
    if (message.rendererPath !== '') {
      writer.uint32(18).string(message.rendererPath)
    }
    if (message.webRuntimeId !== '') {
      writer.uint32(26).string(message.webRuntimeId)
    }
    for (const v of message.electronFlags) {
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

          message.electronPath = reader.string()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.workdirPath = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.rendererPath = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.webRuntimeId = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.electronFlags.push(reader.string())
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
      electronPath: isSet(object.electronPath)
        ? String(object.electronPath)
        : '',
      workdirPath: isSet(object.workdirPath) ? String(object.workdirPath) : '',
      rendererPath: isSet(object.rendererPath)
        ? String(object.rendererPath)
        : '',
      webRuntimeId: isSet(object.webRuntimeId)
        ? String(object.webRuntimeId)
        : '',
      electronFlags: Array.isArray(object?.electronFlags)
        ? object.electronFlags.map((e: any) => String(e))
        : [],
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.electronPath !== '') {
      obj.electronPath = message.electronPath
    }
    if (message.workdirPath !== '') {
      obj.workdirPath = message.workdirPath
    }
    if (message.rendererPath !== '') {
      obj.rendererPath = message.rendererPath
    }
    if (message.webRuntimeId !== '') {
      obj.webRuntimeId = message.webRuntimeId
    }
    if (message.electronFlags?.length) {
      obj.electronFlags = message.electronFlags
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.electronPath = object.electronPath ?? ''
    message.workdirPath = object.workdirPath ?? ''
    message.rendererPath = object.rendererPath ?? ''
    message.webRuntimeId = object.webRuntimeId ?? ''
    message.electronFlags = object.electronFlags?.map((e) => e) || []
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
