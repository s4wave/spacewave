/* eslint-disable */
import { Pod } from "@go/github.com/aperturerobotics/containers/pod/pod.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "forge.lib.containers.pod";

/** Config configures the containers pod exec controller. */
export interface Config {
  /** EngineId is the pod engine to run the pod on. */
  engineId: string;
  /**
   * Name is the name of the pod.
   * Must be set if generate_name is not set.
   * Overrides meta field.
   */
  name: string;
  /**
   * GenerateName is the base name for generating a unique pod name.
   * Must be set if name is not set.
   * Overrides meta field.
   */
  generateName: string;
  /**
   * Meta contains a json or YAML k8s ObjectMeta object.
   * Optional if name or generate_name is set.
   */
  meta: string;
  /**
   * Pod is the pod configuration.
   * worldVolumes.engineId is treated as a WORLD input ID.
   */
  pod:
    | Pod
    | undefined;
  /**
   * PeerId is the peer identifier to use for Pod processes.
   * If unset: uses the peer ID of the Execution controller.
   */
  peerId: string;
  /**
   * VolumeInputs map WORLD_OBJECT inputs to WorldVolume IDs.
   * Overwrites conflicting values in pod.worldVolumes.
   */
  volumeInputs: { [key: string]: string };
  /** Quiet suppresses stdin/stdout logs to os stdio. */
  quiet: boolean;
}

export interface Config_VolumeInputsEntry {
  key: string;
  value: string;
}

function createBaseConfig(): Config {
  return {
    engineId: "",
    name: "",
    generateName: "",
    meta: "",
    pod: undefined,
    peerId: "",
    volumeInputs: {},
    quiet: false,
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.engineId !== "") {
      writer.uint32(10).string(message.engineId);
    }
    if (message.name !== "") {
      writer.uint32(18).string(message.name);
    }
    if (message.generateName !== "") {
      writer.uint32(26).string(message.generateName);
    }
    if (message.meta !== "") {
      writer.uint32(34).string(message.meta);
    }
    if (message.pod !== undefined) {
      Pod.encode(message.pod, writer.uint32(42).fork()).ldelim();
    }
    if (message.peerId !== "") {
      writer.uint32(50).string(message.peerId);
    }
    Object.entries(message.volumeInputs).forEach(([key, value]) => {
      Config_VolumeInputsEntry.encode({ key: key as any, value }, writer.uint32(58).fork()).ldelim();
    });
    if (message.quiet === true) {
      writer.uint32(64).bool(message.quiet);
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
          message.name = reader.string();
          break;
        case 3:
          message.generateName = reader.string();
          break;
        case 4:
          message.meta = reader.string();
          break;
        case 5:
          message.pod = Pod.decode(reader, reader.uint32());
          break;
        case 6:
          message.peerId = reader.string();
          break;
        case 7:
          const entry7 = Config_VolumeInputsEntry.decode(reader, reader.uint32());
          if (entry7.value !== undefined) {
            message.volumeInputs[entry7.key] = entry7.value;
          }
          break;
        case 8:
          message.quiet = reader.bool();
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
      name: isSet(object.name) ? String(object.name) : "",
      generateName: isSet(object.generateName) ? String(object.generateName) : "",
      meta: isSet(object.meta) ? String(object.meta) : "",
      pod: isSet(object.pod) ? Pod.fromJSON(object.pod) : undefined,
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      volumeInputs: isObject(object.volumeInputs)
        ? Object.entries(object.volumeInputs).reduce<{ [key: string]: string }>((acc, [key, value]) => {
          acc[key] = String(value);
          return acc;
        }, {})
        : {},
      quiet: isSet(object.quiet) ? Boolean(object.quiet) : false,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.name !== undefined && (obj.name = message.name);
    message.generateName !== undefined && (obj.generateName = message.generateName);
    message.meta !== undefined && (obj.meta = message.meta);
    message.pod !== undefined && (obj.pod = message.pod ? Pod.toJSON(message.pod) : undefined);
    message.peerId !== undefined && (obj.peerId = message.peerId);
    obj.volumeInputs = {};
    if (message.volumeInputs) {
      Object.entries(message.volumeInputs).forEach(([k, v]) => {
        obj.volumeInputs[k] = v;
      });
    }
    message.quiet !== undefined && (obj.quiet = message.quiet);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.engineId = object.engineId ?? "";
    message.name = object.name ?? "";
    message.generateName = object.generateName ?? "";
    message.meta = object.meta ?? "";
    message.pod = (object.pod !== undefined && object.pod !== null) ? Pod.fromPartial(object.pod) : undefined;
    message.peerId = object.peerId ?? "";
    message.volumeInputs = Object.entries(object.volumeInputs ?? {}).reduce<{ [key: string]: string }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = String(value);
        }
        return acc;
      },
      {},
    );
    message.quiet = object.quiet ?? false;
    return message;
  },
};

function createBaseConfig_VolumeInputsEntry(): Config_VolumeInputsEntry {
  return { key: "", value: "" };
}

export const Config_VolumeInputsEntry = {
  encode(message: Config_VolumeInputsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config_VolumeInputsEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig_VolumeInputsEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config_VolumeInputsEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Config_VolumeInputsEntry | Config_VolumeInputsEntry[]>
      | Iterable<Config_VolumeInputsEntry | Config_VolumeInputsEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_VolumeInputsEntry.encode(p).finish()];
        }
      } else {
        yield* [Config_VolumeInputsEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_VolumeInputsEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_VolumeInputsEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_VolumeInputsEntry.decode(p)];
        }
      } else {
        yield* [Config_VolumeInputsEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config_VolumeInputsEntry {
    return { key: isSet(object.key) ? String(object.key) : "", value: isSet(object.value) ? String(object.value) : "" };
  },

  toJSON(message: Config_VolumeInputsEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config_VolumeInputsEntry>, I>>(object: I): Config_VolumeInputsEntry {
    const message = createBaseConfig_VolumeInputsEntry();
    message.key = object.key ?? "";
    message.value = object.value ?? "";
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
