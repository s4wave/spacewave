/* eslint-disable */
import { Config as Config1 } from "@go/github.com/aperturerobotics/bifrost/daemon/api/api.pb.js";
import { Config as Config2 } from "@go/github.com/aperturerobotics/controllerbus/bus/api/api.pb.js";
import { Config as Config3 } from "@go/github.com/aperturerobotics/hydra/daemon/api/api.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { Config as Config4 } from "../api.pb.js";

export const protobufPackage = "forge.api.controller";

/** Config configures the GRPC API. */
export interface Config {
  /** ListenAddr is the address to listen on for connections. */
  listenAddr: string;
  /** DisableBifrostApi disables the bifrost api. */
  disableBifrostApi: boolean;
  /** BifrostApiConfig are bifrost api config options. */
  bifrostApiConfig:
    | Config1
    | undefined;
  /** DisableBusApi disables the bus api. */
  disableBusApi: boolean;
  /** BusApiConfig are controller-bus bus api config options. */
  busApiConfig:
    | Config2
    | undefined;
  /** DisableHydraApi disables the hydra api. */
  disableHydraApi: boolean;
  /** HydraApiConfig are hydra api options. */
  hydraApiConfig:
    | Config3
    | undefined;
  /** DisableForgeApi disables the forge api. */
  disableForgeApi: boolean;
  /** ForgeApiConfig are forge api options. */
  forgeApiConfig: Config4 | undefined;
}

function createBaseConfig(): Config {
  return {
    listenAddr: "",
    disableBifrostApi: false,
    bifrostApiConfig: undefined,
    disableBusApi: false,
    busApiConfig: undefined,
    disableHydraApi: false,
    hydraApiConfig: undefined,
    disableForgeApi: false,
    forgeApiConfig: undefined,
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.listenAddr !== "") {
      writer.uint32(10).string(message.listenAddr);
    }
    if (message.disableBifrostApi === true) {
      writer.uint32(16).bool(message.disableBifrostApi);
    }
    if (message.bifrostApiConfig !== undefined) {
      Config1.encode(message.bifrostApiConfig, writer.uint32(26).fork()).ldelim();
    }
    if (message.disableBusApi === true) {
      writer.uint32(32).bool(message.disableBusApi);
    }
    if (message.busApiConfig !== undefined) {
      Config2.encode(message.busApiConfig, writer.uint32(42).fork()).ldelim();
    }
    if (message.disableHydraApi === true) {
      writer.uint32(56).bool(message.disableHydraApi);
    }
    if (message.hydraApiConfig !== undefined) {
      Config3.encode(message.hydraApiConfig, writer.uint32(50).fork()).ldelim();
    }
    if (message.disableForgeApi === true) {
      writer.uint32(64).bool(message.disableForgeApi);
    }
    if (message.forgeApiConfig !== undefined) {
      Config4.encode(message.forgeApiConfig, writer.uint32(74).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.listenAddr = reader.string();
          break;
        case 2:
          message.disableBifrostApi = reader.bool();
          break;
        case 3:
          message.bifrostApiConfig = Config1.decode(reader, reader.uint32());
          break;
        case 4:
          message.disableBusApi = reader.bool();
          break;
        case 5:
          message.busApiConfig = Config2.decode(reader, reader.uint32());
          break;
        case 7:
          message.disableHydraApi = reader.bool();
          break;
        case 6:
          message.hydraApiConfig = Config3.decode(reader, reader.uint32());
          break;
        case 8:
          message.disableForgeApi = reader.bool();
          break;
        case 9:
          message.forgeApiConfig = Config4.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()];
        }
      } else {
        yield* [Config.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)];
        }
      } else {
        yield* [Config.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      listenAddr: isSet(object.listenAddr) ? String(object.listenAddr) : "",
      disableBifrostApi: isSet(object.disableBifrostApi) ? Boolean(object.disableBifrostApi) : false,
      bifrostApiConfig: isSet(object.bifrostApiConfig) ? Config1.fromJSON(object.bifrostApiConfig) : undefined,
      disableBusApi: isSet(object.disableBusApi) ? Boolean(object.disableBusApi) : false,
      busApiConfig: isSet(object.busApiConfig) ? Config2.fromJSON(object.busApiConfig) : undefined,
      disableHydraApi: isSet(object.disableHydraApi) ? Boolean(object.disableHydraApi) : false,
      hydraApiConfig: isSet(object.hydraApiConfig) ? Config3.fromJSON(object.hydraApiConfig) : undefined,
      disableForgeApi: isSet(object.disableForgeApi) ? Boolean(object.disableForgeApi) : false,
      forgeApiConfig: isSet(object.forgeApiConfig) ? Config4.fromJSON(object.forgeApiConfig) : undefined,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.listenAddr !== undefined && (obj.listenAddr = message.listenAddr);
    message.disableBifrostApi !== undefined && (obj.disableBifrostApi = message.disableBifrostApi);
    message.bifrostApiConfig !== undefined &&
      (obj.bifrostApiConfig = message.bifrostApiConfig ? Config1.toJSON(message.bifrostApiConfig) : undefined);
    message.disableBusApi !== undefined && (obj.disableBusApi = message.disableBusApi);
    message.busApiConfig !== undefined &&
      (obj.busApiConfig = message.busApiConfig ? Config2.toJSON(message.busApiConfig) : undefined);
    message.disableHydraApi !== undefined && (obj.disableHydraApi = message.disableHydraApi);
    message.hydraApiConfig !== undefined &&
      (obj.hydraApiConfig = message.hydraApiConfig ? Config3.toJSON(message.hydraApiConfig) : undefined);
    message.disableForgeApi !== undefined && (obj.disableForgeApi = message.disableForgeApi);
    message.forgeApiConfig !== undefined &&
      (obj.forgeApiConfig = message.forgeApiConfig ? Config4.toJSON(message.forgeApiConfig) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.listenAddr = object.listenAddr ?? "";
    message.disableBifrostApi = object.disableBifrostApi ?? false;
    message.bifrostApiConfig = (object.bifrostApiConfig !== undefined && object.bifrostApiConfig !== null)
      ? Config1.fromPartial(object.bifrostApiConfig)
      : undefined;
    message.disableBusApi = object.disableBusApi ?? false;
    message.busApiConfig = (object.busApiConfig !== undefined && object.busApiConfig !== null)
      ? Config2.fromPartial(object.busApiConfig)
      : undefined;
    message.disableHydraApi = object.disableHydraApi ?? false;
    message.hydraApiConfig = (object.hydraApiConfig !== undefined && object.hydraApiConfig !== null)
      ? Config3.fromPartial(object.hydraApiConfig)
      : undefined;
    message.disableForgeApi = object.disableForgeApi ?? false;
    message.forgeApiConfig = (object.forgeApiConfig !== undefined && object.forgeApiConfig !== null)
      ? Config4.fromPartial(object.forgeApiConfig)
      : undefined;
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends Long ? string | number | Long : T extends Array<infer U> ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string } ? { [K in keyof Omit<T, "$case">]?: DeepPartial<T[K]> } & { $case: T["$case"] }
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any;
  _m0.configure();
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
