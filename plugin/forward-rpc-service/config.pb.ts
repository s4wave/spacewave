/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { Backoff } from "../../../util/backoff/backoff.pb.js";

export const protobufPackage = "bldr.plugin.forward_rpc_service";

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
   * ServiceIdRe is the regex of service IDs to forward.
   * If empty, will forward any.
   */
  serviceIdRe: string;
  /**
   * ServerIdRe is the regex of server IDs to forward for.
   * If empty, will forward any.
   */
  serverIdRe: string;
  /**
   * Backoff is the backoff config for calling the RPC service.
   * If unset, defaults to reasonable defaults.
   */
  backoff: Backoff | undefined;
}

function createBaseConfig(): Config {
  return { pluginId: "", serviceIdRe: "", serverIdRe: "", backoff: undefined };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.serviceIdRe !== "") {
      writer.uint32(18).string(message.serviceIdRe);
    }
    if (message.serverIdRe !== "") {
      writer.uint32(26).string(message.serverIdRe);
    }
    if (message.backoff !== undefined) {
      Backoff.encode(message.backoff, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.pluginId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.serviceIdRe = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.serverIdRe = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.backoff = Backoff.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config.encode(p).finish()];
        }
      } else {
        yield* [Config.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config.decode(p)];
        }
      } else {
        yield* [Config.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      pluginId: isSet(object.pluginId) ? globalThis.String(object.pluginId) : "",
      serviceIdRe: isSet(object.serviceIdRe) ? globalThis.String(object.serviceIdRe) : "",
      serverIdRe: isSet(object.serverIdRe) ? globalThis.String(object.serverIdRe) : "",
      backoff: isSet(object.backoff) ? Backoff.fromJSON(object.backoff) : undefined,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    if (message.pluginId !== "") {
      obj.pluginId = message.pluginId;
    }
    if (message.serviceIdRe !== "") {
      obj.serviceIdRe = message.serviceIdRe;
    }
    if (message.serverIdRe !== "") {
      obj.serverIdRe = message.serverIdRe;
    }
    if (message.backoff !== undefined) {
      obj.backoff = Backoff.toJSON(message.backoff);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.pluginId = object.pluginId ?? "";
    message.serviceIdRe = object.serviceIdRe ?? "";
    message.serverIdRe = object.serverIdRe ?? "";
    message.backoff = (object.backoff !== undefined && object.backoff !== null)
      ? Backoff.fromPartial(object.backoff)
      : undefined;
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends Long ? string | number | Long : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
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
