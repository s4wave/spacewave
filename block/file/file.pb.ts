/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Blob } from '../blob/blob.pb.js'
import { BlockRef } from '../block.pb.js'

export const protobufPackage = 'file'

/**
 * File defines a pattern for storing a block-addressed file.
 * Changes are transactional and deterministic.
 * File write ranges and write mode are considered.
 * Concurrent O_APPEND is implemented.
 * Blobs are addressed as they are added, and periodically compacted.
 * Some operations partially overwrite old blobs without rewriting them.
 * The File object contains metadata and might contain links to sub-objects.
 * Rabin fingerprinting is used to select deterministic chunk sizes.
 */
export interface File {
  /** TotalSize is the total size of the file. */
  totalSize: Long
  /**
   * RootBlob, if set, contains the entire file in a blob.
   * Used when there is a single Range of data starting at index 0.
   * If unset and len(ranges) == 0, the file is empty (all zeros).
   */
  rootBlob: Blob | undefined
  /** RangeNonce is the next range nonce id to use. */
  rangeNonce: Long
  /**
   * Ranges contains file data ranges.
   * Files are sparse when created.
   * Ranges may overlap.
   */
  ranges: Range[]
}

/**
 * Range contains a chunk of a file.
 * Ranges are sorted by start, then nonce (ascending).
 */
export interface Range {
  /**
   * Nonce is the incrementing nonce of the range.
   * Ranges with a higher nonce overwrite lower nonce.
   */
  nonce: Long
  /** Start contains the starting index of the range. */
  start: Long
  /**
   * Length contains the len of data in the range.
   * Start + length = end index + 1.
   */
  length: Long
  /**
   * Ref contains the blob ref.
   * If the ref is empty, the range represents a hole (zeros).
   */
  ref: BlockRef | undefined
}

function createBaseFile(): File {
  return {
    totalSize: Long.UZERO,
    rootBlob: undefined,
    rangeNonce: Long.UZERO,
    ranges: [],
  }
}

export const File = {
  encode(message: File, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (!message.totalSize.equals(Long.UZERO)) {
      writer.uint32(8).uint64(message.totalSize)
    }
    if (message.rootBlob !== undefined) {
      Blob.encode(message.rootBlob, writer.uint32(18).fork()).ldelim()
    }
    if (!message.rangeNonce.equals(Long.UZERO)) {
      writer.uint32(24).uint64(message.rangeNonce)
    }
    for (const v of message.ranges) {
      Range.encode(v!, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): File {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFile()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.totalSize = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.rootBlob = Blob.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.rangeNonce = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.ranges.push(Range.decode(reader, reader.uint32()))
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
  // Transform<File, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<File | File[]> | Iterable<File | File[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [File.encode(p).finish()]
        }
      } else {
        yield* [File.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, File>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<File> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [File.decode(p)]
        }
      } else {
        yield* [File.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): File {
    return {
      totalSize: isSet(object.totalSize)
        ? Long.fromValue(object.totalSize)
        : Long.UZERO,
      rootBlob: isSet(object.rootBlob)
        ? Blob.fromJSON(object.rootBlob)
        : undefined,
      rangeNonce: isSet(object.rangeNonce)
        ? Long.fromValue(object.rangeNonce)
        : Long.UZERO,
      ranges: globalThis.Array.isArray(object?.ranges)
        ? object.ranges.map((e: any) => Range.fromJSON(e))
        : [],
    }
  },

  toJSON(message: File): unknown {
    const obj: any = {}
    if (!message.totalSize.equals(Long.UZERO)) {
      obj.totalSize = (message.totalSize || Long.UZERO).toString()
    }
    if (message.rootBlob !== undefined) {
      obj.rootBlob = Blob.toJSON(message.rootBlob)
    }
    if (!message.rangeNonce.equals(Long.UZERO)) {
      obj.rangeNonce = (message.rangeNonce || Long.UZERO).toString()
    }
    if (message.ranges?.length) {
      obj.ranges = message.ranges.map((e) => Range.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<File>, I>>(base?: I): File {
    return File.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<File>, I>>(object: I): File {
    const message = createBaseFile()
    message.totalSize =
      object.totalSize !== undefined && object.totalSize !== null
        ? Long.fromValue(object.totalSize)
        : Long.UZERO
    message.rootBlob =
      object.rootBlob !== undefined && object.rootBlob !== null
        ? Blob.fromPartial(object.rootBlob)
        : undefined
    message.rangeNonce =
      object.rangeNonce !== undefined && object.rangeNonce !== null
        ? Long.fromValue(object.rangeNonce)
        : Long.UZERO
    message.ranges = object.ranges?.map((e) => Range.fromPartial(e)) || []
    return message
  },
}

function createBaseRange(): Range {
  return {
    nonce: Long.UZERO,
    start: Long.UZERO,
    length: Long.UZERO,
    ref: undefined,
  }
}

export const Range = {
  encode(message: Range, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (!message.nonce.equals(Long.UZERO)) {
      writer.uint32(8).uint64(message.nonce)
    }
    if (!message.start.equals(Long.UZERO)) {
      writer.uint32(16).uint64(message.start)
    }
    if (!message.length.equals(Long.UZERO)) {
      writer.uint32(24).uint64(message.length)
    }
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Range {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRange()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.nonce = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.start = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.length = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 34) {
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
  // Transform<Range, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Range | Range[]> | Iterable<Range | Range[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Range.encode(p).finish()]
        }
      } else {
        yield* [Range.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Range>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Range> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Range.decode(p)]
        }
      } else {
        yield* [Range.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Range {
    return {
      nonce: isSet(object.nonce) ? Long.fromValue(object.nonce) : Long.UZERO,
      start: isSet(object.start) ? Long.fromValue(object.start) : Long.UZERO,
      length: isSet(object.length) ? Long.fromValue(object.length) : Long.UZERO,
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: Range): unknown {
    const obj: any = {}
    if (!message.nonce.equals(Long.UZERO)) {
      obj.nonce = (message.nonce || Long.UZERO).toString()
    }
    if (!message.start.equals(Long.UZERO)) {
      obj.start = (message.start || Long.UZERO).toString()
    }
    if (!message.length.equals(Long.UZERO)) {
      obj.length = (message.length || Long.UZERO).toString()
    }
    if (message.ref !== undefined) {
      obj.ref = BlockRef.toJSON(message.ref)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Range>, I>>(base?: I): Range {
    return Range.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Range>, I>>(object: I): Range {
    const message = createBaseRange()
    message.nonce =
      object.nonce !== undefined && object.nonce !== null
        ? Long.fromValue(object.nonce)
        : Long.UZERO
    message.start =
      object.start !== undefined && object.start !== null
        ? Long.fromValue(object.start)
        : Long.UZERO
    message.length =
      object.length !== undefined && object.length !== null
        ? Long.fromValue(object.length)
        : Long.UZERO
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
