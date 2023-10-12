/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.plugin.compiler";

/** Config configures the plugin compiler controller. */
export interface Config {
  /** ProjectId overrides the project id set in the project config. */
  projectId: string;
  /**
   * ConfigSet is a ConfigSet to apply on plugin startup.
   * This ConfigSet is applied to the plugin bus.
   * This will be included in the plugin binary.
   * Merged with the plugin compiler ConfigSet field.
   */
  configSet: { [key: string]: ControllerConfig };
  /**
   * HostConfigSet is a ConfigSet to apply to the host on plugin startup.
   * This ConfigSet is applied to the plugin host bus.
   * This will be included in the plugin binary.
   * Adds a config to configSet with ID bldr/plugin/host/configset
   * Merged with the plugin compiler HostConfigSet field.
   */
  hostConfigSet: { [key: string]: ControllerConfig };
  /**
   * GoPkgs is a list of Go packages to scan for controller factories.
   * Looks for package-level functions:
   *  - NewFactory(b bus.Bus) controller.Factory
   *  - BuildFactories(b bus.Bus) []controller.Factory
   * Appended to the list set in the plugin compiler settings.
   */
  goPkgs: string[];
  /**
   * WebPkgs is the list of web packages to externalize and include in the bundle.
   *
   * Externalized web packages (npm modules) are imported separately from the web bundle.
   * They will be deduplicated such that a single version is imported at a time by the app.
   * This is useful for packages that require a single instance per WebDocument.
   *
   * These packages will be available with the LookupWebPkg directive.
   * They will also be available at /b/pkg: e.g. /b/pkg/@my/npm-package/foo/bar/index.js
   *
   * Note: only files & entrypoints imported by at least one js file will be included.
   */
  webPkgs: string[];
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
   * Allowed characters: [Z-a0-9.:]
   * Special value: "wait" - waits for plugin entrypoint to be run manually.
   * Example: ":5000"
   */
  delveAddr: string;
  /**
   * EnableCgo enables cgo in the Go compiler.
   * Cgo is disabled by default as it may cause non-reproducible builds.
   * https://github.com/golang/go/issues/57120#issuecomment-1420752516
   */
  enableCgo: boolean;
  /**
   * EsbuildFlags is a string containing additional flags to pass to esbuild.
   * Flags passed by bldr:esbuild directives can override these values.
   * E.x.: --target es2020
   */
  esbuildFlags: string[];
}

export interface Config_ConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

export interface Config_HostConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

/** PreBuildHookResult is the output of a pre-build hook. */
export interface PreBuildHookResult {
  /**
   * Config is the configuration for the plugin build step.
   * Merged with the existing configuration.
   */
  config: Config | undefined;
}

function createBaseConfig(): Config {
  return {
    projectId: "",
    configSet: {},
    hostConfigSet: {},
    goPkgs: [],
    webPkgs: [],
    disableRpcFetch: false,
    disableFetchAssets: false,
    delveAddr: "",
    enableCgo: false,
    esbuildFlags: [],
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.projectId !== "") {
      writer.uint32(10).string(message.projectId);
    }
    Object.entries(message.configSet).forEach(([key, value]) => {
      Config_ConfigSetEntry.encode({ key: key as any, value }, writer.uint32(18).fork()).ldelim();
    });
    Object.entries(message.hostConfigSet).forEach(([key, value]) => {
      Config_HostConfigSetEntry.encode({ key: key as any, value }, writer.uint32(26).fork()).ldelim();
    });
    for (const v of message.goPkgs) {
      writer.uint32(34).string(v!);
    }
    for (const v of message.webPkgs) {
      writer.uint32(42).string(v!);
    }
    if (message.disableRpcFetch === true) {
      writer.uint32(48).bool(message.disableRpcFetch);
    }
    if (message.disableFetchAssets === true) {
      writer.uint32(56).bool(message.disableFetchAssets);
    }
    if (message.delveAddr !== "") {
      writer.uint32(66).string(message.delveAddr);
    }
    if (message.enableCgo === true) {
      writer.uint32(72).bool(message.enableCgo);
    }
    for (const v of message.esbuildFlags) {
      writer.uint32(82).string(v!);
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

          message.projectId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          const entry2 = Config_ConfigSetEntry.decode(reader, reader.uint32());
          if (entry2.value !== undefined) {
            message.configSet[entry2.key] = entry2.value;
          }
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          const entry3 = Config_HostConfigSetEntry.decode(reader, reader.uint32());
          if (entry3.value !== undefined) {
            message.hostConfigSet[entry3.key] = entry3.value;
          }
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.goPkgs.push(reader.string());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.webPkgs.push(reader.string());
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.disableRpcFetch = reader.bool();
          continue;
        case 7:
          if (tag !== 56) {
            break;
          }

          message.disableFetchAssets = reader.bool();
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.delveAddr = reader.string();
          continue;
        case 9:
          if (tag !== 72) {
            break;
          }

          message.enableCgo = reader.bool();
          continue;
        case 10:
          if (tag !== 82) {
            break;
          }

          message.esbuildFlags.push(reader.string());
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
      projectId: isSet(object.projectId) ? globalThis.String(object.projectId) : "",
      configSet: isObject(object.configSet)
        ? Object.entries(object.configSet).reduce<{ [key: string]: ControllerConfig }>((acc, [key, value]) => {
          acc[key] = ControllerConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      hostConfigSet: isObject(object.hostConfigSet)
        ? Object.entries(object.hostConfigSet).reduce<{ [key: string]: ControllerConfig }>((acc, [key, value]) => {
          acc[key] = ControllerConfig.fromJSON(value);
          return acc;
        }, {})
        : {},
      goPkgs: globalThis.Array.isArray(object?.goPkgs) ? object.goPkgs.map((e: any) => globalThis.String(e)) : [],
      webPkgs: globalThis.Array.isArray(object?.webPkgs) ? object.webPkgs.map((e: any) => globalThis.String(e)) : [],
      disableRpcFetch: isSet(object.disableRpcFetch) ? globalThis.Boolean(object.disableRpcFetch) : false,
      disableFetchAssets: isSet(object.disableFetchAssets) ? globalThis.Boolean(object.disableFetchAssets) : false,
      delveAddr: isSet(object.delveAddr) ? globalThis.String(object.delveAddr) : "",
      enableCgo: isSet(object.enableCgo) ? globalThis.Boolean(object.enableCgo) : false,
      esbuildFlags: globalThis.Array.isArray(object?.esbuildFlags)
        ? object.esbuildFlags.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    if (message.projectId !== "") {
      obj.projectId = message.projectId;
    }
    if (message.configSet) {
      const entries = Object.entries(message.configSet);
      if (entries.length > 0) {
        obj.configSet = {};
        entries.forEach(([k, v]) => {
          obj.configSet[k] = ControllerConfig.toJSON(v);
        });
      }
    }
    if (message.hostConfigSet) {
      const entries = Object.entries(message.hostConfigSet);
      if (entries.length > 0) {
        obj.hostConfigSet = {};
        entries.forEach(([k, v]) => {
          obj.hostConfigSet[k] = ControllerConfig.toJSON(v);
        });
      }
    }
    if (message.goPkgs?.length) {
      obj.goPkgs = message.goPkgs;
    }
    if (message.webPkgs?.length) {
      obj.webPkgs = message.webPkgs;
    }
    if (message.disableRpcFetch === true) {
      obj.disableRpcFetch = message.disableRpcFetch;
    }
    if (message.disableFetchAssets === true) {
      obj.disableFetchAssets = message.disableFetchAssets;
    }
    if (message.delveAddr !== "") {
      obj.delveAddr = message.delveAddr;
    }
    if (message.enableCgo === true) {
      obj.enableCgo = message.enableCgo;
    }
    if (message.esbuildFlags?.length) {
      obj.esbuildFlags = message.esbuildFlags;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.projectId = object.projectId ?? "";
    message.configSet = Object.entries(object.configSet ?? {}).reduce<{ [key: string]: ControllerConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ControllerConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.hostConfigSet = Object.entries(object.hostConfigSet ?? {}).reduce<{ [key: string]: ControllerConfig }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ControllerConfig.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.goPkgs = object.goPkgs?.map((e) => e) || [];
    message.webPkgs = object.webPkgs?.map((e) => e) || [];
    message.disableRpcFetch = object.disableRpcFetch ?? false;
    message.disableFetchAssets = object.disableFetchAssets ?? false;
    message.delveAddr = object.delveAddr ?? "";
    message.enableCgo = object.enableCgo ?? false;
    message.esbuildFlags = object.esbuildFlags?.map((e) => e) || [];
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig_ConfigSetEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.value = ControllerConfig.decode(reader, reader.uint32());
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
  // Transform<Config_ConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Config_ConfigSetEntry | Config_ConfigSetEntry[]>
      | Iterable<Config_ConfigSetEntry | Config_ConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config_ConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [Config_ConfigSetEntry.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_ConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_ConfigSetEntry> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config_ConfigSetEntry.decode(p)];
        }
      } else {
        yield* [Config_ConfigSetEntry.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): Config_ConfigSetEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: Config_ConfigSetEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== undefined) {
      obj.value = ControllerConfig.toJSON(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config_ConfigSetEntry>, I>>(base?: I): Config_ConfigSetEntry {
    return Config_ConfigSetEntry.fromPartial(base ?? ({} as any));
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

function createBaseConfig_HostConfigSetEntry(): Config_HostConfigSetEntry {
  return { key: "", value: undefined };
}

export const Config_HostConfigSetEntry = {
  encode(message: Config_HostConfigSetEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config_HostConfigSetEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig_HostConfigSetEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.value = ControllerConfig.decode(reader, reader.uint32());
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
  // Transform<Config_HostConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Config_HostConfigSetEntry | Config_HostConfigSetEntry[]>
      | Iterable<Config_HostConfigSetEntry | Config_HostConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config_HostConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [Config_HostConfigSetEntry.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_HostConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_HostConfigSetEntry> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config_HostConfigSetEntry.decode(p)];
        }
      } else {
        yield* [Config_HostConfigSetEntry.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): Config_HostConfigSetEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: Config_HostConfigSetEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== undefined) {
      obj.value = ControllerConfig.toJSON(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config_HostConfigSetEntry>, I>>(base?: I): Config_HostConfigSetEntry {
    return Config_HostConfigSetEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Config_HostConfigSetEntry>, I>>(object: I): Config_HostConfigSetEntry {
    const message = createBaseConfig_HostConfigSetEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ControllerConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePreBuildHookResult(): PreBuildHookResult {
  return { config: undefined };
}

export const PreBuildHookResult = {
  encode(message: PreBuildHookResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.config !== undefined) {
      Config.encode(message.config, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PreBuildHookResult {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePreBuildHookResult();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.config = Config.decode(reader, reader.uint32());
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
  // Transform<PreBuildHookResult, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PreBuildHookResult | PreBuildHookResult[]>
      | Iterable<PreBuildHookResult | PreBuildHookResult[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [PreBuildHookResult.encode(p).finish()];
        }
      } else {
        yield* [PreBuildHookResult.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PreBuildHookResult>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PreBuildHookResult> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [PreBuildHookResult.decode(p)];
        }
      } else {
        yield* [PreBuildHookResult.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): PreBuildHookResult {
    return { config: isSet(object.config) ? Config.fromJSON(object.config) : undefined };
  },

  toJSON(message: PreBuildHookResult): unknown {
    const obj: any = {};
    if (message.config !== undefined) {
      obj.config = Config.toJSON(message.config);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<PreBuildHookResult>, I>>(base?: I): PreBuildHookResult {
    return PreBuildHookResult.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<PreBuildHookResult>, I>>(object: I): PreBuildHookResult {
    const message = createBasePreBuildHookResult();
    message.config = (object.config !== undefined && object.config !== null)
      ? Config.fromPartial(object.config)
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
