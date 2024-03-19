/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Value } from '../../value/value.pb.js'

export const protobufPackage = 'forge.lib.kvtx'

/** OpType is the list of operation codes for kvtx. */
export enum OpType {
  /** OpType_NONE - OpType_NONE indicates this Op just sets values for nested ops. */
  OpType_NONE = 0,
  /** OpType_GET - OpType_GET retrieves a BlockRef from the store and copies to output. */
  OpType_GET = 1,
  /** OpType_GET_EXISTS - OpType_GET_EXISTS checks if the key exists and writes a msgpack boolean block to output. */
  OpType_GET_EXISTS = 2,
  /**
   * OpType_CHECK - OpType_CHECK performs a GET operation and checks the value against the input.
   * The input must be a non-empty Cursor.
   * Compares the block reference stored with the given block ref.
   */
  OpType_CHECK = 3,
  /**
   * OpType_CHECK_BLOB - OpType_CHECK_BLOB performs a GET operation and checks the raw value against the input.
   * The input must be a non-empty Blob.
   * Compares the blob contents with the input blob contents.
   */
  OpType_CHECK_BLOB = 4,
  /** OpType_CHECK_EXISTS - OpType_CHECK_EXISTS checks if the given key exists. */
  OpType_CHECK_EXISTS = 5,
  /** OpType_CHECK_NOT_EXISTS - OpType_CHECK_NOT_EXISTS ensures the given key does not exist. */
  OpType_CHECK_NOT_EXISTS = 6,
  /**
   * OpType_SET - OpType_SET sets a BlockRef to the block referenced by the Value.
   * Note: does not handle Blob values, sets as a reference only.
   */
  OpType_SET = 7,
  /**
   * OpType_SET_BLOB - OpType_SET_BLOB treats the input BlockRef as a Blob and stores it.
   * The GET operation will return the Blob contents.
   */
  OpType_SET_BLOB = 8,
  /**
   * OpType_DELETE - OpType_DELETE deletes a key from the store.
   * The old value is written to the output, if set.
   */
  OpType_DELETE = 9,
  UNRECOGNIZED = -1,
}

export function opTypeFromJSON(object: any): OpType {
  switch (object) {
    case 0:
    case 'OpType_NONE':
      return OpType.OpType_NONE
    case 1:
    case 'OpType_GET':
      return OpType.OpType_GET
    case 2:
    case 'OpType_GET_EXISTS':
      return OpType.OpType_GET_EXISTS
    case 3:
    case 'OpType_CHECK':
      return OpType.OpType_CHECK
    case 4:
    case 'OpType_CHECK_BLOB':
      return OpType.OpType_CHECK_BLOB
    case 5:
    case 'OpType_CHECK_EXISTS':
      return OpType.OpType_CHECK_EXISTS
    case 6:
    case 'OpType_CHECK_NOT_EXISTS':
      return OpType.OpType_CHECK_NOT_EXISTS
    case 7:
    case 'OpType_SET':
      return OpType.OpType_SET
    case 8:
    case 'OpType_SET_BLOB':
      return OpType.OpType_SET_BLOB
    case 9:
    case 'OpType_DELETE':
      return OpType.OpType_DELETE
    case -1:
    case 'UNRECOGNIZED':
    default:
      return OpType.UNRECOGNIZED
  }
}

export function opTypeToJSON(object: OpType): string {
  switch (object) {
    case OpType.OpType_NONE:
      return 'OpType_NONE'
    case OpType.OpType_GET:
      return 'OpType_GET'
    case OpType.OpType_GET_EXISTS:
      return 'OpType_GET_EXISTS'
    case OpType.OpType_CHECK:
      return 'OpType_CHECK'
    case OpType.OpType_CHECK_BLOB:
      return 'OpType_CHECK_BLOB'
    case OpType.OpType_CHECK_EXISTS:
      return 'OpType_CHECK_EXISTS'
    case OpType.OpType_CHECK_NOT_EXISTS:
      return 'OpType_CHECK_NOT_EXISTS'
    case OpType.OpType_SET:
      return 'OpType_SET'
    case OpType.OpType_SET_BLOB:
      return 'OpType_SET_BLOB'
    case OpType.OpType_DELETE:
      return 'OpType_DELETE'
    case OpType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * Config is the configuration for the Forge KVTX exec controller.
 * Implements get, set, delete, keys against a k/v block graph.
 * Operations are sequential, failed ops cancel the entire sequence.
 *
 * Inputs:
 *  - store: reference to a kvtx_block.KeyValueStore.
 *  - ... config_input as configured below (optional).
 *  - ... any operation inputs configured below
 * Outputs:
 *  - store: output (modified) KeyValueStore.
 *  - ... any operation outputs configured below.
 */
export interface Config {
  /** Ops is the list of operations to apply. */
  ops: Op[]
  /**
   * ConfigInput is the name of an Input to load additional ops from.
   * The referenced block should contain a ConfigInput object.
   */
  configInput: string
  /** IgnoreErrors warns on errors and continues w/o failing. */
  ignoreErrors: boolean
}

/** ConfigInput is a block containing additional ops from an Input. */
export interface ConfigInput {
  /** Ops is the list of operations to apply. */
  ops: Op[]
}

/** Op is an operation definition. */
export interface Op {
  /** OpType is the operation type to apply. */
  opType: OpType
  /**
   * KeyInput is the name of the Input to use for the Key.
   * The raw block data will be used as the key.
   */
  keyInput: string
  /**
   * Key is the in-line configured key to use.
   * Converted to []byte without nil terminator.
   * If set, overrides input_key.
   */
  key: string
  /**
   * ValueInput is the name of the Input to use for the Value argument.
   *
   * - CHECK: contains the value to check against the GET value.
   * - SET: contains the value to store.
   *
   * If SET, stores the block reference into the tree.
   * If SET_BLOB, stores the block reference into the tree, marking it as a Blob
   * If CHECK, compares the given block reference with the stored.
   * Note: overridden by inline value if set and not empty.
   * Note: overridden by value_string if set and not empty.
   */
  valueInput: string
  /**
   * Value is the in-line value argument.
   *
   * See input field for additional notes.
   */
  value: Value | undefined
  /**
   * ValueString is an in-line string specification of the value.
   * Note: overridden by inline value if set and not empty.
   */
  valueString: string
  /**
   * Output is the name of the output to use for the Value.
   *
   * - GET: contains the value retrieved from the store.
   * - CHECK: same as GET, contains the retrieved value.
   * - SET: contains the value added to the store.
   * - DELETE: contains the value just before deleting.
   *
   * Note: contains the single most recently set value only.
   * Note: stores the block reference as the output Value.
   */
  output: string
  /**
   * Ops is the list of sub-operations to apply.
   * The sub-operations inherit the parent config.
   */
  ops: Op[]
}

function createBaseConfig(): Config {
  return { ops: [], configInput: '', ignoreErrors: false }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.ops) {
      Op.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    if (message.configInput !== '') {
      writer.uint32(18).string(message.configInput)
    }
    if (message.ignoreErrors !== false) {
      writer.uint32(24).bool(message.ignoreErrors)
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

          message.ops.push(Op.decode(reader, reader.uint32()))
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.configInput = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.ignoreErrors = reader.bool()
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
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      ops: globalThis.Array.isArray(object?.ops)
        ? object.ops.map((e: any) => Op.fromJSON(e))
        : [],
      configInput: isSet(object.configInput)
        ? globalThis.String(object.configInput)
        : '',
      ignoreErrors: isSet(object.ignoreErrors)
        ? globalThis.Boolean(object.ignoreErrors)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.ops?.length) {
      obj.ops = message.ops.map((e) => Op.toJSON(e))
    }
    if (message.configInput !== '') {
      obj.configInput = message.configInput
    }
    if (message.ignoreErrors !== false) {
      obj.ignoreErrors = message.ignoreErrors
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.ops = object.ops?.map((e) => Op.fromPartial(e)) || []
    message.configInput = object.configInput ?? ''
    message.ignoreErrors = object.ignoreErrors ?? false
    return message
  },
}

function createBaseConfigInput(): ConfigInput {
  return { ops: [] }
}

export const ConfigInput = {
  encode(
    message: ConfigInput,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.ops) {
      Op.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ConfigInput {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfigInput()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.ops.push(Op.decode(reader, reader.uint32()))
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
  // Transform<ConfigInput, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ConfigInput | ConfigInput[]>
      | Iterable<ConfigInput | ConfigInput[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ConfigInput.encode(p).finish()]
        }
      } else {
        yield* [ConfigInput.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ConfigInput>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ConfigInput> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ConfigInput.decode(p)]
        }
      } else {
        yield* [ConfigInput.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ConfigInput {
    return {
      ops: globalThis.Array.isArray(object?.ops)
        ? object.ops.map((e: any) => Op.fromJSON(e))
        : [],
    }
  },

  toJSON(message: ConfigInput): unknown {
    const obj: any = {}
    if (message.ops?.length) {
      obj.ops = message.ops.map((e) => Op.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ConfigInput>, I>>(base?: I): ConfigInput {
    return ConfigInput.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ConfigInput>, I>>(
    object: I,
  ): ConfigInput {
    const message = createBaseConfigInput()
    message.ops = object.ops?.map((e) => Op.fromPartial(e)) || []
    return message
  },
}

function createBaseOp(): Op {
  return {
    opType: 0,
    keyInput: '',
    key: '',
    valueInput: '',
    value: undefined,
    valueString: '',
    output: '',
    ops: [],
  }
}

export const Op = {
  encode(message: Op, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.opType !== 0) {
      writer.uint32(8).int32(message.opType)
    }
    if (message.keyInput !== '') {
      writer.uint32(18).string(message.keyInput)
    }
    if (message.key !== '') {
      writer.uint32(26).string(message.key)
    }
    if (message.valueInput !== '') {
      writer.uint32(50).string(message.valueInput)
    }
    if (message.value !== undefined) {
      Value.encode(message.value, writer.uint32(58).fork()).ldelim()
    }
    if (message.valueString !== '') {
      writer.uint32(66).string(message.valueString)
    }
    if (message.output !== '') {
      writer.uint32(74).string(message.output)
    }
    for (const v of message.ops) {
      Op.encode(v!, writer.uint32(82).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Op {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opType = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.keyInput = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.key = reader.string()
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.valueInput = reader.string()
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.value = Value.decode(reader, reader.uint32())
          continue
        case 8:
          if (tag !== 66) {
            break
          }

          message.valueString = reader.string()
          continue
        case 9:
          if (tag !== 74) {
            break
          }

          message.output = reader.string()
          continue
        case 10:
          if (tag !== 82) {
            break
          }

          message.ops.push(Op.decode(reader, reader.uint32()))
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
  // Transform<Op, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Op | Op[]> | Iterable<Op | Op[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Op.encode(p).finish()]
        }
      } else {
        yield* [Op.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Op>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Op> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Op.decode(p)]
        }
      } else {
        yield* [Op.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Op {
    return {
      opType: isSet(object.opType) ? opTypeFromJSON(object.opType) : 0,
      keyInput: isSet(object.keyInput)
        ? globalThis.String(object.keyInput)
        : '',
      key: isSet(object.key) ? globalThis.String(object.key) : '',
      valueInput: isSet(object.valueInput)
        ? globalThis.String(object.valueInput)
        : '',
      value: isSet(object.value) ? Value.fromJSON(object.value) : undefined,
      valueString: isSet(object.valueString)
        ? globalThis.String(object.valueString)
        : '',
      output: isSet(object.output) ? globalThis.String(object.output) : '',
      ops: globalThis.Array.isArray(object?.ops)
        ? object.ops.map((e: any) => Op.fromJSON(e))
        : [],
    }
  },

  toJSON(message: Op): unknown {
    const obj: any = {}
    if (message.opType !== 0) {
      obj.opType = opTypeToJSON(message.opType)
    }
    if (message.keyInput !== '') {
      obj.keyInput = message.keyInput
    }
    if (message.key !== '') {
      obj.key = message.key
    }
    if (message.valueInput !== '') {
      obj.valueInput = message.valueInput
    }
    if (message.value !== undefined) {
      obj.value = Value.toJSON(message.value)
    }
    if (message.valueString !== '') {
      obj.valueString = message.valueString
    }
    if (message.output !== '') {
      obj.output = message.output
    }
    if (message.ops?.length) {
      obj.ops = message.ops.map((e) => Op.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Op>, I>>(base?: I): Op {
    return Op.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Op>, I>>(object: I): Op {
    const message = createBaseOp()
    message.opType = object.opType ?? 0
    message.keyInput = object.keyInput ?? ''
    message.key = object.key ?? ''
    message.valueInput = object.valueInput ?? ''
    message.value =
      object.value !== undefined && object.value !== null
        ? Value.fromPartial(object.value)
        : undefined
    message.valueString = object.valueString ?? ''
    message.output = object.output ?? ''
    message.ops = object.ops?.map((e) => Op.fromPartial(e)) || []
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
