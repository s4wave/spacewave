/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "manifest.fetch.world";

/**
 * Config configures a controller to fetch manifests via a world engine.
 * Searches for <manifest> linked manifests to the object key.
 * Resolves the FetchManifest directive.
 */
export interface Config {
  /** EngineId is the world engine id to attach to. */
  engineId: string;
  /**
   * ObjectKeys is the list of object keys to search from for manifests.
   * Searches for <manifest> linked manifests.
   */
  objectKeys: string[];
  /**
   * FetchManifestIdRe is the regex of manifest IDs to fetch with this controller.
   * If empty, will lookup for any FetchManifest directive.
   */
  fetchManifestIdRe: string;
  /** DisableWatch disables watching the world for changes for FetchManifest. */
  disableWatch: boolean;
}

function createBaseConfig(): Config {
  return { engineId: "", objectKeys: [], fetchManifestIdRe: "", disableWatch: false };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.engineId !== "") {
      writer.uint32(10).string(message.engineId);
    }
    for (const v of message.objectKeys) {
      writer.uint32(18).string(v!);
    }
    if (message.fetchManifestIdRe !== "") {
      writer.uint32(26).string(message.fetchManifestIdRe);
    }
    if (message.disableWatch !== false) {
      writer.uint32(32).bool(message.disableWatch);
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

          message.engineId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.objectKeys.push(reader.string());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.fetchManifestIdRe = reader.string();
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.disableWatch = reader.bool();
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
      engineId: isSet(object.engineId) ? globalThis.String(object.engineId) : "",
      objectKeys: globalThis.Array.isArray(object?.objectKeys)
        ? object.objectKeys.map((e: any) => globalThis.String(e))
        : [],
      fetchManifestIdRe: isSet(object.fetchManifestIdRe) ? globalThis.String(object.fetchManifestIdRe) : "",
      disableWatch: isSet(object.disableWatch) ? globalThis.Boolean(object.disableWatch) : false,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    if (message.engineId !== "") {
      obj.engineId = message.engineId;
    }
    if (message.objectKeys?.length) {
      obj.objectKeys = message.objectKeys;
    }
    if (message.fetchManifestIdRe !== "") {
      obj.fetchManifestIdRe = message.fetchManifestIdRe;
    }
    if (message.disableWatch !== false) {
      obj.disableWatch = message.disableWatch;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.engineId = object.engineId ?? "";
    message.objectKeys = object.objectKeys?.map((e) => e) || [];
    message.fetchManifestIdRe = object.fetchManifestIdRe ?? "";
    message.disableWatch = object.disableWatch ?? false;
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
