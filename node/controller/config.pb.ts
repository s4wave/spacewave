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
    writer: _m0.Writer = _m0.Writer.create(),
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
        writer.uint32(26).fork(),
      ).ldelim()
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
          if (tag !== 8) {
            break
          }

          message.disableLookup = reader.bool()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.disableDefaultLookup = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.defaultLookup = ControllerConfig.decode(
            reader,
            reader.uint32(),
          )
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
      disableLookup: isSet(object.disableLookup)
        ? globalThis.Boolean(object.disableLookup)
        : false,
      disableDefaultLookup: isSet(object.disableDefaultLookup)
        ? globalThis.Boolean(object.disableDefaultLookup)
        : false,
      defaultLookup: isSet(object.defaultLookup)
        ? ControllerConfig.fromJSON(object.defaultLookup)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.disableLookup === true) {
      obj.disableLookup = message.disableLookup
    }
    if (message.disableDefaultLookup === true) {
      obj.disableDefaultLookup = message.disableDefaultLookup
    }
    if (message.defaultLookup !== undefined) {
      obj.defaultLookup = ControllerConfig.toJSON(message.defaultLookup)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
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
