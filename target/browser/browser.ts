/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

export const protobufPackage = 'browser'

/** Config is the configuration for the browser controller. */
export interface Config {}

const baseConfig: object = {}

export const Config = {
  encode(_: Config, writer: Writer = Writer.create()): Writer {
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseConfig } as Config
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(_: any): Config {
    const message = { ...baseConfig } as Config
    return message
  },

  toJSON(_: Config): unknown {
    const obj: any = {}
    return obj
  },

  fromPartial(_: DeepPartial<Config>): Config {
    const message = { ...baseConfig } as Config
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
