/* eslint-disable */
import { Config as Config1 } from '@go/github.com/aperturerobotics/bifrost/daemon/api/api.pb.js'
import { Config as Config2 } from '@go/github.com/aperturerobotics/controllerbus/bus/api/api.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config3 } from '../api.pb.js'

export const protobufPackage = 'hydra.api.controller'

/** Config configures the RPC API. */
export interface Config {
  /** ListenAddr is the address to listen on for connections. */
  listenAddr: string
  /** DisableBifrostApi disables the bifrost api. */
  disableBifrostApi: boolean
  /** BifrostApiConfig are bifrost api config options. */
  bifrostApiConfig: Config1 | undefined
  /** DisableBusApi disables the bus api. */
  disableBusApi: boolean
  /** BusApiConfig are controller-bus bus api config options. */
  busApiConfig: Config2 | undefined
  /** HydraApiConfig is hydra api configuration. */
  hydraApiConfig: Config3 | undefined
}

function createBaseConfig(): Config {
  return {
    listenAddr: '',
    disableBifrostApi: false,
    bifrostApiConfig: undefined,
    disableBusApi: false,
    busApiConfig: undefined,
    hydraApiConfig: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.listenAddr !== '') {
      writer.uint32(10).string(message.listenAddr)
    }
    if (message.disableBifrostApi === true) {
      writer.uint32(16).bool(message.disableBifrostApi)
    }
    if (message.bifrostApiConfig !== undefined) {
      Config1.encode(
        message.bifrostApiConfig,
        writer.uint32(26).fork(),
      ).ldelim()
    }
    if (message.disableBusApi === true) {
      writer.uint32(32).bool(message.disableBusApi)
    }
    if (message.busApiConfig !== undefined) {
      Config2.encode(message.busApiConfig, writer.uint32(42).fork()).ldelim()
    }
    if (message.hydraApiConfig !== undefined) {
      Config3.encode(message.hydraApiConfig, writer.uint32(50).fork()).ldelim()
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

          message.listenAddr = reader.string()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.disableBifrostApi = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.bifrostApiConfig = Config1.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.disableBusApi = reader.bool()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.busApiConfig = Config2.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.hydraApiConfig = Config3.decode(reader, reader.uint32())
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
      listenAddr: isSet(object.listenAddr)
        ? globalThis.String(object.listenAddr)
        : '',
      disableBifrostApi: isSet(object.disableBifrostApi)
        ? globalThis.Boolean(object.disableBifrostApi)
        : false,
      bifrostApiConfig: isSet(object.bifrostApiConfig)
        ? Config1.fromJSON(object.bifrostApiConfig)
        : undefined,
      disableBusApi: isSet(object.disableBusApi)
        ? globalThis.Boolean(object.disableBusApi)
        : false,
      busApiConfig: isSet(object.busApiConfig)
        ? Config2.fromJSON(object.busApiConfig)
        : undefined,
      hydraApiConfig: isSet(object.hydraApiConfig)
        ? Config3.fromJSON(object.hydraApiConfig)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.listenAddr !== '') {
      obj.listenAddr = message.listenAddr
    }
    if (message.disableBifrostApi === true) {
      obj.disableBifrostApi = message.disableBifrostApi
    }
    if (message.bifrostApiConfig !== undefined) {
      obj.bifrostApiConfig = Config1.toJSON(message.bifrostApiConfig)
    }
    if (message.disableBusApi === true) {
      obj.disableBusApi = message.disableBusApi
    }
    if (message.busApiConfig !== undefined) {
      obj.busApiConfig = Config2.toJSON(message.busApiConfig)
    }
    if (message.hydraApiConfig !== undefined) {
      obj.hydraApiConfig = Config3.toJSON(message.hydraApiConfig)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.listenAddr = object.listenAddr ?? ''
    message.disableBifrostApi = object.disableBifrostApi ?? false
    message.bifrostApiConfig =
      object.bifrostApiConfig !== undefined && object.bifrostApiConfig !== null
        ? Config1.fromPartial(object.bifrostApiConfig)
        : undefined
    message.disableBusApi = object.disableBusApi ?? false
    message.busApiConfig =
      object.busApiConfig !== undefined && object.busApiConfig !== null
        ? Config2.fromPartial(object.busApiConfig)
        : undefined
    message.hydraApiConfig =
      object.hydraApiConfig !== undefined && object.hydraApiConfig !== null
        ? Config3.fromPartial(object.hydraApiConfig)
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
