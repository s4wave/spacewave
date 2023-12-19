/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'quad'

/** Quad implements a graph quad backed by a protobuf. */
export interface Quad {
  /** Subject is the subject field. */
  subject: string
  /** Predicate is the object field. */
  predicate: string
  /** Obj is the object field. */
  obj: string
  /** Label is the label field. */
  label: string
}

function createBaseQuad(): Quad {
  return { subject: '', predicate: '', obj: '', label: '' }
}

export const Quad = {
  encode(message: Quad, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.subject !== '') {
      writer.uint32(10).string(message.subject)
    }
    if (message.predicate !== '') {
      writer.uint32(18).string(message.predicate)
    }
    if (message.obj !== '') {
      writer.uint32(26).string(message.obj)
    }
    if (message.label !== '') {
      writer.uint32(34).string(message.label)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Quad {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseQuad()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.subject = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.predicate = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.obj = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.label = reader.string()
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
  // Transform<Quad, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Quad | Quad[]> | Iterable<Quad | Quad[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Quad.encode(p).finish()]
        }
      } else {
        yield* [Quad.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Quad>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Quad> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Quad.decode(p)]
        }
      } else {
        yield* [Quad.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Quad {
    return {
      subject: isSet(object.subject) ? globalThis.String(object.subject) : '',
      predicate: isSet(object.predicate)
        ? globalThis.String(object.predicate)
        : '',
      obj: isSet(object.obj) ? globalThis.String(object.obj) : '',
      label: isSet(object.label) ? globalThis.String(object.label) : '',
    }
  },

  toJSON(message: Quad): unknown {
    const obj: any = {}
    if (message.subject !== '') {
      obj.subject = message.subject
    }
    if (message.predicate !== '') {
      obj.predicate = message.predicate
    }
    if (message.obj !== '') {
      obj.obj = message.obj
    }
    if (message.label !== '') {
      obj.label = message.label
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Quad>, I>>(base?: I): Quad {
    return Quad.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Quad>, I>>(object: I): Quad {
    const message = createBaseQuad()
    message.subject = object.subject ?? ''
    message.predicate = object.predicate ?? ''
    message.obj = object.obj ?? ''
    message.label = object.label ?? ''
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
