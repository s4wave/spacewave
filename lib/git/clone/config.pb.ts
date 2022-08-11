/* eslint-disable */
import Long from 'long'
import { CloneOpts, AuthOpts } from '../../../../hydra/git/block/git.pb.js'
import { GitCreateWorktreeOp } from '../../../../hydra/git/world/git.pb.js'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'forge.lib.git.clone'

/**
 * Config is the configuration for cloning Git repositories to a world.
 *
 * Inputs:
 *  - world: the target World engine or state.
 * Outputs:
 *  - repo: snapshot of the Repository object.
 */
export interface Config {
  /** ObjectKey is the object key to create as a Repo. */
  objectKey: string
  /** CloneOpts are the args for the clone operation. */
  cloneOpts: CloneOpts | undefined
  /** AuthOpts applies authentication to the operations. */
  authOpts: AuthOpts | undefined
  /**
   * WorktreeOpts are options for creating the worktree.
   * If cloneOpts.DisableCheckout is set, disables this step.
   */
  worktreeOpts: GitCreateWorktreeOp | undefined
}

function createBaseConfig(): Config {
  return {
    objectKey: '',
    cloneOpts: undefined,
    authOpts: undefined,
    worktreeOpts: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.objectKey !== '') {
      writer.uint32(10).string(message.objectKey)
    }
    if (message.cloneOpts !== undefined) {
      CloneOpts.encode(message.cloneOpts, writer.uint32(18).fork()).ldelim()
    }
    if (message.authOpts !== undefined) {
      AuthOpts.encode(message.authOpts, writer.uint32(26).fork()).ldelim()
    }
    if (message.worktreeOpts !== undefined) {
      GitCreateWorktreeOp.encode(
        message.worktreeOpts,
        writer.uint32(34).fork()
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string()
          break
        case 2:
          message.cloneOpts = CloneOpts.decode(reader, reader.uint32())
          break
        case 3:
          message.authOpts = AuthOpts.decode(reader, reader.uint32())
          break
        case 4:
          message.worktreeOpts = GitCreateWorktreeOp.decode(
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
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : '',
      cloneOpts: isSet(object.cloneOpts)
        ? CloneOpts.fromJSON(object.cloneOpts)
        : undefined,
      authOpts: isSet(object.authOpts)
        ? AuthOpts.fromJSON(object.authOpts)
        : undefined,
      worktreeOpts: isSet(object.worktreeOpts)
        ? GitCreateWorktreeOp.fromJSON(object.worktreeOpts)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.objectKey !== undefined && (obj.objectKey = message.objectKey)
    message.cloneOpts !== undefined &&
      (obj.cloneOpts = message.cloneOpts
        ? CloneOpts.toJSON(message.cloneOpts)
        : undefined)
    message.authOpts !== undefined &&
      (obj.authOpts = message.authOpts
        ? AuthOpts.toJSON(message.authOpts)
        : undefined)
    message.worktreeOpts !== undefined &&
      (obj.worktreeOpts = message.worktreeOpts
        ? GitCreateWorktreeOp.toJSON(message.worktreeOpts)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.objectKey = object.objectKey ?? ''
    message.cloneOpts =
      object.cloneOpts !== undefined && object.cloneOpts !== null
        ? CloneOpts.fromPartial(object.cloneOpts)
        : undefined
    message.authOpts =
      object.authOpts !== undefined && object.authOpts !== null
        ? AuthOpts.fromPartial(object.authOpts)
        : undefined
    message.worktreeOpts =
      object.worktreeOpts !== undefined && object.worktreeOpts !== null
        ? GitCreateWorktreeOp.fromPartial(object.worktreeOpts)
        : undefined
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
