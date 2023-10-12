/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../block.pb.js'

export const protobufPackage = 'block.mock'

/** Root is the root of the mock structure. */
export interface Root {
  /** ExampleSubBlock is a sub-block. */
  exampleSubBlock: SubBlock | undefined
}

/** SubBlock is a example sub-block of Root. */
export interface SubBlock {
  /** ExamplePtr is an example reference. */
  examplePtr: BlockRef | undefined
}

/** Example is the value pointed to by ExamplePtr. */
export interface Example {
  /** Msg is a message. */
  msg: string
}

function createBaseRoot(): Root {
  return { exampleSubBlock: undefined }
}

export const Root = {
  encode(message: Root, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.exampleSubBlock !== undefined) {
      SubBlock.encode(
        message.exampleSubBlock,
        writer.uint32(10).fork(),
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Root {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.exampleSubBlock = SubBlock.decode(reader, reader.uint32())
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
  // Transform<Root, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Root | Root[]> | Iterable<Root | Root[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Root.encode(p).finish()]
        }
      } else {
        yield* [Root.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Root>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Root> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Root.decode(p)]
        }
      } else {
        yield* [Root.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Root {
    return {
      exampleSubBlock: isSet(object.exampleSubBlock)
        ? SubBlock.fromJSON(object.exampleSubBlock)
        : undefined,
    }
  },

  toJSON(message: Root): unknown {
    const obj: any = {}
    if (message.exampleSubBlock !== undefined) {
      obj.exampleSubBlock = SubBlock.toJSON(message.exampleSubBlock)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Root>, I>>(base?: I): Root {
    return Root.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Root>, I>>(object: I): Root {
    const message = createBaseRoot()
    message.exampleSubBlock =
      object.exampleSubBlock !== undefined && object.exampleSubBlock !== null
        ? SubBlock.fromPartial(object.exampleSubBlock)
        : undefined
    return message
  },
}

function createBaseSubBlock(): SubBlock {
  return { examplePtr: undefined }
}

export const SubBlock = {
  encode(
    message: SubBlock,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.examplePtr !== undefined) {
      BlockRef.encode(message.examplePtr, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): SubBlock {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseSubBlock()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.examplePtr = BlockRef.decode(reader, reader.uint32())
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
  // Transform<SubBlock, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<SubBlock | SubBlock[]>
      | Iterable<SubBlock | SubBlock[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [SubBlock.encode(p).finish()]
        }
      } else {
        yield* [SubBlock.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SubBlock>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<SubBlock> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [SubBlock.decode(p)]
        }
      } else {
        yield* [SubBlock.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): SubBlock {
    return {
      examplePtr: isSet(object.examplePtr)
        ? BlockRef.fromJSON(object.examplePtr)
        : undefined,
    }
  },

  toJSON(message: SubBlock): unknown {
    const obj: any = {}
    if (message.examplePtr !== undefined) {
      obj.examplePtr = BlockRef.toJSON(message.examplePtr)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<SubBlock>, I>>(base?: I): SubBlock {
    return SubBlock.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<SubBlock>, I>>(object: I): SubBlock {
    const message = createBaseSubBlock()
    message.examplePtr =
      object.examplePtr !== undefined && object.examplePtr !== null
        ? BlockRef.fromPartial(object.examplePtr)
        : undefined
    return message
  },
}

function createBaseExample(): Example {
  return { msg: '' }
}

export const Example = {
  encode(
    message: Example,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.msg !== '') {
      writer.uint32(10).string(message.msg)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Example {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseExample()
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
  // Transform<Example, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Example | Example[]> | Iterable<Example | Example[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Example.encode(p).finish()]
        }
      } else {
        yield* [Example.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Example>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Example> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Example.decode(p)]
        }
      } else {
        yield* [Example.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Example {
    return { msg: isSet(object.msg) ? globalThis.String(object.msg) : '' }
  },

  toJSON(message: Example): unknown {
    const obj: any = {}
    if (message.msg !== '') {
      obj.msg = message.msg
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Example>, I>>(base?: I): Example {
    return Example.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Example>, I>>(object: I): Example {
    const message = createBaseExample()
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
