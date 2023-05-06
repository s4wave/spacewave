/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.plugin.compiler";

/** Config configures the plugin compiler controller. */
export interface Config {
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
   * HostConfigSet is a ConfigSet to apply to the host on plugin startup.
   * This ConfigSet is applied to the plugin host bus.
   * This will be included in the plugin binary.
   * Adds a config to configSet with ID bldr/plugin/host/configset
   */
  hostConfigSet: { [key: string]: ControllerConfig };
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
  /** ProjectId overrides the project id set in the project config. */
  projectId: string;
  /**
   * EnableCgo enables cgo in the Go compiler.
   * Cgo is disabled by default as it may cause non-reproducible builds.
   * https://github.com/golang/go/issues/57120#issuecomment-1420752516
   */
  enableCgo: boolean;
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
   * GoPackages is a list of Go packages to scan for controller factories.
   * Looks for package-level functions:
   *  - NewFactory(b bus.Bus) controller.Factory
   *  - BuildFactories(b bus.Bus) []controller.Factory
   * Appended to the list set in the plugin compiler settings.
   */
  goPackages: string[];
  /** ProjectId overrides the project id set in the project config. */
  projectId: string;
  /**
   * EnableCgo enables cgo in the Go compiler.
   * Cgo is disabled by default as it may cause non-reproducible builds.
   * https://github.com/golang/go/issues/57120#issuecomment-1420752516
   */
  enableCgo: boolean;
}

export interface PreBuildHookResult_ConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

export interface PreBuildHookResult_HostConfigSetEntry {
  key: string;
  value: ControllerConfig | undefined;
}

function createBaseConfig(): Config {
  return {
    goPackages: [],
    configSet: {},
    hostConfigSet: {},
    disableRpcFetch: false,
    disableFetchAssets: false,
    delveAddr: "",
    projectId: "",
    enableCgo: false,
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.goPackages) {
      writer.uint32(10).string(v!);
    }
    Object.entries(message.configSet).forEach(([key, value]) => {
      Config_ConfigSetEntry.encode({ key: key as any, value }, writer.uint32(18).fork()).ldelim();
    });
    Object.entries(message.hostConfigSet).forEach(([key, value]) => {
      Config_HostConfigSetEntry.encode({ key: key as any, value }, writer.uint32(26).fork()).ldelim();
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
    if (message.projectId !== "") {
      writer.uint32(58).string(message.projectId);
    }
    if (message.enableCgo === true) {
      writer.uint32(64).bool(message.enableCgo);
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

          message.goPackages.push(reader.string());
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
          if (tag !== 32) {
            break;
          }

          message.disableRpcFetch = reader.bool();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.disableFetchAssets = reader.bool();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.delveAddr = reader.string();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.projectId = reader.string();
          continue;
        case 8:
          if (tag !== 64) {
            break;
          }

          message.enableCgo = reader.bool();
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
      goPackages: Array.isArray(object?.goPackages) ? object.goPackages.map((e: any) => String(e)) : [],
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
      disableRpcFetch: isSet(object.disableRpcFetch) ? Boolean(object.disableRpcFetch) : false,
      disableFetchAssets: isSet(object.disableFetchAssets) ? Boolean(object.disableFetchAssets) : false,
      delveAddr: isSet(object.delveAddr) ? String(object.delveAddr) : "",
      projectId: isSet(object.projectId) ? String(object.projectId) : "",
      enableCgo: isSet(object.enableCgo) ? Boolean(object.enableCgo) : false,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
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
    obj.hostConfigSet = {};
    if (message.hostConfigSet) {
      Object.entries(message.hostConfigSet).forEach(([k, v]) => {
        obj.hostConfigSet[k] = ControllerConfig.toJSON(v);
      });
    }
    message.disableRpcFetch !== undefined && (obj.disableRpcFetch = message.disableRpcFetch);
    message.disableFetchAssets !== undefined && (obj.disableFetchAssets = message.disableFetchAssets);
    message.delveAddr !== undefined && (obj.delveAddr = message.delveAddr);
    message.projectId !== undefined && (obj.projectId = message.projectId);
    message.enableCgo !== undefined && (obj.enableCgo = message.enableCgo);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
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
    message.hostConfigSet = Object.entries(object.hostConfigSet ?? {}).reduce<{ [key: string]: ControllerConfig }>(
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
    message.projectId = object.projectId ?? "";
    message.enableCgo = object.enableCgo ?? false;
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

  create<I extends Exact<DeepPartial<Config_ConfigSetEntry>, I>>(base?: I): Config_ConfigSetEntry {
    return Config_ConfigSetEntry.fromPartial(base ?? {});
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
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_HostConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [Config_HostConfigSetEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_HostConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_HostConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_HostConfigSetEntry.decode(p)];
        }
      } else {
        yield* [Config_HostConfigSetEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config_HostConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: Config_HostConfigSetEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config_HostConfigSetEntry>, I>>(base?: I): Config_HostConfigSetEntry {
    return Config_HostConfigSetEntry.fromPartial(base ?? {});
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
  return { configSet: {}, hostConfigSet: {}, goPackages: [], projectId: "", enableCgo: false };
}

export const PreBuildHookResult = {
  encode(message: PreBuildHookResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.configSet).forEach(([key, value]) => {
      PreBuildHookResult_ConfigSetEntry.encode({ key: key as any, value }, writer.uint32(10).fork()).ldelim();
    });
    Object.entries(message.hostConfigSet).forEach(([key, value]) => {
      PreBuildHookResult_HostConfigSetEntry.encode({ key: key as any, value }, writer.uint32(18).fork()).ldelim();
    });
    for (const v of message.goPackages) {
      writer.uint32(26).string(v!);
    }
    if (message.projectId !== "") {
      writer.uint32(34).string(message.projectId);
    }
    if (message.enableCgo === true) {
      writer.uint32(40).bool(message.enableCgo);
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

          const entry1 = PreBuildHookResult_ConfigSetEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.configSet[entry1.key] = entry1.value;
          }
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          const entry2 = PreBuildHookResult_HostConfigSetEntry.decode(reader, reader.uint32());
          if (entry2.value !== undefined) {
            message.hostConfigSet[entry2.key] = entry2.value;
          }
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.goPackages.push(reader.string());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.projectId = reader.string();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.enableCgo = reader.bool();
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
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PreBuildHookResult.encode(p).finish()];
        }
      } else {
        yield* [PreBuildHookResult.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PreBuildHookResult>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PreBuildHookResult> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PreBuildHookResult.decode(p)];
        }
      } else {
        yield* [PreBuildHookResult.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PreBuildHookResult {
    return {
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
      goPackages: Array.isArray(object?.goPackages) ? object.goPackages.map((e: any) => String(e)) : [],
      projectId: isSet(object.projectId) ? String(object.projectId) : "",
      enableCgo: isSet(object.enableCgo) ? Boolean(object.enableCgo) : false,
    };
  },

  toJSON(message: PreBuildHookResult): unknown {
    const obj: any = {};
    obj.configSet = {};
    if (message.configSet) {
      Object.entries(message.configSet).forEach(([k, v]) => {
        obj.configSet[k] = ControllerConfig.toJSON(v);
      });
    }
    obj.hostConfigSet = {};
    if (message.hostConfigSet) {
      Object.entries(message.hostConfigSet).forEach(([k, v]) => {
        obj.hostConfigSet[k] = ControllerConfig.toJSON(v);
      });
    }
    if (message.goPackages) {
      obj.goPackages = message.goPackages.map((e) => e);
    } else {
      obj.goPackages = [];
    }
    message.projectId !== undefined && (obj.projectId = message.projectId);
    message.enableCgo !== undefined && (obj.enableCgo = message.enableCgo);
    return obj;
  },

  create<I extends Exact<DeepPartial<PreBuildHookResult>, I>>(base?: I): PreBuildHookResult {
    return PreBuildHookResult.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PreBuildHookResult>, I>>(object: I): PreBuildHookResult {
    const message = createBasePreBuildHookResult();
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
    message.goPackages = object.goPackages?.map((e) => e) || [];
    message.projectId = object.projectId ?? "";
    message.enableCgo = object.enableCgo ?? false;
    return message;
  },
};

function createBasePreBuildHookResult_ConfigSetEntry(): PreBuildHookResult_ConfigSetEntry {
  return { key: "", value: undefined };
}

export const PreBuildHookResult_ConfigSetEntry = {
  encode(message: PreBuildHookResult_ConfigSetEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PreBuildHookResult_ConfigSetEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePreBuildHookResult_ConfigSetEntry();
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
  // Transform<PreBuildHookResult_ConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PreBuildHookResult_ConfigSetEntry | PreBuildHookResult_ConfigSetEntry[]>
      | Iterable<PreBuildHookResult_ConfigSetEntry | PreBuildHookResult_ConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PreBuildHookResult_ConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [PreBuildHookResult_ConfigSetEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PreBuildHookResult_ConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PreBuildHookResult_ConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PreBuildHookResult_ConfigSetEntry.decode(p)];
        }
      } else {
        yield* [PreBuildHookResult_ConfigSetEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PreBuildHookResult_ConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: PreBuildHookResult_ConfigSetEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PreBuildHookResult_ConfigSetEntry>, I>>(
    base?: I,
  ): PreBuildHookResult_ConfigSetEntry {
    return PreBuildHookResult_ConfigSetEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PreBuildHookResult_ConfigSetEntry>, I>>(
    object: I,
  ): PreBuildHookResult_ConfigSetEntry {
    const message = createBasePreBuildHookResult_ConfigSetEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ControllerConfig.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBasePreBuildHookResult_HostConfigSetEntry(): PreBuildHookResult_HostConfigSetEntry {
  return { key: "", value: undefined };
}

export const PreBuildHookResult_HostConfigSetEntry = {
  encode(message: PreBuildHookResult_HostConfigSetEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PreBuildHookResult_HostConfigSetEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePreBuildHookResult_HostConfigSetEntry();
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
  // Transform<PreBuildHookResult_HostConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PreBuildHookResult_HostConfigSetEntry | PreBuildHookResult_HostConfigSetEntry[]>
      | Iterable<PreBuildHookResult_HostConfigSetEntry | PreBuildHookResult_HostConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PreBuildHookResult_HostConfigSetEntry.encode(p).finish()];
        }
      } else {
        yield* [PreBuildHookResult_HostConfigSetEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PreBuildHookResult_HostConfigSetEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PreBuildHookResult_HostConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PreBuildHookResult_HostConfigSetEntry.decode(p)];
        }
      } else {
        yield* [PreBuildHookResult_HostConfigSetEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PreBuildHookResult_HostConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? ControllerConfig.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: PreBuildHookResult_HostConfigSetEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? ControllerConfig.toJSON(message.value) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PreBuildHookResult_HostConfigSetEntry>, I>>(
    base?: I,
  ): PreBuildHookResult_HostConfigSetEntry {
    return PreBuildHookResult_HostConfigSetEntry.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PreBuildHookResult_HostConfigSetEntry>, I>>(
    object: I,
  ): PreBuildHookResult_HostConfigSetEntry {
    const message = createBasePreBuildHookResult_HostConfigSetEntry();
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
