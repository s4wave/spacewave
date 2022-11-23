/* eslint-disable */
import { Backoff } from "@go/github.com/aperturerobotics/util/backoff/backoff.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "volume.rpc.client";

/**
 * Config configures the rpc volume client.
 * Accesses the AccessVolumes RPC service.
 */
export interface Config {
  /**
   * ServiceId is the service id to use.
   * Must resolve to the AccessVolumes service.
   * Usually: plugin-host/rpc.volume.AccessVolumes
   * Cannot be empty.
   */
  serviceId: string;
  /**
   * VolumeIdRe is a regex string to match volume IDs.
   * Matched volume IDs are forwarded to the RPC service.
   * Matched volume IDs may not necessarily exist on the remote.
   * Set to empty or '.*' to match all volumes.
   */
  volumeIdRe: string;
  /**
   * VolumeIds is a list of volume IDs to load on startup.
   * May be empty.
   */
  volumeIds: string[];
  /**
   * ClientId is the client id to use.
   * May be empty.
   */
  clientId: string;
  /**
   * ReleaseDelay is a delay duration to wait before releasing a unreferenced volume.
   * If empty string, defaults to 1s (1 second).
   */
  releaseDelay: string;
  /**
   * VolumeAliases contains aliases to assign to proxied volumes.
   * Key = the destination volume ID.
   * Value = contains source volume IDs to match.
   * Volume IDs listed here will be proxied regardless of the regex.
   */
  volumeAliases: { [key: string]: VolumeAliases };
  /** Backoff controls retry backoff for the volume rpc client. */
  backoff: Backoff | undefined;
}

export interface Config_VolumeAliasesEntry {
  key: string;
  value: VolumeAliases | undefined;
}

/** VolumeAliases is a list of volume aliases. */
export interface VolumeAliases {
  /** From is a list of volume IDs to alias. */
  from: string[];
}

function createBaseConfig(): Config {
  return {
    serviceId: "",
    volumeIdRe: "",
    volumeIds: [],
    clientId: "",
    releaseDelay: "",
    volumeAliases: {},
    backoff: undefined,
  };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.serviceId !== "") {
      writer.uint32(10).string(message.serviceId);
    }
    if (message.volumeIdRe !== "") {
      writer.uint32(18).string(message.volumeIdRe);
    }
    for (const v of message.volumeIds) {
      writer.uint32(26).string(v!);
    }
    if (message.clientId !== "") {
      writer.uint32(34).string(message.clientId);
    }
    if (message.releaseDelay !== "") {
      writer.uint32(42).string(message.releaseDelay);
    }
    Object.entries(message.volumeAliases).forEach(([key, value]) => {
      Config_VolumeAliasesEntry.encode({ key: key as any, value }, writer.uint32(50).fork()).ldelim();
    });
    if (message.backoff !== undefined) {
      Backoff.encode(message.backoff, writer.uint32(58).fork()).ldelim();
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
          message.serviceId = reader.string();
          break;
        case 2:
          message.volumeIdRe = reader.string();
          break;
        case 3:
          message.volumeIds.push(reader.string());
          break;
        case 4:
          message.clientId = reader.string();
          break;
        case 5:
          message.releaseDelay = reader.string();
          break;
        case 6:
          const entry6 = Config_VolumeAliasesEntry.decode(reader, reader.uint32());
          if (entry6.value !== undefined) {
            message.volumeAliases[entry6.key] = entry6.value;
          }
          break;
        case 7:
          message.backoff = Backoff.decode(reader, reader.uint32());
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
      serviceId: isSet(object.serviceId) ? String(object.serviceId) : "",
      volumeIdRe: isSet(object.volumeIdRe) ? String(object.volumeIdRe) : "",
      volumeIds: Array.isArray(object?.volumeIds) ? object.volumeIds.map((e: any) => String(e)) : [],
      clientId: isSet(object.clientId) ? String(object.clientId) : "",
      releaseDelay: isSet(object.releaseDelay) ? String(object.releaseDelay) : "",
      volumeAliases: isObject(object.volumeAliases)
        ? Object.entries(object.volumeAliases).reduce<{ [key: string]: VolumeAliases }>((acc, [key, value]) => {
          acc[key] = VolumeAliases.fromJSON(value);
          return acc;
        }, {})
        : {},
      backoff: isSet(object.backoff) ? Backoff.fromJSON(object.backoff) : undefined,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.serviceId !== undefined && (obj.serviceId = message.serviceId);
    message.volumeIdRe !== undefined && (obj.volumeIdRe = message.volumeIdRe);
    if (message.volumeIds) {
      obj.volumeIds = message.volumeIds.map((e) => e);
    } else {
      obj.volumeIds = [];
    }
    message.clientId !== undefined && (obj.clientId = message.clientId);
    message.releaseDelay !== undefined && (obj.releaseDelay = message.releaseDelay);
    obj.volumeAliases = {};
    if (message.volumeAliases) {
      Object.entries(message.volumeAliases).forEach(([k, v]) => {
        obj.volumeAliases[k] = VolumeAliases.toJSON(v);
      });
    }
    message.backoff !== undefined && (obj.backoff = message.backoff ? Backoff.toJSON(message.backoff) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.serviceId = object.serviceId ?? "";
    message.volumeIdRe = object.volumeIdRe ?? "";
    message.volumeIds = object.volumeIds?.map((e) => e) || [];
    message.clientId = object.clientId ?? "";
    message.releaseDelay = object.releaseDelay ?? "";
    message.volumeAliases = Object.entries(object.volumeAliases ?? {}).reduce<{ [key: string]: VolumeAliases }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = VolumeAliases.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.backoff = (object.backoff !== undefined && object.backoff !== null)
      ? Backoff.fromPartial(object.backoff)
      : undefined;
    return message;
  },
};

function createBaseConfig_VolumeAliasesEntry(): Config_VolumeAliasesEntry {
  return { key: "", value: undefined };
}

export const Config_VolumeAliasesEntry = {
  encode(message: Config_VolumeAliasesEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      VolumeAliases.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config_VolumeAliasesEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig_VolumeAliasesEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = VolumeAliases.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config_VolumeAliasesEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Config_VolumeAliasesEntry | Config_VolumeAliasesEntry[]>
      | Iterable<Config_VolumeAliasesEntry | Config_VolumeAliasesEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_VolumeAliasesEntry.encode(p).finish()];
        }
      } else {
        yield* [Config_VolumeAliasesEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_VolumeAliasesEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_VolumeAliasesEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_VolumeAliasesEntry.decode(p)];
        }
      } else {
        yield* [Config_VolumeAliasesEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config_VolumeAliasesEntry {
    return {
      key: isSet(object.key) ? String(object.key) : "",
      value: isSet(object.value) ? VolumeAliases.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: Config_VolumeAliasesEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value ? VolumeAliases.toJSON(message.value) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config_VolumeAliasesEntry>, I>>(object: I): Config_VolumeAliasesEntry {
    const message = createBaseConfig_VolumeAliasesEntry();
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? VolumeAliases.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseVolumeAliases(): VolumeAliases {
  return { from: [] };
}

export const VolumeAliases = {
  encode(message: VolumeAliases, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.from) {
      writer.uint32(10).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): VolumeAliases {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseVolumeAliases();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.from.push(reader.string());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<VolumeAliases, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<VolumeAliases | VolumeAliases[]> | Iterable<VolumeAliases | VolumeAliases[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeAliases.encode(p).finish()];
        }
      } else {
        yield* [VolumeAliases.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, VolumeAliases>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<VolumeAliases> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeAliases.decode(p)];
        }
      } else {
        yield* [VolumeAliases.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): VolumeAliases {
    return { from: Array.isArray(object?.from) ? object.from.map((e: any) => String(e)) : [] };
  },

  toJSON(message: VolumeAliases): unknown {
    const obj: any = {};
    if (message.from) {
      obj.from = message.from.map((e) => e);
    } else {
      obj.from = [];
    }
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<VolumeAliases>, I>>(object: I): VolumeAliases {
    const message = createBaseVolumeAliases();
    message.from = object.from?.map((e) => e) || [];
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
