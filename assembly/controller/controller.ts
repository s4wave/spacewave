/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

export const protobufPackage = 'assembly.controller'

/** Config is the Assembly controller configuration. */
export interface Config {
  /** DisableResolver disables resolving ApplyAssembly directives. */
  disableResolver: boolean
  /** DisablePartialSuccess disables the AllowPartialSuccess flag. */
  disablePartialSuccess: boolean
}

const baseConfig: object = {
  disableResolver: false,
  disablePartialSuccess: false,
}

export const Config = {
  encode(message: Config, writer: Writer = Writer.create()): Writer {
    if (message.disableResolver === true) {
      writer.uint32(8).bool(message.disableResolver)
    }
    if (message.disablePartialSuccess === true) {
      writer.uint32(16).bool(message.disablePartialSuccess)
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
          message.disableResolver = reader.bool()
          break
        case 2:
          message.disablePartialSuccess = reader.bool()
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
    if (
      object.disableResolver !== undefined &&
      object.disableResolver !== null
    ) {
      message.disableResolver = Boolean(object.disableResolver)
    } else {
      message.disableResolver = false
    }
    if (
      object.disablePartialSuccess !== undefined &&
      object.disablePartialSuccess !== null
    ) {
      message.disablePartialSuccess = Boolean(object.disablePartialSuccess)
    } else {
      message.disablePartialSuccess = false
    }
    return message
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.disableResolver !== undefined &&
      (obj.disableResolver = message.disableResolver)
    message.disablePartialSuccess !== undefined &&
      (obj.disablePartialSuccess = message.disablePartialSuccess)
    return obj
  },

  fromPartial(object: DeepPartial<Config>): Config {
    const message = { ...baseConfig } as Config
    if (
      object.disableResolver !== undefined &&
      object.disableResolver !== null
    ) {
      message.disableResolver = object.disableResolver
    } else {
      message.disableResolver = false
    }
    if (
      object.disablePartialSuccess !== undefined &&
      object.disablePartialSuccess !== null
    ) {
      message.disablePartialSuccess = object.disablePartialSuccess
    } else {
      message.disablePartialSuccess = false
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
