/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "manifest.fetch.rpc";

/**
 * Config configures a controller to fetch via the ManifestFetch service.
 * Loads a plugin with LoadPlugin and uses its RPC client.
 * Resolves the FetchManifest directive.
 */
export interface Config {
  /** ServiceId is the service id to contact with LookupRpcClient. */
  serviceId: string;
  /** ClientId is the client identifier to use to contact the rpc service. */
  clientId: string;
  /**
   * FetchManifestIdRe is the regex of manifest IDs to fetch with this controller.
   * If empty, will forward any FetchManifest directive to the service.
   */
  fetchManifestIdRe: string;
}

function createBaseConfig(): Config {
  return { serviceId: "", clientId: "", fetchManifestIdRe: "" };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.serviceId !== "") {
      writer.uint32(10).string(message.serviceId);
    }
    if (message.clientId !== "") {
      writer.uint32(18).string(message.clientId);
    }
    if (message.fetchManifestIdRe !== "") {
      writer.uint32(26).string(message.fetchManifestIdRe);
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

          message.serviceId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.clientId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.fetchManifestIdRe = reader.string();
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
      serviceId: isSet(object.serviceId) ? globalThis.String(object.serviceId) : "",
      clientId: isSet(object.clientId) ? globalThis.String(object.clientId) : "",
      fetchManifestIdRe: isSet(object.fetchManifestIdRe) ? globalThis.String(object.fetchManifestIdRe) : "",
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    if (message.serviceId !== "") {
      obj.serviceId = message.serviceId;
    }
    if (message.clientId !== "") {
      obj.clientId = message.clientId;
    }
    if (message.fetchManifestIdRe !== "") {
      obj.fetchManifestIdRe = message.fetchManifestIdRe;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.serviceId = object.serviceId ?? "";
    message.clientId = object.clientId ?? "";
    message.fetchManifestIdRe = object.fetchManifestIdRe ?? "";
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
