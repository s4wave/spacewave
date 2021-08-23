/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

export const protobufPackage = 'browser'

/** Config is the configuration for the browser controller. */
export interface Config {
  /**
   * RuntimeId is the unique ID of this runtime.
   *
   * must be set
   * used to determine the broadcast channel ids
   * determined by the webpage that started the worker
   */
  runtimeId: string
}

const baseConfig: object = { runtimeId: '' }

export const Config = {
  encode(message: Config, writer: Writer = Writer.create()): Writer {
    if (message.runtimeId !== '') {
      writer.uint32(10).string(message.runtimeId)
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
          message.runtimeId = reader.string()
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
    if (object.runtimeId !== undefined && object.runtimeId !== null) {
      message.runtimeId = String(object.runtimeId)
    } else {
      message.runtimeId = ''
    }
    return message
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.runtimeId !== undefined && (obj.runtimeId = message.runtimeId)
    return obj
  },

  fromPartial(object: DeepPartial<Config>): Config {
    const message = { ...baseConfig } as Config
    if (object.runtimeId !== undefined && object.runtimeId !== null) {
      message.runtimeId = object.runtimeId
    } else {
      message.runtimeId = ''
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
