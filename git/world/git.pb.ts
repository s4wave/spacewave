/* eslint-disable */
import { Timestamp } from "@go/github.com/aperturerobotics/timestamp/timestamp.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { ObjectRef } from "../../bucket/bucket.pb.js";
import { UnixfsRef } from "../../unixfs/world/unixfs.pb.js";
import { CheckoutOpts, Index, Reference } from "../block/git.pb.js";

export const protobufPackage = "git.world";

/**
 * GitInitOp is an operation to create a repo with a root ref or empty.
 * If applied as an object op, skips checkout step.
 */
export interface GitInitOp {
  /** ObjectKey is the object key to create as a Repo. */
  objectKey: string;
  /**
   * RepoRef contains the object ref to the Repo.
   * If empty, will create a new blank Repo.
   */
  repoRef:
    | ObjectRef
    | undefined;
  /** DisableCheckout disables creating a worktree. */
  disableCheckout: boolean;
  /**
   * CreateWorktree configures creating the worktree.
   * If unset, uses object_key + "/worktree"
   * If disable_checkout is set, worktree is not created.
   * Applying as an object op implies disable_checkout.
   */
  createWorktree: GitCreateWorktreeOp | undefined;
}

/**
 * Worktree refers to a location where a repo is checked out.
 * Contains an index and an attached working directory.
 */
export interface Worktree {
  /** GitIndex is the git index for the worktree. */
  gitIndex:
    | Index
    | undefined;
  /** HeadRefStore contains the HEAD reference for a worktree and submodules. */
  headRefStore: HeadRefStore | undefined;
}

/** HeadRefStore contains the HEAD reference for a worktree and submodules. */
export interface HeadRefStore {
  /** SubmoduleName is the name of the submodule if this is a submodule. */
  submoduleName: string;
  /**
   * HeadRef is the reference to the HEAD checked out in the worktree.
   * If unset, uses the store HEAD ref.
   */
  headRef:
    | Reference
    | undefined;
  /**
   * Submodules contains the references for the submodules.
   * sorted by name
   */
  submodules: HeadRefStore[];
}

/**
 * GitCreateWorktreeOp creates a Git worktree attached to a Repo.
 * Note: cannot be run as a Object-specific op.
 */
export interface GitCreateWorktreeOp {
  /** ObjectKey is the object key to create as a Worktree. */
  objectKey: string;
  /** RepoObjectKey is the key of the repository object. */
  repoObjectKey: string;
  /** WorkdirRef is a unixfs reference to the workdir. */
  workdirRef:
    | UnixfsRef
    | undefined;
  /** CreateWorkdir indicates to create the workdir if it doesn't exist. */
  createWorkdir: boolean;
  /** CheckoutOpts are options to use when checking out the data. */
  checkoutOpts:
    | CheckoutOpts
    | undefined;
  /** DisableCheckout disables checking out the data to the workdir. */
  disableCheckout: boolean;
  /** Timestamp is the modification time for the workdir ops. */
  timestamp: Timestamp | undefined;
}

/**
 * GitWorktreeCheckoutOp checks out a git revision in a worktree.
 * Note: cannot be run as a Object-specific op.
 */
export interface GitWorktreeCheckoutOp {
  /** ObjectKey is the object key of the Worktree. */
  objectKey: string;
  /** RepoObjectKey is the key of the repository object. */
  repoObjectKey: string;
  /** CheckoutOpts are options to use when checking out the data. */
  checkoutOpts:
    | CheckoutOpts
    | undefined;
  /** Timestamp is the modification time for the workdir ops. */
  timestamp: Timestamp | undefined;
}

function createBaseGitInitOp(): GitInitOp {
  return { objectKey: "", repoRef: undefined, disableCheckout: false, createWorktree: undefined };
}

export const GitInitOp = {
  encode(message: GitInitOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.repoRef !== undefined) {
      ObjectRef.encode(message.repoRef, writer.uint32(18).fork()).ldelim();
    }
    if (message.disableCheckout === true) {
      writer.uint32(24).bool(message.disableCheckout);
    }
    if (message.createWorktree !== undefined) {
      GitCreateWorktreeOp.encode(message.createWorktree, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GitInitOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGitInitOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.repoRef = ObjectRef.decode(reader, reader.uint32());
          break;
        case 3:
          message.disableCheckout = reader.bool();
          break;
        case 4:
          message.createWorktree = GitCreateWorktreeOp.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GitInitOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<GitInitOp | GitInitOp[]> | Iterable<GitInitOp | GitInitOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GitInitOp.encode(p).finish()];
        }
      } else {
        yield* [GitInitOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GitInitOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GitInitOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GitInitOp.decode(p)];
        }
      } else {
        yield* [GitInitOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): GitInitOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      repoRef: isSet(object.repoRef) ? ObjectRef.fromJSON(object.repoRef) : undefined,
      disableCheckout: isSet(object.disableCheckout) ? Boolean(object.disableCheckout) : false,
      createWorktree: isSet(object.createWorktree) ? GitCreateWorktreeOp.fromJSON(object.createWorktree) : undefined,
    };
  },

  toJSON(message: GitInitOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.repoRef !== undefined && (obj.repoRef = message.repoRef ? ObjectRef.toJSON(message.repoRef) : undefined);
    message.disableCheckout !== undefined && (obj.disableCheckout = message.disableCheckout);
    message.createWorktree !== undefined &&
      (obj.createWorktree = message.createWorktree ? GitCreateWorktreeOp.toJSON(message.createWorktree) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<GitInitOp>, I>>(object: I): GitInitOp {
    const message = createBaseGitInitOp();
    message.objectKey = object.objectKey ?? "";
    message.repoRef = (object.repoRef !== undefined && object.repoRef !== null)
      ? ObjectRef.fromPartial(object.repoRef)
      : undefined;
    message.disableCheckout = object.disableCheckout ?? false;
    message.createWorktree = (object.createWorktree !== undefined && object.createWorktree !== null)
      ? GitCreateWorktreeOp.fromPartial(object.createWorktree)
      : undefined;
    return message;
  },
};

function createBaseWorktree(): Worktree {
  return { gitIndex: undefined, headRefStore: undefined };
}

export const Worktree = {
  encode(message: Worktree, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.gitIndex !== undefined) {
      Index.encode(message.gitIndex, writer.uint32(10).fork()).ldelim();
    }
    if (message.headRefStore !== undefined) {
      HeadRefStore.encode(message.headRefStore, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Worktree {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseWorktree();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.gitIndex = Index.decode(reader, reader.uint32());
          break;
        case 2:
          message.headRefStore = HeadRefStore.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Worktree, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Worktree | Worktree[]> | Iterable<Worktree | Worktree[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Worktree.encode(p).finish()];
        }
      } else {
        yield* [Worktree.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Worktree>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Worktree> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Worktree.decode(p)];
        }
      } else {
        yield* [Worktree.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Worktree {
    return {
      gitIndex: isSet(object.gitIndex) ? Index.fromJSON(object.gitIndex) : undefined,
      headRefStore: isSet(object.headRefStore) ? HeadRefStore.fromJSON(object.headRefStore) : undefined,
    };
  },

  toJSON(message: Worktree): unknown {
    const obj: any = {};
    message.gitIndex !== undefined && (obj.gitIndex = message.gitIndex ? Index.toJSON(message.gitIndex) : undefined);
    message.headRefStore !== undefined &&
      (obj.headRefStore = message.headRefStore ? HeadRefStore.toJSON(message.headRefStore) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Worktree>, I>>(object: I): Worktree {
    const message = createBaseWorktree();
    message.gitIndex = (object.gitIndex !== undefined && object.gitIndex !== null)
      ? Index.fromPartial(object.gitIndex)
      : undefined;
    message.headRefStore = (object.headRefStore !== undefined && object.headRefStore !== null)
      ? HeadRefStore.fromPartial(object.headRefStore)
      : undefined;
    return message;
  },
};

function createBaseHeadRefStore(): HeadRefStore {
  return { submoduleName: "", headRef: undefined, submodules: [] };
}

export const HeadRefStore = {
  encode(message: HeadRefStore, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.submoduleName !== "") {
      writer.uint32(10).string(message.submoduleName);
    }
    if (message.headRef !== undefined) {
      Reference.encode(message.headRef, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.submodules) {
      HeadRefStore.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HeadRefStore {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseHeadRefStore();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.submoduleName = reader.string();
          break;
        case 2:
          message.headRef = Reference.decode(reader, reader.uint32());
          break;
        case 3:
          message.submodules.push(HeadRefStore.decode(reader, reader.uint32()));
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<HeadRefStore, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<HeadRefStore | HeadRefStore[]> | Iterable<HeadRefStore | HeadRefStore[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HeadRefStore.encode(p).finish()];
        }
      } else {
        yield* [HeadRefStore.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HeadRefStore>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<HeadRefStore> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HeadRefStore.decode(p)];
        }
      } else {
        yield* [HeadRefStore.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): HeadRefStore {
    return {
      submoduleName: isSet(object.submoduleName) ? String(object.submoduleName) : "",
      headRef: isSet(object.headRef) ? Reference.fromJSON(object.headRef) : undefined,
      submodules: Array.isArray(object?.submodules) ? object.submodules.map((e: any) => HeadRefStore.fromJSON(e)) : [],
    };
  },

  toJSON(message: HeadRefStore): unknown {
    const obj: any = {};
    message.submoduleName !== undefined && (obj.submoduleName = message.submoduleName);
    message.headRef !== undefined && (obj.headRef = message.headRef ? Reference.toJSON(message.headRef) : undefined);
    if (message.submodules) {
      obj.submodules = message.submodules.map((e) => e ? HeadRefStore.toJSON(e) : undefined);
    } else {
      obj.submodules = [];
    }
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<HeadRefStore>, I>>(object: I): HeadRefStore {
    const message = createBaseHeadRefStore();
    message.submoduleName = object.submoduleName ?? "";
    message.headRef = (object.headRef !== undefined && object.headRef !== null)
      ? Reference.fromPartial(object.headRef)
      : undefined;
    message.submodules = object.submodules?.map((e) => HeadRefStore.fromPartial(e)) || [];
    return message;
  },
};

function createBaseGitCreateWorktreeOp(): GitCreateWorktreeOp {
  return {
    objectKey: "",
    repoObjectKey: "",
    workdirRef: undefined,
    createWorkdir: false,
    checkoutOpts: undefined,
    disableCheckout: false,
    timestamp: undefined,
  };
}

export const GitCreateWorktreeOp = {
  encode(message: GitCreateWorktreeOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.repoObjectKey !== "") {
      writer.uint32(18).string(message.repoObjectKey);
    }
    if (message.workdirRef !== undefined) {
      UnixfsRef.encode(message.workdirRef, writer.uint32(26).fork()).ldelim();
    }
    if (message.createWorkdir === true) {
      writer.uint32(32).bool(message.createWorkdir);
    }
    if (message.checkoutOpts !== undefined) {
      CheckoutOpts.encode(message.checkoutOpts, writer.uint32(42).fork()).ldelim();
    }
    if (message.disableCheckout === true) {
      writer.uint32(48).bool(message.disableCheckout);
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(58).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GitCreateWorktreeOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGitCreateWorktreeOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.repoObjectKey = reader.string();
          break;
        case 3:
          message.workdirRef = UnixfsRef.decode(reader, reader.uint32());
          break;
        case 4:
          message.createWorkdir = reader.bool();
          break;
        case 5:
          message.checkoutOpts = CheckoutOpts.decode(reader, reader.uint32());
          break;
        case 6:
          message.disableCheckout = reader.bool();
          break;
        case 7:
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
  // Transform<GitCreateWorktreeOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GitCreateWorktreeOp | GitCreateWorktreeOp[]>
      | Iterable<GitCreateWorktreeOp | GitCreateWorktreeOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GitCreateWorktreeOp.encode(p).finish()];
        }
      } else {
        yield* [GitCreateWorktreeOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GitCreateWorktreeOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GitCreateWorktreeOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GitCreateWorktreeOp.decode(p)];
        }
      } else {
        yield* [GitCreateWorktreeOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): GitCreateWorktreeOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      repoObjectKey: isSet(object.repoObjectKey) ? String(object.repoObjectKey) : "",
      workdirRef: isSet(object.workdirRef) ? UnixfsRef.fromJSON(object.workdirRef) : undefined,
      createWorkdir: isSet(object.createWorkdir) ? Boolean(object.createWorkdir) : false,
      checkoutOpts: isSet(object.checkoutOpts) ? CheckoutOpts.fromJSON(object.checkoutOpts) : undefined,
      disableCheckout: isSet(object.disableCheckout) ? Boolean(object.disableCheckout) : false,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: GitCreateWorktreeOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.repoObjectKey !== undefined && (obj.repoObjectKey = message.repoObjectKey);
    message.workdirRef !== undefined &&
      (obj.workdirRef = message.workdirRef ? UnixfsRef.toJSON(message.workdirRef) : undefined);
    message.createWorkdir !== undefined && (obj.createWorkdir = message.createWorkdir);
    message.checkoutOpts !== undefined &&
      (obj.checkoutOpts = message.checkoutOpts ? CheckoutOpts.toJSON(message.checkoutOpts) : undefined);
    message.disableCheckout !== undefined && (obj.disableCheckout = message.disableCheckout);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<GitCreateWorktreeOp>, I>>(object: I): GitCreateWorktreeOp {
    const message = createBaseGitCreateWorktreeOp();
    message.objectKey = object.objectKey ?? "";
    message.repoObjectKey = object.repoObjectKey ?? "";
    message.workdirRef = (object.workdirRef !== undefined && object.workdirRef !== null)
      ? UnixfsRef.fromPartial(object.workdirRef)
      : undefined;
    message.createWorkdir = object.createWorkdir ?? false;
    message.checkoutOpts = (object.checkoutOpts !== undefined && object.checkoutOpts !== null)
      ? CheckoutOpts.fromPartial(object.checkoutOpts)
      : undefined;
    message.disableCheckout = object.disableCheckout ?? false;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    return message;
  },
};

function createBaseGitWorktreeCheckoutOp(): GitWorktreeCheckoutOp {
  return { objectKey: "", repoObjectKey: "", checkoutOpts: undefined, timestamp: undefined };
}

export const GitWorktreeCheckoutOp = {
  encode(message: GitWorktreeCheckoutOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.repoObjectKey !== "") {
      writer.uint32(18).string(message.repoObjectKey);
    }
    if (message.checkoutOpts !== undefined) {
      CheckoutOpts.encode(message.checkoutOpts, writer.uint32(26).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GitWorktreeCheckoutOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGitWorktreeCheckoutOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.repoObjectKey = reader.string();
          break;
        case 3:
          message.checkoutOpts = CheckoutOpts.decode(reader, reader.uint32());
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
  // Transform<GitWorktreeCheckoutOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GitWorktreeCheckoutOp | GitWorktreeCheckoutOp[]>
      | Iterable<GitWorktreeCheckoutOp | GitWorktreeCheckoutOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GitWorktreeCheckoutOp.encode(p).finish()];
        }
      } else {
        yield* [GitWorktreeCheckoutOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GitWorktreeCheckoutOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GitWorktreeCheckoutOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [GitWorktreeCheckoutOp.decode(p)];
        }
      } else {
        yield* [GitWorktreeCheckoutOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): GitWorktreeCheckoutOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      repoObjectKey: isSet(object.repoObjectKey) ? String(object.repoObjectKey) : "",
      checkoutOpts: isSet(object.checkoutOpts) ? CheckoutOpts.fromJSON(object.checkoutOpts) : undefined,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
    };
  },

  toJSON(message: GitWorktreeCheckoutOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.repoObjectKey !== undefined && (obj.repoObjectKey = message.repoObjectKey);
    message.checkoutOpts !== undefined &&
      (obj.checkoutOpts = message.checkoutOpts ? CheckoutOpts.toJSON(message.checkoutOpts) : undefined);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<GitWorktreeCheckoutOp>, I>>(object: I): GitWorktreeCheckoutOp {
    const message = createBaseGitWorktreeCheckoutOp();
    message.objectKey = object.objectKey ?? "";
    message.repoObjectKey = object.repoObjectKey ?? "";
    message.checkoutOpts = (object.checkoutOpts !== undefined && object.checkoutOpts !== null)
      ? CheckoutOpts.fromPartial(object.checkoutOpts)
      : undefined;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
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
