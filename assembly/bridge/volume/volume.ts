/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

export const protobufPackage = 'bridge.volume'

/** Config configures the hydra volume directive bridge. */
export interface Config {
  /** VolumeId is the volume id to forward requests to on parent bus. */
  volumeId: string
}

const baseConfig: object = { volumeId: '' }

export const Config = {
  encode(message: Config, writer: Writer = Writer.create()): Writer {
    if (message.volumeId !== '') {
      writer.uint32(10).string(message.volumeId)
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
          message.volumeId = reader.string()
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
    if (object.volumeId !== undefined && object.volumeId !== null) {
      message.volumeId = String(object.volumeId)
    } else {
      message.volumeId = ''
    }
    return message
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.volumeId !== undefined && (obj.volumeId = message.volumeId)
    return obj
  },

  fromPartial(object: DeepPartial<Config>): Config {
    const message = { ...baseConfig } as Config
    if (object.volumeId !== undefined && object.volumeId !== null) {
      message.volumeId = object.volumeId
    } else {
      message.volumeId = ''
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
