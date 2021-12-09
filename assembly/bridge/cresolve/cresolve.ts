/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

export const protobufPackage = 'bridge.cresolve'

/** Config configures the controller factory resolver directive bridge. */
export interface Config {
  /**
   * ConfigIdRe filters the controllers resolved using a regex on the config id.
   * If empty, allows any config to be resolved.
   */
  configIdRe: string
}

const baseConfig: object = { configIdRe: '' }

export const Config = {
  encode(message: Config, writer: Writer = Writer.create()): Writer {
    if (message.configIdRe !== '') {
      writer.uint32(10).string(message.configIdRe)
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
          message.configIdRe = reader.string()
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
    if (object.configIdRe !== undefined && object.configIdRe !== null) {
      message.configIdRe = String(object.configIdRe)
    } else {
      message.configIdRe = ''
    }
    return message
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.configIdRe !== undefined && (obj.configIdRe = message.configIdRe)
    return obj
  },

  fromPartial(object: DeepPartial<Config>): Config {
    const message = { ...baseConfig } as Config
    if (object.configIdRe !== undefined && object.configIdRe !== null) {
      message.configIdRe = object.configIdRe
    } else {
      message.configIdRe = ''
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
