/* eslint-disable */
import { BlockRef } from "@go/github.com/aperturerobotics/hydra/block/block.pb.js";
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "plugin";

/** PluginStatus holds basic status for a plugin. */
export interface PluginStatus {
  /** PluginId is the plugin identifier. */
  pluginId: string;
  /** Running indicates the plugin is running. */
  running: boolean;
}

/**
 * PluginManifest contains the metadata and contents for a plugin version.
 * The Manifest represents a specific version for one target architecture.
 */
export interface PluginManifest {
  /** PluginId is the plugin identifier. */
  pluginId: string;
  /**
   * DistFsRef references a UnixFS FS_NODE containing plugin dist binaries.
   * Usually contains the entrypoint binary and needed shared libraries.
   */
  distFsRef:
    | BlockRef
    | undefined;
  /** Entrypoint is the path in the dist fs to the entrypoint binary. */
  entrypoint: string;
  /**
   * AssetsFsRef references a UnixFS FS_NODE containing plugin assets.
   * The assets are not checked out to disk, but are available to the plugin.
   */
  assetsFsRef: BlockRef | undefined;
}

/** LoadPluginRequest is a request to load a plugin while the RPC is active. */
export interface LoadPluginRequest {
  /** PluginId is the plugin identifier to load. */
  pluginId: string;
}

/** LoadPluginResponse is a status response to a LoadPlugin request. */
export interface LoadPluginResponse {
  /** PluginStatus contains the current plugin status object. */
  pluginStatus: PluginStatus | undefined;
}

/** FetchPluginRequest is a request to fetch a plugin binary. */
export interface FetchPluginRequest {
  /** PluginId is the plugin identifier to load. */
  pluginId: string;
}

/** FetchPluginResponse is a response to a FetchPlugin request. */
export interface FetchPluginResponse {
  /** PluginManifest is the root reference to the PluginManifest. */
  pluginManifest: ObjectRef | undefined;
}

function createBasePluginStatus(): PluginStatus {
  return { pluginId: "", running: false };
}

export const PluginStatus = {
  encode(message: PluginStatus, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.running === true) {
      writer.uint32(16).bool(message.running);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginStatus {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginStatus();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginId = reader.string();
          break;
        case 2:
          message.running = reader.bool();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginStatus, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PluginStatus | PluginStatus[]> | Iterable<PluginStatus | PluginStatus[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginStatus.encode(p).finish()];
        }
      } else {
        yield* [PluginStatus.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginStatus>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginStatus> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginStatus.decode(p)];
        }
      } else {
        yield* [PluginStatus.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PluginStatus {
    return {
      pluginId: isSet(object.pluginId) ? String(object.pluginId) : "",
      running: isSet(object.running) ? Boolean(object.running) : false,
    };
  },

  toJSON(message: PluginStatus): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.running !== undefined && (obj.running = message.running);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<PluginStatus>, I>>(object: I): PluginStatus {
    const message = createBasePluginStatus();
    message.pluginId = object.pluginId ?? "";
    message.running = object.running ?? false;
    return message;
  },
};

function createBasePluginManifest(): PluginManifest {
  return { pluginId: "", distFsRef: undefined, entrypoint: "", assetsFsRef: undefined };
}

export const PluginManifest = {
  encode(message: PluginManifest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.distFsRef !== undefined) {
      BlockRef.encode(message.distFsRef, writer.uint32(18).fork()).ldelim();
    }
    if (message.entrypoint !== "") {
      writer.uint32(26).string(message.entrypoint);
    }
    if (message.assetsFsRef !== undefined) {
      BlockRef.encode(message.assetsFsRef, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginManifest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginManifest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginId = reader.string();
          break;
        case 2:
          message.distFsRef = BlockRef.decode(reader, reader.uint32());
          break;
        case 3:
          message.entrypoint = reader.string();
          break;
        case 4:
          message.assetsFsRef = BlockRef.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginManifest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PluginManifest | PluginManifest[]> | Iterable<PluginManifest | PluginManifest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginManifest.encode(p).finish()];
        }
      } else {
        yield* [PluginManifest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginManifest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginManifest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginManifest.decode(p)];
        }
      } else {
        yield* [PluginManifest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PluginManifest {
    return {
      pluginId: isSet(object.pluginId) ? String(object.pluginId) : "",
      distFsRef: isSet(object.distFsRef) ? BlockRef.fromJSON(object.distFsRef) : undefined,
      entrypoint: isSet(object.entrypoint) ? String(object.entrypoint) : "",
      assetsFsRef: isSet(object.assetsFsRef) ? BlockRef.fromJSON(object.assetsFsRef) : undefined,
    };
  },

  toJSON(message: PluginManifest): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.distFsRef !== undefined &&
      (obj.distFsRef = message.distFsRef ? BlockRef.toJSON(message.distFsRef) : undefined);
    message.entrypoint !== undefined && (obj.entrypoint = message.entrypoint);
    message.assetsFsRef !== undefined &&
      (obj.assetsFsRef = message.assetsFsRef ? BlockRef.toJSON(message.assetsFsRef) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<PluginManifest>, I>>(object: I): PluginManifest {
    const message = createBasePluginManifest();
    message.pluginId = object.pluginId ?? "";
    message.distFsRef = (object.distFsRef !== undefined && object.distFsRef !== null)
      ? BlockRef.fromPartial(object.distFsRef)
      : undefined;
    message.entrypoint = object.entrypoint ?? "";
    message.assetsFsRef = (object.assetsFsRef !== undefined && object.assetsFsRef !== null)
      ? BlockRef.fromPartial(object.assetsFsRef)
      : undefined;
    return message;
  },
};

function createBaseLoadPluginRequest(): LoadPluginRequest {
  return { pluginId: "" };
}

export const LoadPluginRequest = {
  encode(message: LoadPluginRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): LoadPluginRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLoadPluginRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<LoadPluginRequest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<LoadPluginRequest | LoadPluginRequest[]> | Iterable<LoadPluginRequest | LoadPluginRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LoadPluginRequest.encode(p).finish()];
        }
      } else {
        yield* [LoadPluginRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, LoadPluginRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<LoadPluginRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LoadPluginRequest.decode(p)];
        }
      } else {
        yield* [LoadPluginRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): LoadPluginRequest {
    return { pluginId: isSet(object.pluginId) ? String(object.pluginId) : "" };
  },

  toJSON(message: LoadPluginRequest): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<LoadPluginRequest>, I>>(object: I): LoadPluginRequest {
    const message = createBaseLoadPluginRequest();
    message.pluginId = object.pluginId ?? "";
    return message;
  },
};

function createBaseLoadPluginResponse(): LoadPluginResponse {
  return { pluginStatus: undefined };
}

export const LoadPluginResponse = {
  encode(message: LoadPluginResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginStatus !== undefined) {
      PluginStatus.encode(message.pluginStatus, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): LoadPluginResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLoadPluginResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginStatus = PluginStatus.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<LoadPluginResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<LoadPluginResponse | LoadPluginResponse[]>
      | Iterable<LoadPluginResponse | LoadPluginResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LoadPluginResponse.encode(p).finish()];
        }
      } else {
        yield* [LoadPluginResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, LoadPluginResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<LoadPluginResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LoadPluginResponse.decode(p)];
        }
      } else {
        yield* [LoadPluginResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): LoadPluginResponse {
    return { pluginStatus: isSet(object.pluginStatus) ? PluginStatus.fromJSON(object.pluginStatus) : undefined };
  },

  toJSON(message: LoadPluginResponse): unknown {
    const obj: any = {};
    message.pluginStatus !== undefined &&
      (obj.pluginStatus = message.pluginStatus ? PluginStatus.toJSON(message.pluginStatus) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<LoadPluginResponse>, I>>(object: I): LoadPluginResponse {
    const message = createBaseLoadPluginResponse();
    message.pluginStatus = (object.pluginStatus !== undefined && object.pluginStatus !== null)
      ? PluginStatus.fromPartial(object.pluginStatus)
      : undefined;
    return message;
  },
};

function createBaseFetchPluginRequest(): FetchPluginRequest {
  return { pluginId: "" };
}

export const FetchPluginRequest = {
  encode(message: FetchPluginRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FetchPluginRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFetchPluginRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FetchPluginRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FetchPluginRequest | FetchPluginRequest[]>
      | Iterable<FetchPluginRequest | FetchPluginRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchPluginRequest.encode(p).finish()];
        }
      } else {
        yield* [FetchPluginRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchPluginRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FetchPluginRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchPluginRequest.decode(p)];
        }
      } else {
        yield* [FetchPluginRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FetchPluginRequest {
    return { pluginId: isSet(object.pluginId) ? String(object.pluginId) : "" };
  },

  toJSON(message: FetchPluginRequest): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FetchPluginRequest>, I>>(object: I): FetchPluginRequest {
    const message = createBaseFetchPluginRequest();
    message.pluginId = object.pluginId ?? "";
    return message;
  },
};

function createBaseFetchPluginResponse(): FetchPluginResponse {
  return { pluginManifest: undefined };
}

export const FetchPluginResponse = {
  encode(message: FetchPluginResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginManifest !== undefined) {
      ObjectRef.encode(message.pluginManifest, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FetchPluginResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFetchPluginResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginManifest = ObjectRef.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FetchPluginResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FetchPluginResponse | FetchPluginResponse[]>
      | Iterable<FetchPluginResponse | FetchPluginResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchPluginResponse.encode(p).finish()];
        }
      } else {
        yield* [FetchPluginResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchPluginResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FetchPluginResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchPluginResponse.decode(p)];
        }
      } else {
        yield* [FetchPluginResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FetchPluginResponse {
    return { pluginManifest: isSet(object.pluginManifest) ? ObjectRef.fromJSON(object.pluginManifest) : undefined };
  },

  toJSON(message: FetchPluginResponse): unknown {
    const obj: any = {};
    message.pluginManifest !== undefined &&
      (obj.pluginManifest = message.pluginManifest ? ObjectRef.toJSON(message.pluginManifest) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FetchPluginResponse>, I>>(object: I): FetchPluginResponse {
    const message = createBaseFetchPluginResponse();
    message.pluginManifest = (object.pluginManifest !== undefined && object.pluginManifest !== null)
      ? ObjectRef.fromPartial(object.pluginManifest)
      : undefined;
    return message;
  },
};

/** PluginHost is the service exposed by the plugin host. */
export interface PluginHost {
  /**
   * LoadPlugin requests to load the plugin with the given ID.
   * The plugin will remain loaded as long as the RPC is active.
   * Multiple requests to load the same plugin are de-duplicated.
   */
  LoadPlugin(request: LoadPluginRequest): AsyncIterable<LoadPluginResponse>;
}

export class PluginHostClientImpl implements PluginHost {
  private readonly rpc: Rpc;
  constructor(rpc: Rpc) {
    this.rpc = rpc;
    this.LoadPlugin = this.LoadPlugin.bind(this);
  }
  LoadPlugin(request: LoadPluginRequest): AsyncIterable<LoadPluginResponse> {
    const data = LoadPluginRequest.encode(request).finish();
    const result = this.rpc.serverStreamingRequest("plugin.PluginHost", "LoadPlugin", data);
    return LoadPluginResponse.decodeTransform(result);
  }
}

/** PluginHost is the service exposed by the plugin host. */
export type PluginHostDefinition = typeof PluginHostDefinition;
export const PluginHostDefinition = {
  name: "PluginHost",
  fullName: "plugin.PluginHost",
  methods: {
    /**
     * LoadPlugin requests to load the plugin with the given ID.
     * The plugin will remain loaded as long as the RPC is active.
     * Multiple requests to load the same plugin are de-duplicated.
     */
    loadPlugin: {
      name: "LoadPlugin",
      requestType: LoadPluginRequest,
      requestStream: false,
      responseType: LoadPluginResponse,
      responseStream: true,
      options: {},
    },
  },
} as const;

/** PluginFetch is a service that fetches plugin manifests by ID. */
export interface PluginFetch {
  /** FetchPlugin requests the plugin binary for the given plugin id. */
  FetchPlugin(request: FetchPluginRequest): Promise<FetchPluginResponse>;
}

export class PluginFetchClientImpl implements PluginFetch {
  private readonly rpc: Rpc;
  constructor(rpc: Rpc) {
    this.rpc = rpc;
    this.FetchPlugin = this.FetchPlugin.bind(this);
  }
  FetchPlugin(request: FetchPluginRequest): Promise<FetchPluginResponse> {
    const data = FetchPluginRequest.encode(request).finish();
    const promise = this.rpc.request("plugin.PluginFetch", "FetchPlugin", data);
    return promise.then((data) => FetchPluginResponse.decode(new _m0.Reader(data)));
  }
}

/** PluginFetch is a service that fetches plugin manifests by ID. */
export type PluginFetchDefinition = typeof PluginFetchDefinition;
export const PluginFetchDefinition = {
  name: "PluginFetch",
  fullName: "plugin.PluginFetch",
  methods: {
    /** FetchPlugin requests the plugin binary for the given plugin id. */
    fetchPlugin: {
      name: "FetchPlugin",
      requestType: FetchPluginRequest,
      requestStream: false,
      responseType: FetchPluginResponse,
      responseStream: false,
      options: {},
    },
  },
} as const;

interface Rpc {
  request(service: string, method: string, data: Uint8Array): Promise<Uint8Array>;
  clientStreamingRequest(service: string, method: string, data: AsyncIterable<Uint8Array>): Promise<Uint8Array>;
  serverStreamingRequest(service: string, method: string, data: Uint8Array): AsyncIterable<Uint8Array>;
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
  ): AsyncIterable<Uint8Array>;
}

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
