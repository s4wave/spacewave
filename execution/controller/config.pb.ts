/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { InputWorld, Target } from '../../target/target.pb.js'
import { Execution } from '../execution.pb.js'

export const protobufPackage = 'execution.controller'

/** Config is the execution controller configuration. */
export interface Config {
  /** EngineId is the world engine id for the forge state. */
  engineId: string
  /**
   * ObjectKey is the Execution state object to attach to.
   * If not exists, waits for it to exist.
   */
  objectKey: string
  /**
   * PeerId is the peer ID to use for the execution controller.
   * If the Execution already has a peer_id set, must match it.
   * If not set, will look up the peer id from the state.
   */
  peerId: string
  /** ResolveControllerConfigTimeout is a timeout for resolving the exec.controller config. */
  resolveControllerConfigTimeout: string
  /** AllowNonExecController allows exec.controller to not implement ExecController. */
  allowNonExecController: boolean
  /** InputWorld is the default value for the "world" input. */
  inputWorld: InputWorld | undefined
}

/** ExecConfig is a configuration for the execution routine. */
export interface ExecConfig {
  /**
   * Execution is the current state of the execution.
   * NOTE: value_set and result are set to nil.
   */
  execution: Execution | undefined
  /**
   * Target is the current target to execute.
   * This is the value of the TargetRef in execution.
   */
  target: Target | undefined
}

function createBaseConfig(): Config {
  return {
    engineId: '',
    objectKey: '',
    peerId: '',
    resolveControllerConfigTimeout: '',
    allowNonExecController: false,
    inputWorld: undefined,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.engineId !== '') {
      writer.uint32(10).string(message.engineId)
    }
    if (message.objectKey !== '') {
      writer.uint32(18).string(message.objectKey)
    }
    if (message.peerId !== '') {
      writer.uint32(26).string(message.peerId)
    }
    if (message.resolveControllerConfigTimeout !== '') {
      writer.uint32(34).string(message.resolveControllerConfigTimeout)
    }
    if (message.allowNonExecController === true) {
      writer.uint32(40).bool(message.allowNonExecController)
    }
    if (message.inputWorld !== undefined) {
      InputWorld.encode(message.inputWorld, writer.uint32(50).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.engineId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.objectKey = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.peerId = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.resolveControllerConfigTimeout = reader.string()
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.allowNonExecController = reader.bool()
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.inputWorld = InputWorld.decode(reader, reader.uint32())
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
      engineId: isSet(object.engineId) ? String(object.engineId) : '',
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
      resolveControllerConfigTimeout: isSet(
        object.resolveControllerConfigTimeout
      )
        ? String(object.resolveControllerConfigTimeout)
        : '',
      allowNonExecController: isSet(object.allowNonExecController)
        ? Boolean(object.allowNonExecController)
        : false,
      inputWorld: isSet(object.inputWorld)
        ? InputWorld.fromJSON(object.inputWorld)
        : undefined,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.engineId !== undefined && (obj.engineId = message.engineId)
    message.objectKey !== undefined && (obj.objectKey = message.objectKey)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    message.resolveControllerConfigTimeout !== undefined &&
      (obj.resolveControllerConfigTimeout =
        message.resolveControllerConfigTimeout)
    message.allowNonExecController !== undefined &&
      (obj.allowNonExecController = message.allowNonExecController)
    message.inputWorld !== undefined &&
      (obj.inputWorld = message.inputWorld
        ? InputWorld.toJSON(message.inputWorld)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.engineId = object.engineId ?? ''
    message.objectKey = object.objectKey ?? ''
    message.peerId = object.peerId ?? ''
    message.resolveControllerConfigTimeout =
      object.resolveControllerConfigTimeout ?? ''
    message.allowNonExecController = object.allowNonExecController ?? false
    message.inputWorld =
      object.inputWorld !== undefined && object.inputWorld !== null
        ? InputWorld.fromPartial(object.inputWorld)
        : undefined
    return message
  },
}

function createBaseExecConfig(): ExecConfig {
  return { execution: undefined, target: undefined }
}

export const ExecConfig = {
  encode(
    message: ExecConfig,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.execution !== undefined) {
      Execution.encode(message.execution, writer.uint32(10).fork()).ldelim()
    }
    if (message.target !== undefined) {
      Target.encode(message.target, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ExecConfig {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseExecConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.execution = Execution.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.target = Target.decode(reader, reader.uint32())
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
  // Transform<ExecConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ExecConfig | ExecConfig[]>
      | Iterable<ExecConfig | ExecConfig[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExecConfig.encode(p).finish()]
        }
      } else {
        yield* [ExecConfig.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ExecConfig>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ExecConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExecConfig.decode(p)]
        }
      } else {
        yield* [ExecConfig.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ExecConfig {
    return {
      execution: isSet(object.execution)
        ? Execution.fromJSON(object.execution)
        : undefined,
      target: isSet(object.target) ? Target.fromJSON(object.target) : undefined,
    }
  },

  toJSON(message: ExecConfig): unknown {
    const obj: any = {}
    message.execution !== undefined &&
      (obj.execution = message.execution
        ? Execution.toJSON(message.execution)
        : undefined)
    message.target !== undefined &&
      (obj.target = message.target ? Target.toJSON(message.target) : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<ExecConfig>, I>>(base?: I): ExecConfig {
    return ExecConfig.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ExecConfig>, I>>(
    object: I
  ): ExecConfig {
    const message = createBaseExecConfig()
    message.execution =
      object.execution !== undefined && object.execution !== null
        ? Execution.fromPartial(object.execution)
        : undefined
    message.target =
      object.target !== undefined && object.target !== null
        ? Target.fromPartial(object.target)
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
