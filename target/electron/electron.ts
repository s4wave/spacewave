/* eslint-disable */
import Long from 'long'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'electron'

/** Config is the configuration for the electron runtime. */
export interface Config {
  /** ElectronPath is the path to the electron runtime. */
  electronPath: string
  /** RendererPath is the path to the renderer. */
  rendererPath: string
  /**
   * StoragePath is the path to store data in.
   * If unset, uses the user's config dir.
   */
  storagePath: string
}

function createBaseConfig(): Config {
  return { electronPath: '', rendererPath: '', storagePath: '' }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.electronPath !== '') {
      writer.uint32(10).string(message.electronPath)
    }
    if (message.rendererPath !== '') {
      writer.uint32(18).string(message.rendererPath)
    }
    if (message.storagePath !== '') {
      writer.uint32(26).string(message.storagePath)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.electronPath = reader.string()
          break
        case 2:
          message.rendererPath = reader.string()
          break
        case 3:
          message.storagePath = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): Config {
    return {
      electronPath: isSet(object.electronPath)
        ? String(object.electronPath)
        : '',
      rendererPath: isSet(object.rendererPath)
        ? String(object.rendererPath)
        : '',
      storagePath: isSet(object.storagePath) ? String(object.storagePath) : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.electronPath !== undefined &&
      (obj.electronPath = message.electronPath)
    message.rendererPath !== undefined &&
      (obj.rendererPath = message.rendererPath)
    message.storagePath !== undefined && (obj.storagePath = message.storagePath)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.electronPath = object.electronPath ?? ''
    message.rendererPath = object.rendererPath ?? ''
    message.storagePath = object.storagePath ?? ''
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
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & Record<
        Exclude<keyof I, KeysOfUnion<P>>,
        never
      >

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
