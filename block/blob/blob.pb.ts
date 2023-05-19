/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../block.pb.js'

export const protobufPackage = 'blob'

/** BlobType defines the types of blobs. */
export enum BlobType {
  /**
   * BlobType_RAW - BlobType_RAW indicates the blob contains the data inline.
   * Default value: readers should check the value is actually zero.
   * This value being zero will save some encoding space.
   */
  BlobType_RAW = 0,
  /**
   * BlobType_CHUNKED - BlobType_CHUNKED indicates the chunked blob format.
   * Stores data in sequential chunks of data, selected in a deterministic way.
   */
  BlobType_CHUNKED = 1,
  UNRECOGNIZED = -1,
}

export function blobTypeFromJSON(object: any): BlobType {
  switch (object) {
    case 0:
    case 'BlobType_RAW':
      return BlobType.BlobType_RAW
    case 1:
    case 'BlobType_CHUNKED':
      return BlobType.BlobType_CHUNKED
    case -1:
    case 'UNRECOGNIZED':
    default:
      return BlobType.UNRECOGNIZED
  }
}

export function blobTypeToJSON(object: BlobType): string {
  switch (object) {
    case BlobType.BlobType_RAW:
      return 'BlobType_RAW'
    case BlobType.BlobType_CHUNKED:
      return 'BlobType_CHUNKED'
    case BlobType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** ChunkerType is the set of known chunker types. */
export enum ChunkerType {
  /** ChunkerType_DEFAULT - ChunkerType_DEFAULT builds/appends using the default chunker (RABIN). */
  ChunkerType_DEFAULT = 0,
  /** ChunkerType_RABIN - ChunkerType_RABIN uses rabin fingerprinting to chunk. */
  ChunkerType_RABIN = 1,
  UNRECOGNIZED = -1,
}

export function chunkerTypeFromJSON(object: any): ChunkerType {
  switch (object) {
    case 0:
    case 'ChunkerType_DEFAULT':
      return ChunkerType.ChunkerType_DEFAULT
    case 1:
    case 'ChunkerType_RABIN':
      return ChunkerType.ChunkerType_RABIN
    case -1:
    case 'UNRECOGNIZED':
    default:
      return ChunkerType.UNRECOGNIZED
  }
}

export function chunkerTypeToJSON(object: ChunkerType): string {
  switch (object) {
    case ChunkerType.ChunkerType_DEFAULT:
      return 'ChunkerType_DEFAULT'
    case ChunkerType.ChunkerType_RABIN:
      return 'ChunkerType_RABIN'
    case ChunkerType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * Blob defines multiple patterns for storing large blobs of data.
 * All behaviors are deterministic with change validation routines.
 * The Blob object contains metadata and might contain links to sub-objects.
 */
export interface Blob {
  /** BlobType is the blob type. */
  blobType: BlobType
  /** TotalSize is the total size of the blob. */
  totalSize: Long
  /**
   * RawData contains in-line data for the raw blob type.
   * index=0 size=total_size if BlobType_RAW
   */
  rawData: Uint8Array
  /** ChunkIndex contains the information for CHUNKED type. */
  chunkIndex: ChunkIndex | undefined
}

/** BuildBlobOpts are options to control the BuildBlob process. */
export interface BuildBlobOpts {
  /**
   * RawHighWaterMark is the limit for a raw block size.
   * Defaults to 512KB if unset.
   */
  rawHighWaterMark: Long
  /** ChunkerArgs configures the chunker to use. */
  chunkerArgs: ChunkerArgs | undefined
}

/** ChunkIndex is the root of the chunked blob type. */
export interface ChunkIndex {
  /**
   * Chunks contains the in-line list of chunks.
   * Sequential.
   */
  chunks: Chunk[]
  /** ChunkerArgs are optional arguments for the chunker. */
  chunkerArgs: ChunkerArgs | undefined
}

/** ChunkerArgs configures the chunking algorithm. */
export interface ChunkerArgs {
  /**
   * ChunkerType is the chunking algorithm used.
   * Defaults to ChunkerType_RABIN if not set.
   */
  chunkerType: ChunkerType
  /**
   * RabinArgs are arguments for the rabin chunker.
   * ChunkerType_RABIN
   */
  rabinArgs: RabinArgs | undefined
}

/**
 * RabinArgs are arguments for the rabin chunker.
 *
 * The default polynomial is 0x2df7f4e3b27061
 */
export interface RabinArgs {
  /**
   * Rabin polynomial.
   * Optional.
   */
  pol: Long
  /**
   * RandomPol enables randomizing pol instead of using the default.
   * This is not recommended.
   * If pol != 0 this field is ignored.
   */
  randomPol: boolean
  /**
   * ChunkingMinSize is the minimum size for a chunk.
   * Defaults to 256KB.
   */
  chunkingMinSize: Long
  /**
   * ChunkingMaxSize is the maxmium size for a chunk.
   * Defaults to ~786KB (786432 bytes).
   */
  chunkingMaxSize: Long
}

/** Chunk contains in-line information about a data chunk. */
export interface Chunk {
  /**
   * DataRef is the reference to the data.
   * If empty, indicates a range of zeros.
   */
  dataRef: BlockRef | undefined
  /** Size is the size of the chunk. */
  size: Long
  /**
   * Start is the start position of the chunk.
   * Must be equal to the sum of all previous chunks sizes.
   */
  start: Long
}

function createBaseBlob(): Blob {
  return {
    blobType: 0,
    totalSize: Long.UZERO,
    rawData: new Uint8Array(),
    chunkIndex: undefined,
  }
}

export const Blob = {
  encode(message: Blob, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.blobType !== 0) {
      writer.uint32(8).int32(message.blobType)
    }
    if (!message.totalSize.isZero()) {
      writer.uint32(16).uint64(message.totalSize)
    }
    if (message.rawData.length !== 0) {
      writer.uint32(26).bytes(message.rawData)
    }
    if (message.chunkIndex !== undefined) {
      ChunkIndex.encode(message.chunkIndex, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Blob {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBlob()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.blobType = reader.int32() as any
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.totalSize = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.rawData = reader.bytes()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.chunkIndex = ChunkIndex.decode(reader, reader.uint32())
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
  // Transform<Blob, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Blob | Blob[]> | Iterable<Blob | Blob[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Blob.encode(p).finish()]
        }
      } else {
        yield* [Blob.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Blob>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Blob> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Blob.decode(p)]
        }
      } else {
        yield* [Blob.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Blob {
    return {
      blobType: isSet(object.blobType) ? blobTypeFromJSON(object.blobType) : 0,
      totalSize: isSet(object.totalSize)
        ? Long.fromValue(object.totalSize)
        : Long.UZERO,
      rawData: isSet(object.rawData)
        ? bytesFromBase64(object.rawData)
        : new Uint8Array(),
      chunkIndex: isSet(object.chunkIndex)
        ? ChunkIndex.fromJSON(object.chunkIndex)
        : undefined,
    }
  },

  toJSON(message: Blob): unknown {
    const obj: any = {}
    message.blobType !== undefined &&
      (obj.blobType = blobTypeToJSON(message.blobType))
    message.totalSize !== undefined &&
      (obj.totalSize = (message.totalSize || Long.UZERO).toString())
    message.rawData !== undefined &&
      (obj.rawData = base64FromBytes(
        message.rawData !== undefined ? message.rawData : new Uint8Array()
      ))
    message.chunkIndex !== undefined &&
      (obj.chunkIndex = message.chunkIndex
        ? ChunkIndex.toJSON(message.chunkIndex)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<Blob>, I>>(base?: I): Blob {
    return Blob.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Blob>, I>>(object: I): Blob {
    const message = createBaseBlob()
    message.blobType = object.blobType ?? 0
    message.totalSize =
      object.totalSize !== undefined && object.totalSize !== null
        ? Long.fromValue(object.totalSize)
        : Long.UZERO
    message.rawData = object.rawData ?? new Uint8Array()
    message.chunkIndex =
      object.chunkIndex !== undefined && object.chunkIndex !== null
        ? ChunkIndex.fromPartial(object.chunkIndex)
        : undefined
    return message
  },
}

function createBaseBuildBlobOpts(): BuildBlobOpts {
  return { rawHighWaterMark: Long.UZERO, chunkerArgs: undefined }
}

export const BuildBlobOpts = {
  encode(
    message: BuildBlobOpts,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (!message.rawHighWaterMark.isZero()) {
      writer.uint32(8).uint64(message.rawHighWaterMark)
    }
    if (message.chunkerArgs !== undefined) {
      ChunkerArgs.encode(message.chunkerArgs, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildBlobOpts {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseBuildBlobOpts()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.rawHighWaterMark = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.chunkerArgs = ChunkerArgs.decode(reader, reader.uint32())
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
  // Transform<BuildBlobOpts, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<BuildBlobOpts | BuildBlobOpts[]>
      | Iterable<BuildBlobOpts | BuildBlobOpts[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BuildBlobOpts.encode(p).finish()]
        }
      } else {
        yield* [BuildBlobOpts.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, BuildBlobOpts>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<BuildBlobOpts> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [BuildBlobOpts.decode(p)]
        }
      } else {
        yield* [BuildBlobOpts.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): BuildBlobOpts {
    return {
      rawHighWaterMark: isSet(object.rawHighWaterMark)
        ? Long.fromValue(object.rawHighWaterMark)
        : Long.UZERO,
      chunkerArgs: isSet(object.chunkerArgs)
        ? ChunkerArgs.fromJSON(object.chunkerArgs)
        : undefined,
    }
  },

  toJSON(message: BuildBlobOpts): unknown {
    const obj: any = {}
    message.rawHighWaterMark !== undefined &&
      (obj.rawHighWaterMark = (
        message.rawHighWaterMark || Long.UZERO
      ).toString())
    message.chunkerArgs !== undefined &&
      (obj.chunkerArgs = message.chunkerArgs
        ? ChunkerArgs.toJSON(message.chunkerArgs)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<BuildBlobOpts>, I>>(
    base?: I
  ): BuildBlobOpts {
    return BuildBlobOpts.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<BuildBlobOpts>, I>>(
    object: I
  ): BuildBlobOpts {
    const message = createBaseBuildBlobOpts()
    message.rawHighWaterMark =
      object.rawHighWaterMark !== undefined && object.rawHighWaterMark !== null
        ? Long.fromValue(object.rawHighWaterMark)
        : Long.UZERO
    message.chunkerArgs =
      object.chunkerArgs !== undefined && object.chunkerArgs !== null
        ? ChunkerArgs.fromPartial(object.chunkerArgs)
        : undefined
    return message
  },
}

function createBaseChunkIndex(): ChunkIndex {
  return { chunks: [], chunkerArgs: undefined }
}

export const ChunkIndex = {
  encode(
    message: ChunkIndex,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.chunks) {
      Chunk.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    if (message.chunkerArgs !== undefined) {
      ChunkerArgs.encode(message.chunkerArgs, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ChunkIndex {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseChunkIndex()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.chunks.push(Chunk.decode(reader, reader.uint32()))
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.chunkerArgs = ChunkerArgs.decode(reader, reader.uint32())
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
  // Transform<ChunkIndex, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ChunkIndex | ChunkIndex[]>
      | Iterable<ChunkIndex | ChunkIndex[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ChunkIndex.encode(p).finish()]
        }
      } else {
        yield* [ChunkIndex.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ChunkIndex>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ChunkIndex> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ChunkIndex.decode(p)]
        }
      } else {
        yield* [ChunkIndex.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ChunkIndex {
    return {
      chunks: Array.isArray(object?.chunks)
        ? object.chunks.map((e: any) => Chunk.fromJSON(e))
        : [],
      chunkerArgs: isSet(object.chunkerArgs)
        ? ChunkerArgs.fromJSON(object.chunkerArgs)
        : undefined,
    }
  },

  toJSON(message: ChunkIndex): unknown {
    const obj: any = {}
    if (message.chunks) {
      obj.chunks = message.chunks.map((e) => (e ? Chunk.toJSON(e) : undefined))
    } else {
      obj.chunks = []
    }
    message.chunkerArgs !== undefined &&
      (obj.chunkerArgs = message.chunkerArgs
        ? ChunkerArgs.toJSON(message.chunkerArgs)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<ChunkIndex>, I>>(base?: I): ChunkIndex {
    return ChunkIndex.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ChunkIndex>, I>>(
    object: I
  ): ChunkIndex {
    const message = createBaseChunkIndex()
    message.chunks = object.chunks?.map((e) => Chunk.fromPartial(e)) || []
    message.chunkerArgs =
      object.chunkerArgs !== undefined && object.chunkerArgs !== null
        ? ChunkerArgs.fromPartial(object.chunkerArgs)
        : undefined
    return message
  },
}

function createBaseChunkerArgs(): ChunkerArgs {
  return { chunkerType: 0, rabinArgs: undefined }
}

export const ChunkerArgs = {
  encode(
    message: ChunkerArgs,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.chunkerType !== 0) {
      writer.uint32(8).int32(message.chunkerType)
    }
    if (message.rabinArgs !== undefined) {
      RabinArgs.encode(message.rabinArgs, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ChunkerArgs {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseChunkerArgs()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.chunkerType = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.rabinArgs = RabinArgs.decode(reader, reader.uint32())
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
  // Transform<ChunkerArgs, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ChunkerArgs | ChunkerArgs[]>
      | Iterable<ChunkerArgs | ChunkerArgs[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ChunkerArgs.encode(p).finish()]
        }
      } else {
        yield* [ChunkerArgs.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ChunkerArgs>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ChunkerArgs> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ChunkerArgs.decode(p)]
        }
      } else {
        yield* [ChunkerArgs.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ChunkerArgs {
    return {
      chunkerType: isSet(object.chunkerType)
        ? chunkerTypeFromJSON(object.chunkerType)
        : 0,
      rabinArgs: isSet(object.rabinArgs)
        ? RabinArgs.fromJSON(object.rabinArgs)
        : undefined,
    }
  },

  toJSON(message: ChunkerArgs): unknown {
    const obj: any = {}
    message.chunkerType !== undefined &&
      (obj.chunkerType = chunkerTypeToJSON(message.chunkerType))
    message.rabinArgs !== undefined &&
      (obj.rabinArgs = message.rabinArgs
        ? RabinArgs.toJSON(message.rabinArgs)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<ChunkerArgs>, I>>(base?: I): ChunkerArgs {
    return ChunkerArgs.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ChunkerArgs>, I>>(
    object: I
  ): ChunkerArgs {
    const message = createBaseChunkerArgs()
    message.chunkerType = object.chunkerType ?? 0
    message.rabinArgs =
      object.rabinArgs !== undefined && object.rabinArgs !== null
        ? RabinArgs.fromPartial(object.rabinArgs)
        : undefined
    return message
  },
}

function createBaseRabinArgs(): RabinArgs {
  return {
    pol: Long.UZERO,
    randomPol: false,
    chunkingMinSize: Long.UZERO,
    chunkingMaxSize: Long.UZERO,
  }
}

export const RabinArgs = {
  encode(
    message: RabinArgs,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (!message.pol.isZero()) {
      writer.uint32(8).uint64(message.pol)
    }
    if (message.randomPol === true) {
      writer.uint32(32).bool(message.randomPol)
    }
    if (!message.chunkingMinSize.isZero()) {
      writer.uint32(16).uint64(message.chunkingMinSize)
    }
    if (!message.chunkingMaxSize.isZero()) {
      writer.uint32(24).uint64(message.chunkingMaxSize)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RabinArgs {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRabinArgs()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.pol = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.randomPol = reader.bool()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.chunkingMinSize = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.chunkingMaxSize = reader.uint64() as Long
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
  // Transform<RabinArgs, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RabinArgs | RabinArgs[]>
      | Iterable<RabinArgs | RabinArgs[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RabinArgs.encode(p).finish()]
        }
      } else {
        yield* [RabinArgs.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RabinArgs>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<RabinArgs> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RabinArgs.decode(p)]
        }
      } else {
        yield* [RabinArgs.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RabinArgs {
    return {
      pol: isSet(object.pol) ? Long.fromValue(object.pol) : Long.UZERO,
      randomPol: isSet(object.randomPol) ? Boolean(object.randomPol) : false,
      chunkingMinSize: isSet(object.chunkingMinSize)
        ? Long.fromValue(object.chunkingMinSize)
        : Long.UZERO,
      chunkingMaxSize: isSet(object.chunkingMaxSize)
        ? Long.fromValue(object.chunkingMaxSize)
        : Long.UZERO,
    }
  },

  toJSON(message: RabinArgs): unknown {
    const obj: any = {}
    message.pol !== undefined &&
      (obj.pol = (message.pol || Long.UZERO).toString())
    message.randomPol !== undefined && (obj.randomPol = message.randomPol)
    message.chunkingMinSize !== undefined &&
      (obj.chunkingMinSize = (message.chunkingMinSize || Long.UZERO).toString())
    message.chunkingMaxSize !== undefined &&
      (obj.chunkingMaxSize = (message.chunkingMaxSize || Long.UZERO).toString())
    return obj
  },

  create<I extends Exact<DeepPartial<RabinArgs>, I>>(base?: I): RabinArgs {
    return RabinArgs.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<RabinArgs>, I>>(
    object: I
  ): RabinArgs {
    const message = createBaseRabinArgs()
    message.pol =
      object.pol !== undefined && object.pol !== null
        ? Long.fromValue(object.pol)
        : Long.UZERO
    message.randomPol = object.randomPol ?? false
    message.chunkingMinSize =
      object.chunkingMinSize !== undefined && object.chunkingMinSize !== null
        ? Long.fromValue(object.chunkingMinSize)
        : Long.UZERO
    message.chunkingMaxSize =
      object.chunkingMaxSize !== undefined && object.chunkingMaxSize !== null
        ? Long.fromValue(object.chunkingMaxSize)
        : Long.UZERO
    return message
  },
}

function createBaseChunk(): Chunk {
  return { dataRef: undefined, size: Long.UZERO, start: Long.UZERO }
}

export const Chunk = {
  encode(message: Chunk, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.dataRef !== undefined) {
      BlockRef.encode(message.dataRef, writer.uint32(10).fork()).ldelim()
    }
    if (!message.size.isZero()) {
      writer.uint32(16).uint64(message.size)
    }
    if (!message.start.isZero()) {
      writer.uint32(24).uint64(message.start)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Chunk {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseChunk()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.dataRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.size = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.start = reader.uint64() as Long
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
  // Transform<Chunk, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Chunk | Chunk[]> | Iterable<Chunk | Chunk[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Chunk.encode(p).finish()]
        }
      } else {
        yield* [Chunk.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Chunk>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Chunk> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Chunk.decode(p)]
        }
      } else {
        yield* [Chunk.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Chunk {
    return {
      dataRef: isSet(object.dataRef)
        ? BlockRef.fromJSON(object.dataRef)
        : undefined,
      size: isSet(object.size) ? Long.fromValue(object.size) : Long.UZERO,
      start: isSet(object.start) ? Long.fromValue(object.start) : Long.UZERO,
    }
  },

  toJSON(message: Chunk): unknown {
    const obj: any = {}
    message.dataRef !== undefined &&
      (obj.dataRef = message.dataRef
        ? BlockRef.toJSON(message.dataRef)
        : undefined)
    message.size !== undefined &&
      (obj.size = (message.size || Long.UZERO).toString())
    message.start !== undefined &&
      (obj.start = (message.start || Long.UZERO).toString())
    return obj
  },

  create<I extends Exact<DeepPartial<Chunk>, I>>(base?: I): Chunk {
    return Chunk.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Chunk>, I>>(object: I): Chunk {
    const message = createBaseChunk()
    message.dataRef =
      object.dataRef !== undefined && object.dataRef !== null
        ? BlockRef.fromPartial(object.dataRef)
        : undefined
    message.size =
      object.size !== undefined && object.size !== null
        ? Long.fromValue(object.size)
        : Long.UZERO
    message.start =
      object.start !== undefined && object.start !== null
        ? Long.fromValue(object.start)
        : Long.UZERO
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
