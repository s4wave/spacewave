/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../../../block/block.pb.js'

export const protobufPackage = 'kvtx.block.iavl'

/** Node is a node in the tree. */
export interface Node {
  /**
   * Height contains the item's height.
   * Height is distance from the leaf.
   */
  height: number
  /** Size contains the node's size. */
  size: Long
  /** Key contains the node's key. */
  key: Uint8Array
  /**
   * ValueRef contains a reference to the item's value.
   * Set only if height == 0.
   */
  valueRef: BlockRef | undefined
  /**
   * ValueRefBlob indicates that the ValueRef is a Blob.
   * If false, Get() will return the raw data of the block.
   */
  valueRefBlob: boolean
  /**
   * LeftChildRef contains the left child ref.
   * Set only if height != 0.
   */
  leftChildRef: BlockRef | undefined
  /**
   * RightChildRef contains the right child ref.
   * Set only if height != 0.
   */
  rightChildRef: BlockRef | undefined
}

function createBaseNode(): Node {
  return {
    height: 0,
    size: Long.UZERO,
    key: new Uint8Array(),
    valueRef: undefined,
    valueRefBlob: false,
    leftChildRef: undefined,
    rightChildRef: undefined,
  }
}

export const Node = {
  encode(message: Node, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.height !== 0) {
      writer.uint32(8).uint32(message.height)
    }
    if (!message.size.isZero()) {
      writer.uint32(16).uint64(message.size)
    }
    if (message.key.length !== 0) {
      writer.uint32(26).bytes(message.key)
    }
    if (message.valueRef !== undefined) {
      BlockRef.encode(message.valueRef, writer.uint32(58).fork()).ldelim()
    }
    if (message.valueRefBlob === true) {
      writer.uint32(64).bool(message.valueRefBlob)
    }
    if (message.leftChildRef !== undefined) {
      BlockRef.encode(message.leftChildRef, writer.uint32(42).fork()).ldelim()
    }
    if (message.rightChildRef !== undefined) {
      BlockRef.encode(message.rightChildRef, writer.uint32(50).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Node {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseNode()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 8) {
            break
          }

          message.height = reader.uint32()
          continue
        case 2:
          if (tag != 16) {
            break
          }

          message.size = reader.uint64() as Long
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.key = reader.bytes()
          continue
        case 7:
          if (tag != 58) {
            break
          }

          message.valueRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 8:
          if (tag != 64) {
            break
          }

          message.valueRefBlob = reader.bool()
          continue
        case 5:
          if (tag != 42) {
            break
          }

          message.leftChildRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag != 50) {
            break
          }

          message.rightChildRef = BlockRef.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) == 4 || tag == 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Node, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Node | Node[]> | Iterable<Node | Node[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Node.encode(p).finish()]
        }
      } else {
        yield* [Node.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Node>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Node> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Node.decode(p)]
        }
      } else {
        yield* [Node.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Node {
    return {
      height: isSet(object.height) ? Number(object.height) : 0,
      size: isSet(object.size) ? Long.fromValue(object.size) : Long.UZERO,
      key: isSet(object.key) ? bytesFromBase64(object.key) : new Uint8Array(),
      valueRef: isSet(object.valueRef)
        ? BlockRef.fromJSON(object.valueRef)
        : undefined,
      valueRefBlob: isSet(object.valueRefBlob)
        ? Boolean(object.valueRefBlob)
        : false,
      leftChildRef: isSet(object.leftChildRef)
        ? BlockRef.fromJSON(object.leftChildRef)
        : undefined,
      rightChildRef: isSet(object.rightChildRef)
        ? BlockRef.fromJSON(object.rightChildRef)
        : undefined,
    }
  },

  toJSON(message: Node): unknown {
    const obj: any = {}
    message.height !== undefined && (obj.height = Math.round(message.height))
    message.size !== undefined &&
      (obj.size = (message.size || Long.UZERO).toString())
    message.key !== undefined &&
      (obj.key = base64FromBytes(
        message.key !== undefined ? message.key : new Uint8Array()
      ))
    message.valueRef !== undefined &&
      (obj.valueRef = message.valueRef
        ? BlockRef.toJSON(message.valueRef)
        : undefined)
    message.valueRefBlob !== undefined &&
      (obj.valueRefBlob = message.valueRefBlob)
    message.leftChildRef !== undefined &&
      (obj.leftChildRef = message.leftChildRef
        ? BlockRef.toJSON(message.leftChildRef)
        : undefined)
    message.rightChildRef !== undefined &&
      (obj.rightChildRef = message.rightChildRef
        ? BlockRef.toJSON(message.rightChildRef)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<Node>, I>>(base?: I): Node {
    return Node.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Node>, I>>(object: I): Node {
    const message = createBaseNode()
    message.height = object.height ?? 0
    message.size =
      object.size !== undefined && object.size !== null
        ? Long.fromValue(object.size)
        : Long.UZERO
    message.key = object.key ?? new Uint8Array()
    message.valueRef =
      object.valueRef !== undefined && object.valueRef !== null
        ? BlockRef.fromPartial(object.valueRef)
        : undefined
    message.valueRefBlob = object.valueRefBlob ?? false
    message.leftChildRef =
      object.leftChildRef !== undefined && object.leftChildRef !== null
        ? BlockRef.fromPartial(object.leftChildRef)
        : undefined
    message.rightChildRef =
      object.rightChildRef !== undefined && object.rightChildRef !== null
        ? BlockRef.fromPartial(object.rightChildRef)
        : undefined
    return message
  },
}

declare var self: any | undefined
declare var window: any | undefined
declare var global: any | undefined
var tsProtoGlobalThis: any = (() => {
  if (typeof globalThis !== 'undefined') {
    return globalThis
  }
  if (typeof self !== 'undefined') {
    return self
  }
  if (typeof window !== 'undefined') {
    return window
  }
  if (typeof global !== 'undefined') {
    return global
  }
  throw 'Unable to locate global object'
})()

function bytesFromBase64(b64: string): Uint8Array {
  if (tsProtoGlobalThis.Buffer) {
    return Uint8Array.from(tsProtoGlobalThis.Buffer.from(b64, 'base64'))
  } else {
    const bin = tsProtoGlobalThis.atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i)
    }
    return arr
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (tsProtoGlobalThis.Buffer) {
    return tsProtoGlobalThis.Buffer.from(arr).toString('base64')
  } else {
    const bin: string[] = []
    arr.forEach((byte) => {
      bin.push(String.fromCharCode(byte))
    })
    return tsProtoGlobalThis.btoa(bin.join(''))
  }
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
