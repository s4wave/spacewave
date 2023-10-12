/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'fibheap'

/** Entry is an entry in the heap. */
export interface Entry {
  /** Degree is the degree of the entry. */
  degree: number
  /** Marked indicates if the entry is marked. */
  marked: boolean
  /** Next is the key of the next entry. */
  next: Uint8Array
  /** Prev is the key of the previous entry. */
  prev: Uint8Array
  /** Child is the key of the child entry. */
  child: Uint8Array
  /** Parent is the key of the parent entry. */
  parent: Uint8Array
  /** Priority is the numerical priority of the entry. */
  priority: number
}

/** Root is the root object of the heap. */
export interface Root {
  /** Min is the key of the current minimum item. */
  min: Uint8Array
  /** MinPriority is the priority of the current minimum item. */
  minPriority: number
  /** Size is the current size of the heap. */
  size: number
}

function createBaseEntry(): Entry {
  return {
    degree: 0,
    marked: false,
    next: new Uint8Array(0),
    prev: new Uint8Array(0),
    child: new Uint8Array(0),
    parent: new Uint8Array(0),
    priority: 0,
  }
}

export const Entry = {
  encode(message: Entry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.degree !== 0) {
      writer.uint32(8).int32(message.degree)
    }
    if (message.marked === true) {
      writer.uint32(16).bool(message.marked)
    }
    if (message.next.length !== 0) {
      writer.uint32(26).bytes(message.next)
    }
    if (message.prev.length !== 0) {
      writer.uint32(34).bytes(message.prev)
    }
    if (message.child.length !== 0) {
      writer.uint32(42).bytes(message.child)
    }
    if (message.parent.length !== 0) {
      writer.uint32(50).bytes(message.parent)
    }
    if (message.priority !== 0) {
      writer.uint32(57).double(message.priority)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Entry {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.degree = reader.int32()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.marked = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.next = reader.bytes()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.prev = reader.bytes()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.child = reader.bytes()
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.parent = reader.bytes()
          continue
        case 7:
          if (tag !== 57) {
            break
          }

          message.priority = reader.double()
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
  // Transform<Entry, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Entry | Entry[]> | Iterable<Entry | Entry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Entry.encode(p).finish()]
        }
      } else {
        yield* [Entry.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Entry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Entry> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Entry.decode(p)]
        }
      } else {
        yield* [Entry.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Entry {
    return {
      degree: isSet(object.degree) ? globalThis.Number(object.degree) : 0,
      marked: isSet(object.marked) ? globalThis.Boolean(object.marked) : false,
      next: isSet(object.next)
        ? bytesFromBase64(object.next)
        : new Uint8Array(0),
      prev: isSet(object.prev)
        ? bytesFromBase64(object.prev)
        : new Uint8Array(0),
      child: isSet(object.child)
        ? bytesFromBase64(object.child)
        : new Uint8Array(0),
      parent: isSet(object.parent)
        ? bytesFromBase64(object.parent)
        : new Uint8Array(0),
      priority: isSet(object.priority) ? globalThis.Number(object.priority) : 0,
    }
  },

  toJSON(message: Entry): unknown {
    const obj: any = {}
    if (message.degree !== 0) {
      obj.degree = Math.round(message.degree)
    }
    if (message.marked === true) {
      obj.marked = message.marked
    }
    if (message.next.length !== 0) {
      obj.next = base64FromBytes(message.next)
    }
    if (message.prev.length !== 0) {
      obj.prev = base64FromBytes(message.prev)
    }
    if (message.child.length !== 0) {
      obj.child = base64FromBytes(message.child)
    }
    if (message.parent.length !== 0) {
      obj.parent = base64FromBytes(message.parent)
    }
    if (message.priority !== 0) {
      obj.priority = message.priority
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Entry>, I>>(base?: I): Entry {
    return Entry.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Entry>, I>>(object: I): Entry {
    const message = createBaseEntry()
    message.degree = object.degree ?? 0
    message.marked = object.marked ?? false
    message.next = object.next ?? new Uint8Array(0)
    message.prev = object.prev ?? new Uint8Array(0)
    message.child = object.child ?? new Uint8Array(0)
    message.parent = object.parent ?? new Uint8Array(0)
    message.priority = object.priority ?? 0
    return message
  },
}

function createBaseRoot(): Root {
  return { min: new Uint8Array(0), minPriority: 0, size: 0 }
}

export const Root = {
  encode(message: Root, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.min.length !== 0) {
      writer.uint32(10).bytes(message.min)
    }
    if (message.minPriority !== 0) {
      writer.uint32(17).double(message.minPriority)
    }
    if (message.size !== 0) {
      writer.uint32(24).uint32(message.size)
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

          message.min = reader.bytes()
          continue
        case 2:
          if (tag !== 17) {
            break
          }

          message.minPriority = reader.double()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.size = reader.uint32()
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
      min: isSet(object.min) ? bytesFromBase64(object.min) : new Uint8Array(0),
      minPriority: isSet(object.minPriority)
        ? globalThis.Number(object.minPriority)
        : 0,
      size: isSet(object.size) ? globalThis.Number(object.size) : 0,
    }
  },

  toJSON(message: Root): unknown {
    const obj: any = {}
    if (message.min.length !== 0) {
      obj.min = base64FromBytes(message.min)
    }
    if (message.minPriority !== 0) {
      obj.minPriority = message.minPriority
    }
    if (message.size !== 0) {
      obj.size = Math.round(message.size)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Root>, I>>(base?: I): Root {
    return Root.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Root>, I>>(object: I): Root {
    const message = createBaseRoot()
    message.min = object.min ?? new Uint8Array(0)
    message.minPriority = object.minPriority ?? 0
    message.size = object.size ?? 0
    return message
  },
}

function bytesFromBase64(b64: string): Uint8Array {
  if (globalThis.Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, 'base64'))
  } else {
    const bin = globalThis.atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i)
    }
    return arr
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (globalThis.Buffer) {
    return globalThis.Buffer.from(arr).toString('base64')
  } else {
    const bin: string[] = []
    arr.forEach((byte) => {
      bin.push(globalThis.String.fromCharCode(byte))
    })
    return globalThis.btoa(bin.join(''))
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
