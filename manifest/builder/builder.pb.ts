/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { Manifest, ManifestMeta, ManifestRef } from "../manifest.pb.js";

export const protobufPackage = "bldr.manifest.builder";

/** BuilderConfig is common configuration for a manifest builder routine. */
export interface BuilderConfig {
  /** ManifestMeta is the metadata of the manifest to build. */
  manifestMeta:
    | ManifestMeta
    | undefined;
  /** SourcePath is the path to the project source root. */
  sourcePath: string;
  /** DistSourcePath is the path to the bldr dist source root. */
  distSourcePath: string;
  /** WorkingPath is the path to use for codegen and working state. */
  workingPath: string;
  /** EngineId is the world engine to store the manifest. */
  engineId: string;
  /** ObjectKey is the key to store the manifest. */
  objectKey: string;
  /**
   * LinkObjectKeys is the list of object keys to link to the manifest.
   * NOTE: also used to search for other manifests in the dist compiler.
   */
  linkObjectKeys: string[];
  /** PeerId is the peer ID to use for world transactions. */
  peerId: string;
  /**
   * ProjectId is the project identifier.
   * Must be a valid-dns-label.
   * Used to construct the application storage and dist bundle filenames.
   */
  projectId: string;
}

/** BuilderResult is the result of a builder run. */
export interface BuilderResult {
  /** Manifest is the manifest object. */
  manifest:
    | Manifest
    | undefined;
  /** ManifestRef is the manifest object ref. */
  manifestRef:
    | ManifestRef
    | undefined;
  /**
   * InputManifest details which files were used to produce Manifest.
   * Used for change detection.
   */
  inputManifest: InputManifest | undefined;
}

/** InputManifest is an object describing the consumed source files. */
export interface InputManifest {
  /**
   * Files is the list of consumed source files.
   * Optional.
   */
  files: InputManifest_File[];
  /**
   * Metadata is additional builder-specific metadata about the output.
   * Optional.
   */
  metadata: Uint8Array;
}

/** File is a file in the source manifest. */
export interface InputManifest_File {
  /** Path is the path of the file in the source directory. */
  path: string;
  /**
   * Metadata is additional builder-specific metadata about the file.
   * Optional.
   */
  metadata: Uint8Array;
}

/** BuildManifestArgs are arguments passed to the BuildManifest function. */
export interface BuildManifestArgs {
  /**
   * BuilderConfig is the builder configuration.
   * Must be set.
   */
  builderConfig:
    | BuilderConfig
    | undefined;
  /**
   * PrevBuilderResult is the previous builder result, if applicable.
   * Set only if we are re-building the manifest after a file changed.
   * May be nil.
   */
  prevBuilderResult:
    | BuilderResult
    | undefined;
  /**
   * ChangedFiles is the list of files from PrevBuilderResult InputManifest
   * filtered to contain only files that changed since the previous build. //
   * Set only if PrevBuilderResult is set.
   * May be nil.
   */
  changedFiles: InputManifest_File[];
}

function createBaseBuilderConfig(): BuilderConfig {
  return {
    manifestMeta: undefined,
    sourcePath: "",
    distSourcePath: "",
    workingPath: "",
    engineId: "",
    objectKey: "",
    linkObjectKeys: [],
    peerId: "",
    projectId: "",
  };
}

export const BuilderConfig = {
  encode(message: BuilderConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.manifestMeta !== undefined) {
      ManifestMeta.encode(message.manifestMeta, writer.uint32(10).fork()).ldelim();
    }
    if (message.sourcePath !== "") {
      writer.uint32(18).string(message.sourcePath);
    }
    if (message.distSourcePath !== "") {
      writer.uint32(26).string(message.distSourcePath);
    }
    if (message.workingPath !== "") {
      writer.uint32(34).string(message.workingPath);
    }
    if (message.engineId !== "") {
      writer.uint32(42).string(message.engineId);
    }
    if (message.objectKey !== "") {
      writer.uint32(50).string(message.objectKey);
    }
    for (const v of message.linkObjectKeys) {
      writer.uint32(58).string(v!);
    }
    if (message.peerId !== "") {
      writer.uint32(66).string(message.peerId);
    }
    if (message.projectId !== "") {
      writer.uint32(74).string(message.projectId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.manifestMeta = ManifestMeta.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.sourcePath = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.distSourcePath = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.workingPath = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.engineId = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.objectKey = reader.string();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.linkObjectKeys.push(reader.string());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.peerId = reader.string();
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.projectId = reader.string();
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
  // Transform<BuilderConfig, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BuilderConfig | BuilderConfig[]> | Iterable<BuilderConfig | BuilderConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [BuilderConfig.encode(p).finish()];
        }
      } else {
        yield* [BuilderConfig.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BuilderConfig>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BuilderConfig> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [BuilderConfig.decode(p)];
        }
      } else {
        yield* [BuilderConfig.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): BuilderConfig {
    return {
      manifestMeta: isSet(object.manifestMeta) ? ManifestMeta.fromJSON(object.manifestMeta) : undefined,
      sourcePath: isSet(object.sourcePath) ? globalThis.String(object.sourcePath) : "",
      distSourcePath: isSet(object.distSourcePath) ? globalThis.String(object.distSourcePath) : "",
      workingPath: isSet(object.workingPath) ? globalThis.String(object.workingPath) : "",
      engineId: isSet(object.engineId) ? globalThis.String(object.engineId) : "",
      objectKey: isSet(object.objectKey) ? globalThis.String(object.objectKey) : "",
      linkObjectKeys: globalThis.Array.isArray(object?.linkObjectKeys)
        ? object.linkObjectKeys.map((e: any) => globalThis.String(e))
        : [],
      peerId: isSet(object.peerId) ? globalThis.String(object.peerId) : "",
      projectId: isSet(object.projectId) ? globalThis.String(object.projectId) : "",
    };
  },

  toJSON(message: BuilderConfig): unknown {
    const obj: any = {};
    if (message.manifestMeta !== undefined) {
      obj.manifestMeta = ManifestMeta.toJSON(message.manifestMeta);
    }
    if (message.sourcePath !== "") {
      obj.sourcePath = message.sourcePath;
    }
    if (message.distSourcePath !== "") {
      obj.distSourcePath = message.distSourcePath;
    }
    if (message.workingPath !== "") {
      obj.workingPath = message.workingPath;
    }
    if (message.engineId !== "") {
      obj.engineId = message.engineId;
    }
    if (message.objectKey !== "") {
      obj.objectKey = message.objectKey;
    }
    if (message.linkObjectKeys?.length) {
      obj.linkObjectKeys = message.linkObjectKeys;
    }
    if (message.peerId !== "") {
      obj.peerId = message.peerId;
    }
    if (message.projectId !== "") {
      obj.projectId = message.projectId;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig>, I>>(base?: I): BuilderConfig {
    return BuilderConfig.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig>, I>>(object: I): BuilderConfig {
    const message = createBaseBuilderConfig();
    message.manifestMeta = (object.manifestMeta !== undefined && object.manifestMeta !== null)
      ? ManifestMeta.fromPartial(object.manifestMeta)
      : undefined;
    message.sourcePath = object.sourcePath ?? "";
    message.distSourcePath = object.distSourcePath ?? "";
    message.workingPath = object.workingPath ?? "";
    message.engineId = object.engineId ?? "";
    message.objectKey = object.objectKey ?? "";
    message.linkObjectKeys = object.linkObjectKeys?.map((e) => e) || [];
    message.peerId = object.peerId ?? "";
    message.projectId = object.projectId ?? "";
    return message;
  },
};

function createBaseBuilderResult(): BuilderResult {
  return { manifest: undefined, manifestRef: undefined, inputManifest: undefined };
}

export const BuilderResult = {
  encode(message: BuilderResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.manifest !== undefined) {
      Manifest.encode(message.manifest, writer.uint32(10).fork()).ldelim();
    }
    if (message.manifestRef !== undefined) {
      ManifestRef.encode(message.manifestRef, writer.uint32(18).fork()).ldelim();
    }
    if (message.inputManifest !== undefined) {
      InputManifest.encode(message.inputManifest, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderResult {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderResult();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.manifest = Manifest.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.manifestRef = ManifestRef.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.inputManifest = InputManifest.decode(reader, reader.uint32());
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
  // Transform<BuilderResult, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BuilderResult | BuilderResult[]> | Iterable<BuilderResult | BuilderResult[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [BuilderResult.encode(p).finish()];
        }
      } else {
        yield* [BuilderResult.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BuilderResult>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BuilderResult> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [BuilderResult.decode(p)];
        }
      } else {
        yield* [BuilderResult.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): BuilderResult {
    return {
      manifest: isSet(object.manifest) ? Manifest.fromJSON(object.manifest) : undefined,
      manifestRef: isSet(object.manifestRef) ? ManifestRef.fromJSON(object.manifestRef) : undefined,
      inputManifest: isSet(object.inputManifest) ? InputManifest.fromJSON(object.inputManifest) : undefined,
    };
  },

  toJSON(message: BuilderResult): unknown {
    const obj: any = {};
    if (message.manifest !== undefined) {
      obj.manifest = Manifest.toJSON(message.manifest);
    }
    if (message.manifestRef !== undefined) {
      obj.manifestRef = ManifestRef.toJSON(message.manifestRef);
    }
    if (message.inputManifest !== undefined) {
      obj.inputManifest = InputManifest.toJSON(message.inputManifest);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderResult>, I>>(base?: I): BuilderResult {
    return BuilderResult.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderResult>, I>>(object: I): BuilderResult {
    const message = createBaseBuilderResult();
    message.manifest = (object.manifest !== undefined && object.manifest !== null)
      ? Manifest.fromPartial(object.manifest)
      : undefined;
    message.manifestRef = (object.manifestRef !== undefined && object.manifestRef !== null)
      ? ManifestRef.fromPartial(object.manifestRef)
      : undefined;
    message.inputManifest = (object.inputManifest !== undefined && object.inputManifest !== null)
      ? InputManifest.fromPartial(object.inputManifest)
      : undefined;
    return message;
  },
};

function createBaseInputManifest(): InputManifest {
  return { files: [], metadata: new Uint8Array(0) };
}

export const InputManifest = {
  encode(message: InputManifest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.files) {
      InputManifest_File.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.metadata.length !== 0) {
      writer.uint32(18).bytes(message.metadata);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputManifest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInputManifest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.files.push(InputManifest_File.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.metadata = reader.bytes();
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
  // Transform<InputManifest, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<InputManifest | InputManifest[]> | Iterable<InputManifest | InputManifest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [InputManifest.encode(p).finish()];
        }
      } else {
        yield* [InputManifest.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, InputManifest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<InputManifest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [InputManifest.decode(p)];
        }
      } else {
        yield* [InputManifest.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): InputManifest {
    return {
      files: globalThis.Array.isArray(object?.files)
        ? object.files.map((e: any) => InputManifest_File.fromJSON(e))
        : [],
      metadata: isSet(object.metadata) ? bytesFromBase64(object.metadata) : new Uint8Array(0),
    };
  },

  toJSON(message: InputManifest): unknown {
    const obj: any = {};
    if (message.files?.length) {
      obj.files = message.files.map((e) => InputManifest_File.toJSON(e));
    }
    if (message.metadata.length !== 0) {
      obj.metadata = base64FromBytes(message.metadata);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InputManifest>, I>>(base?: I): InputManifest {
    return InputManifest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InputManifest>, I>>(object: I): InputManifest {
    const message = createBaseInputManifest();
    message.files = object.files?.map((e) => InputManifest_File.fromPartial(e)) || [];
    message.metadata = object.metadata ?? new Uint8Array(0);
    return message;
  },
};

function createBaseInputManifest_File(): InputManifest_File {
  return { path: "", metadata: new Uint8Array(0) };
}

export const InputManifest_File = {
  encode(message: InputManifest_File, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.path !== "") {
      writer.uint32(10).string(message.path);
    }
    if (message.metadata.length !== 0) {
      writer.uint32(18).bytes(message.metadata);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputManifest_File {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInputManifest_File();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.path = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.metadata = reader.bytes();
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
  // Transform<InputManifest_File, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<InputManifest_File | InputManifest_File[]>
      | Iterable<InputManifest_File | InputManifest_File[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [InputManifest_File.encode(p).finish()];
        }
      } else {
        yield* [InputManifest_File.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, InputManifest_File>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<InputManifest_File> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [InputManifest_File.decode(p)];
        }
      } else {
        yield* [InputManifest_File.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): InputManifest_File {
    return {
      path: isSet(object.path) ? globalThis.String(object.path) : "",
      metadata: isSet(object.metadata) ? bytesFromBase64(object.metadata) : new Uint8Array(0),
    };
  },

  toJSON(message: InputManifest_File): unknown {
    const obj: any = {};
    if (message.path !== "") {
      obj.path = message.path;
    }
    if (message.metadata.length !== 0) {
      obj.metadata = base64FromBytes(message.metadata);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InputManifest_File>, I>>(base?: I): InputManifest_File {
    return InputManifest_File.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InputManifest_File>, I>>(object: I): InputManifest_File {
    const message = createBaseInputManifest_File();
    message.path = object.path ?? "";
    message.metadata = object.metadata ?? new Uint8Array(0);
    return message;
  },
};

function createBaseBuildManifestArgs(): BuildManifestArgs {
  return { builderConfig: undefined, prevBuilderResult: undefined, changedFiles: [] };
}

export const BuildManifestArgs = {
  encode(message: BuildManifestArgs, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.builderConfig !== undefined) {
      BuilderConfig.encode(message.builderConfig, writer.uint32(10).fork()).ldelim();
    }
    if (message.prevBuilderResult !== undefined) {
      BuilderResult.encode(message.prevBuilderResult, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.changedFiles) {
      InputManifest_File.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildManifestArgs {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildManifestArgs();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.builderConfig = BuilderConfig.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.prevBuilderResult = BuilderResult.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.changedFiles.push(InputManifest_File.decode(reader, reader.uint32()));
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
  // Transform<BuildManifestArgs, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BuildManifestArgs | BuildManifestArgs[]> | Iterable<BuildManifestArgs | BuildManifestArgs[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [BuildManifestArgs.encode(p).finish()];
        }
      } else {
        yield* [BuildManifestArgs.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BuildManifestArgs>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BuildManifestArgs> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [BuildManifestArgs.decode(p)];
        }
      } else {
        yield* [BuildManifestArgs.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): BuildManifestArgs {
    return {
      builderConfig: isSet(object.builderConfig) ? BuilderConfig.fromJSON(object.builderConfig) : undefined,
      prevBuilderResult: isSet(object.prevBuilderResult) ? BuilderResult.fromJSON(object.prevBuilderResult) : undefined,
      changedFiles: globalThis.Array.isArray(object?.changedFiles)
        ? object.changedFiles.map((e: any) => InputManifest_File.fromJSON(e))
        : [],
    };
  },

  toJSON(message: BuildManifestArgs): unknown {
    const obj: any = {};
    if (message.builderConfig !== undefined) {
      obj.builderConfig = BuilderConfig.toJSON(message.builderConfig);
    }
    if (message.prevBuilderResult !== undefined) {
      obj.prevBuilderResult = BuilderResult.toJSON(message.prevBuilderResult);
    }
    if (message.changedFiles?.length) {
      obj.changedFiles = message.changedFiles.map((e) => InputManifest_File.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildManifestArgs>, I>>(base?: I): BuildManifestArgs {
    return BuildManifestArgs.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildManifestArgs>, I>>(object: I): BuildManifestArgs {
    const message = createBaseBuildManifestArgs();
    message.builderConfig = (object.builderConfig !== undefined && object.builderConfig !== null)
      ? BuilderConfig.fromPartial(object.builderConfig)
      : undefined;
    message.prevBuilderResult = (object.prevBuilderResult !== undefined && object.prevBuilderResult !== null)
      ? BuilderResult.fromPartial(object.prevBuilderResult)
      : undefined;
    message.changedFiles = object.changedFiles?.map((e) => InputManifest_File.fromPartial(e)) || [];
    return message;
  },
};

function bytesFromBase64(b64: string): Uint8Array {
  if ((globalThis as any).Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, "base64"));
  } else {
    const bin = globalThis.atob(b64);
    const arr = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i);
    }
    return arr;
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if ((globalThis as any).Buffer) {
    return globalThis.Buffer.from(arr).toString("base64");
  } else {
    const bin: string[] = [];
    arr.forEach((byte) => {
      bin.push(globalThis.String.fromCharCode(byte));
    });
    return globalThis.btoa(bin.join(""));
  }
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
