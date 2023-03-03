/* eslint-disable */
import {
  ExecControllerRequest,
  ExecControllerResponse,
} from "@go/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.js";
import { BlockRef } from "@go/github.com/aperturerobotics/hydra/block/block.pb.js";
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import { VolumeInfo } from "@go/github.com/aperturerobotics/hydra/volume/volume.pb.js";
import { Timestamp } from "@go/github.com/aperturerobotics/timestamp/timestamp.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.plugin";

/** PluginStatus holds basic status for a plugin. */
export interface PluginStatus {
  /** PluginId is the plugin identifier. */
  pluginId: string;
  /** Running indicates the plugin is running. */
  running: boolean;
}

/** PluginManifestMeta is basic metadata about a manifest or bundle. */
export interface PluginManifestMeta {
  /** PluginId is the plugin identifier. */
  pluginId: string;
  /**
   * BuildType is the type of build this is.
   * Usually "development" or "production".
   */
  buildType: string;
  /** PluginPlatformId is the plugin platform ID. */
  pluginPlatformId: string;
  /**
   * Rev is the revision number of the manifest.
   * Higher revision numbers take priority over lower.
   * The number is incremented with each manifest build.
   */
  rev: Long;
}

/**
 * PluginManifest contains the metadata and contents for a plugin version.
 * The Manifest represents a specific version for one target architecture.
 */
export interface PluginManifest {
  /** Meta is the plugin manifest metadata. */
  meta:
    | PluginManifestMeta
    | undefined;
  /** Entrypoint is the path in the dist fs to the entrypoint binary. */
  entrypoint: string;
  /**
   * DistFsRef references a UnixFS FS_NODE containing plugin dist binaries.
   * Usually contains the entrypoint binary and needed shared libraries.
   */
  distFsRef:
    | BlockRef
    | undefined;
  /**
   * AssetsFsRef references a UnixFS FS_NODE containing plugin assets.
   * The assets are not checked out to disk, but are available to the plugin.
   */
  assetsFsRef: BlockRef | undefined;
}

/** PluginManifestBundle contains the metadata for a bundle of PluginManifest. */
export interface PluginManifestBundle {
  /** PluginManifestRefs contains the set of manifest references. */
  pluginManifestRefs: PluginManifestRef[];
  /** Timestamp is the timestamp the bundle was created. */
  timestamp: Timestamp | undefined;
}

/** PluginManifestRef is a reference to a PluginManifest with some hints. */
export interface PluginManifestRef {
  /**
   * Meta is the plugin manifest metadata.
   * Must match the ManifestRef.Meta field.
   */
  meta:
    | PluginManifestMeta
    | undefined;
  /** ManifestRef is the reference to the plugin manifest. */
  manifestRef: ObjectRef | undefined;
}

/** GetPluginInfoRequest is a request to return the information for the current plugin. */
export interface GetPluginInfoRequest {
}

/** GetPluginInfoResponse is the response to the GetPluginInfo request. */
export interface GetPluginInfoResponse {
  /** PluginId is the plugin identifier. */
  pluginId: string;
  /** PluginManifest is the reference to the PluginManifest object. */
  pluginManifest:
    | ObjectRef
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

function createBasePluginManifestMeta(): PluginManifestMeta {
  return { pluginId: "", buildType: "", pluginPlatformId: "", rev: Long.UZERO };
}

export const PluginManifestMeta = {
  encode(message: PluginManifestMeta, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.buildType !== "") {
      writer.uint32(18).string(message.buildType);
    }
    if (message.pluginPlatformId !== "") {
      writer.uint32(26).string(message.pluginPlatformId);
    }
    if (!message.rev.isZero()) {
      writer.uint32(32).uint64(message.rev);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginManifestMeta {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginManifestMeta();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginId = reader.string();
          break;
        case 2:
          message.buildType = reader.string();
          break;
        case 3:
          message.pluginPlatformId = reader.string();
          break;
        case 4:
          message.rev = reader.uint64() as Long;
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginManifestMeta, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PluginManifestMeta | PluginManifestMeta[]>
      | Iterable<PluginManifestMeta | PluginManifestMeta[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginManifestMeta.encode(p).finish()];
        }
      } else {
        yield* [PluginManifestMeta.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginManifestMeta>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginManifestMeta> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginManifestMeta.decode(p)];
        }
      } else {
        yield* [PluginManifestMeta.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PluginManifestMeta {
    return {
      pluginId: isSet(object.pluginId) ? String(object.pluginId) : "",
      buildType: isSet(object.buildType) ? String(object.buildType) : "",
      pluginPlatformId: isSet(object.pluginPlatformId) ? String(object.pluginPlatformId) : "",
      rev: isSet(object.rev) ? Long.fromValue(object.rev) : Long.UZERO,
    };
  },

  toJSON(message: PluginManifestMeta): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.buildType !== undefined && (obj.buildType = message.buildType);
    message.pluginPlatformId !== undefined && (obj.pluginPlatformId = message.pluginPlatformId);
    message.rev !== undefined && (obj.rev = (message.rev || Long.UZERO).toString());
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginManifestMeta>, I>>(base?: I): PluginManifestMeta {
    return PluginManifestMeta.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PluginManifestMeta>, I>>(object: I): PluginManifestMeta {
    const message = createBasePluginManifestMeta();
    message.pluginId = object.pluginId ?? "";
    message.buildType = object.buildType ?? "";
    message.pluginPlatformId = object.pluginPlatformId ?? "";
    message.rev = (object.rev !== undefined && object.rev !== null) ? Long.fromValue(object.rev) : Long.UZERO;
    return message;
  },
};

function createBasePluginManifest(): PluginManifest {
  return { meta: undefined, entrypoint: "", distFsRef: undefined, assetsFsRef: undefined };
}

export const PluginManifest = {
  encode(message: PluginManifest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.meta !== undefined) {
      PluginManifestMeta.encode(message.meta, writer.uint32(10).fork()).ldelim();
    }
    if (message.entrypoint !== "") {
      writer.uint32(18).string(message.entrypoint);
    }
    if (message.distFsRef !== undefined) {
      BlockRef.encode(message.distFsRef, writer.uint32(26).fork()).ldelim();
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
          message.meta = PluginManifestMeta.decode(reader, reader.uint32());
          break;
        case 2:
          message.entrypoint = reader.string();
          break;
        case 3:
          message.distFsRef = BlockRef.decode(reader, reader.uint32());
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
      meta: isSet(object.meta) ? PluginManifestMeta.fromJSON(object.meta) : undefined,
      entrypoint: isSet(object.entrypoint) ? String(object.entrypoint) : "",
      distFsRef: isSet(object.distFsRef) ? BlockRef.fromJSON(object.distFsRef) : undefined,
      assetsFsRef: isSet(object.assetsFsRef) ? BlockRef.fromJSON(object.assetsFsRef) : undefined,
    };
  },

  toJSON(message: PluginManifest): unknown {
    const obj: any = {};
    message.meta !== undefined && (obj.meta = message.meta ? PluginManifestMeta.toJSON(message.meta) : undefined);
    message.entrypoint !== undefined && (obj.entrypoint = message.entrypoint);
    message.distFsRef !== undefined &&
      (obj.distFsRef = message.distFsRef ? BlockRef.toJSON(message.distFsRef) : undefined);
    message.assetsFsRef !== undefined &&
      (obj.assetsFsRef = message.assetsFsRef ? BlockRef.toJSON(message.assetsFsRef) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginManifest>, I>>(base?: I): PluginManifest {
    return PluginManifest.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PluginManifest>, I>>(object: I): PluginManifest {
    const message = createBasePluginManifest();
    message.meta = (object.meta !== undefined && object.meta !== null)
      ? PluginManifestMeta.fromPartial(object.meta)
      : undefined;
    message.entrypoint = object.entrypoint ?? "";
    message.distFsRef = (object.distFsRef !== undefined && object.distFsRef !== null)
      ? BlockRef.fromPartial(object.distFsRef)
      : undefined;
    message.assetsFsRef = (object.assetsFsRef !== undefined && object.assetsFsRef !== null)
      ? BlockRef.fromPartial(object.assetsFsRef)
      : undefined;
    return message;
  },
};

function createBasePluginManifestBundle(): PluginManifestBundle {
  return { pluginManifestRefs: [], timestamp: undefined };
}

export const PluginManifestBundle = {
  encode(message: PluginManifestBundle, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.pluginManifestRefs) {
      PluginManifestRef.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginManifestBundle {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginManifestBundle();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginManifestRefs.push(PluginManifestRef.decode(reader, reader.uint32()));
          break;
        case 2:
          message.timestamp = Timestamp.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginManifestBundle, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PluginManifestBundle | PluginManifestBundle[]>
      | Iterable<PluginManifestBundle | PluginManifestBundle[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginManifestBundle.encode(p).finish()];
        }
      } else {
        yield* [PluginManifestBundle.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginManifestBundle>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginManifestBundle> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginManifestBundle.decode(p)];
        }
      } else {
        yield* [PluginManifestBundle.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PluginManifestBundle {
    return {
      pluginManifestRefs: Array.isArray(object?.pluginManifestRefs)
        ? object.pluginManifestRefs.map((e: any) => PluginManifestRef.fromJSON(e))
        : [],
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: PluginManifestBundle): unknown {
    const obj: any = {};
    if (message.pluginManifestRefs) {
      obj.pluginManifestRefs = message.pluginManifestRefs.map((e) => e ? PluginManifestRef.toJSON(e) : undefined);
    } else {
      obj.pluginManifestRefs = [];
    }
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginManifestBundle>, I>>(base?: I): PluginManifestBundle {
    return PluginManifestBundle.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PluginManifestBundle>, I>>(object: I): PluginManifestBundle {
    const message = createBasePluginManifestBundle();
    message.pluginManifestRefs = object.pluginManifestRefs?.map((e) => PluginManifestRef.fromPartial(e)) || [];
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBasePluginManifestRef(): PluginManifestRef {
  return { meta: undefined, manifestRef: undefined };
}

export const PluginManifestRef = {
  encode(message: PluginManifestRef, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.meta !== undefined) {
      PluginManifestMeta.encode(message.meta, writer.uint32(10).fork()).ldelim();
    }
    if (message.manifestRef !== undefined) {
      ObjectRef.encode(message.manifestRef, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginManifestRef {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginManifestRef();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.meta = PluginManifestMeta.decode(reader, reader.uint32());
          break;
        case 2:
          message.manifestRef = ObjectRef.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginManifestRef, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PluginManifestRef | PluginManifestRef[]> | Iterable<PluginManifestRef | PluginManifestRef[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginManifestRef.encode(p).finish()];
        }
      } else {
        yield* [PluginManifestRef.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginManifestRef>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginManifestRef> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PluginManifestRef.decode(p)];
        }
      } else {
        yield* [PluginManifestRef.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PluginManifestRef {
    return {
      meta: isSet(object.meta) ? PluginManifestMeta.fromJSON(object.meta) : undefined,
      manifestRef: isSet(object.manifestRef) ? ObjectRef.fromJSON(object.manifestRef) : undefined,
    };
  },

  toJSON(message: PluginManifestRef): unknown {
    const obj: any = {};
    message.meta !== undefined && (obj.meta = message.meta ? PluginManifestMeta.toJSON(message.meta) : undefined);
    message.manifestRef !== undefined &&
      (obj.manifestRef = message.manifestRef ? ObjectRef.toJSON(message.manifestRef) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginManifestRef>, I>>(base?: I): PluginManifestRef {
    return PluginManifestRef.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<PluginManifestRef>, I>>(object: I): PluginManifestRef {
    const message = createBasePluginManifestRef();
    message.meta = (object.meta !== undefined && object.meta !== null)
      ? PluginManifestMeta.fromPartial(object.meta)
      : undefined;
    message.manifestRef = (object.manifestRef !== undefined && object.manifestRef !== null)
      ? ObjectRef.fromPartial(object.manifestRef)
      : undefined;
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
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetPluginInfoRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        default:
          reader.skipType(tag & 7);
          break;
      }
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
  return { pluginId: "", pluginManifest: undefined, hostVolumeInfo: undefined };
}

export const GetPluginInfoResponse = {
  encode(message: GetPluginInfoResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginId !== "") {
      writer.uint32(10).string(message.pluginId);
    }
    if (message.pluginManifest !== undefined) {
      ObjectRef.encode(message.pluginManifest, writer.uint32(18).fork()).ldelim();
    }
    if (message.hostVolumeInfo !== undefined) {
      VolumeInfo.encode(message.hostVolumeInfo, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetPluginInfoResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetPluginInfoResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pluginId = reader.string();
          break;
        case 2:
          message.pluginManifest = ObjectRef.decode(reader, reader.uint32());
          break;
        case 3:
          message.hostVolumeInfo = VolumeInfo.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
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
      pluginManifest: isSet(object.pluginManifest) ? ObjectRef.fromJSON(object.pluginManifest) : undefined,
      hostVolumeInfo: isSet(object.hostVolumeInfo) ? VolumeInfo.fromJSON(object.hostVolumeInfo) : undefined,
    };
  },

  toJSON(message: GetPluginInfoResponse): unknown {
    const obj: any = {};
    message.pluginId !== undefined && (obj.pluginId = message.pluginId);
    message.pluginManifest !== undefined &&
      (obj.pluginManifest = message.pluginManifest ? ObjectRef.toJSON(message.pluginManifest) : undefined);
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
    message.pluginManifest = (object.pluginManifest !== undefined && object.pluginManifest !== null)
      ? ObjectRef.fromPartial(object.pluginManifest)
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

  create<I extends Exact<DeepPartial<FetchPluginRequest>, I>>(base?: I): FetchPluginRequest {
    return FetchPluginRequest.fromPartial(base ?? {});
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

  create<I extends Exact<DeepPartial<FetchPluginResponse>, I>>(base?: I): FetchPluginResponse {
    return FetchPluginResponse.fromPartial(base ?? {});
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
  /** GetPluginInfo returns the information for the current plugin. */
  GetPluginInfo(request: GetPluginInfoRequest, abortSignal?: AbortSignal): Promise<GetPluginInfoResponse>;
  /**
   * LoadPlugin requests to load the plugin with the given ID.
   * The plugin will remain loaded as long as the RPC is active.
   * Multiple requests to load the same plugin are de-duplicated.
   */
  LoadPlugin(request: LoadPluginRequest, abortSignal?: AbortSignal): AsyncIterable<LoadPluginResponse>;
  /** ExecController executes a controller configuration on the bus. */
  ExecController(request: ExecControllerRequest, abortSignal?: AbortSignal): AsyncIterable<ExecControllerResponse>;
}

export class PluginHostClientImpl implements PluginHost {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "bldr.plugin.PluginHost";
    this.rpc = rpc;
    this.GetPluginInfo = this.GetPluginInfo.bind(this);
    this.LoadPlugin = this.LoadPlugin.bind(this);
    this.ExecController = this.ExecController.bind(this);
  }
  GetPluginInfo(request: GetPluginInfoRequest, abortSignal?: AbortSignal): Promise<GetPluginInfoResponse> {
    const data = GetPluginInfoRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "GetPluginInfo", data, abortSignal || undefined);
    return promise.then((data) => GetPluginInfoResponse.decode(new _m0.Reader(data)));
  }

  LoadPlugin(request: LoadPluginRequest, abortSignal?: AbortSignal): AsyncIterable<LoadPluginResponse> {
    const data = LoadPluginRequest.encode(request).finish();
    const result = this.rpc.serverStreamingRequest(this.service, "LoadPlugin", data, abortSignal || undefined);
    return LoadPluginResponse.decodeTransform(result);
  }

  ExecController(request: ExecControllerRequest, abortSignal?: AbortSignal): AsyncIterable<ExecControllerResponse> {
    const data = ExecControllerRequest.encode(request).finish();
    const result = this.rpc.serverStreamingRequest(this.service, "ExecController", data, abortSignal || undefined);
    return ExecControllerResponse.decodeTransform(result);
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
    /** ExecController executes a controller configuration on the bus. */
    execController: {
      name: "ExecController",
      requestType: ExecControllerRequest,
      requestStream: false,
      responseType: ExecControllerResponse,
      responseStream: true,
      options: {},
    },
  },
} as const;

/** PluginFetch is a service that fetches plugin manifests by ID. */
export interface PluginFetch {
  /** FetchPlugin requests the plugin binary for the given plugin id. */
  FetchPlugin(request: FetchPluginRequest, abortSignal?: AbortSignal): Promise<FetchPluginResponse>;
}

export class PluginFetchClientImpl implements PluginFetch {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "bldr.plugin.PluginFetch";
    this.rpc = rpc;
    this.FetchPlugin = this.FetchPlugin.bind(this);
  }
  FetchPlugin(request: FetchPluginRequest, abortSignal?: AbortSignal): Promise<FetchPluginResponse> {
    const data = FetchPluginRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "FetchPlugin", data, abortSignal || undefined);
    return promise.then((data) => FetchPluginResponse.decode(new _m0.Reader(data)));
  }
}

/** PluginFetch is a service that fetches plugin manifests by ID. */
export type PluginFetchDefinition = typeof PluginFetchDefinition;
export const PluginFetchDefinition = {
  name: "PluginFetch",
  fullName: "bldr.plugin.PluginFetch",
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
