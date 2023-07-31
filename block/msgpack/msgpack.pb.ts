/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Blob } from '../blob/blob.pb.js'

export const protobufPackage = 'msgpack'

/**
 * MsgpackBlob is a block containing data packed with msgpack.
 *
 * API: BuildMsgpackBlob(object interface{}) -> pack into MsgpackBlob.
 * Decode: blob.UnmarshalMsgpack() -> object interface{}
 */
export interface MsgpackBlob {
  /**
   * Blob contains the encoded data in a blob.
   *
   * If small enough, the blob will be stored in-line.
   */
  blob: Blob | undefined
}

function createBaseMsgpackBlob(): MsgpackBlob {
  return { blob: undefined }
}

export const MsgpackBlob = {
  encode(
    message: MsgpackBlob,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.blob !== undefined) {
      Blob.encode(message.blob, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MsgpackBlob {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseMsgpackBlob()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.blob = Blob.decode(reader, reader.uint32())
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
  // Transform<MsgpackBlob, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<MsgpackBlob | MsgpackBlob[]>
      | Iterable<MsgpackBlob | MsgpackBlob[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MsgpackBlob.encode(p).finish()]
        }
      } else {
        yield* [MsgpackBlob.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MsgpackBlob>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<MsgpackBlob> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MsgpackBlob.decode(p)]
        }
      } else {
        yield* [MsgpackBlob.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): MsgpackBlob {
    return { blob: isSet(object.blob) ? Blob.fromJSON(object.blob) : undefined }
  },

  toJSON(message: MsgpackBlob): unknown {
    const obj: any = {}
    if (message.blob !== undefined) {
      obj.blob = Blob.toJSON(message.blob)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<MsgpackBlob>, I>>(base?: I): MsgpackBlob {
    return MsgpackBlob.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<MsgpackBlob>, I>>(
    object: I,
  ): MsgpackBlob {
    const message = createBaseMsgpackBlob()
    message.blob =
      object.blob !== undefined && object.blob !== null
        ? Blob.fromPartial(object.blob)
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
