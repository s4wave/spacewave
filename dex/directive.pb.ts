/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../block/block.pb.js'

export const protobufPackage = 'dex'

/** LookupBlockFromNetworkRequest requests that Data-Exchange (DEX) find data. */
export interface LookupBlockFromNetworkRequest {
  /** BucketId is the associated bucket ID with the lookup. */
  bucketId: string
  /** Ref is the reference. */
  ref: BlockRef | undefined
}

function createBaseLookupBlockFromNetworkRequest(): LookupBlockFromNetworkRequest {
  return { bucketId: '', ref: undefined }
}

export const LookupBlockFromNetworkRequest = {
  encode(
    message: LookupBlockFromNetworkRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.bucketId !== '') {
      writer.uint32(10).string(message.bucketId)
    }
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): LookupBlockFromNetworkRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseLookupBlockFromNetworkRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.bucketId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.ref = BlockRef.decode(reader, reader.uint32())
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
  // Transform<LookupBlockFromNetworkRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          LookupBlockFromNetworkRequest | LookupBlockFromNetworkRequest[]
        >
      | Iterable<
          LookupBlockFromNetworkRequest | LookupBlockFromNetworkRequest[]
        >
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LookupBlockFromNetworkRequest.encode(p).finish()]
        }
      } else {
        yield* [LookupBlockFromNetworkRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, LookupBlockFromNetworkRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<LookupBlockFromNetworkRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LookupBlockFromNetworkRequest.decode(p)]
        }
      } else {
        yield* [LookupBlockFromNetworkRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): LookupBlockFromNetworkRequest {
    return {
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : '',
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: LookupBlockFromNetworkRequest): unknown {
    const obj: any = {}
    message.bucketId !== undefined && (obj.bucketId = message.bucketId)
    message.ref !== undefined &&
      (obj.ref = message.ref ? BlockRef.toJSON(message.ref) : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<LookupBlockFromNetworkRequest>, I>>(
    base?: I
  ): LookupBlockFromNetworkRequest {
    return LookupBlockFromNetworkRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<LookupBlockFromNetworkRequest>, I>>(
    object: I
  ): LookupBlockFromNetworkRequest {
    const message = createBaseLookupBlockFromNetworkRequest()
    message.bucketId = object.bucketId ?? ''
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
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
