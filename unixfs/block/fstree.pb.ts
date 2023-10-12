/* eslint-disable */
import { Timestamp } from '@go/github.com/aperturerobotics/timestamp/timestamp.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../../block/block.pb.js'
import { File } from '../../block/file/file.pb.js'

export const protobufPackage = 'unixfs.block'

/** NodeType indicates the type of node. */
export enum NodeType {
  /** NodeType_UNKNOWN - NodeType_UNKNOWN is the unknown node type. */
  NodeType_UNKNOWN = 0,
  /** NodeType_DIRECTORY - NodeType_DIRECTORY is a directory node. */
  NodeType_DIRECTORY = 1,
  /** NodeType_FILE - NodeType_FILE is a file node. */
  NodeType_FILE = 2,
  /** NodeType_SYMLINK - NodeType_SYMLINK is a symbolic link. */
  NodeType_SYMLINK = 3,
  UNRECOGNIZED = -1,
}

export function nodeTypeFromJSON(object: any): NodeType {
  switch (object) {
    case 0:
    case 'NodeType_UNKNOWN':
      return NodeType.NodeType_UNKNOWN
    case 1:
    case 'NodeType_DIRECTORY':
      return NodeType.NodeType_DIRECTORY
    case 2:
    case 'NodeType_FILE':
      return NodeType.NodeType_FILE
    case 3:
    case 'NodeType_SYMLINK':
      return NodeType.NodeType_SYMLINK
    case -1:
    case 'UNRECOGNIZED':
    default:
      return NodeType.UNRECOGNIZED
  }
}

export function nodeTypeToJSON(object: NodeType): string {
  switch (object) {
    case NodeType.NodeType_UNKNOWN:
      return 'NodeType_UNKNOWN'
    case NodeType.NodeType_DIRECTORY:
      return 'NodeType_DIRECTORY'
    case NodeType.NodeType_FILE:
      return 'NodeType_FILE'
    case NodeType.NodeType_SYMLINK:
      return 'NodeType_SYMLINK'
    case NodeType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** FSChangeType is a type of change to the filesystem. */
export enum FSChangeType {
  FSChangeType_INVALID = 0,
  /** FSChangeType_MKNOD - FSChangeType_MKNOD creates one or more directory entries. */
  FSChangeType_MKNOD = 1,
  /** FSChangeType_FILE_WRITE - FSChangeType_FILE_WRITE writes a blob to a file node at an offset. */
  FSChangeType_FILE_WRITE = 2,
  /** FSChangeType_FILE_REMOVE - FSChangeType_REMOVE removes one or more nodes. */
  FSChangeType_FILE_REMOVE = 3,
  UNRECOGNIZED = -1,
}

export function fSChangeTypeFromJSON(object: any): FSChangeType {
  switch (object) {
    case 0:
    case 'FSChangeType_INVALID':
      return FSChangeType.FSChangeType_INVALID
    case 1:
    case 'FSChangeType_MKNOD':
      return FSChangeType.FSChangeType_MKNOD
    case 2:
    case 'FSChangeType_FILE_WRITE':
      return FSChangeType.FSChangeType_FILE_WRITE
    case 3:
    case 'FSChangeType_FILE_REMOVE':
      return FSChangeType.FSChangeType_FILE_REMOVE
    case -1:
    case 'UNRECOGNIZED':
    default:
      return FSChangeType.UNRECOGNIZED
  }
}

export function fSChangeTypeToJSON(object: FSChangeType): string {
  switch (object) {
    case FSChangeType.FSChangeType_INVALID:
      return 'FSChangeType_INVALID'
    case FSChangeType.FSChangeType_MKNOD:
      return 'FSChangeType_MKNOD'
    case FSChangeType.FSChangeType_FILE_WRITE:
      return 'FSChangeType_FILE_WRITE'
    case FSChangeType.FSChangeType_FILE_REMOVE:
      return 'FSChangeType_FILE_REMOVE'
    case FSChangeType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * FSNode is a node in the tree.
 * Roughly translates to an inode.
 */
export interface FSNode {
  /** NodeType marks the type of the node. */
  nodeType: NodeType
  /** ModTime is the modification timestamp. */
  modTime: Timestamp | undefined
  /**
   * Permissions are the unixfs permissions bitset.
   * Note: the mode portion of this field must be zero.
   */
  permissions: number
  /**
   * File contains a file if the node is of type FILE.
   *
   * sub-block ID 4
   */
  file: File | undefined
  /**
   * DirectoryEntry contains a sorted list of directory entries (dirents).
   *
   * *DirentSlice sub-block ID 5
   */
  directoryEntry: Dirent[]
  /** Symlink contains the symlink data if the node is of type SYMLINK. */
  symlink: FSSymlink | undefined
}

/** Dirent contains a directory entry. */
export interface Dirent {
  /** Name is the name of the directory entry. */
  name: string
  /**
   * NodeRef is the reference of the child FSNode.
   * may be empty.
   *
   * reference id 2
   */
  nodeRef: BlockRef | undefined
  /** NodeType is the node type of the child FSNode. */
  nodeType: NodeType
}

/** FSSymlink contains symbolic link data. */
export interface FSSymlink {
  /** TargetPath is the destination of the symbolic link. */
  targetPath: FSPath | undefined
}

/** FSObject is the root of a FSNode which may have edges to other dirents. */
export interface FSObject {
  /** Config is the filesystem configuration. */
  config: FSConfig | undefined
  /** FsNode is the root filesystem node. */
  fsNode: FSNode | undefined
  /**
   * LastChange is the current head of the changelog linked list.
   * If seqno == 0, this field is empty.
   * Seqno is incremented on change, even if disable_changelog is set.
   */
  lastChange: FSChange | undefined
}

/** FSHostVolume is a volume provided by the host environment. */
export interface FSHostVolume {
  /**
   * VolumeId is the host volume ID.
   * For example: the docker volume id.
   */
  volumeId: string
}

/** FSConfig are optional configuration flags. */
export interface FSConfig {
  /**
   * DisableChangelog indicates the changelog is not in use.
   * Watchers will perform a full cache flush on every block change.
   */
  disableChangelog: boolean
}

/** FSPath is a path in the filesystem. */
export interface FSPath {
  /** Nodes are the node names in the path. */
  nodes: string[]
  /** Absolute indicates the path is an absolute path (starting at /). */
  absolute: boolean
}

/**
 * FSChange is an entry in the changelog.
 * A transaction may convert into multiple changes.
 */
export interface FSChange {
  /** Seqno is the sequence number of this change. */
  seqno: Long
  /** PrevRef is the reference to the previous change. */
  prevRef: BlockRef | undefined
  /** ChangeType is the type of change this is. */
  changeType: FSChangeType
  /**
   * TransactionRef is the reference to the associated transaction.
   * This is transparent to the core registry code.
   */
  transactionRef: BlockRef | undefined
  /**
   * Paths are the associated paths.
   * Mknod: the nodes that will be created.
   * Write: the file node to write to.
   * Remove: the nodes to remove.
   */
  paths: FSPath[]
  /**
   * NodeType is the transaction node type.
   * Mknod: the type of node to create.
   */
  nodeType: NodeType
  /**
   * ValueRef are the reference(s) to the updated value.
   * Mknod: the references to the new inodes.
   * Write: this is the reference to the updated file inode.
   */
  valueRef: BlockRef[]
}

function createBaseFSNode(): FSNode {
  return {
    nodeType: 0,
    modTime: undefined,
    permissions: 0,
    file: undefined,
    directoryEntry: [],
    symlink: undefined,
  }
}

export const FSNode = {
  encode(
    message: FSNode,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.nodeType !== 0) {
      writer.uint32(8).int32(message.nodeType)
    }
    if (message.modTime !== undefined) {
      Timestamp.encode(message.modTime, writer.uint32(18).fork()).ldelim()
    }
    if (message.permissions !== 0) {
      writer.uint32(24).uint32(message.permissions)
    }
    if (message.file !== undefined) {
      File.encode(message.file, writer.uint32(34).fork()).ldelim()
    }
    for (const v of message.directoryEntry) {
      Dirent.encode(v!, writer.uint32(42).fork()).ldelim()
    }
    if (message.symlink !== undefined) {
      FSSymlink.encode(message.symlink, writer.uint32(50).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSNode {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSNode()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.nodeType = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.modTime = Timestamp.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.permissions = reader.uint32()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.file = File.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.directoryEntry.push(Dirent.decode(reader, reader.uint32()))
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.symlink = FSSymlink.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSNode, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FSNode | FSNode[]> | Iterable<FSNode | FSNode[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSNode.encode(p).finish()]
        }
      } else {
        yield* [FSNode.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSNode>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSNode> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSNode.decode(p)]
        }
      } else {
        yield* [FSNode.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSNode {
    return {
      nodeType: isSet(object.nodeType) ? nodeTypeFromJSON(object.nodeType) : 0,
      modTime: isSet(object.modTime)
        ? Timestamp.fromJSON(object.modTime)
        : undefined,
      permissions: isSet(object.permissions)
        ? globalThis.Number(object.permissions)
        : 0,
      file: isSet(object.file) ? File.fromJSON(object.file) : undefined,
      directoryEntry: globalThis.Array.isArray(object?.directoryEntry)
        ? object.directoryEntry.map((e: any) => Dirent.fromJSON(e))
        : [],
      symlink: isSet(object.symlink)
        ? FSSymlink.fromJSON(object.symlink)
        : undefined,
    }
  },

  toJSON(message: FSNode): unknown {
    const obj: any = {}
    if (message.nodeType !== 0) {
      obj.nodeType = nodeTypeToJSON(message.nodeType)
    }
    if (message.modTime !== undefined) {
      obj.modTime = Timestamp.toJSON(message.modTime)
    }
    if (message.permissions !== 0) {
      obj.permissions = Math.round(message.permissions)
    }
    if (message.file !== undefined) {
      obj.file = File.toJSON(message.file)
    }
    if (message.directoryEntry?.length) {
      obj.directoryEntry = message.directoryEntry.map((e) => Dirent.toJSON(e))
    }
    if (message.symlink !== undefined) {
      obj.symlink = FSSymlink.toJSON(message.symlink)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSNode>, I>>(base?: I): FSNode {
    return FSNode.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSNode>, I>>(object: I): FSNode {
    const message = createBaseFSNode()
    message.nodeType = object.nodeType ?? 0
    message.modTime =
      object.modTime !== undefined && object.modTime !== null
        ? Timestamp.fromPartial(object.modTime)
        : undefined
    message.permissions = object.permissions ?? 0
    message.file =
      object.file !== undefined && object.file !== null
        ? File.fromPartial(object.file)
        : undefined
    message.directoryEntry =
      object.directoryEntry?.map((e) => Dirent.fromPartial(e)) || []
    message.symlink =
      object.symlink !== undefined && object.symlink !== null
        ? FSSymlink.fromPartial(object.symlink)
        : undefined
    return message
  },
}

function createBaseDirent(): Dirent {
  return { name: '', nodeRef: undefined, nodeType: 0 }
}

export const Dirent = {
  encode(
    message: Dirent,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.nodeRef !== undefined) {
      BlockRef.encode(message.nodeRef, writer.uint32(18).fork()).ldelim()
    }
    if (message.nodeType !== 0) {
      writer.uint32(24).int32(message.nodeType)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Dirent {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDirent()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.name = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.nodeRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.nodeType = reader.int32() as any
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Dirent, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Dirent | Dirent[]> | Iterable<Dirent | Dirent[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Dirent.encode(p).finish()]
        }
      } else {
        yield* [Dirent.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Dirent>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Dirent> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Dirent.decode(p)]
        }
      } else {
        yield* [Dirent.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Dirent {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      nodeRef: isSet(object.nodeRef)
        ? BlockRef.fromJSON(object.nodeRef)
        : undefined,
      nodeType: isSet(object.nodeType) ? nodeTypeFromJSON(object.nodeType) : 0,
    }
  },

  toJSON(message: Dirent): unknown {
    const obj: any = {}
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.nodeRef !== undefined) {
      obj.nodeRef = BlockRef.toJSON(message.nodeRef)
    }
    if (message.nodeType !== 0) {
      obj.nodeType = nodeTypeToJSON(message.nodeType)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Dirent>, I>>(base?: I): Dirent {
    return Dirent.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Dirent>, I>>(object: I): Dirent {
    const message = createBaseDirent()
    message.name = object.name ?? ''
    message.nodeRef =
      object.nodeRef !== undefined && object.nodeRef !== null
        ? BlockRef.fromPartial(object.nodeRef)
        : undefined
    message.nodeType = object.nodeType ?? 0
    return message
  },
}

function createBaseFSSymlink(): FSSymlink {
  return { targetPath: undefined }
}

export const FSSymlink = {
  encode(
    message: FSSymlink,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.targetPath !== undefined) {
      FSPath.encode(message.targetPath, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSSymlink {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSSymlink()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.targetPath = FSPath.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSSymlink, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSSymlink | FSSymlink[]>
      | Iterable<FSSymlink | FSSymlink[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSSymlink.encode(p).finish()]
        }
      } else {
        yield* [FSSymlink.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSSymlink>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSSymlink> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSSymlink.decode(p)]
        }
      } else {
        yield* [FSSymlink.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSSymlink {
    return {
      targetPath: isSet(object.targetPath)
        ? FSPath.fromJSON(object.targetPath)
        : undefined,
    }
  },

  toJSON(message: FSSymlink): unknown {
    const obj: any = {}
    if (message.targetPath !== undefined) {
      obj.targetPath = FSPath.toJSON(message.targetPath)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSSymlink>, I>>(base?: I): FSSymlink {
    return FSSymlink.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSSymlink>, I>>(
    object: I,
  ): FSSymlink {
    const message = createBaseFSSymlink()
    message.targetPath =
      object.targetPath !== undefined && object.targetPath !== null
        ? FSPath.fromPartial(object.targetPath)
        : undefined
    return message
  },
}

function createBaseFSObject(): FSObject {
  return { config: undefined, fsNode: undefined, lastChange: undefined }
}

export const FSObject = {
  encode(
    message: FSObject,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.config !== undefined) {
      FSConfig.encode(message.config, writer.uint32(10).fork()).ldelim()
    }
    if (message.fsNode !== undefined) {
      FSNode.encode(message.fsNode, writer.uint32(18).fork()).ldelim()
    }
    if (message.lastChange !== undefined) {
      FSChange.encode(message.lastChange, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSObject {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSObject()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.config = FSConfig.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.fsNode = FSNode.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.lastChange = FSChange.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSObject, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSObject | FSObject[]>
      | Iterable<FSObject | FSObject[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSObject.encode(p).finish()]
        }
      } else {
        yield* [FSObject.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSObject>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSObject> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSObject.decode(p)]
        }
      } else {
        yield* [FSObject.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSObject {
    return {
      config: isSet(object.config)
        ? FSConfig.fromJSON(object.config)
        : undefined,
      fsNode: isSet(object.fsNode) ? FSNode.fromJSON(object.fsNode) : undefined,
      lastChange: isSet(object.lastChange)
        ? FSChange.fromJSON(object.lastChange)
        : undefined,
    }
  },

  toJSON(message: FSObject): unknown {
    const obj: any = {}
    if (message.config !== undefined) {
      obj.config = FSConfig.toJSON(message.config)
    }
    if (message.fsNode !== undefined) {
      obj.fsNode = FSNode.toJSON(message.fsNode)
    }
    if (message.lastChange !== undefined) {
      obj.lastChange = FSChange.toJSON(message.lastChange)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSObject>, I>>(base?: I): FSObject {
    return FSObject.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSObject>, I>>(object: I): FSObject {
    const message = createBaseFSObject()
    message.config =
      object.config !== undefined && object.config !== null
        ? FSConfig.fromPartial(object.config)
        : undefined
    message.fsNode =
      object.fsNode !== undefined && object.fsNode !== null
        ? FSNode.fromPartial(object.fsNode)
        : undefined
    message.lastChange =
      object.lastChange !== undefined && object.lastChange !== null
        ? FSChange.fromPartial(object.lastChange)
        : undefined
    return message
  },
}

function createBaseFSHostVolume(): FSHostVolume {
  return { volumeId: '' }
}

export const FSHostVolume = {
  encode(
    message: FSHostVolume,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.volumeId !== '') {
      writer.uint32(10).string(message.volumeId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSHostVolume {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSHostVolume()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.volumeId = reader.string()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSHostVolume, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSHostVolume | FSHostVolume[]>
      | Iterable<FSHostVolume | FSHostVolume[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSHostVolume.encode(p).finish()]
        }
      } else {
        yield* [FSHostVolume.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSHostVolume>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSHostVolume> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSHostVolume.decode(p)]
        }
      } else {
        yield* [FSHostVolume.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSHostVolume {
    return {
      volumeId: isSet(object.volumeId)
        ? globalThis.String(object.volumeId)
        : '',
    }
  },

  toJSON(message: FSHostVolume): unknown {
    const obj: any = {}
    if (message.volumeId !== '') {
      obj.volumeId = message.volumeId
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSHostVolume>, I>>(
    base?: I,
  ): FSHostVolume {
    return FSHostVolume.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSHostVolume>, I>>(
    object: I,
  ): FSHostVolume {
    const message = createBaseFSHostVolume()
    message.volumeId = object.volumeId ?? ''
    return message
  },
}

function createBaseFSConfig(): FSConfig {
  return { disableChangelog: false }
}

export const FSConfig = {
  encode(
    message: FSConfig,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.disableChangelog === true) {
      writer.uint32(8).bool(message.disableChangelog)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSConfig {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.disableChangelog = reader.bool()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSConfig | FSConfig[]>
      | Iterable<FSConfig | FSConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSConfig.encode(p).finish()]
        }
      } else {
        yield* [FSConfig.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSConfig>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSConfig> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSConfig.decode(p)]
        }
      } else {
        yield* [FSConfig.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSConfig {
    return {
      disableChangelog: isSet(object.disableChangelog)
        ? globalThis.Boolean(object.disableChangelog)
        : false,
    }
  },

  toJSON(message: FSConfig): unknown {
    const obj: any = {}
    if (message.disableChangelog === true) {
      obj.disableChangelog = message.disableChangelog
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSConfig>, I>>(base?: I): FSConfig {
    return FSConfig.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSConfig>, I>>(object: I): FSConfig {
    const message = createBaseFSConfig()
    message.disableChangelog = object.disableChangelog ?? false
    return message
  },
}

function createBaseFSPath(): FSPath {
  return { nodes: [], absolute: false }
}

export const FSPath = {
  encode(
    message: FSPath,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.nodes) {
      writer.uint32(10).string(v!)
    }
    if (message.absolute === true) {
      writer.uint32(16).bool(message.absolute)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSPath {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSPath()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.nodes.push(reader.string())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.absolute = reader.bool()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSPath, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<FSPath | FSPath[]> | Iterable<FSPath | FSPath[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSPath.encode(p).finish()]
        }
      } else {
        yield* [FSPath.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSPath>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSPath> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSPath.decode(p)]
        }
      } else {
        yield* [FSPath.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSPath {
    return {
      nodes: globalThis.Array.isArray(object?.nodes)
        ? object.nodes.map((e: any) => globalThis.String(e))
        : [],
      absolute: isSet(object.absolute)
        ? globalThis.Boolean(object.absolute)
        : false,
    }
  },

  toJSON(message: FSPath): unknown {
    const obj: any = {}
    if (message.nodes?.length) {
      obj.nodes = message.nodes
    }
    if (message.absolute === true) {
      obj.absolute = message.absolute
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSPath>, I>>(base?: I): FSPath {
    return FSPath.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSPath>, I>>(object: I): FSPath {
    const message = createBaseFSPath()
    message.nodes = object.nodes?.map((e) => e) || []
    message.absolute = object.absolute ?? false
    return message
  },
}

function createBaseFSChange(): FSChange {
  return {
    seqno: Long.UZERO,
    prevRef: undefined,
    changeType: 0,
    transactionRef: undefined,
    paths: [],
    nodeType: 0,
    valueRef: [],
  }
}

export const FSChange = {
  encode(
    message: FSChange,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.seqno.isZero()) {
      writer.uint32(8).uint64(message.seqno)
    }
    if (message.prevRef !== undefined) {
      BlockRef.encode(message.prevRef, writer.uint32(18).fork()).ldelim()
    }
    if (message.changeType !== 0) {
      writer.uint32(24).int32(message.changeType)
    }
    if (message.transactionRef !== undefined) {
      BlockRef.encode(message.transactionRef, writer.uint32(34).fork()).ldelim()
    }
    for (const v of message.paths) {
      FSPath.encode(v!, writer.uint32(42).fork()).ldelim()
    }
    if (message.nodeType !== 0) {
      writer.uint32(48).int32(message.nodeType)
    }
    for (const v of message.valueRef) {
      BlockRef.encode(v!, writer.uint32(66).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSChange {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSChange()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.seqno = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.prevRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.changeType = reader.int32() as any
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.transactionRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.paths.push(FSPath.decode(reader, reader.uint32()))
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.nodeType = reader.int32() as any
          continue
        case 8:
          if (tag !== 66) {
            break
          }

          message.valueRef.push(BlockRef.decode(reader, reader.uint32()))
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSChange, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSChange | FSChange[]>
      | Iterable<FSChange | FSChange[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSChange.encode(p).finish()]
        }
      } else {
        yield* [FSChange.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSChange>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSChange> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSChange.decode(p)]
        }
      } else {
        yield* [FSChange.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSChange {
    return {
      seqno: isSet(object.seqno) ? Long.fromValue(object.seqno) : Long.UZERO,
      prevRef: isSet(object.prevRef)
        ? BlockRef.fromJSON(object.prevRef)
        : undefined,
      changeType: isSet(object.changeType)
        ? fSChangeTypeFromJSON(object.changeType)
        : 0,
      transactionRef: isSet(object.transactionRef)
        ? BlockRef.fromJSON(object.transactionRef)
        : undefined,
      paths: globalThis.Array.isArray(object?.paths)
        ? object.paths.map((e: any) => FSPath.fromJSON(e))
        : [],
      nodeType: isSet(object.nodeType) ? nodeTypeFromJSON(object.nodeType) : 0,
      valueRef: globalThis.Array.isArray(object?.valueRef)
        ? object.valueRef.map((e: any) => BlockRef.fromJSON(e))
        : [],
    }
  },

  toJSON(message: FSChange): unknown {
    const obj: any = {}
    if (!message.seqno.isZero()) {
      obj.seqno = (message.seqno || Long.UZERO).toString()
    }
    if (message.prevRef !== undefined) {
      obj.prevRef = BlockRef.toJSON(message.prevRef)
    }
    if (message.changeType !== 0) {
      obj.changeType = fSChangeTypeToJSON(message.changeType)
    }
    if (message.transactionRef !== undefined) {
      obj.transactionRef = BlockRef.toJSON(message.transactionRef)
    }
    if (message.paths?.length) {
      obj.paths = message.paths.map((e) => FSPath.toJSON(e))
    }
    if (message.nodeType !== 0) {
      obj.nodeType = nodeTypeToJSON(message.nodeType)
    }
    if (message.valueRef?.length) {
      obj.valueRef = message.valueRef.map((e) => BlockRef.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSChange>, I>>(base?: I): FSChange {
    return FSChange.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSChange>, I>>(object: I): FSChange {
    const message = createBaseFSChange()
    message.seqno =
      object.seqno !== undefined && object.seqno !== null
        ? Long.fromValue(object.seqno)
        : Long.UZERO
    message.prevRef =
      object.prevRef !== undefined && object.prevRef !== null
        ? BlockRef.fromPartial(object.prevRef)
        : undefined
    message.changeType = object.changeType ?? 0
    message.transactionRef =
      object.transactionRef !== undefined && object.transactionRef !== null
        ? BlockRef.fromPartial(object.transactionRef)
        : undefined
    message.paths = object.paths?.map((e) => FSPath.fromPartial(e)) || []
    message.nodeType = object.nodeType ?? 0
    message.valueRef =
      object.valueRef?.map((e) => BlockRef.fromPartial(e)) || []
    return message
  },
}

type Builtin =
  | Date
  | Function
  | Uint8Array
  | string
  | number
  | boolean
  | undefined

export type DeepPartial<T> = T extends Builtin
  ? T
  : T extends Long
  ? string | number | Long
  : T extends globalThis.Array<infer U>
  ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string }
  ? { [K in keyof Omit<T, '$case'>]?: DeepPartial<T[K]> } & {
      $case: T['$case']
    }
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

type KeysOfUnion<T> = T extends T ? keyof T : never
export type Exact<P, I extends P> = P extends Builtin
  ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & {
      [K in Exclude<keyof I, KeysOfUnion<P>>]: never
    }

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
