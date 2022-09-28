/* eslint-disable */
import { Timestamp } from "@go/github.com/aperturerobotics/timestamp/timestamp.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { BlockRef } from "../../block/block.pb.js";
import { ObjectRef } from "../../bucket/bucket.pb.js";
import { FSPath, FSSymlink, NodeType, nodeTypeFromJSON, nodeTypeToJSON } from "../block/fstree.pb.js";

export const protobufPackage = "unixfs.world";

/** FSType indicates the type of unixfs reference. */
export enum FSType {
  /**
   * FSType_UNKNOWN - FSType_UNKNOWN is the zero type.
   * Defaults depending on the context.
   */
  FSType_UNKNOWN = 0,
  /** FSType_FS_NODE - FSType_FS_NODE is a raw fstree FSNode block (without changelog). */
  FSType_FS_NODE = 1,
  /** FSType_FS_OBJECT - FSType_FS_OBJECT is a FSObject tree (with changelog). */
  FSType_FS_OBJECT = 2,
  /** FSType_FS_HOST_VOLUME - FSType_FS_HOST_VOLUME is a FSHostVolume object. */
  FSType_FS_HOST_VOLUME = 3,
  UNRECOGNIZED = -1,
}

export function fSTypeFromJSON(object: any): FSType {
  switch (object) {
    case 0:
    case "FSType_UNKNOWN":
      return FSType.FSType_UNKNOWN;
    case 1:
    case "FSType_FS_NODE":
      return FSType.FSType_FS_NODE;
    case 2:
    case "FSType_FS_OBJECT":
      return FSType.FSType_FS_OBJECT;
    case 3:
    case "FSType_FS_HOST_VOLUME":
      return FSType.FSType_FS_HOST_VOLUME;
    case -1:
    case "UNRECOGNIZED":
    default:
      return FSType.UNRECOGNIZED;
  }
}

export function fSTypeToJSON(object: FSType): string {
  switch (object) {
    case FSType.FSType_UNKNOWN:
      return "FSType_UNKNOWN";
    case FSType.FSType_FS_NODE:
      return "FSType_FS_NODE";
    case FSType.FSType_FS_OBJECT:
      return "FSType_FS_OBJECT";
    case FSType.FSType_FS_HOST_VOLUME:
      return "FSType_FS_HOST_VOLUME";
    case FSType.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

/** UnixfsRef is a reference to a UnixFS object in a World. */
export interface UnixfsRef {
  /** ObjectKey is the object key to open as a UnixFS. */
  objectKey: string;
  /**
   * FsType sets the expected filesystem object type at object_key.
   * If unset (0) reads the type from the graph.
   * Defaults to FS_NODE if nothing else set.
   */
  fsType: FSType;
  /**
   * Path is the location within the FS.
   * If empty, defaults to / (the root).
   */
  path: FSPath | undefined;
}

/**
 * FsInitOp is an operation to create a unixfs filesystem with a root ref or empty.
 * Can be applied as either an object op or a world op.
 */
export interface FsInitOp {
  /** ObjectKey is the object key to create as a UnixFS. */
  objectKey: string;
  /** FsType sets the filesystem object type to create. */
  fsType: FSType;
  /**
   * FsRef contains a initial object ref to use the root of the UnixFS.
   * If empty, will create a new blank fs.
   */
  fsRef:
    | ObjectRef
    | undefined;
  /**
   * FsRefType is the FSType of the ref.
   * Defaults to FsType_FS_NODE.
   */
  fsRefType: FSType;
  /** FsOverwrite indicates to overwrite any existing object. */
  fsOverwrite: boolean;
  /** Timestamp is the modification time for the fs root. */
  timestamp: Timestamp | undefined;
}

/**
 * FsMknodOp is an operation to create one or more inodes at paths.
 * Can be applied as either an object op or a world op.
 */
export interface FsMknodOp {
  /**
   * ObjectKey is the object key to start at.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** Paths are the paths to the new inodes. */
  paths: FSPath[];
  /**
   * Permissions is the permissions bitset.
   * If zero uses defaults.
   */
  permissions: number;
  /** NodeType is the node type to create. */
  nodeType: NodeType;
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * FsSymlinkOp is an operation to create a symbolic link.
 * Can be applied as either an object op or a world op.
 */
export interface FsSymlinkOp {
  /**
   * ObjectKey is the object key to start at.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** Path is the paths to the symbolic link source. */
  path:
    | FSPath
    | undefined;
  /** Symlink is the contents of the symlink. */
  symlink:
    | FSSymlink
    | undefined;
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * FsSetPermissionsOp is an operation to set the permissions at the paths.
 * The file mode portion of the permissions bitset will be ignored.
 * Can be applied as either an object op or a world op.
 */
export interface FsSetPermissionsOp {
  /**
   * ObjectKey is the object key to start at.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** Paths are the paths to update permissions for. */
  paths: FSPath[];
  /** Permissions is the permissions bitset. */
  permissions: number;
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * FsSetModTimestampOp is an operation to set the modification timestamp at the paths.
 * Can be applied as either an object op or a world op.
 */
export interface FsSetModTimestampOp {
  /**
   * ObjectKey is the object key to start at.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** Paths are the paths to update the timestamp for. */
  paths: FSPath[];
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * FsWriteOp is an operation to write some data to a file.
 * Can be applied as either an object op or a world op.
 */
export interface FsWriteOp {
  /**
   * ObjectKey is the object key to start at.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** Path is the path to write to. */
  path:
    | FSPath
    | undefined;
  /** Offset is the location to write the data to. */
  offset: Long;
  /** BlobRef is the reference to the Blob to write. */
  blobRef:
    | BlockRef
    | undefined;
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * FsTruncateOp shrinks or extends a file to the specified size.
 * The extended part will be a sparse range (hole) reading as zeros.
 * Can be applied as either an object op or a world op.
 */
export interface FsTruncateOp {
  /**
   * ObjectKey is the object key to start at.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** Path is the path to write to. */
  path:
    | FSPath
    | undefined;
  /** FileSize is the new size to truncate to. */
  fileSize: Long;
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * FsCopyOp recursively copies a source path to a destination, overwriting destination.
 * Note: this does not allow cross-FS copy.
 * Can be applied as either an object op or a world op.
 */
export interface FsCopyOp {
  /**
   * ObjectKey is the object key to copy from.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** SrcPath is the path to copy from. */
  srcPath:
    | FSPath
    | undefined;
  /** DestPath is the path to copy to. */
  destPath:
    | FSPath
    | undefined;
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * FsRenameOp recursively moves a source path to a destination, overwriting destination.
 * This applies a FsCopyOp followed by a Remove in the same operation.
 * Note: this does not allow cross-FS rename.
 * Can be applied as either an object op or a world op.
 */
export interface FsRenameOp {
  /**
   * ObjectKey is the object key to copy from.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** SrcPath is the path to move from. */
  srcPath:
    | FSPath
    | undefined;
  /** DestPath is the path to move to. */
  destPath:
    | FSPath
    | undefined;
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * FsRemoveOp is an operation to delete inodes from the tree.
 * Can be applied as either an object op or a world op.
 */
export interface FsRemoveOp {
  /**
   * ObjectKey is the object key to start at.
   * Ignored if applied as an object op.
   */
  objectKey: string;
  /**
   * FsType is the type of object located at ObjectKey.
   * Defaults to FsType_FS_NODE.
   */
  fsType: FSType;
  /** Paths are the paths to delete. */
  paths: FSPath[];
  /** Timestamp is the modification time. */
  timestamp: Timestamp | undefined;
}

/**
 * MountValue is the contents of the quad value field for a mount.
 * inode -> unixfs/mount -> inode <value=mount-value>
 */
export interface MountValue {
  /**
   * Mountpoint is the path inside the inode to mount at.
   * If empty assumes /
   */
  mountpoint: string;
  /**
   * Prefix is the path inside the target inode to link to.
   * If empty assumes /
   */
  prefix: string;
}

/**
 * RefValue is the contents of the quad value field for a ref.
 * i.e.: somewhere git/workdir -> inode <ref-value>
 */
export interface RefValue {
  /**
   * FsType sets the expected filesystem object type at the target.
   * If unset (0) reads the type from the graph.
   * Defaults to FS_NODE if nothing else set.
   */
  fsType: FSType;
  /**
   * Path is the location within the FS.
   * If empty, defaults to / (the root).
   */
  path: FSPath | undefined;
}

function createBaseUnixfsRef(): UnixfsRef {
  return { objectKey: "", fsType: 0, path: undefined };
}

export const UnixfsRef = {
  encode(message: UnixfsRef, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    if (message.path !== undefined) {
      FSPath.encode(message.path, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): UnixfsRef {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseUnixfsRef();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.path = FSPath.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<UnixfsRef, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<UnixfsRef | UnixfsRef[]> | Iterable<UnixfsRef | UnixfsRef[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [UnixfsRef.encode(p).finish()];
        }
      } else {
        yield* [UnixfsRef.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, UnixfsRef>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<UnixfsRef> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [UnixfsRef.decode(p)];
        }
      } else {
        yield* [UnixfsRef.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): UnixfsRef {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      path: isSet(object.path) ? FSPath.fromJSON(object.path) : undefined,
    };
  },

  toJSON(message: UnixfsRef): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    message.path !== undefined && (obj.path = message.path ? FSPath.toJSON(message.path) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<UnixfsRef>, I>>(object: I): UnixfsRef {
    const message = createBaseUnixfsRef();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.path = (object.path !== undefined && object.path !== null) ? FSPath.fromPartial(object.path) : undefined;
    return message;
  },
};

function createBaseFsInitOp(): FsInitOp {
  return { objectKey: "", fsType: 0, fsRef: undefined, fsRefType: 0, fsOverwrite: false, timestamp: undefined };
}

export const FsInitOp = {
  encode(message: FsInitOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    if (message.fsRef !== undefined) {
      ObjectRef.encode(message.fsRef, writer.uint32(26).fork()).ldelim();
    }
    if (message.fsRefType !== 0) {
      writer.uint32(32).int32(message.fsRefType);
    }
    if (message.fsOverwrite === true) {
      writer.uint32(40).bool(message.fsOverwrite);
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsInitOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsInitOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.fsRef = ObjectRef.decode(reader, reader.uint32());
          break;
        case 4:
          message.fsRefType = reader.int32() as any;
          break;
        case 5:
          message.fsOverwrite = reader.bool();
          break;
        case 6:
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
  // Transform<FsInitOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FsInitOp | FsInitOp[]> | Iterable<FsInitOp | FsInitOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsInitOp.encode(p).finish()];
        }
      } else {
        yield* [FsInitOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsInitOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsInitOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsInitOp.decode(p)];
        }
      } else {
        yield* [FsInitOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsInitOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      fsRef: isSet(object.fsRef) ? ObjectRef.fromJSON(object.fsRef) : undefined,
      fsRefType: isSet(object.fsRefType) ? fSTypeFromJSON(object.fsRefType) : 0,
      fsOverwrite: isSet(object.fsOverwrite) ? Boolean(object.fsOverwrite) : false,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsInitOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    message.fsRef !== undefined && (obj.fsRef = message.fsRef ? ObjectRef.toJSON(message.fsRef) : undefined);
    message.fsRefType !== undefined && (obj.fsRefType = fSTypeToJSON(message.fsRefType));
    message.fsOverwrite !== undefined && (obj.fsOverwrite = message.fsOverwrite);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsInitOp>, I>>(object: I): FsInitOp {
    const message = createBaseFsInitOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.fsRef = (object.fsRef !== undefined && object.fsRef !== null)
      ? ObjectRef.fromPartial(object.fsRef)
      : undefined;
    message.fsRefType = object.fsRefType ?? 0;
    message.fsOverwrite = object.fsOverwrite ?? false;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsMknodOp(): FsMknodOp {
  return { objectKey: "", fsType: 0, paths: [], permissions: 0, nodeType: 0, timestamp: undefined };
}

export const FsMknodOp = {
  encode(message: FsMknodOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    for (const v of message.paths) {
      FSPath.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    if (message.permissions !== 0) {
      writer.uint32(32).uint32(message.permissions);
    }
    if (message.nodeType !== 0) {
      writer.uint32(40).int32(message.nodeType);
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsMknodOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsMknodOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.paths.push(FSPath.decode(reader, reader.uint32()));
          break;
        case 4:
          message.permissions = reader.uint32();
          break;
        case 5:
          message.nodeType = reader.int32() as any;
          break;
        case 6:
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
  // Transform<FsMknodOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FsMknodOp | FsMknodOp[]> | Iterable<FsMknodOp | FsMknodOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsMknodOp.encode(p).finish()];
        }
      } else {
        yield* [FsMknodOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsMknodOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsMknodOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsMknodOp.decode(p)];
        }
      } else {
        yield* [FsMknodOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsMknodOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      paths: Array.isArray(object?.paths) ? object.paths.map((e: any) => FSPath.fromJSON(e)) : [],
      permissions: isSet(object.permissions) ? Number(object.permissions) : 0,
      nodeType: isSet(object.nodeType) ? nodeTypeFromJSON(object.nodeType) : 0,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsMknodOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    if (message.paths) {
      obj.paths = message.paths.map((e) => e ? FSPath.toJSON(e) : undefined);
    } else {
      obj.paths = [];
    }
    message.permissions !== undefined && (obj.permissions = Math.round(message.permissions));
    message.nodeType !== undefined && (obj.nodeType = nodeTypeToJSON(message.nodeType));
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsMknodOp>, I>>(object: I): FsMknodOp {
    const message = createBaseFsMknodOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.paths = object.paths?.map((e) => FSPath.fromPartial(e)) || [];
    message.permissions = object.permissions ?? 0;
    message.nodeType = object.nodeType ?? 0;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsSymlinkOp(): FsSymlinkOp {
  return { objectKey: "", fsType: 0, path: undefined, symlink: undefined, timestamp: undefined };
}

export const FsSymlinkOp = {
  encode(message: FsSymlinkOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    if (message.path !== undefined) {
      FSPath.encode(message.path, writer.uint32(26).fork()).ldelim();
    }
    if (message.symlink !== undefined) {
      FSSymlink.encode(message.symlink, writer.uint32(34).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsSymlinkOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsSymlinkOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.path = FSPath.decode(reader, reader.uint32());
          break;
        case 4:
          message.symlink = FSSymlink.decode(reader, reader.uint32());
          break;
        case 5:
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
  // Transform<FsSymlinkOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FsSymlinkOp | FsSymlinkOp[]> | Iterable<FsSymlinkOp | FsSymlinkOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsSymlinkOp.encode(p).finish()];
        }
      } else {
        yield* [FsSymlinkOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsSymlinkOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsSymlinkOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsSymlinkOp.decode(p)];
        }
      } else {
        yield* [FsSymlinkOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsSymlinkOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      path: isSet(object.path) ? FSPath.fromJSON(object.path) : undefined,
      symlink: isSet(object.symlink) ? FSSymlink.fromJSON(object.symlink) : undefined,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsSymlinkOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    message.path !== undefined && (obj.path = message.path ? FSPath.toJSON(message.path) : undefined);
    message.symlink !== undefined && (obj.symlink = message.symlink ? FSSymlink.toJSON(message.symlink) : undefined);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsSymlinkOp>, I>>(object: I): FsSymlinkOp {
    const message = createBaseFsSymlinkOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.path = (object.path !== undefined && object.path !== null) ? FSPath.fromPartial(object.path) : undefined;
    message.symlink = (object.symlink !== undefined && object.symlink !== null)
      ? FSSymlink.fromPartial(object.symlink)
      : undefined;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsSetPermissionsOp(): FsSetPermissionsOp {
  return { objectKey: "", fsType: 0, paths: [], permissions: 0, timestamp: undefined };
}

export const FsSetPermissionsOp = {
  encode(message: FsSetPermissionsOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    for (const v of message.paths) {
      FSPath.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    if (message.permissions !== 0) {
      writer.uint32(32).uint32(message.permissions);
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsSetPermissionsOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsSetPermissionsOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.paths.push(FSPath.decode(reader, reader.uint32()));
          break;
        case 4:
          message.permissions = reader.uint32();
          break;
        case 5:
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
  // Transform<FsSetPermissionsOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FsSetPermissionsOp | FsSetPermissionsOp[]>
      | Iterable<FsSetPermissionsOp | FsSetPermissionsOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsSetPermissionsOp.encode(p).finish()];
        }
      } else {
        yield* [FsSetPermissionsOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsSetPermissionsOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsSetPermissionsOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsSetPermissionsOp.decode(p)];
        }
      } else {
        yield* [FsSetPermissionsOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsSetPermissionsOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      paths: Array.isArray(object?.paths) ? object.paths.map((e: any) => FSPath.fromJSON(e)) : [],
      permissions: isSet(object.permissions) ? Number(object.permissions) : 0,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsSetPermissionsOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    if (message.paths) {
      obj.paths = message.paths.map((e) => e ? FSPath.toJSON(e) : undefined);
    } else {
      obj.paths = [];
    }
    message.permissions !== undefined && (obj.permissions = Math.round(message.permissions));
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsSetPermissionsOp>, I>>(object: I): FsSetPermissionsOp {
    const message = createBaseFsSetPermissionsOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.paths = object.paths?.map((e) => FSPath.fromPartial(e)) || [];
    message.permissions = object.permissions ?? 0;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsSetModTimestampOp(): FsSetModTimestampOp {
  return { objectKey: "", fsType: 0, paths: [], timestamp: undefined };
}

export const FsSetModTimestampOp = {
  encode(message: FsSetModTimestampOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    for (const v of message.paths) {
      FSPath.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsSetModTimestampOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsSetModTimestampOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.paths.push(FSPath.decode(reader, reader.uint32()));
          break;
        case 4:
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
  // Transform<FsSetModTimestampOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FsSetModTimestampOp | FsSetModTimestampOp[]>
      | Iterable<FsSetModTimestampOp | FsSetModTimestampOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsSetModTimestampOp.encode(p).finish()];
        }
      } else {
        yield* [FsSetModTimestampOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsSetModTimestampOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsSetModTimestampOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsSetModTimestampOp.decode(p)];
        }
      } else {
        yield* [FsSetModTimestampOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsSetModTimestampOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      paths: Array.isArray(object?.paths) ? object.paths.map((e: any) => FSPath.fromJSON(e)) : [],
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsSetModTimestampOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    if (message.paths) {
      obj.paths = message.paths.map((e) => e ? FSPath.toJSON(e) : undefined);
    } else {
      obj.paths = [];
    }
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsSetModTimestampOp>, I>>(object: I): FsSetModTimestampOp {
    const message = createBaseFsSetModTimestampOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.paths = object.paths?.map((e) => FSPath.fromPartial(e)) || [];
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsWriteOp(): FsWriteOp {
  return { objectKey: "", fsType: 0, path: undefined, offset: Long.ZERO, blobRef: undefined, timestamp: undefined };
}

export const FsWriteOp = {
  encode(message: FsWriteOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    if (message.path !== undefined) {
      FSPath.encode(message.path, writer.uint32(26).fork()).ldelim();
    }
    if (!message.offset.isZero()) {
      writer.uint32(32).int64(message.offset);
    }
    if (message.blobRef !== undefined) {
      BlockRef.encode(message.blobRef, writer.uint32(42).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsWriteOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsWriteOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.path = FSPath.decode(reader, reader.uint32());
          break;
        case 4:
          message.offset = reader.int64() as Long;
          break;
        case 5:
          message.blobRef = BlockRef.decode(reader, reader.uint32());
          break;
        case 6:
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
  // Transform<FsWriteOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FsWriteOp | FsWriteOp[]> | Iterable<FsWriteOp | FsWriteOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsWriteOp.encode(p).finish()];
        }
      } else {
        yield* [FsWriteOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsWriteOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsWriteOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsWriteOp.decode(p)];
        }
      } else {
        yield* [FsWriteOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsWriteOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      path: isSet(object.path) ? FSPath.fromJSON(object.path) : undefined,
      offset: isSet(object.offset) ? Long.fromValue(object.offset) : Long.ZERO,
      blobRef: isSet(object.blobRef) ? BlockRef.fromJSON(object.blobRef) : undefined,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsWriteOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    message.path !== undefined && (obj.path = message.path ? FSPath.toJSON(message.path) : undefined);
    message.offset !== undefined && (obj.offset = (message.offset || Long.ZERO).toString());
    message.blobRef !== undefined && (obj.blobRef = message.blobRef ? BlockRef.toJSON(message.blobRef) : undefined);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsWriteOp>, I>>(object: I): FsWriteOp {
    const message = createBaseFsWriteOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.path = (object.path !== undefined && object.path !== null) ? FSPath.fromPartial(object.path) : undefined;
    message.offset = (object.offset !== undefined && object.offset !== null)
      ? Long.fromValue(object.offset)
      : Long.ZERO;
    message.blobRef = (object.blobRef !== undefined && object.blobRef !== null)
      ? BlockRef.fromPartial(object.blobRef)
      : undefined;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsTruncateOp(): FsTruncateOp {
  return { objectKey: "", fsType: 0, path: undefined, fileSize: Long.ZERO, timestamp: undefined };
}

export const FsTruncateOp = {
  encode(message: FsTruncateOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    if (message.path !== undefined) {
      FSPath.encode(message.path, writer.uint32(26).fork()).ldelim();
    }
    if (!message.fileSize.isZero()) {
      writer.uint32(32).int64(message.fileSize);
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsTruncateOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsTruncateOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.path = FSPath.decode(reader, reader.uint32());
          break;
        case 4:
          message.fileSize = reader.int64() as Long;
          break;
        case 5:
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
  // Transform<FsTruncateOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FsTruncateOp | FsTruncateOp[]> | Iterable<FsTruncateOp | FsTruncateOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsTruncateOp.encode(p).finish()];
        }
      } else {
        yield* [FsTruncateOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsTruncateOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsTruncateOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsTruncateOp.decode(p)];
        }
      } else {
        yield* [FsTruncateOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsTruncateOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      path: isSet(object.path) ? FSPath.fromJSON(object.path) : undefined,
      fileSize: isSet(object.fileSize) ? Long.fromValue(object.fileSize) : Long.ZERO,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsTruncateOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    message.path !== undefined && (obj.path = message.path ? FSPath.toJSON(message.path) : undefined);
    message.fileSize !== undefined && (obj.fileSize = (message.fileSize || Long.ZERO).toString());
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsTruncateOp>, I>>(object: I): FsTruncateOp {
    const message = createBaseFsTruncateOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.path = (object.path !== undefined && object.path !== null) ? FSPath.fromPartial(object.path) : undefined;
    message.fileSize = (object.fileSize !== undefined && object.fileSize !== null)
      ? Long.fromValue(object.fileSize)
      : Long.ZERO;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsCopyOp(): FsCopyOp {
  return { objectKey: "", fsType: 0, srcPath: undefined, destPath: undefined, timestamp: undefined };
}

export const FsCopyOp = {
  encode(message: FsCopyOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    if (message.srcPath !== undefined) {
      FSPath.encode(message.srcPath, writer.uint32(26).fork()).ldelim();
    }
    if (message.destPath !== undefined) {
      FSPath.encode(message.destPath, writer.uint32(34).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsCopyOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsCopyOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.srcPath = FSPath.decode(reader, reader.uint32());
          break;
        case 4:
          message.destPath = FSPath.decode(reader, reader.uint32());
          break;
        case 5:
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
  // Transform<FsCopyOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FsCopyOp | FsCopyOp[]> | Iterable<FsCopyOp | FsCopyOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsCopyOp.encode(p).finish()];
        }
      } else {
        yield* [FsCopyOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsCopyOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsCopyOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsCopyOp.decode(p)];
        }
      } else {
        yield* [FsCopyOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsCopyOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      srcPath: isSet(object.srcPath) ? FSPath.fromJSON(object.srcPath) : undefined,
      destPath: isSet(object.destPath) ? FSPath.fromJSON(object.destPath) : undefined,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsCopyOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    message.srcPath !== undefined && (obj.srcPath = message.srcPath ? FSPath.toJSON(message.srcPath) : undefined);
    message.destPath !== undefined && (obj.destPath = message.destPath ? FSPath.toJSON(message.destPath) : undefined);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsCopyOp>, I>>(object: I): FsCopyOp {
    const message = createBaseFsCopyOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.srcPath = (object.srcPath !== undefined && object.srcPath !== null)
      ? FSPath.fromPartial(object.srcPath)
      : undefined;
    message.destPath = (object.destPath !== undefined && object.destPath !== null)
      ? FSPath.fromPartial(object.destPath)
      : undefined;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsRenameOp(): FsRenameOp {
  return { objectKey: "", fsType: 0, srcPath: undefined, destPath: undefined, timestamp: undefined };
}

export const FsRenameOp = {
  encode(message: FsRenameOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    if (message.srcPath !== undefined) {
      FSPath.encode(message.srcPath, writer.uint32(26).fork()).ldelim();
    }
    if (message.destPath !== undefined) {
      FSPath.encode(message.destPath, writer.uint32(34).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsRenameOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsRenameOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.srcPath = FSPath.decode(reader, reader.uint32());
          break;
        case 4:
          message.destPath = FSPath.decode(reader, reader.uint32());
          break;
        case 5:
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
  // Transform<FsRenameOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FsRenameOp | FsRenameOp[]> | Iterable<FsRenameOp | FsRenameOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsRenameOp.encode(p).finish()];
        }
      } else {
        yield* [FsRenameOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsRenameOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsRenameOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsRenameOp.decode(p)];
        }
      } else {
        yield* [FsRenameOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsRenameOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      srcPath: isSet(object.srcPath) ? FSPath.fromJSON(object.srcPath) : undefined,
      destPath: isSet(object.destPath) ? FSPath.fromJSON(object.destPath) : undefined,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsRenameOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    message.srcPath !== undefined && (obj.srcPath = message.srcPath ? FSPath.toJSON(message.srcPath) : undefined);
    message.destPath !== undefined && (obj.destPath = message.destPath ? FSPath.toJSON(message.destPath) : undefined);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsRenameOp>, I>>(object: I): FsRenameOp {
    const message = createBaseFsRenameOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.srcPath = (object.srcPath !== undefined && object.srcPath !== null)
      ? FSPath.fromPartial(object.srcPath)
      : undefined;
    message.destPath = (object.destPath !== undefined && object.destPath !== null)
      ? FSPath.fromPartial(object.destPath)
      : undefined;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseFsRemoveOp(): FsRemoveOp {
  return { objectKey: "", fsType: 0, paths: [], timestamp: undefined };
}

export const FsRemoveOp = {
  encode(message: FsRemoveOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.fsType !== 0) {
      writer.uint32(16).int32(message.fsType);
    }
    for (const v of message.paths) {
      FSPath.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FsRemoveOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFsRemoveOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.fsType = reader.int32() as any;
          break;
        case 3:
          message.paths.push(FSPath.decode(reader, reader.uint32()));
          break;
        case 4:
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
  // Transform<FsRemoveOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FsRemoveOp | FsRemoveOp[]> | Iterable<FsRemoveOp | FsRemoveOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsRemoveOp.encode(p).finish()];
        }
      } else {
        yield* [FsRemoveOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FsRemoveOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FsRemoveOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FsRemoveOp.decode(p)];
        }
      } else {
        yield* [FsRemoveOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): FsRemoveOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      paths: Array.isArray(object?.paths) ? object.paths.map((e: any) => FSPath.fromJSON(e)) : [],
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: FsRemoveOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    if (message.paths) {
      obj.paths = message.paths.map((e) => e ? FSPath.toJSON(e) : undefined);
    } else {
      obj.paths = [];
    }
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<FsRemoveOp>, I>>(object: I): FsRemoveOp {
    const message = createBaseFsRemoveOp();
    message.objectKey = object.objectKey ?? "";
    message.fsType = object.fsType ?? 0;
    message.paths = object.paths?.map((e) => FSPath.fromPartial(e)) || [];
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseMountValue(): MountValue {
  return { mountpoint: "", prefix: "" };
}

export const MountValue = {
  encode(message: MountValue, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.mountpoint !== "") {
      writer.uint32(10).string(message.mountpoint);
    }
    if (message.prefix !== "") {
      writer.uint32(18).string(message.prefix);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MountValue {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMountValue();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.mountpoint = reader.string();
          break;
        case 2:
          message.prefix = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<MountValue, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<MountValue | MountValue[]> | Iterable<MountValue | MountValue[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MountValue.encode(p).finish()];
        }
      } else {
        yield* [MountValue.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MountValue>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<MountValue> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MountValue.decode(p)];
        }
      } else {
        yield* [MountValue.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): MountValue {
    return {
      mountpoint: isSet(object.mountpoint) ? String(object.mountpoint) : "",
      prefix: isSet(object.prefix) ? String(object.prefix) : "",
    };
  },

  toJSON(message: MountValue): unknown {
    const obj: any = {};
    message.mountpoint !== undefined && (obj.mountpoint = message.mountpoint);
    message.prefix !== undefined && (obj.prefix = message.prefix);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<MountValue>, I>>(object: I): MountValue {
    const message = createBaseMountValue();
    message.mountpoint = object.mountpoint ?? "";
    message.prefix = object.prefix ?? "";
    return message;
  },
};

function createBaseRefValue(): RefValue {
  return { fsType: 0, path: undefined };
}

export const RefValue = {
  encode(message: RefValue, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.fsType !== 0) {
      writer.uint32(8).int32(message.fsType);
    }
    if (message.path !== undefined) {
      FSPath.encode(message.path, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RefValue {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRefValue();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.fsType = reader.int32() as any;
          break;
        case 2:
          message.path = FSPath.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RefValue, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<RefValue | RefValue[]> | Iterable<RefValue | RefValue[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RefValue.encode(p).finish()];
        }
      } else {
        yield* [RefValue.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RefValue>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RefValue> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RefValue.decode(p)];
        }
      } else {
        yield* [RefValue.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): RefValue {
    return {
      fsType: isSet(object.fsType) ? fSTypeFromJSON(object.fsType) : 0,
      path: isSet(object.path) ? FSPath.fromJSON(object.path) : undefined,
    };
  },

  toJSON(message: RefValue): unknown {
    const obj: any = {};
    message.fsType !== undefined && (obj.fsType = fSTypeToJSON(message.fsType));
    message.path !== undefined && (obj.path = message.path ? FSPath.toJSON(message.path) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<RefValue>, I>>(object: I): RefValue {
    const message = createBaseRefValue();
    message.fsType = object.fsType ?? 0;
    message.path = (object.path !== undefined && object.path !== null) ? FSPath.fromPartial(object.path) : undefined;
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
