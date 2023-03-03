/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "plugin.static";

/** Config is the static plugin manifest loader config. */
export interface Config {
  /** EngineId is the world engine id to attach to. */
  engineId: string;
  /**
   * PluginHostKey is the PluginHost object to attach to.
   * Waits for it to exist.
   * Reads / writes linked PluginManifest objects.
   */
  pluginHostKey: string;
  /**
   * PluginPlatformId is the plugin platform ID.
   * If set, filters manifests to this platform ID.
   */
  pluginPlatformId: string;
  /** PeerId is the peer ID to use for world transactions. */
  peerId: string;
  /** LoadPlugin creates a LoadPlugin directive after loading the manifest. */
  loadPlugin: boolean;
}

function createBaseConfig(): Config {
  return { engineId: "", pluginHostKey: "", pluginPlatformId: "", peerId: "", loadPlugin: false };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.engineId !== "") {
      writer.uint32(10).string(message.engineId);
    }
    if (message.pluginHostKey !== "") {
      writer.uint32(18).string(message.pluginHostKey);
    }
    if (message.pluginPlatformId !== "") {
      writer.uint32(26).string(message.pluginPlatformId);
    }
    if (message.peerId !== "") {
      writer.uint32(34).string(message.peerId);
    }
    if (message.loadPlugin === true) {
      writer.uint32(40).bool(message.loadPlugin);
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
          message.engineId = reader.string();
          break;
        case 2:
          message.pluginHostKey = reader.string();
          break;
        case 3:
          message.pluginPlatformId = reader.string();
          break;
        case 4:
          message.peerId = reader.string();
          break;
        case 5:
          message.loadPlugin = reader.bool();
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
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      pluginHostKey: isSet(object.pluginHostKey) ? String(object.pluginHostKey) : "",
      pluginPlatformId: isSet(object.pluginPlatformId) ? String(object.pluginPlatformId) : "",
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      loadPlugin: isSet(object.loadPlugin) ? Boolean(object.loadPlugin) : false,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.pluginHostKey !== undefined && (obj.pluginHostKey = message.pluginHostKey);
    message.pluginPlatformId !== undefined && (obj.pluginPlatformId = message.pluginPlatformId);
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.loadPlugin !== undefined && (obj.loadPlugin = message.loadPlugin);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.engineId = object.engineId ?? "";
    message.pluginHostKey = object.pluginHostKey ?? "";
    message.pluginPlatformId = object.pluginPlatformId ?? "";
    message.peerId = object.peerId ?? "";
    message.loadPlugin = object.loadPlugin ?? false;
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
