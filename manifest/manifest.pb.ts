/* eslint-disable */
import { BlockRef } from "@go/github.com/aperturerobotics/hydra/block/block.pb.js";
import { ObjectRef } from "@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js";
import { Timestamp } from "@go/github.com/aperturerobotics/timestamp/timestamp.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "bldr.manifest";

/** ManifestMeta is basic metadata about a manifest. */
export interface ManifestMeta {
  /** ManifestId is the identifier of the manifest. */
  manifestId: string;
  /**
   * BuildType is the type of build this is.
   * Usually "development" or "production".
   */
  buildType: string;
  /** PlatformId is the target platform ID. */
  platformId: string;
  /**
   * Rev is the rev number of the manifest.
   * Higher revision numbers take priority over lower.
   * The number is incremented with each manifest build.
   */
  rev: Long;
}

/**
 * Manifest contains metadata and contents.
 * The Manifest represents a specific version for one target architecture.
 */
export interface Manifest {
  /** Meta is the manifest metadata. */
  meta:
    | ManifestMeta
    | undefined;
  /** Entrypoint is the path in the dist fs to the entrypoint binary. */
  entrypoint: string;
  /**
   * DistFsRef references a UnixFS FS_NODE containing distribution files.
   * Dist files are checked out to the disk when starting the plugin.
   */
  distFsRef:
    | BlockRef
    | undefined;
  /**
   * AssetsFsRef references a UnixFS FS_NODE containing assets.
   * Asset files are accessible by the plugin at runtime in-memory.
   */
  assetsFsRef: BlockRef | undefined;
}

/** ManifestRef is a reference to a Manifest with some hints. */
export interface ManifestRef {
  /**
   * Meta is the manifest metadata.
   * Must match the ManifestRef.Meta field.
   */
  meta:
    | ManifestMeta
    | undefined;
  /** ManifestRef is the reference to the manifest. */
  manifestRef: ObjectRef | undefined;
}

/** ManifestBundle contains the metadata for a bundle of Manifest. */
export interface ManifestBundle {
  /** ManifestRefs contains the set of manifest references. */
  manifestRefs: ManifestRef[];
  /** Timestamp is the timestamp the bundle was created. */
  timestamp: Timestamp | undefined;
}

/** FetchManifestRequest is a request to fetch a manifest binary. */
export interface FetchManifestRequest {
  /**
   * ManifestMeta is the metadata to fetch.
   * May be partially empty.
   */
  manifestMeta: ManifestMeta | undefined;
}

/** FetchManifestResponse is a response to a FetchManifest request. */
export interface FetchManifestResponse {
  /** ManifestRef is the reference to the Manifest. */
  manifestRef: ManifestRef | undefined;
}

function createBaseManifestMeta(): ManifestMeta {
  return { manifestId: "", buildType: "", platformId: "", rev: Long.UZERO };
}

export const ManifestMeta = {
  encode(message: ManifestMeta, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.manifestId !== "") {
      writer.uint32(10).string(message.manifestId);
    }
    if (message.buildType !== "") {
      writer.uint32(18).string(message.buildType);
    }
    if (message.platformId !== "") {
      writer.uint32(26).string(message.platformId);
    }
    if (!message.rev.isZero()) {
      writer.uint32(32).uint64(message.rev);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ManifestMeta {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseManifestMeta();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.manifestId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.buildType = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.platformId = reader.string();
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.rev = reader.uint64() as Long;
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
  // Transform<ManifestMeta, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ManifestMeta | ManifestMeta[]> | Iterable<ManifestMeta | ManifestMeta[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestMeta.encode(p).finish()];
        }
      } else {
        yield* [ManifestMeta.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ManifestMeta>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ManifestMeta> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestMeta.decode(p)];
        }
      } else {
        yield* [ManifestMeta.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): ManifestMeta {
    return {
      manifestId: isSet(object.manifestId) ? globalThis.String(object.manifestId) : "",
      buildType: isSet(object.buildType) ? globalThis.String(object.buildType) : "",
      platformId: isSet(object.platformId) ? globalThis.String(object.platformId) : "",
      rev: isSet(object.rev) ? Long.fromValue(object.rev) : Long.UZERO,
    };
  },

  toJSON(message: ManifestMeta): unknown {
    const obj: any = {};
    if (message.manifestId !== "") {
      obj.manifestId = message.manifestId;
    }
    if (message.buildType !== "") {
      obj.buildType = message.buildType;
    }
    if (message.platformId !== "") {
      obj.platformId = message.platformId;
    }
    if (!message.rev.isZero()) {
      obj.rev = (message.rev || Long.UZERO).toString();
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ManifestMeta>, I>>(base?: I): ManifestMeta {
    return ManifestMeta.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ManifestMeta>, I>>(object: I): ManifestMeta {
    const message = createBaseManifestMeta();
    message.manifestId = object.manifestId ?? "";
    message.buildType = object.buildType ?? "";
    message.platformId = object.platformId ?? "";
    message.rev = (object.rev !== undefined && object.rev !== null) ? Long.fromValue(object.rev) : Long.UZERO;
    return message;
  },
};

function createBaseManifest(): Manifest {
  return { meta: undefined, entrypoint: "", distFsRef: undefined, assetsFsRef: undefined };
}

export const Manifest = {
  encode(message: Manifest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.meta !== undefined) {
      ManifestMeta.encode(message.meta, writer.uint32(10).fork()).ldelim();
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

  decode(input: _m0.Reader | Uint8Array, length?: number): Manifest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseManifest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.meta = ManifestMeta.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.entrypoint = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.distFsRef = BlockRef.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.assetsFsRef = BlockRef.decode(reader, reader.uint32());
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
  // Transform<Manifest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Manifest | Manifest[]> | Iterable<Manifest | Manifest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Manifest.encode(p).finish()];
        }
      } else {
        yield* [Manifest.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Manifest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Manifest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Manifest.decode(p)];
        }
      } else {
        yield* [Manifest.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): Manifest {
    return {
      meta: isSet(object.meta) ? ManifestMeta.fromJSON(object.meta) : undefined,
      entrypoint: isSet(object.entrypoint) ? globalThis.String(object.entrypoint) : "",
      distFsRef: isSet(object.distFsRef) ? BlockRef.fromJSON(object.distFsRef) : undefined,
      assetsFsRef: isSet(object.assetsFsRef) ? BlockRef.fromJSON(object.assetsFsRef) : undefined,
    };
  },

  toJSON(message: Manifest): unknown {
    const obj: any = {};
    if (message.meta !== undefined) {
      obj.meta = ManifestMeta.toJSON(message.meta);
    }
    if (message.entrypoint !== "") {
      obj.entrypoint = message.entrypoint;
    }
    if (message.distFsRef !== undefined) {
      obj.distFsRef = BlockRef.toJSON(message.distFsRef);
    }
    if (message.assetsFsRef !== undefined) {
      obj.assetsFsRef = BlockRef.toJSON(message.assetsFsRef);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Manifest>, I>>(base?: I): Manifest {
    return Manifest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Manifest>, I>>(object: I): Manifest {
    const message = createBaseManifest();
    message.meta = (object.meta !== undefined && object.meta !== null)
      ? ManifestMeta.fromPartial(object.meta)
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

function createBaseManifestRef(): ManifestRef {
  return { meta: undefined, manifestRef: undefined };
}

export const ManifestRef = {
  encode(message: ManifestRef, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.meta !== undefined) {
      ManifestMeta.encode(message.meta, writer.uint32(10).fork()).ldelim();
    }
    if (message.manifestRef !== undefined) {
      ObjectRef.encode(message.manifestRef, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ManifestRef {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseManifestRef();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.meta = ManifestMeta.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.manifestRef = ObjectRef.decode(reader, reader.uint32());
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
  // Transform<ManifestRef, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ManifestRef | ManifestRef[]> | Iterable<ManifestRef | ManifestRef[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestRef.encode(p).finish()];
        }
      } else {
        yield* [ManifestRef.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ManifestRef>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ManifestRef> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestRef.decode(p)];
        }
      } else {
        yield* [ManifestRef.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): ManifestRef {
    return {
      meta: isSet(object.meta) ? ManifestMeta.fromJSON(object.meta) : undefined,
      manifestRef: isSet(object.manifestRef) ? ObjectRef.fromJSON(object.manifestRef) : undefined,
    };
  },

  toJSON(message: ManifestRef): unknown {
    const obj: any = {};
    if (message.meta !== undefined) {
      obj.meta = ManifestMeta.toJSON(message.meta);
    }
    if (message.manifestRef !== undefined) {
      obj.manifestRef = ObjectRef.toJSON(message.manifestRef);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ManifestRef>, I>>(base?: I): ManifestRef {
    return ManifestRef.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ManifestRef>, I>>(object: I): ManifestRef {
    const message = createBaseManifestRef();
    message.meta = (object.meta !== undefined && object.meta !== null)
      ? ManifestMeta.fromPartial(object.meta)
      : undefined;
    message.manifestRef = (object.manifestRef !== undefined && object.manifestRef !== null)
      ? ObjectRef.fromPartial(object.manifestRef)
      : undefined;
    return message;
  },
};

function createBaseManifestBundle(): ManifestBundle {
  return { manifestRefs: [], timestamp: undefined };
}

export const ManifestBundle = {
  encode(message: ManifestBundle, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.manifestRefs) {
      ManifestRef.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ManifestBundle {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseManifestBundle();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.manifestRefs.push(ManifestRef.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32());
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
  // Transform<ManifestBundle, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ManifestBundle | ManifestBundle[]> | Iterable<ManifestBundle | ManifestBundle[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestBundle.encode(p).finish()];
        }
      } else {
        yield* [ManifestBundle.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ManifestBundle>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ManifestBundle> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [ManifestBundle.decode(p)];
        }
      } else {
        yield* [ManifestBundle.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): ManifestBundle {
    return {
      manifestRefs: globalThis.Array.isArray(object?.manifestRefs)
        ? object.manifestRefs.map((e: any) => ManifestRef.fromJSON(e))
        : [],
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: ManifestBundle): unknown {
    const obj: any = {};
    if (message.manifestRefs?.length) {
      obj.manifestRefs = message.manifestRefs.map((e) => ManifestRef.toJSON(e));
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ManifestBundle>, I>>(base?: I): ManifestBundle {
    return ManifestBundle.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ManifestBundle>, I>>(object: I): ManifestBundle {
    const message = createBaseManifestBundle();
    message.manifestRefs = object.manifestRefs?.map((e) => ManifestRef.fromPartial(e)) || [];
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFetchManifestRequest(): FetchManifestRequest {
  return { manifestMeta: undefined };
}

export const FetchManifestRequest = {
  encode(message: FetchManifestRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.manifestMeta !== undefined) {
      ManifestMeta.encode(message.manifestMeta, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FetchManifestRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFetchManifestRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.manifestMeta = ManifestMeta.decode(reader, reader.uint32());
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
  // Transform<FetchManifestRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FetchManifestRequest | FetchManifestRequest[]>
      | Iterable<FetchManifestRequest | FetchManifestRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [FetchManifestRequest.encode(p).finish()];
        }
      } else {
        yield* [FetchManifestRequest.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchManifestRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FetchManifestRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [FetchManifestRequest.decode(p)];
        }
      } else {
        yield* [FetchManifestRequest.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): FetchManifestRequest {
    return { manifestMeta: isSet(object.manifestMeta) ? ManifestMeta.fromJSON(object.manifestMeta) : undefined };
  },

  toJSON(message: FetchManifestRequest): unknown {
    const obj: any = {};
    if (message.manifestMeta !== undefined) {
      obj.manifestMeta = ManifestMeta.toJSON(message.manifestMeta);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<FetchManifestRequest>, I>>(base?: I): FetchManifestRequest {
    return FetchManifestRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<FetchManifestRequest>, I>>(object: I): FetchManifestRequest {
    const message = createBaseFetchManifestRequest();
    message.manifestMeta = (object.manifestMeta !== undefined && object.manifestMeta !== null)
      ? ManifestMeta.fromPartial(object.manifestMeta)
      : undefined;
    return message;
  },
};

function createBaseFetchManifestResponse(): FetchManifestResponse {
  return { manifestRef: undefined };
}

export const FetchManifestResponse = {
  encode(message: FetchManifestResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.manifestRef !== undefined) {
      ManifestRef.encode(message.manifestRef, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FetchManifestResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFetchManifestResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.manifestRef = ManifestRef.decode(reader, reader.uint32());
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
  // Transform<FetchManifestResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FetchManifestResponse | FetchManifestResponse[]>
      | Iterable<FetchManifestResponse | FetchManifestResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [FetchManifestResponse.encode(p).finish()];
        }
      } else {
        yield* [FetchManifestResponse.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchManifestResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FetchManifestResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [FetchManifestResponse.decode(p)];
        }
      } else {
        yield* [FetchManifestResponse.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): FetchManifestResponse {
    return { manifestRef: isSet(object.manifestRef) ? ManifestRef.fromJSON(object.manifestRef) : undefined };
  },

  toJSON(message: FetchManifestResponse): unknown {
    const obj: any = {};
    if (message.manifestRef !== undefined) {
      obj.manifestRef = ManifestRef.toJSON(message.manifestRef);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<FetchManifestResponse>, I>>(base?: I): FetchManifestResponse {
    return FetchManifestResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<FetchManifestResponse>, I>>(object: I): FetchManifestResponse {
    const message = createBaseFetchManifestResponse();
    message.manifestRef = (object.manifestRef !== undefined && object.manifestRef !== null)
      ? ManifestRef.fromPartial(object.manifestRef)
      : undefined;
    return message;
  },
};

/** ManifestFetch is a service that fetches manifests by metadata. */
export interface ManifestFetch {
  /**
   * FetchManifest requests the manifest for the given metadata.
   * The metadata may not be an exact match.
   */
  FetchManifest(request: FetchManifestRequest, abortSignal?: AbortSignal): Promise<FetchManifestResponse>;
}

export const ManifestFetchServiceName = "bldr.manifest.ManifestFetch";
export class ManifestFetchClientImpl implements ManifestFetch {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || ManifestFetchServiceName;
    this.rpc = rpc;
    this.FetchManifest = this.FetchManifest.bind(this);
  }
  FetchManifest(request: FetchManifestRequest, abortSignal?: AbortSignal): Promise<FetchManifestResponse> {
    const data = FetchManifestRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "FetchManifest", data, abortSignal || undefined);
    return promise.then((data) => FetchManifestResponse.decode(_m0.Reader.create(data)));
  }
}

/** ManifestFetch is a service that fetches manifests by metadata. */
export type ManifestFetchDefinition = typeof ManifestFetchDefinition;
export const ManifestFetchDefinition = {
  name: "ManifestFetch",
  fullName: "bldr.manifest.ManifestFetch",
  methods: {
    /**
     * FetchManifest requests the manifest for the given metadata.
     * The metadata may not be an exact match.
     */
    fetchManifest: {
      name: "FetchManifest",
      requestType: FetchManifestRequest,
      requestStream: false,
      responseType: FetchManifestResponse,
      responseStream: false,
      options: {},
    },
  },
} as const;

interface Rpc {
  request(service: string, method: string, data: Uint8Array, abortSignal?: AbortSignal): Promise<Uint8Array>;
}

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
