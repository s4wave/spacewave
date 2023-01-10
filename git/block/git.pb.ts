/* eslint-disable */
import { Hash } from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
import { Timestamp } from '@go/github.com/aperturerobotics/timestamp/timestamp.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Blob, ChunkerArgs } from '../../block/blob/blob.pb.js'
import { BlockRef } from '../../block/block.pb.js'
import { KeyValueStore } from '../../kvtx/block/kvtx.pb.js'

export const protobufPackage = 'git.block'

/**
 * ReferenceType are the types of reference objects.
 * Note: the values match the Git reference type values.
 */
export enum ReferenceType {
  ReferenceType_INVALID = 0,
  ReferenceType_HASH = 1,
  ReferenceType_SYMBOLIC = 2,
  UNRECOGNIZED = -1,
}

export function referenceTypeFromJSON(object: any): ReferenceType {
  switch (object) {
    case 0:
    case 'ReferenceType_INVALID':
      return ReferenceType.ReferenceType_INVALID
    case 1:
    case 'ReferenceType_HASH':
      return ReferenceType.ReferenceType_HASH
    case 2:
    case 'ReferenceType_SYMBOLIC':
      return ReferenceType.ReferenceType_SYMBOLIC
    case -1:
    case 'UNRECOGNIZED':
    default:
      return ReferenceType.UNRECOGNIZED
  }
}

export function referenceTypeToJSON(object: ReferenceType): string {
  switch (object) {
    case ReferenceType.ReferenceType_INVALID:
      return 'ReferenceType_INVALID'
    case ReferenceType.ReferenceType_HASH:
      return 'ReferenceType_HASH'
    case ReferenceType.ReferenceType_SYMBOLIC:
      return 'ReferenceType_SYMBOLIC'
    case ReferenceType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * EncodedObjectType are the types of encoded objects.
 * Note: the values match the Git object type values.
 */
export enum EncodedObjectType {
  EncodedObjectType_INVALID = 0,
  EncodedObjectType_COMMIT = 1,
  EncodedObjectType_TREE = 2,
  EncodedObjectType_BLOB = 3,
  EncodedObjectType_TAG = 4,
  /** EncodedObjectType_OFS_DELTA - 5 reserved for future expansion */
  EncodedObjectType_OFS_DELTA = 6,
  EncodedObjectType_REF_DELTA = 7,
  UNRECOGNIZED = -1,
}

export function encodedObjectTypeFromJSON(object: any): EncodedObjectType {
  switch (object) {
    case 0:
    case 'EncodedObjectType_INVALID':
      return EncodedObjectType.EncodedObjectType_INVALID
    case 1:
    case 'EncodedObjectType_COMMIT':
      return EncodedObjectType.EncodedObjectType_COMMIT
    case 2:
    case 'EncodedObjectType_TREE':
      return EncodedObjectType.EncodedObjectType_TREE
    case 3:
    case 'EncodedObjectType_BLOB':
      return EncodedObjectType.EncodedObjectType_BLOB
    case 4:
    case 'EncodedObjectType_TAG':
      return EncodedObjectType.EncodedObjectType_TAG
    case 6:
    case 'EncodedObjectType_OFS_DELTA':
      return EncodedObjectType.EncodedObjectType_OFS_DELTA
    case 7:
    case 'EncodedObjectType_REF_DELTA':
      return EncodedObjectType.EncodedObjectType_REF_DELTA
    case -1:
    case 'UNRECOGNIZED':
    default:
      return EncodedObjectType.UNRECOGNIZED
  }
}

export function encodedObjectTypeToJSON(object: EncodedObjectType): string {
  switch (object) {
    case EncodedObjectType.EncodedObjectType_INVALID:
      return 'EncodedObjectType_INVALID'
    case EncodedObjectType.EncodedObjectType_COMMIT:
      return 'EncodedObjectType_COMMIT'
    case EncodedObjectType.EncodedObjectType_TREE:
      return 'EncodedObjectType_TREE'
    case EncodedObjectType.EncodedObjectType_BLOB:
      return 'EncodedObjectType_BLOB'
    case EncodedObjectType.EncodedObjectType_TAG:
      return 'EncodedObjectType_TAG'
    case EncodedObjectType.EncodedObjectType_OFS_DELTA:
      return 'EncodedObjectType_OFS_DELTA'
    case EncodedObjectType.EncodedObjectType_REF_DELTA:
      return 'EncodedObjectType_REF_DELTA'
    case EncodedObjectType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** TagMode is the available modes for fetching tags. */
export enum TagMode {
  /**
   * TagMode_DEFAULT - TagMode_DEFAULT indicates to use default tag mode.
   * Default is to fetch ALL.
   */
  TagMode_DEFAULT = 0,
  /** TagMode_NONE - TagMode_NONE fetches no tags from the remote. */
  TagMode_NONE = 1,
  /** TagMode_ALL - TagMode_ALL fetches all tags from the remote. */
  TagMode_ALL = 2,
  /** TagMode_FOLLOWING - TagMode_FOLLOWING fetches only tags that refer to commits being cloned. */
  TagMode_FOLLOWING = 3,
  UNRECOGNIZED = -1,
}

export function tagModeFromJSON(object: any): TagMode {
  switch (object) {
    case 0:
    case 'TagMode_DEFAULT':
      return TagMode.TagMode_DEFAULT
    case 1:
    case 'TagMode_NONE':
      return TagMode.TagMode_NONE
    case 2:
    case 'TagMode_ALL':
      return TagMode.TagMode_ALL
    case 3:
    case 'TagMode_FOLLOWING':
      return TagMode.TagMode_FOLLOWING
    case -1:
    case 'UNRECOGNIZED':
    default:
      return TagMode.UNRECOGNIZED
  }
}

export function tagModeToJSON(object: TagMode): string {
  switch (object) {
    case TagMode.TagMode_DEFAULT:
      return 'TagMode_DEFAULT'
    case TagMode.TagMode_NONE:
      return 'TagMode_NONE'
    case TagMode.TagMode_ALL:
      return 'TagMode_ALL'
    case TagMode.TagMode_FOLLOWING:
      return 'TagMode_FOLLOWING'
    case TagMode.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * Repo contains a Git repository.
 * Changes are transactional and deterministic.
 */
export interface Repo {
  /** ReferencesStore contains the named references store. */
  referencesStore: ReferencesStore | undefined
  /** ModuleReferencesStore contains the named submodules store. */
  moduleReferencesStore: ModuleReferencesStore | undefined
  /** EncodedObjectStore contains the encoded objects tree. */
  encodedObjectStore: EncodedObjectStore | undefined
  /** ShallowRefsStoreRef contains the list of shallow refs. */
  shallowRefsStoreRef: BlockRef | undefined
  /**
   * GitConfig contains the git configuration marshaled in Git format.
   * Some fields are always dropped, and repo is always marked as bare.
   */
  gitConfig: string
}

/** EncodedObjectStore contains the encoded objects store. */
export interface EncodedObjectStore {
  /** KvtxRoot is the root of the object tree. */
  kvtxRoot: KeyValueStore | undefined
  /** ChunkerArgs are arguments passed to ensure consistent chunking. */
  chunkerArgs: ChunkerArgs | undefined
}

/** ReferencesStore maps between ReferenceName and Reference. */
export interface ReferencesStore {
  /**
   * KvtxRoot is the root of the reference tree.
   * Contains value type Reference.
   */
  kvtxRoot: KeyValueStore | undefined
}

/** ModuleReferences maps between submodule name and a block ref to the Repo. */
export interface ModuleReferencesStore {
  /**
   * KvtxRoot is the root of the module reference tree.
   * Key: submodule name, value: Submodule object.
   */
  kvtxRoot: KeyValueStore | undefined
}

/** ShallowRefsStore contains the list of shallow refs. */
export interface ShallowRefsStore {
  /** ShallowRefs contains the list of shallow reference hashes. */
  shallowRefs: Hash[]
}

/** Submodule contains a sub-module reference. */
export interface Submodule {
  /** Name is the name of the submodule. */
  name: string
  /** RepoRef is the reference to the Repo object. */
  repoRef: BlockRef | undefined
}

/**
 * Reference contains a repository reference.
 * Go type: plumbing.Reference
 */
export interface Reference {
  /**
   * Name contains the reference name.
   * Go type: plumbing.ReferenceName
   */
  name: string
  /**
   * ReferenceType contains the reference type.
   * One of: HashReference(1), SymbolicReference(2)
   */
  referenceType: ReferenceType
  /**
   * Hash contains the sha1 hash (20 bytes) if hash reference.
   * Note: currently, this is enforced to hash type SHA1.
   */
  hash: Hash | undefined
  /** TargetReferenceName is the target reference name if symbolic. */
  targetReferenceName: string
}

/** EncodedObject contains an encoded object, stored as a Blob. */
export interface EncodedObject {
  /** DataBlob is the encoded object data. */
  dataBlob: Blob | undefined
  /**
   * DataHash is the hash of DataBlob.
   * Note: currently, this is enforced to hash type SHA1.
   */
  dataHash: Hash | undefined
  /** ObjectType is the encoded object type. */
  encodedObjectType: EncodedObjectType
}

/** Index stores a git index. */
export interface Index {
  /** Version is the index version (usually 2). */
  version: number
  /** Entries is the list of entries represented by the Index. */
  entries: IndexEntry[]
  /** Cache represents the "cached tree" extension. */
  cache: Tree | undefined
  /** ResolveUndo represents the "resolve undo" extension. */
  resolveUndo: ResolveUndo | undefined
  /** EndOfIndexEntry represents the "End of Index Entry" extension */
  endOfIndexEntry: EndOfIndexEntry | undefined
}

/**
 * Tree contains pre-computed hashes for trees that can be derived from the
 * index. It helps speed up tree object generation from index for a new commit.
 */
export interface Tree {
  /** Entries are entries in the tree. */
  entries: TreeEntry[]
}

/** TreeEntry is an entry in the tree. */
export interface TreeEntry {
  /** Path is the path within the parent directory. */
  path: string
  /**
   * Entries is the number of entries in the index that is covered by the tree
   * this entry represents.
   */
  entries: number
  /** Trees is the number that represents the number of subtrees this tree has. */
  trees: number
  /**
   * Hash is the hash of the object that would result from writing this span of
   * index as a tree. Note: currently this is sha1.
   */
  hash: Hash | undefined
}

/**
 * ResolveUndo is used when a conflict is resolved (e.g. with "git add path"),
 * these higher stage entries are removed and a stage-0 entry with proper
 * resolution is added. When these higher stage entries are removed, they are
 * saved in the resolve undo extension.
 */
export interface ResolveUndo {
  /** Entries is the list of resolve undo entries. */
  entries: ResolveUndoEntry[]
}

/** ResolveUndoEntry contains the information about a conflict when is resolved. */
export interface ResolveUndoEntry {
  /** Path is the path within the parent directory. */
  path: string
  /** Stages are the merge conflict stages. */
  stages: { [key: number]: Hash }
}

export interface ResolveUndoEntry_StagesEntry {
  key: number
  value: Hash | undefined
}

/**
 * EndOfIndexEntry is the End of Index Entry (EOIE) is used to locate the end of
 * the variable length index entries and the beginning of the extensions.
 */
export interface EndOfIndexEntry {
  /** Offset is the offset to the end of the index entries. */
  offset: number
  /** Hash is the sha-1 of the extension types and their sizes. */
  hash: Hash | undefined
}

/** IndexEntry stores an entry in the git index. */
export interface IndexEntry {
  /**
   * DataHash is the hash of the index entry.
   * Note: currently, this is enforced to hash type SHA1.
   */
  dataHash: Hash | undefined
  /** Name is the entry path name relative to top directory. */
  name: string
  /** CreatedAt is the time when the path was created. */
  createdAt: Timestamp | undefined
  /** ModifiedAt is the time when the path was modified. */
  modifiedAt: Timestamp | undefined
  /** Dev is the device of the tracked path. */
  dev: number
  /** Inode is the inode of the tracked path. */
  inode: number
  /**
   * Mode is the Git file mode used for the entry.
   * i.e. Dir, Regular, Executable, Symlink, Submodule
   */
  fileMode: number
  /** Uid is the user id of the owner. */
  uid: number
  /** Gid is the group id of the owner. */
  gid: number
  /** Size is the length in bytes for regular files. */
  size: number
  /**
   * Stage contains the merging state of the index item.
   * https://git-scm.com/book/en/v2/Git-Tools-Advanced-Merging
   */
  stage: number
  /** SkipWorktree is used in sparse checkouts. */
  skipWorktree: boolean
  /**
   * IntentToAdd indicates the path will be added later.
   * git add -N
   */
  intentToAdd: boolean
}

/** AuthOpts configures strategies for authenticating with a Git server. */
export interface AuthOpts {
  /** Username is the ssh username to authenticate with. */
  username: string
  /** PeerId configures looking up peer priv key by id for ssh. */
  peerId: string
}

/** CloneOpts are options for a Git clone. */
export interface CloneOpts {
  /** Url is the Git URL to clone from. */
  url: string
  /** RemoteName is the name of the remote to add, by default "origin." */
  remoteName: string
  /** Ref is the reference name to clone, uses default if empty. */
  ref: string
  /** SingleBranch fetches the ref and nothing more. */
  singleBranch: boolean
  /** DisableCheckout disables setting the Worktree and Workdir. */
  disableCheckout: boolean
  /**
   * Depth limits to the specific number of commits.
   * If zero, fetches all of the commits.
   */
  depth: number
  /** Recursive indicates submodules will be fetched as well. */
  recursive: boolean
  /** TagMode controls the fetching of tags. */
  tagMode: TagMode
  /** Insecure indicates that TLS checks should be skipped. */
  insecure: boolean
  /** CaBundle contains additional CA certificates to trust. */
  caBundle: string
}

/** CheckoutOpts are options when checking out a repo. */
export interface CheckoutOpts {
  /**
   * Commit is the commit hash to check out.
   * Note: currently, this is enforced to hash type SHA1.
   */
  commit: Hash | undefined
  /**
   * Branch is the branch to check out.
   * If !create, cannot be set if commit is also set.
   */
  branch: string
  /** Create indicates to create a branch from the specified commit. */
  create: boolean
  /**
   * Force indicates to continue even if index or working tree is not HEAD.
   * Throws away any changes when checking out.
   */
  force: boolean
  /**
   * Keep maintains index or working dir changes.
   * Cannot be set if force is also set.
   */
  keep: boolean
}

function createBaseRepo(): Repo {
  return {
    referencesStore: undefined,
    moduleReferencesStore: undefined,
    encodedObjectStore: undefined,
    shallowRefsStoreRef: undefined,
    gitConfig: '',
  }
}

export const Repo = {
  encode(message: Repo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.referencesStore !== undefined) {
      ReferencesStore.encode(
        message.referencesStore,
        writer.uint32(10).fork()
      ).ldelim()
    }
    if (message.moduleReferencesStore !== undefined) {
      ModuleReferencesStore.encode(
        message.moduleReferencesStore,
        writer.uint32(18).fork()
      ).ldelim()
    }
    if (message.encodedObjectStore !== undefined) {
      EncodedObjectStore.encode(
        message.encodedObjectStore,
        writer.uint32(26).fork()
      ).ldelim()
    }
    if (message.shallowRefsStoreRef !== undefined) {
      BlockRef.encode(
        message.shallowRefsStoreRef,
        writer.uint32(34).fork()
      ).ldelim()
    }
    if (message.gitConfig !== '') {
      writer.uint32(42).string(message.gitConfig)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Repo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRepo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.referencesStore = ReferencesStore.decode(
            reader,
            reader.uint32()
          )
          break
        case 2:
          message.moduleReferencesStore = ModuleReferencesStore.decode(
            reader,
            reader.uint32()
          )
          break
        case 3:
          message.encodedObjectStore = EncodedObjectStore.decode(
            reader,
            reader.uint32()
          )
          break
        case 4:
          message.shallowRefsStoreRef = BlockRef.decode(reader, reader.uint32())
          break
        case 5:
          message.gitConfig = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Repo, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Repo | Repo[]> | Iterable<Repo | Repo[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Repo.encode(p).finish()]
        }
      } else {
        yield* [Repo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Repo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Repo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Repo.decode(p)]
        }
      } else {
        yield* [Repo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Repo {
    return {
      referencesStore: isSet(object.referencesStore)
        ? ReferencesStore.fromJSON(object.referencesStore)
        : undefined,
      moduleReferencesStore: isSet(object.moduleReferencesStore)
        ? ModuleReferencesStore.fromJSON(object.moduleReferencesStore)
        : undefined,
      encodedObjectStore: isSet(object.encodedObjectStore)
        ? EncodedObjectStore.fromJSON(object.encodedObjectStore)
        : undefined,
      shallowRefsStoreRef: isSet(object.shallowRefsStoreRef)
        ? BlockRef.fromJSON(object.shallowRefsStoreRef)
        : undefined,
      gitConfig: isSet(object.gitConfig) ? String(object.gitConfig) : '',
    }
  },

  toJSON(message: Repo): unknown {
    const obj: any = {}
    message.referencesStore !== undefined &&
      (obj.referencesStore = message.referencesStore
        ? ReferencesStore.toJSON(message.referencesStore)
        : undefined)
    message.moduleReferencesStore !== undefined &&
      (obj.moduleReferencesStore = message.moduleReferencesStore
        ? ModuleReferencesStore.toJSON(message.moduleReferencesStore)
        : undefined)
    message.encodedObjectStore !== undefined &&
      (obj.encodedObjectStore = message.encodedObjectStore
        ? EncodedObjectStore.toJSON(message.encodedObjectStore)
        : undefined)
    message.shallowRefsStoreRef !== undefined &&
      (obj.shallowRefsStoreRef = message.shallowRefsStoreRef
        ? BlockRef.toJSON(message.shallowRefsStoreRef)
        : undefined)
    message.gitConfig !== undefined && (obj.gitConfig = message.gitConfig)
    return obj
  },

  create<I extends Exact<DeepPartial<Repo>, I>>(base?: I): Repo {
    return Repo.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Repo>, I>>(object: I): Repo {
    const message = createBaseRepo()
    message.referencesStore =
      object.referencesStore !== undefined && object.referencesStore !== null
        ? ReferencesStore.fromPartial(object.referencesStore)
        : undefined
    message.moduleReferencesStore =
      object.moduleReferencesStore !== undefined &&
      object.moduleReferencesStore !== null
        ? ModuleReferencesStore.fromPartial(object.moduleReferencesStore)
        : undefined
    message.encodedObjectStore =
      object.encodedObjectStore !== undefined &&
      object.encodedObjectStore !== null
        ? EncodedObjectStore.fromPartial(object.encodedObjectStore)
        : undefined
    message.shallowRefsStoreRef =
      object.shallowRefsStoreRef !== undefined &&
      object.shallowRefsStoreRef !== null
        ? BlockRef.fromPartial(object.shallowRefsStoreRef)
        : undefined
    message.gitConfig = object.gitConfig ?? ''
    return message
  },
}

function createBaseEncodedObjectStore(): EncodedObjectStore {
  return { kvtxRoot: undefined, chunkerArgs: undefined }
}

export const EncodedObjectStore = {
  encode(
    message: EncodedObjectStore,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.kvtxRoot !== undefined) {
      KeyValueStore.encode(message.kvtxRoot, writer.uint32(10).fork()).ldelim()
    }
    if (message.chunkerArgs !== undefined) {
      ChunkerArgs.encode(message.chunkerArgs, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EncodedObjectStore {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEncodedObjectStore()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.kvtxRoot = KeyValueStore.decode(reader, reader.uint32())
          break
        case 2:
          message.chunkerArgs = ChunkerArgs.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EncodedObjectStore, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<EncodedObjectStore | EncodedObjectStore[]>
      | Iterable<EncodedObjectStore | EncodedObjectStore[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EncodedObjectStore.encode(p).finish()]
        }
      } else {
        yield* [EncodedObjectStore.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EncodedObjectStore>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<EncodedObjectStore> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EncodedObjectStore.decode(p)]
        }
      } else {
        yield* [EncodedObjectStore.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): EncodedObjectStore {
    return {
      kvtxRoot: isSet(object.kvtxRoot)
        ? KeyValueStore.fromJSON(object.kvtxRoot)
        : undefined,
      chunkerArgs: isSet(object.chunkerArgs)
        ? ChunkerArgs.fromJSON(object.chunkerArgs)
        : undefined,
    }
  },

  toJSON(message: EncodedObjectStore): unknown {
    const obj: any = {}
    message.kvtxRoot !== undefined &&
      (obj.kvtxRoot = message.kvtxRoot
        ? KeyValueStore.toJSON(message.kvtxRoot)
        : undefined)
    message.chunkerArgs !== undefined &&
      (obj.chunkerArgs = message.chunkerArgs
        ? ChunkerArgs.toJSON(message.chunkerArgs)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<EncodedObjectStore>, I>>(
    base?: I
  ): EncodedObjectStore {
    return EncodedObjectStore.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<EncodedObjectStore>, I>>(
    object: I
  ): EncodedObjectStore {
    const message = createBaseEncodedObjectStore()
    message.kvtxRoot =
      object.kvtxRoot !== undefined && object.kvtxRoot !== null
        ? KeyValueStore.fromPartial(object.kvtxRoot)
        : undefined
    message.chunkerArgs =
      object.chunkerArgs !== undefined && object.chunkerArgs !== null
        ? ChunkerArgs.fromPartial(object.chunkerArgs)
        : undefined
    return message
  },
}

function createBaseReferencesStore(): ReferencesStore {
  return { kvtxRoot: undefined }
}

export const ReferencesStore = {
  encode(
    message: ReferencesStore,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.kvtxRoot !== undefined) {
      KeyValueStore.encode(message.kvtxRoot, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReferencesStore {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseReferencesStore()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.kvtxRoot = KeyValueStore.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ReferencesStore, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ReferencesStore | ReferencesStore[]>
      | Iterable<ReferencesStore | ReferencesStore[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReferencesStore.encode(p).finish()]
        }
      } else {
        yield* [ReferencesStore.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReferencesStore>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ReferencesStore> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ReferencesStore.decode(p)]
        }
      } else {
        yield* [ReferencesStore.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ReferencesStore {
    return {
      kvtxRoot: isSet(object.kvtxRoot)
        ? KeyValueStore.fromJSON(object.kvtxRoot)
        : undefined,
    }
  },

  toJSON(message: ReferencesStore): unknown {
    const obj: any = {}
    message.kvtxRoot !== undefined &&
      (obj.kvtxRoot = message.kvtxRoot
        ? KeyValueStore.toJSON(message.kvtxRoot)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<ReferencesStore>, I>>(
    base?: I
  ): ReferencesStore {
    return ReferencesStore.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ReferencesStore>, I>>(
    object: I
  ): ReferencesStore {
    const message = createBaseReferencesStore()
    message.kvtxRoot =
      object.kvtxRoot !== undefined && object.kvtxRoot !== null
        ? KeyValueStore.fromPartial(object.kvtxRoot)
        : undefined
    return message
  },
}

function createBaseModuleReferencesStore(): ModuleReferencesStore {
  return { kvtxRoot: undefined }
}

export const ModuleReferencesStore = {
  encode(
    message: ModuleReferencesStore,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.kvtxRoot !== undefined) {
      KeyValueStore.encode(message.kvtxRoot, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): ModuleReferencesStore {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseModuleReferencesStore()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.kvtxRoot = KeyValueStore.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ModuleReferencesStore, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ModuleReferencesStore | ModuleReferencesStore[]>
      | Iterable<ModuleReferencesStore | ModuleReferencesStore[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ModuleReferencesStore.encode(p).finish()]
        }
      } else {
        yield* [ModuleReferencesStore.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ModuleReferencesStore>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ModuleReferencesStore> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ModuleReferencesStore.decode(p)]
        }
      } else {
        yield* [ModuleReferencesStore.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ModuleReferencesStore {
    return {
      kvtxRoot: isSet(object.kvtxRoot)
        ? KeyValueStore.fromJSON(object.kvtxRoot)
        : undefined,
    }
  },

  toJSON(message: ModuleReferencesStore): unknown {
    const obj: any = {}
    message.kvtxRoot !== undefined &&
      (obj.kvtxRoot = message.kvtxRoot
        ? KeyValueStore.toJSON(message.kvtxRoot)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<ModuleReferencesStore>, I>>(
    base?: I
  ): ModuleReferencesStore {
    return ModuleReferencesStore.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ModuleReferencesStore>, I>>(
    object: I
  ): ModuleReferencesStore {
    const message = createBaseModuleReferencesStore()
    message.kvtxRoot =
      object.kvtxRoot !== undefined && object.kvtxRoot !== null
        ? KeyValueStore.fromPartial(object.kvtxRoot)
        : undefined
    return message
  },
}

function createBaseShallowRefsStore(): ShallowRefsStore {
  return { shallowRefs: [] }
}

export const ShallowRefsStore = {
  encode(
    message: ShallowRefsStore,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.shallowRefs) {
      Hash.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ShallowRefsStore {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseShallowRefsStore()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.shallowRefs.push(Hash.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ShallowRefsStore, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ShallowRefsStore | ShallowRefsStore[]>
      | Iterable<ShallowRefsStore | ShallowRefsStore[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ShallowRefsStore.encode(p).finish()]
        }
      } else {
        yield* [ShallowRefsStore.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ShallowRefsStore>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ShallowRefsStore> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ShallowRefsStore.decode(p)]
        }
      } else {
        yield* [ShallowRefsStore.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ShallowRefsStore {
    return {
      shallowRefs: Array.isArray(object?.shallowRefs)
        ? object.shallowRefs.map((e: any) => Hash.fromJSON(e))
        : [],
    }
  },

  toJSON(message: ShallowRefsStore): unknown {
    const obj: any = {}
    if (message.shallowRefs) {
      obj.shallowRefs = message.shallowRefs.map((e) =>
        e ? Hash.toJSON(e) : undefined
      )
    } else {
      obj.shallowRefs = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ShallowRefsStore>, I>>(
    base?: I
  ): ShallowRefsStore {
    return ShallowRefsStore.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ShallowRefsStore>, I>>(
    object: I
  ): ShallowRefsStore {
    const message = createBaseShallowRefsStore()
    message.shallowRefs =
      object.shallowRefs?.map((e) => Hash.fromPartial(e)) || []
    return message
  },
}

function createBaseSubmodule(): Submodule {
  return { name: '', repoRef: undefined }
}

export const Submodule = {
  encode(
    message: Submodule,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.repoRef !== undefined) {
      BlockRef.encode(message.repoRef, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Submodule {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseSubmodule()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string()
          break
        case 2:
          message.repoRef = BlockRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Submodule, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Submodule | Submodule[]>
      | Iterable<Submodule | Submodule[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Submodule.encode(p).finish()]
        }
      } else {
        yield* [Submodule.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Submodule>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Submodule> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Submodule.decode(p)]
        }
      } else {
        yield* [Submodule.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Submodule {
    return {
      name: isSet(object.name) ? String(object.name) : '',
      repoRef: isSet(object.repoRef)
        ? BlockRef.fromJSON(object.repoRef)
        : undefined,
    }
  },

  toJSON(message: Submodule): unknown {
    const obj: any = {}
    message.name !== undefined && (obj.name = message.name)
    message.repoRef !== undefined &&
      (obj.repoRef = message.repoRef
        ? BlockRef.toJSON(message.repoRef)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<Submodule>, I>>(base?: I): Submodule {
    return Submodule.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Submodule>, I>>(
    object: I
  ): Submodule {
    const message = createBaseSubmodule()
    message.name = object.name ?? ''
    message.repoRef =
      object.repoRef !== undefined && object.repoRef !== null
        ? BlockRef.fromPartial(object.repoRef)
        : undefined
    return message
  },
}

function createBaseReference(): Reference {
  return {
    name: '',
    referenceType: 0,
    hash: undefined,
    targetReferenceName: '',
  }
}

export const Reference = {
  encode(
    message: Reference,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.referenceType !== 0) {
      writer.uint32(16).int32(message.referenceType)
    }
    if (message.hash !== undefined) {
      Hash.encode(message.hash, writer.uint32(26).fork()).ldelim()
    }
    if (message.targetReferenceName !== '') {
      writer.uint32(34).string(message.targetReferenceName)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Reference {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseReference()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string()
          break
        case 2:
          message.referenceType = reader.int32() as any
          break
        case 3:
          message.hash = Hash.decode(reader, reader.uint32())
          break
        case 4:
          message.targetReferenceName = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Reference, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Reference | Reference[]>
      | Iterable<Reference | Reference[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Reference.encode(p).finish()]
        }
      } else {
        yield* [Reference.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Reference>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Reference> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Reference.decode(p)]
        }
      } else {
        yield* [Reference.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Reference {
    return {
      name: isSet(object.name) ? String(object.name) : '',
      referenceType: isSet(object.referenceType)
        ? referenceTypeFromJSON(object.referenceType)
        : 0,
      hash: isSet(object.hash) ? Hash.fromJSON(object.hash) : undefined,
      targetReferenceName: isSet(object.targetReferenceName)
        ? String(object.targetReferenceName)
        : '',
    }
  },

  toJSON(message: Reference): unknown {
    const obj: any = {}
    message.name !== undefined && (obj.name = message.name)
    message.referenceType !== undefined &&
      (obj.referenceType = referenceTypeToJSON(message.referenceType))
    message.hash !== undefined &&
      (obj.hash = message.hash ? Hash.toJSON(message.hash) : undefined)
    message.targetReferenceName !== undefined &&
      (obj.targetReferenceName = message.targetReferenceName)
    return obj
  },

  create<I extends Exact<DeepPartial<Reference>, I>>(base?: I): Reference {
    return Reference.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Reference>, I>>(
    object: I
  ): Reference {
    const message = createBaseReference()
    message.name = object.name ?? ''
    message.referenceType = object.referenceType ?? 0
    message.hash =
      object.hash !== undefined && object.hash !== null
        ? Hash.fromPartial(object.hash)
        : undefined
    message.targetReferenceName = object.targetReferenceName ?? ''
    return message
  },
}

function createBaseEncodedObject(): EncodedObject {
  return { dataBlob: undefined, dataHash: undefined, encodedObjectType: 0 }
}

export const EncodedObject = {
  encode(
    message: EncodedObject,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.dataBlob !== undefined) {
      Blob.encode(message.dataBlob, writer.uint32(10).fork()).ldelim()
    }
    if (message.dataHash !== undefined) {
      Hash.encode(message.dataHash, writer.uint32(18).fork()).ldelim()
    }
    if (message.encodedObjectType !== 0) {
      writer.uint32(24).int32(message.encodedObjectType)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EncodedObject {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEncodedObject()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.dataBlob = Blob.decode(reader, reader.uint32())
          break
        case 2:
          message.dataHash = Hash.decode(reader, reader.uint32())
          break
        case 3:
          message.encodedObjectType = reader.int32() as any
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EncodedObject, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<EncodedObject | EncodedObject[]>
      | Iterable<EncodedObject | EncodedObject[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EncodedObject.encode(p).finish()]
        }
      } else {
        yield* [EncodedObject.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EncodedObject>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<EncodedObject> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EncodedObject.decode(p)]
        }
      } else {
        yield* [EncodedObject.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): EncodedObject {
    return {
      dataBlob: isSet(object.dataBlob)
        ? Blob.fromJSON(object.dataBlob)
        : undefined,
      dataHash: isSet(object.dataHash)
        ? Hash.fromJSON(object.dataHash)
        : undefined,
      encodedObjectType: isSet(object.encodedObjectType)
        ? encodedObjectTypeFromJSON(object.encodedObjectType)
        : 0,
    }
  },

  toJSON(message: EncodedObject): unknown {
    const obj: any = {}
    message.dataBlob !== undefined &&
      (obj.dataBlob = message.dataBlob
        ? Blob.toJSON(message.dataBlob)
        : undefined)
    message.dataHash !== undefined &&
      (obj.dataHash = message.dataHash
        ? Hash.toJSON(message.dataHash)
        : undefined)
    message.encodedObjectType !== undefined &&
      (obj.encodedObjectType = encodedObjectTypeToJSON(
        message.encodedObjectType
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<EncodedObject>, I>>(
    base?: I
  ): EncodedObject {
    return EncodedObject.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<EncodedObject>, I>>(
    object: I
  ): EncodedObject {
    const message = createBaseEncodedObject()
    message.dataBlob =
      object.dataBlob !== undefined && object.dataBlob !== null
        ? Blob.fromPartial(object.dataBlob)
        : undefined
    message.dataHash =
      object.dataHash !== undefined && object.dataHash !== null
        ? Hash.fromPartial(object.dataHash)
        : undefined
    message.encodedObjectType = object.encodedObjectType ?? 0
    return message
  },
}

function createBaseIndex(): Index {
  return {
    version: 0,
    entries: [],
    cache: undefined,
    resolveUndo: undefined,
    endOfIndexEntry: undefined,
  }
}

export const Index = {
  encode(message: Index, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.version !== 0) {
      writer.uint32(8).uint32(message.version)
    }
    for (const v of message.entries) {
      IndexEntry.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    if (message.cache !== undefined) {
      Tree.encode(message.cache, writer.uint32(26).fork()).ldelim()
    }
    if (message.resolveUndo !== undefined) {
      ResolveUndo.encode(message.resolveUndo, writer.uint32(34).fork()).ldelim()
    }
    if (message.endOfIndexEntry !== undefined) {
      EndOfIndexEntry.encode(
        message.endOfIndexEntry,
        writer.uint32(42).fork()
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Index {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseIndex()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.version = reader.uint32()
          break
        case 2:
          message.entries.push(IndexEntry.decode(reader, reader.uint32()))
          break
        case 3:
          message.cache = Tree.decode(reader, reader.uint32())
          break
        case 4:
          message.resolveUndo = ResolveUndo.decode(reader, reader.uint32())
          break
        case 5:
          message.endOfIndexEntry = EndOfIndexEntry.decode(
            reader,
            reader.uint32()
          )
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Index, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Index | Index[]> | Iterable<Index | Index[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Index.encode(p).finish()]
        }
      } else {
        yield* [Index.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Index>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Index> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Index.decode(p)]
        }
      } else {
        yield* [Index.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Index {
    return {
      version: isSet(object.version) ? Number(object.version) : 0,
      entries: Array.isArray(object?.entries)
        ? object.entries.map((e: any) => IndexEntry.fromJSON(e))
        : [],
      cache: isSet(object.cache) ? Tree.fromJSON(object.cache) : undefined,
      resolveUndo: isSet(object.resolveUndo)
        ? ResolveUndo.fromJSON(object.resolveUndo)
        : undefined,
      endOfIndexEntry: isSet(object.endOfIndexEntry)
        ? EndOfIndexEntry.fromJSON(object.endOfIndexEntry)
        : undefined,
    }
  },

  toJSON(message: Index): unknown {
    const obj: any = {}
    message.version !== undefined && (obj.version = Math.round(message.version))
    if (message.entries) {
      obj.entries = message.entries.map((e) =>
        e ? IndexEntry.toJSON(e) : undefined
      )
    } else {
      obj.entries = []
    }
    message.cache !== undefined &&
      (obj.cache = message.cache ? Tree.toJSON(message.cache) : undefined)
    message.resolveUndo !== undefined &&
      (obj.resolveUndo = message.resolveUndo
        ? ResolveUndo.toJSON(message.resolveUndo)
        : undefined)
    message.endOfIndexEntry !== undefined &&
      (obj.endOfIndexEntry = message.endOfIndexEntry
        ? EndOfIndexEntry.toJSON(message.endOfIndexEntry)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<Index>, I>>(base?: I): Index {
    return Index.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Index>, I>>(object: I): Index {
    const message = createBaseIndex()
    message.version = object.version ?? 0
    message.entries =
      object.entries?.map((e) => IndexEntry.fromPartial(e)) || []
    message.cache =
      object.cache !== undefined && object.cache !== null
        ? Tree.fromPartial(object.cache)
        : undefined
    message.resolveUndo =
      object.resolveUndo !== undefined && object.resolveUndo !== null
        ? ResolveUndo.fromPartial(object.resolveUndo)
        : undefined
    message.endOfIndexEntry =
      object.endOfIndexEntry !== undefined && object.endOfIndexEntry !== null
        ? EndOfIndexEntry.fromPartial(object.endOfIndexEntry)
        : undefined
    return message
  },
}

function createBaseTree(): Tree {
  return { entries: [] }
}

export const Tree = {
  encode(message: Tree, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.entries) {
      TreeEntry.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Tree {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTree()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.entries.push(TreeEntry.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Tree, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Tree | Tree[]> | Iterable<Tree | Tree[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Tree.encode(p).finish()]
        }
      } else {
        yield* [Tree.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Tree>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Tree> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Tree.decode(p)]
        }
      } else {
        yield* [Tree.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Tree {
    return {
      entries: Array.isArray(object?.entries)
        ? object.entries.map((e: any) => TreeEntry.fromJSON(e))
        : [],
    }
  },

  toJSON(message: Tree): unknown {
    const obj: any = {}
    if (message.entries) {
      obj.entries = message.entries.map((e) =>
        e ? TreeEntry.toJSON(e) : undefined
      )
    } else {
      obj.entries = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Tree>, I>>(base?: I): Tree {
    return Tree.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Tree>, I>>(object: I): Tree {
    const message = createBaseTree()
    message.entries = object.entries?.map((e) => TreeEntry.fromPartial(e)) || []
    return message
  },
}

function createBaseTreeEntry(): TreeEntry {
  return { path: '', entries: 0, trees: 0, hash: undefined }
}

export const TreeEntry = {
  encode(
    message: TreeEntry,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.path !== '') {
      writer.uint32(10).string(message.path)
    }
    if (message.entries !== 0) {
      writer.uint32(16).int32(message.entries)
    }
    if (message.trees !== 0) {
      writer.uint32(24).int32(message.trees)
    }
    if (message.hash !== undefined) {
      Hash.encode(message.hash, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TreeEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTreeEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.path = reader.string()
          break
        case 2:
          message.entries = reader.int32()
          break
        case 3:
          message.trees = reader.int32()
          break
        case 4:
          message.hash = Hash.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TreeEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TreeEntry | TreeEntry[]>
      | Iterable<TreeEntry | TreeEntry[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TreeEntry.encode(p).finish()]
        }
      } else {
        yield* [TreeEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TreeEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TreeEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TreeEntry.decode(p)]
        }
      } else {
        yield* [TreeEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TreeEntry {
    return {
      path: isSet(object.path) ? String(object.path) : '',
      entries: isSet(object.entries) ? Number(object.entries) : 0,
      trees: isSet(object.trees) ? Number(object.trees) : 0,
      hash: isSet(object.hash) ? Hash.fromJSON(object.hash) : undefined,
    }
  },

  toJSON(message: TreeEntry): unknown {
    const obj: any = {}
    message.path !== undefined && (obj.path = message.path)
    message.entries !== undefined && (obj.entries = Math.round(message.entries))
    message.trees !== undefined && (obj.trees = Math.round(message.trees))
    message.hash !== undefined &&
      (obj.hash = message.hash ? Hash.toJSON(message.hash) : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<TreeEntry>, I>>(base?: I): TreeEntry {
    return TreeEntry.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TreeEntry>, I>>(
    object: I
  ): TreeEntry {
    const message = createBaseTreeEntry()
    message.path = object.path ?? ''
    message.entries = object.entries ?? 0
    message.trees = object.trees ?? 0
    message.hash =
      object.hash !== undefined && object.hash !== null
        ? Hash.fromPartial(object.hash)
        : undefined
    return message
  },
}

function createBaseResolveUndo(): ResolveUndo {
  return { entries: [] }
}

export const ResolveUndo = {
  encode(
    message: ResolveUndo,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.entries) {
      ResolveUndoEntry.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResolveUndo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseResolveUndo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.entries.push(ResolveUndoEntry.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ResolveUndo, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ResolveUndo | ResolveUndo[]>
      | Iterable<ResolveUndo | ResolveUndo[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResolveUndo.encode(p).finish()]
        }
      } else {
        yield* [ResolveUndo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ResolveUndo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ResolveUndo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResolveUndo.decode(p)]
        }
      } else {
        yield* [ResolveUndo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ResolveUndo {
    return {
      entries: Array.isArray(object?.entries)
        ? object.entries.map((e: any) => ResolveUndoEntry.fromJSON(e))
        : [],
    }
  },

  toJSON(message: ResolveUndo): unknown {
    const obj: any = {}
    if (message.entries) {
      obj.entries = message.entries.map((e) =>
        e ? ResolveUndoEntry.toJSON(e) : undefined
      )
    } else {
      obj.entries = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ResolveUndo>, I>>(base?: I): ResolveUndo {
    return ResolveUndo.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ResolveUndo>, I>>(
    object: I
  ): ResolveUndo {
    const message = createBaseResolveUndo()
    message.entries =
      object.entries?.map((e) => ResolveUndoEntry.fromPartial(e)) || []
    return message
  },
}

function createBaseResolveUndoEntry(): ResolveUndoEntry {
  return { path: '', stages: {} }
}

export const ResolveUndoEntry = {
  encode(
    message: ResolveUndoEntry,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.path !== '') {
      writer.uint32(10).string(message.path)
    }
    Object.entries(message.stages).forEach(([key, value]) => {
      ResolveUndoEntry_StagesEntry.encode(
        { key: key as any, value },
        writer.uint32(18).fork()
      ).ldelim()
    })
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResolveUndoEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseResolveUndoEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.path = reader.string()
          break
        case 2:
          const entry2 = ResolveUndoEntry_StagesEntry.decode(
            reader,
            reader.uint32()
          )
          if (entry2.value !== undefined) {
            message.stages[entry2.key] = entry2.value
          }
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ResolveUndoEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ResolveUndoEntry | ResolveUndoEntry[]>
      | Iterable<ResolveUndoEntry | ResolveUndoEntry[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResolveUndoEntry.encode(p).finish()]
        }
      } else {
        yield* [ResolveUndoEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ResolveUndoEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ResolveUndoEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResolveUndoEntry.decode(p)]
        }
      } else {
        yield* [ResolveUndoEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ResolveUndoEntry {
    return {
      path: isSet(object.path) ? String(object.path) : '',
      stages: isObject(object.stages)
        ? Object.entries(object.stages).reduce<{ [key: number]: Hash }>(
            (acc, [key, value]) => {
              acc[Number(key)] = Hash.fromJSON(value)
              return acc
            },
            {}
          )
        : {},
    }
  },

  toJSON(message: ResolveUndoEntry): unknown {
    const obj: any = {}
    message.path !== undefined && (obj.path = message.path)
    obj.stages = {}
    if (message.stages) {
      Object.entries(message.stages).forEach(([k, v]) => {
        obj.stages[k] = Hash.toJSON(v)
      })
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ResolveUndoEntry>, I>>(
    base?: I
  ): ResolveUndoEntry {
    return ResolveUndoEntry.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ResolveUndoEntry>, I>>(
    object: I
  ): ResolveUndoEntry {
    const message = createBaseResolveUndoEntry()
    message.path = object.path ?? ''
    message.stages = Object.entries(object.stages ?? {}).reduce<{
      [key: number]: Hash
    }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[Number(key)] = Hash.fromPartial(value)
      }
      return acc
    }, {})
    return message
  },
}

function createBaseResolveUndoEntry_StagesEntry(): ResolveUndoEntry_StagesEntry {
  return { key: 0, value: undefined }
}

export const ResolveUndoEntry_StagesEntry = {
  encode(
    message: ResolveUndoEntry_StagesEntry,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.key !== 0) {
      writer.uint32(8).uint32(message.key)
    }
    if (message.value !== undefined) {
      Hash.encode(message.value, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): ResolveUndoEntry_StagesEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseResolveUndoEntry_StagesEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.key = reader.uint32()
          break
        case 2:
          message.value = Hash.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ResolveUndoEntry_StagesEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          ResolveUndoEntry_StagesEntry | ResolveUndoEntry_StagesEntry[]
        >
      | Iterable<ResolveUndoEntry_StagesEntry | ResolveUndoEntry_StagesEntry[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResolveUndoEntry_StagesEntry.encode(p).finish()]
        }
      } else {
        yield* [ResolveUndoEntry_StagesEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ResolveUndoEntry_StagesEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ResolveUndoEntry_StagesEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResolveUndoEntry_StagesEntry.decode(p)]
        }
      } else {
        yield* [ResolveUndoEntry_StagesEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ResolveUndoEntry_StagesEntry {
    return {
      key: isSet(object.key) ? Number(object.key) : 0,
      value: isSet(object.value) ? Hash.fromJSON(object.value) : undefined,
    }
  },

  toJSON(message: ResolveUndoEntry_StagesEntry): unknown {
    const obj: any = {}
    message.key !== undefined && (obj.key = Math.round(message.key))
    message.value !== undefined &&
      (obj.value = message.value ? Hash.toJSON(message.value) : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<ResolveUndoEntry_StagesEntry>, I>>(
    base?: I
  ): ResolveUndoEntry_StagesEntry {
    return ResolveUndoEntry_StagesEntry.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ResolveUndoEntry_StagesEntry>, I>>(
    object: I
  ): ResolveUndoEntry_StagesEntry {
    const message = createBaseResolveUndoEntry_StagesEntry()
    message.key = object.key ?? 0
    message.value =
      object.value !== undefined && object.value !== null
        ? Hash.fromPartial(object.value)
        : undefined
    return message
  },
}

function createBaseEndOfIndexEntry(): EndOfIndexEntry {
  return { offset: 0, hash: undefined }
}

export const EndOfIndexEntry = {
  encode(
    message: EndOfIndexEntry,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.offset !== 0) {
      writer.uint32(8).uint32(message.offset)
    }
    if (message.hash !== undefined) {
      Hash.encode(message.hash, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EndOfIndexEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEndOfIndexEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.offset = reader.uint32()
          break
        case 2:
          message.hash = Hash.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EndOfIndexEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<EndOfIndexEntry | EndOfIndexEntry[]>
      | Iterable<EndOfIndexEntry | EndOfIndexEntry[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EndOfIndexEntry.encode(p).finish()]
        }
      } else {
        yield* [EndOfIndexEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EndOfIndexEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<EndOfIndexEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EndOfIndexEntry.decode(p)]
        }
      } else {
        yield* [EndOfIndexEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): EndOfIndexEntry {
    return {
      offset: isSet(object.offset) ? Number(object.offset) : 0,
      hash: isSet(object.hash) ? Hash.fromJSON(object.hash) : undefined,
    }
  },

  toJSON(message: EndOfIndexEntry): unknown {
    const obj: any = {}
    message.offset !== undefined && (obj.offset = Math.round(message.offset))
    message.hash !== undefined &&
      (obj.hash = message.hash ? Hash.toJSON(message.hash) : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<EndOfIndexEntry>, I>>(
    base?: I
  ): EndOfIndexEntry {
    return EndOfIndexEntry.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<EndOfIndexEntry>, I>>(
    object: I
  ): EndOfIndexEntry {
    const message = createBaseEndOfIndexEntry()
    message.offset = object.offset ?? 0
    message.hash =
      object.hash !== undefined && object.hash !== null
        ? Hash.fromPartial(object.hash)
        : undefined
    return message
  },
}

function createBaseIndexEntry(): IndexEntry {
  return {
    dataHash: undefined,
    name: '',
    createdAt: undefined,
    modifiedAt: undefined,
    dev: 0,
    inode: 0,
    fileMode: 0,
    uid: 0,
    gid: 0,
    size: 0,
    stage: 0,
    skipWorktree: false,
    intentToAdd: false,
  }
}

export const IndexEntry = {
  encode(
    message: IndexEntry,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.dataHash !== undefined) {
      Hash.encode(message.dataHash, writer.uint32(10).fork()).ldelim()
    }
    if (message.name !== '') {
      writer.uint32(18).string(message.name)
    }
    if (message.createdAt !== undefined) {
      Timestamp.encode(message.createdAt, writer.uint32(26).fork()).ldelim()
    }
    if (message.modifiedAt !== undefined) {
      Timestamp.encode(message.modifiedAt, writer.uint32(34).fork()).ldelim()
    }
    if (message.dev !== 0) {
      writer.uint32(40).uint32(message.dev)
    }
    if (message.inode !== 0) {
      writer.uint32(48).uint32(message.inode)
    }
    if (message.fileMode !== 0) {
      writer.uint32(56).uint32(message.fileMode)
    }
    if (message.uid !== 0) {
      writer.uint32(64).uint32(message.uid)
    }
    if (message.gid !== 0) {
      writer.uint32(72).uint32(message.gid)
    }
    if (message.size !== 0) {
      writer.uint32(80).uint32(message.size)
    }
    if (message.stage !== 0) {
      writer.uint32(88).uint32(message.stage)
    }
    if (message.skipWorktree === true) {
      writer.uint32(96).bool(message.skipWorktree)
    }
    if (message.intentToAdd === true) {
      writer.uint32(104).bool(message.intentToAdd)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): IndexEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseIndexEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.dataHash = Hash.decode(reader, reader.uint32())
          break
        case 2:
          message.name = reader.string()
          break
        case 3:
          message.createdAt = Timestamp.decode(reader, reader.uint32())
          break
        case 4:
          message.modifiedAt = Timestamp.decode(reader, reader.uint32())
          break
        case 5:
          message.dev = reader.uint32()
          break
        case 6:
          message.inode = reader.uint32()
          break
        case 7:
          message.fileMode = reader.uint32()
          break
        case 8:
          message.uid = reader.uint32()
          break
        case 9:
          message.gid = reader.uint32()
          break
        case 10:
          message.size = reader.uint32()
          break
        case 11:
          message.stage = reader.uint32()
          break
        case 12:
          message.skipWorktree = reader.bool()
          break
        case 13:
          message.intentToAdd = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<IndexEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<IndexEntry | IndexEntry[]>
      | Iterable<IndexEntry | IndexEntry[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [IndexEntry.encode(p).finish()]
        }
      } else {
        yield* [IndexEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, IndexEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<IndexEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [IndexEntry.decode(p)]
        }
      } else {
        yield* [IndexEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): IndexEntry {
    return {
      dataHash: isSet(object.dataHash)
        ? Hash.fromJSON(object.dataHash)
        : undefined,
      name: isSet(object.name) ? String(object.name) : '',
      createdAt: isSet(object.createdAt)
        ? Timestamp.fromJSON(object.createdAt)
        : undefined,
      modifiedAt: isSet(object.modifiedAt)
        ? Timestamp.fromJSON(object.modifiedAt)
        : undefined,
      dev: isSet(object.dev) ? Number(object.dev) : 0,
      inode: isSet(object.inode) ? Number(object.inode) : 0,
      fileMode: isSet(object.fileMode) ? Number(object.fileMode) : 0,
      uid: isSet(object.uid) ? Number(object.uid) : 0,
      gid: isSet(object.gid) ? Number(object.gid) : 0,
      size: isSet(object.size) ? Number(object.size) : 0,
      stage: isSet(object.stage) ? Number(object.stage) : 0,
      skipWorktree: isSet(object.skipWorktree)
        ? Boolean(object.skipWorktree)
        : false,
      intentToAdd: isSet(object.intentToAdd)
        ? Boolean(object.intentToAdd)
        : false,
    }
  },

  toJSON(message: IndexEntry): unknown {
    const obj: any = {}
    message.dataHash !== undefined &&
      (obj.dataHash = message.dataHash
        ? Hash.toJSON(message.dataHash)
        : undefined)
    message.name !== undefined && (obj.name = message.name)
    message.createdAt !== undefined &&
      (obj.createdAt = message.createdAt
        ? Timestamp.toJSON(message.createdAt)
        : undefined)
    message.modifiedAt !== undefined &&
      (obj.modifiedAt = message.modifiedAt
        ? Timestamp.toJSON(message.modifiedAt)
        : undefined)
    message.dev !== undefined && (obj.dev = Math.round(message.dev))
    message.inode !== undefined && (obj.inode = Math.round(message.inode))
    message.fileMode !== undefined &&
      (obj.fileMode = Math.round(message.fileMode))
    message.uid !== undefined && (obj.uid = Math.round(message.uid))
    message.gid !== undefined && (obj.gid = Math.round(message.gid))
    message.size !== undefined && (obj.size = Math.round(message.size))
    message.stage !== undefined && (obj.stage = Math.round(message.stage))
    message.skipWorktree !== undefined &&
      (obj.skipWorktree = message.skipWorktree)
    message.intentToAdd !== undefined && (obj.intentToAdd = message.intentToAdd)
    return obj
  },

  create<I extends Exact<DeepPartial<IndexEntry>, I>>(base?: I): IndexEntry {
    return IndexEntry.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<IndexEntry>, I>>(
    object: I
  ): IndexEntry {
    const message = createBaseIndexEntry()
    message.dataHash =
      object.dataHash !== undefined && object.dataHash !== null
        ? Hash.fromPartial(object.dataHash)
        : undefined
    message.name = object.name ?? ''
    message.createdAt =
      object.createdAt !== undefined && object.createdAt !== null
        ? Timestamp.fromPartial(object.createdAt)
        : undefined
    message.modifiedAt =
      object.modifiedAt !== undefined && object.modifiedAt !== null
        ? Timestamp.fromPartial(object.modifiedAt)
        : undefined
    message.dev = object.dev ?? 0
    message.inode = object.inode ?? 0
    message.fileMode = object.fileMode ?? 0
    message.uid = object.uid ?? 0
    message.gid = object.gid ?? 0
    message.size = object.size ?? 0
    message.stage = object.stage ?? 0
    message.skipWorktree = object.skipWorktree ?? false
    message.intentToAdd = object.intentToAdd ?? false
    return message
  },
}

function createBaseAuthOpts(): AuthOpts {
  return { username: '', peerId: '' }
}

export const AuthOpts = {
  encode(
    message: AuthOpts,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.username !== '') {
      writer.uint32(10).string(message.username)
    }
    if (message.peerId !== '') {
      writer.uint32(18).string(message.peerId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): AuthOpts {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseAuthOpts()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.username = reader.string()
          break
        case 2:
          message.peerId = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<AuthOpts, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<AuthOpts | AuthOpts[]>
      | Iterable<AuthOpts | AuthOpts[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [AuthOpts.encode(p).finish()]
        }
      } else {
        yield* [AuthOpts.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, AuthOpts>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<AuthOpts> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [AuthOpts.decode(p)]
        }
      } else {
        yield* [AuthOpts.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): AuthOpts {
    return {
      username: isSet(object.username) ? String(object.username) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
    }
  },

  toJSON(message: AuthOpts): unknown {
    const obj: any = {}
    message.username !== undefined && (obj.username = message.username)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    return obj
  },

  create<I extends Exact<DeepPartial<AuthOpts>, I>>(base?: I): AuthOpts {
    return AuthOpts.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<AuthOpts>, I>>(object: I): AuthOpts {
    const message = createBaseAuthOpts()
    message.username = object.username ?? ''
    message.peerId = object.peerId ?? ''
    return message
  },
}

function createBaseCloneOpts(): CloneOpts {
  return {
    url: '',
    remoteName: '',
    ref: '',
    singleBranch: false,
    disableCheckout: false,
    depth: 0,
    recursive: false,
    tagMode: 0,
    insecure: false,
    caBundle: '',
  }
}

export const CloneOpts = {
  encode(
    message: CloneOpts,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.url !== '') {
      writer.uint32(10).string(message.url)
    }
    if (message.remoteName !== '') {
      writer.uint32(18).string(message.remoteName)
    }
    if (message.ref !== '') {
      writer.uint32(26).string(message.ref)
    }
    if (message.singleBranch === true) {
      writer.uint32(32).bool(message.singleBranch)
    }
    if (message.disableCheckout === true) {
      writer.uint32(40).bool(message.disableCheckout)
    }
    if (message.depth !== 0) {
      writer.uint32(48).uint32(message.depth)
    }
    if (message.recursive === true) {
      writer.uint32(56).bool(message.recursive)
    }
    if (message.tagMode !== 0) {
      writer.uint32(64).int32(message.tagMode)
    }
    if (message.insecure === true) {
      writer.uint32(72).bool(message.insecure)
    }
    if (message.caBundle !== '') {
      writer.uint32(82).string(message.caBundle)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): CloneOpts {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCloneOpts()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.url = reader.string()
          break
        case 2:
          message.remoteName = reader.string()
          break
        case 3:
          message.ref = reader.string()
          break
        case 4:
          message.singleBranch = reader.bool()
          break
        case 5:
          message.disableCheckout = reader.bool()
          break
        case 6:
          message.depth = reader.uint32()
          break
        case 7:
          message.recursive = reader.bool()
          break
        case 8:
          message.tagMode = reader.int32() as any
          break
        case 9:
          message.insecure = reader.bool()
          break
        case 10:
          message.caBundle = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<CloneOpts, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<CloneOpts | CloneOpts[]>
      | Iterable<CloneOpts | CloneOpts[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [CloneOpts.encode(p).finish()]
        }
      } else {
        yield* [CloneOpts.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, CloneOpts>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<CloneOpts> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [CloneOpts.decode(p)]
        }
      } else {
        yield* [CloneOpts.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): CloneOpts {
    return {
      url: isSet(object.url) ? String(object.url) : '',
      remoteName: isSet(object.remoteName) ? String(object.remoteName) : '',
      ref: isSet(object.ref) ? String(object.ref) : '',
      singleBranch: isSet(object.singleBranch)
        ? Boolean(object.singleBranch)
        : false,
      disableCheckout: isSet(object.disableCheckout)
        ? Boolean(object.disableCheckout)
        : false,
      depth: isSet(object.depth) ? Number(object.depth) : 0,
      recursive: isSet(object.recursive) ? Boolean(object.recursive) : false,
      tagMode: isSet(object.tagMode) ? tagModeFromJSON(object.tagMode) : 0,
      insecure: isSet(object.insecure) ? Boolean(object.insecure) : false,
      caBundle: isSet(object.caBundle) ? String(object.caBundle) : '',
    }
  },

  toJSON(message: CloneOpts): unknown {
    const obj: any = {}
    message.url !== undefined && (obj.url = message.url)
    message.remoteName !== undefined && (obj.remoteName = message.remoteName)
    message.ref !== undefined && (obj.ref = message.ref)
    message.singleBranch !== undefined &&
      (obj.singleBranch = message.singleBranch)
    message.disableCheckout !== undefined &&
      (obj.disableCheckout = message.disableCheckout)
    message.depth !== undefined && (obj.depth = Math.round(message.depth))
    message.recursive !== undefined && (obj.recursive = message.recursive)
    message.tagMode !== undefined &&
      (obj.tagMode = tagModeToJSON(message.tagMode))
    message.insecure !== undefined && (obj.insecure = message.insecure)
    message.caBundle !== undefined && (obj.caBundle = message.caBundle)
    return obj
  },

  create<I extends Exact<DeepPartial<CloneOpts>, I>>(base?: I): CloneOpts {
    return CloneOpts.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<CloneOpts>, I>>(
    object: I
  ): CloneOpts {
    const message = createBaseCloneOpts()
    message.url = object.url ?? ''
    message.remoteName = object.remoteName ?? ''
    message.ref = object.ref ?? ''
    message.singleBranch = object.singleBranch ?? false
    message.disableCheckout = object.disableCheckout ?? false
    message.depth = object.depth ?? 0
    message.recursive = object.recursive ?? false
    message.tagMode = object.tagMode ?? 0
    message.insecure = object.insecure ?? false
    message.caBundle = object.caBundle ?? ''
    return message
  },
}

function createBaseCheckoutOpts(): CheckoutOpts {
  return {
    commit: undefined,
    branch: '',
    create: false,
    force: false,
    keep: false,
  }
}

export const CheckoutOpts = {
  encode(
    message: CheckoutOpts,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.commit !== undefined) {
      Hash.encode(message.commit, writer.uint32(10).fork()).ldelim()
    }
    if (message.branch !== '') {
      writer.uint32(18).string(message.branch)
    }
    if (message.create === true) {
      writer.uint32(24).bool(message.create)
    }
    if (message.force === true) {
      writer.uint32(32).bool(message.force)
    }
    if (message.keep === true) {
      writer.uint32(40).bool(message.keep)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): CheckoutOpts {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCheckoutOpts()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.commit = Hash.decode(reader, reader.uint32())
          break
        case 2:
          message.branch = reader.string()
          break
        case 3:
          message.create = reader.bool()
          break
        case 4:
          message.force = reader.bool()
          break
        case 5:
          message.keep = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<CheckoutOpts, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<CheckoutOpts | CheckoutOpts[]>
      | Iterable<CheckoutOpts | CheckoutOpts[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [CheckoutOpts.encode(p).finish()]
        }
      } else {
        yield* [CheckoutOpts.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, CheckoutOpts>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<CheckoutOpts> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [CheckoutOpts.decode(p)]
        }
      } else {
        yield* [CheckoutOpts.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): CheckoutOpts {
    return {
      commit: isSet(object.commit) ? Hash.fromJSON(object.commit) : undefined,
      branch: isSet(object.branch) ? String(object.branch) : '',
      create: isSet(object.create) ? Boolean(object.create) : false,
      force: isSet(object.force) ? Boolean(object.force) : false,
      keep: isSet(object.keep) ? Boolean(object.keep) : false,
    }
  },

  toJSON(message: CheckoutOpts): unknown {
    const obj: any = {}
    message.commit !== undefined &&
      (obj.commit = message.commit ? Hash.toJSON(message.commit) : undefined)
    message.branch !== undefined && (obj.branch = message.branch)
    message.create !== undefined && (obj.create = message.create)
    message.force !== undefined && (obj.force = message.force)
    message.keep !== undefined && (obj.keep = message.keep)
    return obj
  },

  create<I extends Exact<DeepPartial<CheckoutOpts>, I>>(
    base?: I
  ): CheckoutOpts {
    return CheckoutOpts.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<CheckoutOpts>, I>>(
    object: I
  ): CheckoutOpts {
    const message = createBaseCheckoutOpts()
    message.commit =
      object.commit !== undefined && object.commit !== null
        ? Hash.fromPartial(object.commit)
        : undefined
    message.branch = object.branch ?? ''
    message.create = object.create ?? false
    message.force = object.force ?? false
    message.keep = object.keep ?? false
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
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
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

function isObject(value: any): boolean {
  return typeof value === 'object' && value !== null
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
