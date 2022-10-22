/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { PluginBuilderConfig } from "../builder/builder.pb.js";

export const protobufPackage = "plugin.compiler";

/** Config configures the plugin compiler controller. */
export interface Config {
  /**
   * PluginBuilderConfig contains common config for the plugin builder.
   * Overridden by the project controller.
   */
  pluginBuilderConfig:
    | PluginBuilderConfig
    | undefined;
  /**
   * GoPackages is the list of Go packages to scan for controller factories.
   * Looks for package-level functions:
   *  - NewFactory(b bus.Bus) controller.Factory
   *  - BuildFactories(b bus.Bus) []controller.Factory
   */
  goPackages: string[];
  /**
   * ConfigSet is a ConfigSet to apply on plugin startup.
   * This ConfigSet is applied to the plugin bus.
   * This will be included in the plugin binary.
   */
  configSet: { [key: string]: ControllerConfig };
  /**
   * DisableRpcFetch disables the default Fetch RPC service handler.
   * The handler handles the Fetch service by creating a directive.
   * You can also override config ID "rpc-fetch" in the config-set.
   * This service is used for the ServiceWorker HTTP calls.
   */
  disableRpcFetch: boolean;
  /**
   * DisableFetchAssets disables the default web assets service handler.
   * The handler handles Fetch directives with the assets FS.
   * This service is used for the ServiceWorker HTTP calls.
   * This usually should be disabled if using custom HTTP handlers.
   * Override this using config ID "plugin-assets" in the config-set.
   */
  disableFetchAssets: boolean;
  /**
   * DelveAddr is the address to listen for Delve remote connections.
   * If the build mode is dev and this is set, uses delve to run the plugin.
   * Ignored if build mode is not dev.
   * Special value: "wait" - waits for plugin entrypoint to be run manually.
   * Allowed characters: [Z-a0-9.:]
   * Example: ":5000"
   */
  delveAddr: string;
}

export interface Config_ConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

function createBaseConfig(): Config {
  return {
    pluginBuilderConfig: undefined,
    goPackages: [],
    configSet: {},
    disableRpcFetch: false,
    disableFetchAssets: false,
    delveAddr: "",
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginBuilderConfig !== undefined) {
      PluginBuilderConfig.encode(message.pluginBuilderConfig, writer.uint32(10).fork()).ldelim();
    }
    for (const v of message.goPackages) {
      writer.uint32(18).string(v!);
    }
    Object.entries(message.configSet).forEach(([key, value]) => {
      Config_ConfigSetEntry.encode({ key: key as any, value }, writer.uint32(26).fork()).ldelim();
    });
    if (message.disableRpcFetch === true) {
      writer.uint32(32).bool(message.disableRpcFetch);
    }
    if (message.disableFetchAssets === true) {
      writer.uint32(40).bool(message.disableFetchAssets);
    }
    if (message.delveAddr !== "") {
      writer.uint32(50).string(message.delveAddr);
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
          message.pluginBuilderConfig = PluginBuilderConfig.decode(reader, reader.uint32());
          break;
        case 2:
          message.goPackages.push(reader.string());
          break;
        case 3:
          const entry3 = Config_ConfigSetEntry.decode(reader, reader.uint32());
          if (entry3.value !== undefined) {
            message.configSet[entry3.key] = entry3.value;
          }
          break;
        case 4:
          message.disableRpcFetch = reader.bool();
          break;
        case 5:
          message.disableFetchAssets = reader.bool();
          break;
        case 6:
          message.delveAddr = reader.string();
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
      pluginBuilderConfig: isSet(object.pluginBuilderConfig)
        ? PluginBuilderConfig.fromJSON(object.pluginBuilderConfig)
        : undefined,
      goPackages: Array.isArray(object?.goPackages) ? object.goPackages.map((e: any) => String(e)) : [],
      configSet: isObject(object.configSet)
        ? Object.entries(object.configSet).reduce<{ [key: string]: ControllerConfig }>((acc, [key, value]) => {
          acc[key] = ControllerConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      disableRpcFetch: isSet(object.disableRpcFetch) ? Boolean(object.disableRpcFetch) : false,
      disableFetchAssets: isSet(object.disableFetchAssets) ? Boolean(object.disableFetchAssets) : false,
      delveAddr: isSet(object.delveAddr) ? String(object.delveAddr) : "",
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.pluginBuilderConfig !== undefined &&
      (obj.pluginBuilderConfig = message.pluginBuilderConfig
        ? PluginBuilderConfig.toJSON(message.pluginBuilderConfig)
        : undefined);
    if (message.goPackages) {
      obj.goPackages = message.goPackages.map((e) => e);
    } else {
      obj.goPackages = [];
    }
    obj.configSet = {};
    if (message.configSet) {
      Object.entries(message.configSet).forEach(([k, v]) => {
        obj.configSet[k] = ControllerConfig.toJSON(v);
      });
    }
    message.disableRpcFetch !== undefined && (obj.disableRpcFetch = message.disableRpcFetch);
    message.disableFetchAssets !== undefined && (obj.disableFetchAssets = message.disableFetchAssets);
    message.delveAddr !== undefined && (obj.delveAddr = message.delveAddr);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.pluginBuilderConfig = (object.pluginBuilderConfig !== undefined && object.pluginBuilderConfig !== null)
      ? PluginBuilderConfig.fromPartial(object.pluginBuilderConfig)
      : undefined;
    message.goPackages = object.goPackages?.map((e) => e) || [];
    message.configSet = Object.entries(object.configSet ?? {}).reduce<{ [key: string]: ControllerConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ControllerConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.disableRpcFetch = object.disableRpcFetch ?? false;
    message.disableFetchAssets = object.disableFetchAssets ?? false;
    message.delveAddr = object.delveAddr ?? "";
    return message;
  },
};

function createBaseConfig_ConfigSetEntry(): Config_ConfigSetEntry {
  return { key: "", value: undefined };
}

export const Config_ConfigSetEntry = {
  encode(message: Config_ConfigSetEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config_ConfigSetEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig_ConfigSetEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = ControllerConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config_ConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Config_ConfigSetEntry | Config_ConfigSetEntry[]>
      | Iterable<Config_ConfigSetEntry | Config_ConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_ConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [Config_ConfigSetEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_ConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_ConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_ConfigSetEntry.decode(p)];
        }
      } else {
        yield* [Config_ConfigSetEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config_ConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: Config_ConfigSetEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config_ConfigSetEntry>, I>>(object: I): Config_ConfigSetEntry {
    const message = createBaseConfig_ConfigSetEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ControllerConfig.fromPartial(object.value)
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
