/* eslint-disable */
import {
  Hash,
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from "@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "block";

/** BlockRef is a block content ID reference. */
export interface BlockRef {
  /** Hash is the hash of the object. */
  hash: Hash | undefined;
}

/** PutOpts are options that can be passed to PutBlock. */
export interface PutOpts {
  /**
   * HashType is the hash type to use.
   * If unset (0 value) will use default for the store.
   */
  hashType: HashType;
}

function createBaseBlockRef(): BlockRef {
  return { hash: undefined };
}

export const BlockRef = {
  encode(message: BlockRef, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.hash !== undefined) {
      Hash.encode(message.hash, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BlockRef {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBlockRef();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.hash = Hash.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<BlockRef, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<BlockRef | BlockRef[]> | Iterable<BlockRef | BlockRef[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BlockRef.encode(p).finish()];
        }
      } else {
        yield* [BlockRef.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BlockRef>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<BlockRef> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BlockRef.decode(p)];
        }
      } else {
        yield* [BlockRef.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): BlockRef {
    return { hash: isSet(object.hash) ? Hash.fromJSON(object.hash) : undefined };
  },

  toJSON(message: BlockRef): unknown {
    const obj: any = {};
    message.hash !== undefined && (obj.hash = message.hash ? Hash.toJSON(message.hash) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<BlockRef>, I>>(object: I): BlockRef {
    const message = createBaseBlockRef();
    message.hash = (object.hash !== undefined && object.hash !== null) ? Hash.fromPartial(object.hash) : undefined;
    return message;
  },
};

function createBasePutOpts(): PutOpts {
  return { hashType: 0 };
}

export const PutOpts = {
  encode(message: PutOpts, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.hashType !== 0) {
      writer.uint32(8).int32(message.hashType);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PutOpts {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePutOpts();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.hashType = reader.int32() as any;
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PutOpts, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PutOpts | PutOpts[]> | Iterable<PutOpts | PutOpts[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PutOpts.encode(p).finish()];
        }
      } else {
        yield* [PutOpts.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PutOpts>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PutOpts> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PutOpts.decode(p)];
        }
      } else {
        yield* [PutOpts.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PutOpts {
    return { hashType: isSet(object.hashType) ? hashTypeFromJSON(object.hashType) : 0 };
  },

  toJSON(message: PutOpts): unknown {
    const obj: any = {};
    message.hashType !== undefined && (obj.hashType = hashTypeToJSON(message.hashType));
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<PutOpts>, I>>(object: I): PutOpts {
    const message = createBasePutOpts();
    message.hashType = object.hashType ?? 0;
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends Long ? string | number | Long : T extends Array<infer U> ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string } ? { [K in keyof Omit<T, "$case">]?: DeepPartial<T[K]> } & { $case: T["$case"] }
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any;
  _m0.configure();
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
