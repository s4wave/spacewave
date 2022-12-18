/* eslint-disable */
import { ControllerConfig } from '@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'node.controller'

/** Config is the node controller config. */
export interface Config {
  /** DisableLookup disables lookup processing entirely. */
  disableLookup: boolean
  /**
   * DisableDefaultLookup disables the default lookup controller.
   * If a controller is defined in a bucket config this has no effect.
   */
  disableDefaultLookup: boolean
  /**
   * DefaultLookup overrides the default lookup controller.
   * If this is empty, uses the hard-coded default controller.
   * If DisableDefaultLookup is set, this has no effect.
   * If a controller is defined in a bucket config this has no effect.
   */
  defaultLookup: ControllerConfig | undefined
}

function createBaseConfig(): Config {
  return {
    disableLookup: false,
    disableDefaultLookup: false,
    defaultLookup: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.disableLookup === true) {
      writer.uint32(8).bool(message.disableLookup)
    }
    if (message.disableDefaultLookup === true) {
      writer.uint32(16).bool(message.disableDefaultLookup)
    }
    if (message.defaultLookup !== undefined) {
      ControllerConfig.encode(
        message.defaultLookup,
        writer.uint32(26).fork()
      ).ldelim()
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
          message.disableLookup = reader.bool()
          break
        case 2:
          message.disableDefaultLookup = reader.bool()
          break
        case 3:
          message.defaultLookup = ControllerConfig.decode(
            reader,
            reader.uint32()
          )
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>
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
      | Iterable<Uint8Array | Uint8Array[]>
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
      disableLookup: isSet(object.disableLookup)
        ? Boolean(object.disableLookup)
        : false,
      disableDefaultLookup: isSet(object.disableDefaultLookup)
        ? Boolean(object.disableDefaultLookup)
        : false,
      defaultLookup: isSet(object.defaultLookup)
        ? ControllerConfig.fromJSON(object.defaultLookup)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.disableLookup !== undefined &&
      (obj.disableLookup = message.disableLookup)
    message.disableDefaultLookup !== undefined &&
      (obj.disableDefaultLookup = message.disableDefaultLookup)
    message.defaultLookup !== undefined &&
      (obj.defaultLookup = message.defaultLookup
        ? ControllerConfig.toJSON(message.defaultLookup)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.disableLookup = object.disableLookup ?? false
    message.disableDefaultLookup = object.disableDefaultLookup ?? false
    message.defaultLookup =
      object.defaultLookup !== undefined && object.defaultLookup !== null
        ? ControllerConfig.fromPartial(object.defaultLookup)
        : undefined
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
