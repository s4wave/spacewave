/* eslint-disable */
import { ControllerConfig } from '@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Value } from '../value/value.pb.js'

export const protobufPackage = 'forge.target'

/** InputType is the list of possible input types. */
export enum InputType {
  /** InputType_UNKNOWN - InputType_UNKNOWN is the zero output type. */
  InputType_UNKNOWN = 0,
  /** InputType_VALUE - InputType_VALUE is an in-line value input. */
  InputType_VALUE = 1,
  /** InputType_ALIAS - InputType_ALIAS aliases the input to another named input. */
  InputType_ALIAS = 2,
  /** InputType_WORLD - InputType_WORLD passes a handle to a Hydra World as an input. */
  InputType_WORLD = 3,
  /**
   * InputType_WORLD_OBJECT - InputType_WORLD_OBJECT passes a Value with the latest Object ref and a
   * world object handle attached to a WORLD input.
   */
  InputType_WORLD_OBJECT = 4,
  UNRECOGNIZED = -1,
}

export function inputTypeFromJSON(object: any): InputType {
  switch (object) {
    case 0:
    case 'InputType_UNKNOWN':
      return InputType.InputType_UNKNOWN
    case 1:
    case 'InputType_VALUE':
      return InputType.InputType_VALUE
    case 2:
    case 'InputType_ALIAS':
      return InputType.InputType_ALIAS
    case 3:
    case 'InputType_WORLD':
      return InputType.InputType_WORLD
    case 4:
    case 'InputType_WORLD_OBJECT':
      return InputType.InputType_WORLD_OBJECT
    case -1:
    case 'UNRECOGNIZED':
    default:
      return InputType.UNRECOGNIZED
  }
}

export function inputTypeToJSON(object: InputType): string {
  switch (object) {
    case InputType.InputType_UNKNOWN:
      return 'InputType_UNKNOWN'
    case InputType.InputType_VALUE:
      return 'InputType_VALUE'
    case InputType.InputType_ALIAS:
      return 'InputType_ALIAS'
    case InputType.InputType_WORLD:
      return 'InputType_WORLD'
    case InputType.InputType_WORLD_OBJECT:
      return 'InputType_WORLD_OBJECT'
    case InputType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** OutputType is the list of possible output types. */
export enum OutputType {
  /** OutputType_UNKNOWN - OutputType_UNKNOWN is the zero output type. */
  OutputType_UNKNOWN = 0,
  /** OutputType_EXEC - OutputType_EXEC is an output value mounted from an exec instance. */
  OutputType_EXEC = 1,
  /** OutputType_VALUE - OutputType_VALUE is an in-line output value (specified in the target). */
  OutputType_VALUE = 2,
  UNRECOGNIZED = -1,
}

export function outputTypeFromJSON(object: any): OutputType {
  switch (object) {
    case 0:
    case 'OutputType_UNKNOWN':
      return OutputType.OutputType_UNKNOWN
    case 1:
    case 'OutputType_EXEC':
      return OutputType.OutputType_EXEC
    case 2:
    case 'OutputType_VALUE':
      return OutputType.OutputType_VALUE
    case -1:
    case 'UNRECOGNIZED':
    default:
      return OutputType.UNRECOGNIZED
  }
}

export function outputTypeToJSON(object: OutputType): string {
  switch (object) {
    case OutputType.OutputType_UNKNOWN:
      return 'OutputType_UNKNOWN'
    case OutputType.OutputType_EXEC:
      return 'OutputType_EXEC'
    case OutputType.OutputType_VALUE:
      return 'OutputType_VALUE'
    case OutputType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Target contains a desired Task target configuration. */
export interface Target {
  /**
   * Inputs is the mapping of inputs.
   * Not necessarily sorted or unique, later values override earlier.
   */
  inputs: Input[]
  /**
   * Outputs is the mapping of outputs.
   * Not necessarily sorted or unique, later values override earlier.
   */
  outputs: Output[]
  /** Exec is the execution pass configuration. */
  exec: Exec | undefined
}

/** Input contains configuration for a Task target input. */
export interface Input {
  /** Name is the unique name of the input. */
  name: string
  /**
   * InputType is the type of input.
   * If UNKNOWN and Alias is set, assumes ALIAS.
   */
  inputType: InputType
  /** Alias is the name of the target aliased Input. */
  alias: string
  /** WatchChanges will restart the Task if the Value changes. */
  watchChanges: boolean
  /**
   * Value is the in-line data for the value input type.
   * InputType_VALUE
   */
  value: Value | undefined
  /**
   * World contains the args for the world input type.
   * InputType_WORLD
   */
  world: InputWorld | undefined
  /**
   * WorldObject contains the args for the world object input type.
   * Can be used for change detection: re-run Target when object changes.
   * InputType_WORLD_OBJECT
   */
  worldObject: InputWorldObject | undefined
}

/**
 * InputWorld are args for the world input type.
 * InputType_WORLD
 */
export interface InputWorld {
  /** EngineId is the world engine ID to lookup. */
  engineId: string
  /**
   * LookupImmediate indicates the execution controller should lookup and wait
   * for the world engine to be ready before starting execution. If false, the
   * execution controller will pass a BusEngine handle which will lookup the
   * engine lazily (on first request).
   */
  lookupImmediate: boolean
}

/**
 * InputWorldObject are args for the world object input type.
 * InputType_WORLD_OBJECT
 */
export interface InputWorldObject {
  /**
   * World is the name of the world input to lookup on.
   * If unset, defaults to the Forge Job world.
   */
  world: string
  /** ObjectKey is the object key to lookup. */
  objectKey: string
  /**
   * ObjectRev is the minimum object rev to wait for.
   * If set, waits for the object to exist.
   * If object_rev == 0, does not wait for the object to exist.
   */
  objectRev: Long
}

/**
 * Output contains configuration of a target task output.
 * This specifies where to get the Output value from.
 */
export interface Output {
  /** Name is the unique name of the output. */
  name: string
  /** OutputType is the type of output. */
  outputType: OutputType
  /**
   * ExecOutput is the name of the exec output to mount.
   * OutputType_EXEC
   */
  execOutput: string
  /**
   * Value is an in-line output value.
   * OutputType_VALUE
   */
  value: Value | undefined
}

/** Exec contains target execution configuration. */
export interface Exec {
  /** Disable is a flag to ignore the below contents and inhibit the exec step. */
  disable: boolean
  /** Controller indicates to run a controllerbus controller. */
  controller: ControllerConfig | undefined
}

/** ValueSet is the set of values satisfying inputs/outputs of a target. */
export interface ValueSet {
  /**
   * Inputs is the set of inputs.
   * Unique by the "name" field.
   * Sorted by name.
   */
  inputs: Value[]
  /**
   * Outputs is the set of outputs.
   * Unique by the "name" field.
   * Sorted by name.
   */
  outputs: Value[]
}

function createBaseTarget(): Target {
  return { inputs: [], outputs: [], exec: undefined }
}

export const Target = {
  encode(
    message: Target,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.inputs) {
      Input.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    for (const v of message.outputs) {
      Output.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    if (message.exec !== undefined) {
      Exec.encode(message.exec, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Target {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTarget()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.inputs.push(Input.decode(reader, reader.uint32()))
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.outputs.push(Output.decode(reader, reader.uint32()))
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.exec = Exec.decode(reader, reader.uint32())
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
  // Transform<Target, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Target | Target[]> | Iterable<Target | Target[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Target.encode(p).finish()]
        }
      } else {
        yield* [Target.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Target>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Target> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Target.decode(p)]
        }
      } else {
        yield* [Target.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Target {
    return {
      inputs: globalThis.Array.isArray(object?.inputs)
        ? object.inputs.map((e: any) => Input.fromJSON(e))
        : [],
      outputs: globalThis.Array.isArray(object?.outputs)
        ? object.outputs.map((e: any) => Output.fromJSON(e))
        : [],
      exec: isSet(object.exec) ? Exec.fromJSON(object.exec) : undefined,
    }
  },

  toJSON(message: Target): unknown {
    const obj: any = {}
    if (message.inputs?.length) {
      obj.inputs = message.inputs.map((e) => Input.toJSON(e))
    }
    if (message.outputs?.length) {
      obj.outputs = message.outputs.map((e) => Output.toJSON(e))
    }
    if (message.exec !== undefined) {
      obj.exec = Exec.toJSON(message.exec)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Target>, I>>(base?: I): Target {
    return Target.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Target>, I>>(object: I): Target {
    const message = createBaseTarget()
    message.inputs = object.inputs?.map((e) => Input.fromPartial(e)) || []
    message.outputs = object.outputs?.map((e) => Output.fromPartial(e)) || []
    message.exec =
      object.exec !== undefined && object.exec !== null
        ? Exec.fromPartial(object.exec)
        : undefined
    return message
  },
}

function createBaseInput(): Input {
  return {
    name: '',
    inputType: 0,
    alias: '',
    watchChanges: false,
    value: undefined,
    world: undefined,
    worldObject: undefined,
  }
}

export const Input = {
  encode(message: Input, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.inputType !== 0) {
      writer.uint32(16).int32(message.inputType)
    }
    if (message.alias !== '') {
      writer.uint32(26).string(message.alias)
    }
    if (message.watchChanges === true) {
      writer.uint32(56).bool(message.watchChanges)
    }
    if (message.value !== undefined) {
      Value.encode(message.value, writer.uint32(34).fork()).ldelim()
    }
    if (message.world !== undefined) {
      InputWorld.encode(message.world, writer.uint32(42).fork()).ldelim()
    }
    if (message.worldObject !== undefined) {
      InputWorldObject.encode(
        message.worldObject,
        writer.uint32(50).fork(),
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Input {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseInput()
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
          if (tag !== 16) {
            break
          }

          message.inputType = reader.int32() as any
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.alias = reader.string()
          continue
        case 7:
          if (tag !== 56) {
            break
          }

          message.watchChanges = reader.bool()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.value = Value.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.world = InputWorld.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.worldObject = InputWorldObject.decode(reader, reader.uint32())
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
  // Transform<Input, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Input | Input[]> | Iterable<Input | Input[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Input.encode(p).finish()]
        }
      } else {
        yield* [Input.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Input>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Input> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Input.decode(p)]
        }
      } else {
        yield* [Input.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Input {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      inputType: isSet(object.inputType)
        ? inputTypeFromJSON(object.inputType)
        : 0,
      alias: isSet(object.alias) ? globalThis.String(object.alias) : '',
      watchChanges: isSet(object.watchChanges)
        ? globalThis.Boolean(object.watchChanges)
        : false,
      value: isSet(object.value) ? Value.fromJSON(object.value) : undefined,
      world: isSet(object.world)
        ? InputWorld.fromJSON(object.world)
        : undefined,
      worldObject: isSet(object.worldObject)
        ? InputWorldObject.fromJSON(object.worldObject)
        : undefined,
    }
  },

  toJSON(message: Input): unknown {
    const obj: any = {}
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.inputType !== 0) {
      obj.inputType = inputTypeToJSON(message.inputType)
    }
    if (message.alias !== '') {
      obj.alias = message.alias
    }
    if (message.watchChanges === true) {
      obj.watchChanges = message.watchChanges
    }
    if (message.value !== undefined) {
      obj.value = Value.toJSON(message.value)
    }
    if (message.world !== undefined) {
      obj.world = InputWorld.toJSON(message.world)
    }
    if (message.worldObject !== undefined) {
      obj.worldObject = InputWorldObject.toJSON(message.worldObject)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Input>, I>>(base?: I): Input {
    return Input.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Input>, I>>(object: I): Input {
    const message = createBaseInput()
    message.name = object.name ?? ''
    message.inputType = object.inputType ?? 0
    message.alias = object.alias ?? ''
    message.watchChanges = object.watchChanges ?? false
    message.value =
      object.value !== undefined && object.value !== null
        ? Value.fromPartial(object.value)
        : undefined
    message.world =
      object.world !== undefined && object.world !== null
        ? InputWorld.fromPartial(object.world)
        : undefined
    message.worldObject =
      object.worldObject !== undefined && object.worldObject !== null
        ? InputWorldObject.fromPartial(object.worldObject)
        : undefined
    return message
  },
}

function createBaseInputWorld(): InputWorld {
  return { engineId: '', lookupImmediate: false }
}

export const InputWorld = {
  encode(
    message: InputWorld,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.engineId !== '') {
      writer.uint32(10).string(message.engineId)
    }
    if (message.lookupImmediate === true) {
      writer.uint32(16).bool(message.lookupImmediate)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputWorld {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseInputWorld()
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
          if (tag !== 16) {
            break
          }

          message.lookupImmediate = reader.bool()
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
  // Transform<InputWorld, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<InputWorld | InputWorld[]>
      | Iterable<InputWorld | InputWorld[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [InputWorld.encode(p).finish()]
        }
      } else {
        yield* [InputWorld.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, InputWorld>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<InputWorld> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [InputWorld.decode(p)]
        }
      } else {
        yield* [InputWorld.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): InputWorld {
    return {
      engineId: isSet(object.engineId)
        ? globalThis.String(object.engineId)
        : '',
      lookupImmediate: isSet(object.lookupImmediate)
        ? globalThis.Boolean(object.lookupImmediate)
        : false,
    }
  },

  toJSON(message: InputWorld): unknown {
    const obj: any = {}
    if (message.engineId !== '') {
      obj.engineId = message.engineId
    }
    if (message.lookupImmediate === true) {
      obj.lookupImmediate = message.lookupImmediate
    }
    return obj
  },

  create<I extends Exact<DeepPartial<InputWorld>, I>>(base?: I): InputWorld {
    return InputWorld.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<InputWorld>, I>>(
    object: I,
  ): InputWorld {
    const message = createBaseInputWorld()
    message.engineId = object.engineId ?? ''
    message.lookupImmediate = object.lookupImmediate ?? false
    return message
  },
}

function createBaseInputWorldObject(): InputWorldObject {
  return { world: '', objectKey: '', objectRev: Long.UZERO }
}

export const InputWorldObject = {
  encode(
    message: InputWorldObject,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.world !== '') {
      writer.uint32(10).string(message.world)
    }
    if (message.objectKey !== '') {
      writer.uint32(18).string(message.objectKey)
    }
    if (!message.objectRev.isZero()) {
      writer.uint32(24).uint64(message.objectRev)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputWorldObject {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseInputWorldObject()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.world = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.objectKey = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.objectRev = reader.uint64() as Long
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
  // Transform<InputWorldObject, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<InputWorldObject | InputWorldObject[]>
      | Iterable<InputWorldObject | InputWorldObject[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [InputWorldObject.encode(p).finish()]
        }
      } else {
        yield* [InputWorldObject.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, InputWorldObject>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<InputWorldObject> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [InputWorldObject.decode(p)]
        }
      } else {
        yield* [InputWorldObject.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): InputWorldObject {
    return {
      world: isSet(object.world) ? globalThis.String(object.world) : '',
      objectKey: isSet(object.objectKey)
        ? globalThis.String(object.objectKey)
        : '',
      objectRev: isSet(object.objectRev)
        ? Long.fromValue(object.objectRev)
        : Long.UZERO,
    }
  },

  toJSON(message: InputWorldObject): unknown {
    const obj: any = {}
    if (message.world !== '') {
      obj.world = message.world
    }
    if (message.objectKey !== '') {
      obj.objectKey = message.objectKey
    }
    if (!message.objectRev.isZero()) {
      obj.objectRev = (message.objectRev || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<InputWorldObject>, I>>(
    base?: I,
  ): InputWorldObject {
    return InputWorldObject.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<InputWorldObject>, I>>(
    object: I,
  ): InputWorldObject {
    const message = createBaseInputWorldObject()
    message.world = object.world ?? ''
    message.objectKey = object.objectKey ?? ''
    message.objectRev =
      object.objectRev !== undefined && object.objectRev !== null
        ? Long.fromValue(object.objectRev)
        : Long.UZERO
    return message
  },
}

function createBaseOutput(): Output {
  return { name: '', outputType: 0, execOutput: '', value: undefined }
}

export const Output = {
  encode(
    message: Output,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.outputType !== 0) {
      writer.uint32(16).int32(message.outputType)
    }
    if (message.execOutput !== '') {
      writer.uint32(26).string(message.execOutput)
    }
    if (message.value !== undefined) {
      Value.encode(message.value, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Output {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOutput()
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
          if (tag !== 16) {
            break
          }

          message.outputType = reader.int32() as any
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.execOutput = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.value = Value.decode(reader, reader.uint32())
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
  // Transform<Output, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Output | Output[]> | Iterable<Output | Output[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Output.encode(p).finish()]
        }
      } else {
        yield* [Output.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Output>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Output> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Output.decode(p)]
        }
      } else {
        yield* [Output.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Output {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      outputType: isSet(object.outputType)
        ? outputTypeFromJSON(object.outputType)
        : 0,
      execOutput: isSet(object.execOutput)
        ? globalThis.String(object.execOutput)
        : '',
      value: isSet(object.value) ? Value.fromJSON(object.value) : undefined,
    }
  },

  toJSON(message: Output): unknown {
    const obj: any = {}
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.outputType !== 0) {
      obj.outputType = outputTypeToJSON(message.outputType)
    }
    if (message.execOutput !== '') {
      obj.execOutput = message.execOutput
    }
    if (message.value !== undefined) {
      obj.value = Value.toJSON(message.value)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Output>, I>>(base?: I): Output {
    return Output.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Output>, I>>(object: I): Output {
    const message = createBaseOutput()
    message.name = object.name ?? ''
    message.outputType = object.outputType ?? 0
    message.execOutput = object.execOutput ?? ''
    message.value =
      object.value !== undefined && object.value !== null
        ? Value.fromPartial(object.value)
        : undefined
    return message
  },
}

function createBaseExec(): Exec {
  return { disable: false, controller: undefined }
}

export const Exec = {
  encode(message: Exec, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.disable === true) {
      writer.uint32(8).bool(message.disable)
    }
    if (message.controller !== undefined) {
      ControllerConfig.encode(
        message.controller,
        writer.uint32(18).fork(),
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Exec {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseExec()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.disable = reader.bool()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.controller = ControllerConfig.decode(reader, reader.uint32())
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
  // Transform<Exec, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Exec | Exec[]> | Iterable<Exec | Exec[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Exec.encode(p).finish()]
        }
      } else {
        yield* [Exec.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Exec>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Exec> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Exec.decode(p)]
        }
      } else {
        yield* [Exec.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Exec {
    return {
      disable: isSet(object.disable)
        ? globalThis.Boolean(object.disable)
        : false,
      controller: isSet(object.controller)
        ? ControllerConfig.fromJSON(object.controller)
        : undefined,
    }
  },

  toJSON(message: Exec): unknown {
    const obj: any = {}
    if (message.disable === true) {
      obj.disable = message.disable
    }
    if (message.controller !== undefined) {
      obj.controller = ControllerConfig.toJSON(message.controller)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Exec>, I>>(base?: I): Exec {
    return Exec.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Exec>, I>>(object: I): Exec {
    const message = createBaseExec()
    message.disable = object.disable ?? false
    message.controller =
      object.controller !== undefined && object.controller !== null
        ? ControllerConfig.fromPartial(object.controller)
        : undefined
    return message
  },
}

function createBaseValueSet(): ValueSet {
  return { inputs: [], outputs: [] }
}

export const ValueSet = {
  encode(
    message: ValueSet,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.inputs) {
      Value.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    for (const v of message.outputs) {
      Value.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ValueSet {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseValueSet()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.inputs.push(Value.decode(reader, reader.uint32()))
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.outputs.push(Value.decode(reader, reader.uint32()))
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
  // Transform<ValueSet, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ValueSet | ValueSet[]>
      | Iterable<ValueSet | ValueSet[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ValueSet.encode(p).finish()]
        }
      } else {
        yield* [ValueSet.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ValueSet>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ValueSet> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ValueSet.decode(p)]
        }
      } else {
        yield* [ValueSet.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ValueSet {
    return {
      inputs: globalThis.Array.isArray(object?.inputs)
        ? object.inputs.map((e: any) => Value.fromJSON(e))
        : [],
      outputs: globalThis.Array.isArray(object?.outputs)
        ? object.outputs.map((e: any) => Value.fromJSON(e))
        : [],
    }
  },

  toJSON(message: ValueSet): unknown {
    const obj: any = {}
    if (message.inputs?.length) {
      obj.inputs = message.inputs.map((e) => Value.toJSON(e))
    }
    if (message.outputs?.length) {
      obj.outputs = message.outputs.map((e) => Value.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ValueSet>, I>>(base?: I): ValueSet {
    return ValueSet.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ValueSet>, I>>(object: I): ValueSet {
    const message = createBaseValueSet()
    message.inputs = object.inputs?.map((e) => Value.fromPartial(e)) || []
    message.outputs = object.outputs?.map((e) => Value.fromPartial(e)) || []
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
