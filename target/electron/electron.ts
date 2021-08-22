/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

export const protobufPackage = 'electron'

/** Config is the configuration for the electron runtime. */
export interface Config {
  /** ElectronPath is the path to the electron runtime. */
  electronPath: string
  /** RendererPath is the path to the renderer. */
  rendererPath: string
}

const baseConfig: object = { electronPath: '', rendererPath: '' }

export const Config = {
  encode(message: Config, writer: Writer = Writer.create()): Writer {
    if (message.electronPath !== '') {
      writer.uint32(10).string(message.electronPath)
    }
    if (message.rendererPath !== '') {
      writer.uint32(18).string(message.rendererPath)
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseConfig } as Config
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.electronPath = reader.string()
          break
        case 2:
          message.rendererPath = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): Config {
    const message = { ...baseConfig } as Config
    if (object.electronPath !== undefined && object.electronPath !== null) {
      message.electronPath = String(object.electronPath)
    } else {
      message.electronPath = ''
    }
    if (object.rendererPath !== undefined && object.rendererPath !== null) {
      message.rendererPath = String(object.rendererPath)
    } else {
      message.rendererPath = ''
    }
    return message
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.electronPath !== undefined &&
      (obj.electronPath = message.electronPath)
    message.rendererPath !== undefined &&
      (obj.rendererPath = message.rendererPath)
    return obj
  },

  fromPartial(object: DeepPartial<Config>): Config {
    const message = { ...baseConfig } as Config
    if (object.electronPath !== undefined && object.electronPath !== null) {
      message.electronPath = object.electronPath
    } else {
      message.electronPath = ''
    }
    if (object.rendererPath !== undefined && object.rendererPath !== null) {
      message.rendererPath = object.rendererPath
    } else {
      message.rendererPath = ''
    }
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
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

// If you get a compile-error about 'Constructor<Long> and ... have no overlap',
// add '--ts_proto_opt=esModuleInterop=true' as a flag when calling 'protoc'.
if (util.Long !== Long) {
  util.Long = Long as any
  configure()
}
