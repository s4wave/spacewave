/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { ControllerConfig } from "../../controllerbus/controller/configset/proto/configset.pb.js";
import { Timestamp } from "../../timestamp/timestamp.pb.js";
import { BlockRef, PutOpts } from "../block/block.pb.js";
import { Config as Config1 } from "../block/transform/transform.pb.js";

export const protobufPackage = "bucket";

/** Config is a bucket configuration object. */
export interface Config {
  /** Id is the bucket identifier. */
  id: string;
  /**
   * Version is the configuration version.
   * increment by 1 on modification
   */
  version: number;
  /** Reconcilers contains the list of bucket reconcilers. */
  reconcilers: ReconcilerConfig[];
  /** PutOpts are the default put options for the bucket. */
  putOpts:
    | PutOpts
    | undefined;
  /** Lookup controls the lookup confiuration. */
  lookup: LookupConfig | undefined;
}

/** BucketInfo is general information about a bucket. */
export interface BucketInfo {
  /** Config contains the current latest bucket configuration. */
  config: Config | undefined;
}

/** ReconcilerConfig configures a reconciler. */
export interface ReconcilerConfig {
  /** Id contains the reconciler id. */
  id: string;
  /** Controller contains the controller configuration. */
  controller:
    | ControllerConfig
    | undefined;
  /** FilterPut disables receiving put events. */
  filterPut: boolean;
}

/** LookupConfig configures the bucket behavior across multiple volumes. */
export interface LookupConfig {
  /**
   * Disble indicates we should not service cross-volume calls against this
   * bucket.
   */
  disable: boolean;
  /**
   * Controller contains the lookup controller configuration.
   * If unset, will default to the node-default lookup controller.
   * If disabled, this field will be ignored.
   */
  controller: ControllerConfig | undefined;
}

/** ApplyBucketConfigResult is the result of the ApplyBucketConfig directive. */
export interface ApplyBucketConfigResult {
  /** VolumeId is the volume ID for this apply event. */
  volumeId: string;
  /** BucketId returns the bucket ID for this apply event. */
  bucketId: string;
  /** BucketConf returns the bucket configuration applied. */
  bucketConf:
    | Config
    | undefined;
  /** OldBucketConf returns the previous bucket configuration. */
  oldBucketConf:
    | Config
    | undefined;
  /** Timestamp returns the timestamp of the event. */
  timestamp:
    | Timestamp
    | undefined;
  /** Updated indicates if the value was updated. */
  updated: boolean;
  /**
   * Error contains any error applying the value.
   * Note: all other values might be empty if this is set.
   */
  error: string;
}

/**
 * ObjectRef is a reference that may contain a transformation config reference
 * and/or a bucket ID change.
 */
export interface ObjectRef {
  /** RootRef is the root block ref. */
  rootRef:
    | BlockRef
    | undefined;
  /**
   * BucketId is the bucket id, if switching buckets.
   * May be empty to indicate same bucket.
   */
  bucketId: string;
  /**
   * TransformConfRef is the transformation configuration block ref.
   * Must be encoded with the same transformation config as the parent.
   * May be empty to indicate same conf as parent.
   */
  transformConfRef:
    | BlockRef
    | undefined;
  /** TransformConf is an in-line transform configuration. */
  transformConf: Config1 | undefined;
}

/** BucketOpArgs are common arguments for a bucket operation. */
export interface BucketOpArgs {
  /** BucketId is the bucket ID to operate on. */
  bucketId: string;
  /**
   * VolumeId is the volume ID to operate on.
   * If empty, will use the lookup controller.
   */
  volumeId: string;
}

function createBaseConfig(): Config {
  return { id: "", version: 0, reconcilers: [], putOpts: undefined, lookup: undefined };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.version !== 0) {
      writer.uint32(16).uint32(message.version);
    }
    for (const v of message.reconcilers) {
      ReconcilerConfig.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    if (message.putOpts !== undefined) {
      PutOpts.encode(message.putOpts, writer.uint32(34).fork()).ldelim();
    }
    if (message.lookup !== undefined) {
      LookupConfig.encode(message.lookup, writer.uint32(42).fork()).ldelim();
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
          message.id = reader.string();
          break;
        case 2:
          message.version = reader.uint32();
          break;
        case 3:
          message.reconcilers.push(ReconcilerConfig.decode(reader, reader.uint32()));
          break;
        case 4:
          message.putOpts = PutOpts.decode(reader, reader.uint32());
          break;
        case 5:
          message.lookup = LookupConfig.decode(reader, reader.uint32());
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
      id: isSet(object.id) ? String(object.id) : "",
      version: isSet(object.version) ? Number(object.version) : 0,
      reconcilers: Array.isArray(object?.reconcilers)
        ? object.reconcilers.map((e: any) => ReconcilerConfig.fromJSON(e))
        : [],
      putOpts: isSet(object.putOpts) ? PutOpts.fromJSON(object.putOpts) : undefined,
      lookup: isSet(object.lookup) ? LookupConfig.fromJSON(object.lookup) : undefined,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.id !== undefined && (obj.id = message.id);
    message.version !== undefined && (obj.version = Math.round(message.version));
    if (message.reconcilers) {
      obj.reconcilers = message.reconcilers.map((e) => e ? ReconcilerConfig.toJSON(e) : undefined);
    } else {
      obj.reconcilers = [];
    }
    message.putOpts !== undefined && (obj.putOpts = message.putOpts ? PutOpts.toJSON(message.putOpts) : undefined);
    message.lookup !== undefined && (obj.lookup = message.lookup ? LookupConfig.toJSON(message.lookup) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.id = object.id ?? "";
    message.version = object.version ?? 0;
    message.reconcilers = object.reconcilers?.map((e) => ReconcilerConfig.fromPartial(e)) || [];
    message.putOpts = (object.putOpts !== undefined && object.putOpts !== null)
      ? PutOpts.fromPartial(object.putOpts)
      : undefined;
    message.lookup = (object.lookup !== undefined && object.lookup !== null)
      ? LookupConfig.fromPartial(object.lookup)
      : undefined;
    return message;
  },
};

function createBaseBucketInfo(): BucketInfo {
  return { config: undefined };
}

export const BucketInfo = {
  encode(message: BucketInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.config !== undefined) {
      Config.encode(message.config, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BucketInfo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBucketInfo();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.config = Config.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<BucketInfo, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BucketInfo | BucketInfo[]> | Iterable<BucketInfo | BucketInfo[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketInfo.encode(p).finish()];
        }
      } else {
        yield* [BucketInfo.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BucketInfo>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BucketInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketInfo.decode(p)];
        }
      } else {
        yield* [BucketInfo.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): BucketInfo {
    return { config: isSet(object.config) ? Config.fromJSON(object.config) : undefined };
  },

  toJSON(message: BucketInfo): unknown {
    const obj: any = {};
    message.config !== undefined && (obj.config = message.config ? Config.toJSON(message.config) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<BucketInfo>, I>>(object: I): BucketInfo {
    const message = createBaseBucketInfo();
    message.config = (object.config !== undefined && object.config !== null)
      ? Config.fromPartial(object.config)
      : undefined;
    return message;
  },
};

function createBaseReconcilerConfig(): ReconcilerConfig {
  return { id: "", controller: undefined, filterPut: false };
}

export const ReconcilerConfig = {
  encode(message: ReconcilerConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.controller !== undefined) {
      ControllerConfig.encode(message.controller, writer.uint32(18).fork()).ldelim();
    }
    if (message.filterPut === true) {
      writer.uint32(24).bool(message.filterPut);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReconcilerConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReconcilerConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.string();
          break;
        case 2:
          message.controller = ControllerConfig.decode(reader, reader.uint32());
          break;
        case 3:
          message.filterPut = reader.bool();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ReconcilerConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ReconcilerConfig | ReconcilerConfig[]> | Iterable<ReconcilerConfig | ReconcilerConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReconcilerConfig.encode(p).finish()];
        }
      } else {
        yield* [ReconcilerConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReconcilerConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReconcilerConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReconcilerConfig.decode(p)];
        }
      } else {
        yield* [ReconcilerConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ReconcilerConfig {
    return {
      id: isSet(object.id) ? String(object.id) : "",
      controller: isSet(object.controller) ? ControllerConfig.fromJSON(object.controller) : undefined,
      filterPut: isSet(object.filterPut) ? Boolean(object.filterPut) : false,
    };
  },

  toJSON(message: ReconcilerConfig): unknown {
    const obj: any = {};
    message.id !== undefined && (obj.id = message.id);
    message.controller !== undefined &&
      (obj.controller = message.controller ? ControllerConfig.toJSON(message.controller) : undefined);
    message.filterPut !== undefined && (obj.filterPut = message.filterPut);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ReconcilerConfig>, I>>(object: I): ReconcilerConfig {
    const message = createBaseReconcilerConfig();
    message.id = object.id ?? "";
    message.controller = (object.controller !== undefined && object.controller !== null)
      ? ControllerConfig.fromPartial(object.controller)
      : undefined;
    message.filterPut = object.filterPut ?? false;
    return message;
  },
};

function createBaseLookupConfig(): LookupConfig {
  return { disable: false, controller: undefined };
}

export const LookupConfig = {
  encode(message: LookupConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.disable === true) {
      writer.uint32(8).bool(message.disable);
    }
    if (message.controller !== undefined) {
      ControllerConfig.encode(message.controller, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): LookupConfig {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLookupConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.disable = reader.bool();
          break;
        case 2:
          message.controller = ControllerConfig.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<LookupConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<LookupConfig | LookupConfig[]> | Iterable<LookupConfig | LookupConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LookupConfig.encode(p).finish()];
        }
      } else {
        yield* [LookupConfig.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, LookupConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<LookupConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LookupConfig.decode(p)];
        }
      } else {
        yield* [LookupConfig.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): LookupConfig {
    return {
      disable: isSet(object.disable) ? Boolean(object.disable) : false,
      controller: isSet(object.controller) ? ControllerConfig.fromJSON(object.controller) : undefined,
    };
  },

  toJSON(message: LookupConfig): unknown {
    const obj: any = {};
    message.disable !== undefined && (obj.disable = message.disable);
    message.controller !== undefined &&
      (obj.controller = message.controller ? ControllerConfig.toJSON(message.controller) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<LookupConfig>, I>>(object: I): LookupConfig {
    const message = createBaseLookupConfig();
    message.disable = object.disable ?? false;
    message.controller = (object.controller !== undefined && object.controller !== null)
      ? ControllerConfig.fromPartial(object.controller)
      : undefined;
    return message;
  },
};

function createBaseApplyBucketConfigResult(): ApplyBucketConfigResult {
  return {
    volumeId: "",
    bucketId: "",
    bucketConf: undefined,
    oldBucketConf: undefined,
    timestamp: undefined,
    updated: false,
    error: "",
  };
}

export const ApplyBucketConfigResult = {
  encode(message: ApplyBucketConfigResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.volumeId !== "") {
      writer.uint32(10).string(message.volumeId);
    }
    if (message.bucketId !== "") {
      writer.uint32(18).string(message.bucketId);
    }
    if (message.bucketConf !== undefined) {
      Config.encode(message.bucketConf, writer.uint32(26).fork()).ldelim();
    }
    if (message.oldBucketConf !== undefined) {
      Config.encode(message.oldBucketConf, writer.uint32(34).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(42).fork()).ldelim();
    }
    if (message.updated === true) {
      writer.uint32(48).bool(message.updated);
    }
    if (message.error !== "") {
      writer.uint32(58).string(message.error);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ApplyBucketConfigResult {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseApplyBucketConfigResult();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.volumeId = reader.string();
          break;
        case 2:
          message.bucketId = reader.string();
          break;
        case 3:
          message.bucketConf = Config.decode(reader, reader.uint32());
          break;
        case 4:
          message.oldBucketConf = Config.decode(reader, reader.uint32());
          break;
        case 5:
          message.timestamp = Timestamp.decode(reader, reader.uint32());
          break;
        case 6:
          message.updated = reader.bool();
          break;
        case 7:
          message.error = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ApplyBucketConfigResult, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ApplyBucketConfigResult | ApplyBucketConfigResult[]>
      | Iterable<ApplyBucketConfigResult | ApplyBucketConfigResult[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigResult.encode(p).finish()];
        }
      } else {
        yield* [ApplyBucketConfigResult.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ApplyBucketConfigResult>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ApplyBucketConfigResult> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfigResult.decode(p)];
        }
      } else {
        yield* [ApplyBucketConfigResult.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ApplyBucketConfigResult {
    return {
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : "",
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : "",
      bucketConf: isSet(object.bucketConf) ? Config.fromJSON(object.bucketConf) : undefined,
      oldBucketConf: isSet(object.oldBucketConf) ? Config.fromJSON(object.oldBucketConf) : undefined,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
      updated: isSet(object.updated) ? Boolean(object.updated) : false,
      error: isSet(object.error) ? String(object.error) : "",
    };
  },

  toJSON(message: ApplyBucketConfigResult): unknown {
    const obj: any = {};
    message.volumeId !== undefined && (obj.volumeId = message.volumeId);
    message.bucketId !== undefined && (obj.bucketId = message.bucketId);
    message.bucketConf !== undefined &&
      (obj.bucketConf = message.bucketConf ? Config.toJSON(message.bucketConf) : undefined);
    message.oldBucketConf !== undefined &&
      (obj.oldBucketConf = message.oldBucketConf ? Config.toJSON(message.oldBucketConf) : undefined);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    message.updated !== undefined && (obj.updated = message.updated);
    message.error !== undefined && (obj.error = message.error);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ApplyBucketConfigResult>, I>>(object: I): ApplyBucketConfigResult {
    const message = createBaseApplyBucketConfigResult();
    message.volumeId = object.volumeId ?? "";
    message.bucketId = object.bucketId ?? "";
    message.bucketConf = (object.bucketConf !== undefined && object.bucketConf !== null)
      ? Config.fromPartial(object.bucketConf)
      : undefined;
    message.oldBucketConf = (object.oldBucketConf !== undefined && object.oldBucketConf !== null)
      ? Config.fromPartial(object.oldBucketConf)
      : undefined;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    message.updated = object.updated ?? false;
    message.error = object.error ?? "";
    return message;
  },
};

function createBaseObjectRef(): ObjectRef {
  return { rootRef: undefined, bucketId: "", transformConfRef: undefined, transformConf: undefined };
}

export const ObjectRef = {
  encode(message: ObjectRef, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.rootRef !== undefined) {
      BlockRef.encode(message.rootRef, writer.uint32(10).fork()).ldelim();
    }
    if (message.bucketId !== "") {
      writer.uint32(18).string(message.bucketId);
    }
    if (message.transformConfRef !== undefined) {
      BlockRef.encode(message.transformConfRef, writer.uint32(26).fork()).ldelim();
    }
    if (message.transformConf !== undefined) {
      Config1.encode(message.transformConf, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ObjectRef {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseObjectRef();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.rootRef = BlockRef.decode(reader, reader.uint32());
          break;
        case 2:
          message.bucketId = reader.string();
          break;
        case 3:
          message.transformConfRef = BlockRef.decode(reader, reader.uint32());
          break;
        case 4:
          message.transformConf = Config1.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ObjectRef, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ObjectRef | ObjectRef[]> | Iterable<ObjectRef | ObjectRef[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ObjectRef.encode(p).finish()];
        }
      } else {
        yield* [ObjectRef.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ObjectRef>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ObjectRef> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ObjectRef.decode(p)];
        }
      } else {
        yield* [ObjectRef.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ObjectRef {
    return {
      rootRef: isSet(object.rootRef) ? BlockRef.fromJSON(object.rootRef) : undefined,
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : "",
      transformConfRef: isSet(object.transformConfRef) ? BlockRef.fromJSON(object.transformConfRef) : undefined,
      transformConf: isSet(object.transformConf) ? Config1.fromJSON(object.transformConf) : undefined,
    };
  },

  toJSON(message: ObjectRef): unknown {
    const obj: any = {};
    message.rootRef !== undefined && (obj.rootRef = message.rootRef ? BlockRef.toJSON(message.rootRef) : undefined);
    message.bucketId !== undefined && (obj.bucketId = message.bucketId);
    message.transformConfRef !== undefined &&
      (obj.transformConfRef = message.transformConfRef ? BlockRef.toJSON(message.transformConfRef) : undefined);
    message.transformConf !== undefined &&
      (obj.transformConf = message.transformConf ? Config1.toJSON(message.transformConf) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<ObjectRef>, I>>(object: I): ObjectRef {
    const message = createBaseObjectRef();
    message.rootRef = (object.rootRef !== undefined && object.rootRef !== null)
      ? BlockRef.fromPartial(object.rootRef)
      : undefined;
    message.bucketId = object.bucketId ?? "";
    message.transformConfRef = (object.transformConfRef !== undefined && object.transformConfRef !== null)
      ? BlockRef.fromPartial(object.transformConfRef)
      : undefined;
    message.transformConf = (object.transformConf !== undefined && object.transformConf !== null)
      ? Config1.fromPartial(object.transformConf)
      : undefined;
    return message;
  },
};

function createBaseBucketOpArgs(): BucketOpArgs {
  return { bucketId: "", volumeId: "" };
}

export const BucketOpArgs = {
  encode(message: BucketOpArgs, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bucketId !== "") {
      writer.uint32(10).string(message.bucketId);
    }
    if (message.volumeId !== "") {
      writer.uint32(18).string(message.volumeId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BucketOpArgs {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBucketOpArgs();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.bucketId = reader.string();
          break;
        case 2:
          message.volumeId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<BucketOpArgs, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BucketOpArgs | BucketOpArgs[]> | Iterable<BucketOpArgs | BucketOpArgs[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketOpArgs.encode(p).finish()];
        }
      } else {
        yield* [BucketOpArgs.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BucketOpArgs>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BucketOpArgs> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BucketOpArgs.decode(p)];
        }
      } else {
        yield* [BucketOpArgs.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): BucketOpArgs {
    return {
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : "",
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : "",
    };
  },

  toJSON(message: BucketOpArgs): unknown {
    const obj: any = {};
    message.bucketId !== undefined && (obj.bucketId = message.bucketId);
    message.volumeId !== undefined && (obj.volumeId = message.volumeId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<BucketOpArgs>, I>>(object: I): BucketOpArgs {
    const message = createBaseBucketOpArgs();
    message.bucketId = object.bucketId ?? "";
    message.volumeId = object.volumeId ?? "";
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
