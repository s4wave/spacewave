/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "plugin.host.forward_rpc_service";

/**
 * Config configures forwarding rpc services to plugins.
 * Loads a plugin with LoadPlugin and uses its RPC client.
 * Calls the AccessRpcService service.
 * Resolves the LookupRpcService directive.
 */
export interface Config {
  /** PluginId is the plugin to load and use as AccessRpcService. */
  pluginId: string;
  /**
   * ServiceIdRegex is the regex of service IDs to forward.
   * If empty, will forward any.
   */
  serviceIdRegex: string;
  /**
   * ServerIdRegex is the regex of server IDs to forward for.
   * If empty, will forward any.
   */
  serverIdRegex: string;
}

function createBaseConfig(): Config {
  return { pluginId: "", serviceIdRegex: "", serverIdRegex: "" };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.serviceIdRegex !== "") {
      writer.uint32(18).string(message.serviceIdRegex);
    }
    if (message.serverIdRegex !== "") {
      writer.uint32(26).string(message.serverIdRegex);
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
          message.pluginId = reader.string();
          break;
        case 2:
          message.serviceIdRegex = reader.string();
          break;
        case 3:
          message.serverIdRegex = reader.string();
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
      pluginId: isSet(object.pluginId) ? String(object.pluginId) : "",
      serviceIdRegex: isSet(object.serviceIdRegex) ? String(object.serviceIdRegex) : "",
      serverIdRegex: isSet(object.serverIdRegex) ? String(object.serverIdRegex) : "",
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.serviceIdRegex !== undefined && (obj.serviceIdRegex = message.serviceIdRegex);
    message.serverIdRegex !== undefined && (obj.serverIdRegex = message.serverIdRegex);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.pluginId = object.pluginId ?? "";
    message.serviceIdRegex = object.serviceIdRegex ?? "";
    message.serverIdRegex = object.serverIdRegex ?? "";
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
