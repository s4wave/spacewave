/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bldr.example'

/** Config configures the Demo controller. */
export interface Config {
  /** RunDemo runs the full demo routine. */
  runDemo: boolean
}

/** ExampleProps contains properties for the example component. */
export interface ExampleProps {
  /** Msg is the message to display. */
  msg: string
}

function createBaseConfig(): Config {
  return { runDemo: false }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.runDemo === true) {
      writer.uint32(8).bool(message.runDemo)
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
          if (tag !== 8) {
            break
          }

          message.runDemo = reader.bool()
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
      | Iterable<Uint8Array | Uint8Array[]>,
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
    return { runDemo: isSet(object.runDemo) ? Boolean(object.runDemo) : false }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.runDemo === true) {
      obj.runDemo = message.runDemo
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.runDemo = object.runDemo ?? false
    return message
  },
}

function createBaseExampleProps(): ExampleProps {
  return { msg: '' }
}

export const ExampleProps = {
  encode(
    message: ExampleProps,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.msg !== '') {
      writer.uint32(10).string(message.msg)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ExampleProps {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseExampleProps()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.msg = reader.string()
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
  // Transform<ExampleProps, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ExampleProps | ExampleProps[]>
      | Iterable<ExampleProps | ExampleProps[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExampleProps.encode(p).finish()]
        }
      } else {
        yield* [ExampleProps.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ExampleProps>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ExampleProps> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExampleProps.decode(p)]
        }
      } else {
        yield* [ExampleProps.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ExampleProps {
    return { msg: isSet(object.msg) ? String(object.msg) : '' }
  },

  toJSON(message: ExampleProps): unknown {
    const obj: any = {}
    if (message.msg !== '') {
      obj.msg = message.msg
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ExampleProps>, I>>(
    base?: I,
  ): ExampleProps {
    return ExampleProps.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ExampleProps>, I>>(
    object: I,
  ): ExampleProps {
    const message = createBaseExampleProps()
    message.msg = object.msg ?? ''
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
