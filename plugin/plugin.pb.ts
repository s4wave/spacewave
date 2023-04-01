/* eslint-disable */
import {
  ExecControllerRequest,
  ExecControllerResponse,
} from "@go/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.js";
import { VolumeInfo } from "@go/github.com/aperturerobotics/hydra/volume/volume.pb.js";
import { RpcStreamPacket } from "@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { ManifestRef } from "../manifest/manifest.pb.js";

export const protobufPackage = "bldr.plugin";

/** PluginStatus holds basic status for a plugin. */
export interface PluginStatus {
  /** PluginId is the plugin identifier. */
  pluginId: string;
  /** Running indicates the plugin is running. */
  running: boolean;
}

/** GetPluginInfoRequest is a request to return the information for the current plugin. */
export interface GetPluginInfoRequest {
}

/** GetPluginInfoResponse is the response to the GetPluginInfo request. */
export interface GetPluginInfoResponse {
  /** PluginId is the plugin identifier. */
  pluginId: string;
  /** ManifestRef is the reference to the Manifest object. */
  manifestRef:
    | ManifestRef
    | undefined;
  /**
   * HostVolumeInfo is the information for the host Volume.
   * The volume is exposed with a ProxyVolume.
   */
  hostVolumeInfo: VolumeInfo | undefined;
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginStatus();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.pluginId = reader.string();
          continue;
        case 2:
          if (tag != 16) {
            break;
          }

          message.running = reader.bool();
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
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

  create<I extends Exact<DeepPartial<PluginStatus>, I>>(base?: I): PluginStatus {
    return PluginStatus.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PluginStatus>, I>>(object: I): PluginStatus {
    const message = createBasePluginStatus();
    message.pluginId = object.pluginId ?? "";
    message.running = object.running ?? false;
    return message;
  },
};

function createBaseGetPluginInfoRequest(): GetPluginInfoRequest {
  return {};
}

export const GetPluginInfoRequest = {
  encode(_: GetPluginInfoRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetPluginInfoRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetPluginInfoRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetPluginInfoRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetPluginInfoRequest | GetPluginInfoRequest[]>
      | Iterable<GetPluginInfoRequest | GetPluginInfoRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetPluginInfoRequest.encode(p).finish()];
        }
      } else {
        yield* [GetPluginInfoRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetPluginInfoRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetPluginInfoRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetPluginInfoRequest.decode(p)];
        }
      } else {
        yield* [GetPluginInfoRequest.decode(pkt)];
      }
    }
  },

  fromJSON(_: any): GetPluginInfoRequest {
    return {};
  },

  toJSON(_: GetPluginInfoRequest): unknown {
    const obj: any = {};
    return obj;
  },

  create<I extends Exact<DeepPartial<GetPluginInfoRequest>, I>>(base?: I): GetPluginInfoRequest {
    return GetPluginInfoRequest.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<GetPluginInfoRequest>, I>>(_: I): GetPluginInfoRequest {
    const message = createBaseGetPluginInfoRequest();
    return message;
  },
};

function createBaseGetPluginInfoResponse(): GetPluginInfoResponse {
  return { pluginId: "", manifestRef: undefined, hostVolumeInfo: undefined };
}

export const GetPluginInfoResponse = {
  encode(message: GetPluginInfoResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.manifestRef !== undefined) {
      ManifestRef.encode(message.manifestRef, writer.uint32(18).fork()).ldelim();
    }
    if (message.hostVolumeInfo !== undefined) {
      VolumeInfo.encode(message.hostVolumeInfo, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetPluginInfoResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetPluginInfoResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.pluginId = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.manifestRef = ManifestRef.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.hostVolumeInfo = VolumeInfo.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetPluginInfoResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetPluginInfoResponse | GetPluginInfoResponse[]>
      | Iterable<GetPluginInfoResponse | GetPluginInfoResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetPluginInfoResponse.encode(p).finish()];
        }
      } else {
        yield* [GetPluginInfoResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetPluginInfoResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetPluginInfoResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GetPluginInfoResponse.decode(p)];
        }
      } else {
        yield* [GetPluginInfoResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): GetPluginInfoResponse {
    return {
      pluginId: isSet(object.pluginId) ? String(object.pluginId) : "",
      manifestRef: isSet(object.manifestRef) ? ManifestRef.fromJSON(object.manifestRef) : undefined,
      hostVolumeInfo: isSet(object.hostVolumeInfo) ? VolumeInfo.fromJSON(object.hostVolumeInfo) : undefined,
    };
  },

  toJSON(message: GetPluginInfoResponse): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.manifestRef !== undefined &&
      (obj.manifestRef = message.manifestRef ? ManifestRef.toJSON(message.manifestRef) : undefined);
    message.hostVolumeInfo !== undefined &&
      (obj.hostVolumeInfo = message.hostVolumeInfo ? VolumeInfo.toJSON(message.hostVolumeInfo) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<GetPluginInfoResponse>, I>>(base?: I): GetPluginInfoResponse {
    return GetPluginInfoResponse.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<GetPluginInfoResponse>, I>>(object: I): GetPluginInfoResponse {
    const message = createBaseGetPluginInfoResponse();
    message.pluginId = object.pluginId ?? "";
    message.manifestRef = (object.manifestRef !== undefined && object.manifestRef !== null)
      ? ManifestRef.fromPartial(object.manifestRef)
      : undefined;
    message.hostVolumeInfo = (object.hostVolumeInfo !== undefined && object.hostVolumeInfo !== null)
      ? VolumeInfo.fromPartial(object.hostVolumeInfo)
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLoadPluginRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.pluginId = reader.string();
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
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

  create<I extends Exact<DeepPartial<LoadPluginRequest>, I>>(base?: I): LoadPluginRequest {
    return LoadPluginRequest.fromPartial(base ?? {});
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
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLoadPluginResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.pluginStatus = PluginStatus.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
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

  create<I extends Exact<DeepPartial<LoadPluginResponse>, I>>(base?: I): LoadPluginResponse {
    return LoadPluginResponse.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<LoadPluginResponse>, I>>(object: I): LoadPluginResponse {
    const message = createBaseLoadPluginResponse();
    message.pluginStatus = (object.pluginStatus !== undefined && object.pluginStatus !== null)
      ? PluginStatus.fromPartial(object.pluginStatus)
      : undefined;
    return message;
  },
};

/** PluginHost is the service exposed by the plugin host. */
export interface PluginHost {
  /** GetPluginInfo returns the information for the current plugin. */
  GetPluginInfo(request: GetPluginInfoRequest, abortSignal?: AbortSignal): Promise<GetPluginInfoResponse>;
  /** ExecController executes a controller configuration on the bus. */
  ExecController(request: ExecControllerRequest, abortSignal?: AbortSignal): AsyncIterable<ExecControllerResponse>;
  /**
   * LoadPlugin requests to load the plugin with the given ID.
   * The plugin will remain loaded as long as the RPC is active.
   * Multiple requests to load the same plugin are de-duplicated.
   */
  LoadPlugin(request: LoadPluginRequest, abortSignal?: AbortSignal): AsyncIterable<LoadPluginResponse>;
  /**
   * PluginRpc forwards an RPC call to a remote plugin.
   * The plugin will remain loaded as long as the RPC is active.
   * Component ID: plugin id
   */
  PluginRpc(request: AsyncIterable<RpcStreamPacket>, abortSignal?: AbortSignal): AsyncIterable<RpcStreamPacket>;
}

export class PluginHostClientImpl implements PluginHost {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "bldr.plugin.PluginHost";
    this.rpc = rpc;
    this.GetPluginInfo = this.GetPluginInfo.bind(this);
    this.ExecController = this.ExecController.bind(this);
    this.LoadPlugin = this.LoadPlugin.bind(this);
    this.PluginRpc = this.PluginRpc.bind(this);
  }
  GetPluginInfo(request: GetPluginInfoRequest, abortSignal?: AbortSignal): Promise<GetPluginInfoResponse> {
    const data = GetPluginInfoRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "GetPluginInfo", data, abortSignal || undefined);
    return promise.then((data) => GetPluginInfoResponse.decode(_m0.Reader.create(data)));
  }

  ExecController(request: ExecControllerRequest, abortSignal?: AbortSignal): AsyncIterable<ExecControllerResponse> {
    const data = ExecControllerRequest.encode(request).finish();
    const result = this.rpc.serverStreamingRequest(this.service, "ExecController", data, abortSignal || undefined);
    return ExecControllerResponse.decodeTransform(result);
  }

  LoadPlugin(request: LoadPluginRequest, abortSignal?: AbortSignal): AsyncIterable<LoadPluginResponse> {
    const data = LoadPluginRequest.encode(request).finish();
    const result = this.rpc.serverStreamingRequest(this.service, "LoadPlugin", data, abortSignal || undefined);
    return LoadPluginResponse.decodeTransform(result);
  }

  PluginRpc(request: AsyncIterable<RpcStreamPacket>, abortSignal?: AbortSignal): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request);
    const result = this.rpc.bidirectionalStreamingRequest(this.service, "PluginRpc", data, abortSignal || undefined);
    return RpcStreamPacket.decodeTransform(result);
  }
}

/** PluginHost is the service exposed by the plugin host. */
export type PluginHostDefinition = typeof PluginHostDefinition;
export const PluginHostDefinition = {
  name: "PluginHost",
  fullName: "bldr.plugin.PluginHost",
  methods: {
    /** GetPluginInfo returns the information for the current plugin. */
    getPluginInfo: {
      name: "GetPluginInfo",
      requestType: GetPluginInfoRequest,
      requestStream: false,
      responseType: GetPluginInfoResponse,
      responseStream: false,
      options: {},
    },
    /** ExecController executes a controller configuration on the bus. */
    execController: {
      name: "ExecController",
      requestType: ExecControllerRequest,
      requestStream: false,
      responseType: ExecControllerResponse,
      responseStream: true,
      options: {},
    },
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
    /**
     * PluginRpc forwards an RPC call to a remote plugin.
     * The plugin will remain loaded as long as the RPC is active.
     * Component ID: plugin id
     */
    pluginRpc: {
      name: "PluginRpc",
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
  },
} as const;

/** Plugin is the service exposed by the plugin. */
export interface Plugin {
  /**
   * PluginRpc handles an RPC call from a remote plugin.
   * Component ID: remote plugin id
   */
  PluginRpc(request: AsyncIterable<RpcStreamPacket>, abortSignal?: AbortSignal): AsyncIterable<RpcStreamPacket>;
}

export class PluginClientImpl implements Plugin {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "bldr.plugin.Plugin";
    this.rpc = rpc;
    this.PluginRpc = this.PluginRpc.bind(this);
  }
  PluginRpc(request: AsyncIterable<RpcStreamPacket>, abortSignal?: AbortSignal): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request);
    const result = this.rpc.bidirectionalStreamingRequest(this.service, "PluginRpc", data, abortSignal || undefined);
    return RpcStreamPacket.decodeTransform(result);
  }
}

/** Plugin is the service exposed by the plugin. */
export type PluginDefinition = typeof PluginDefinition;
export const PluginDefinition = {
  name: "Plugin",
  fullName: "bldr.plugin.Plugin",
  methods: {
    /**
     * PluginRpc handles an RPC call from a remote plugin.
     * Component ID: remote plugin id
     */
    pluginRpc: {
      name: "PluginRpc",
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
  },
} as const;

interface Rpc {
  request(service: string, method: string, data: Uint8Array, abortSignal?: AbortSignal): Promise<Uint8Array>;
  clientStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>;
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array>;
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
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
