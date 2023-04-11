/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'transform.lz4'

/** BlockSize is the list of available block sizes. */
export enum BlockSize {
  /** BlockSize_4MB - BlockSize_4MB is the default 4 megabyte block size. */
  BlockSize_4MB = 0,
  /** BlockSize_64KB - BlockSize_64KB is the 64 kilobyte block size. */
  BlockSize_64KB = 1,
  /** BlockSize_256KB - BlockSize_256KB is the 256 kilobyte block size. */
  BlockSize_256KB = 2,
  /** BlockSize_1MB - BlockSize_1MB is the 1 megabyte block size. */
  BlockSize_1MB = 3,
  UNRECOGNIZED = -1,
}

export function blockSizeFromJSON(object: any): BlockSize {
  switch (object) {
    case 0:
    case 'BlockSize_4MB':
      return BlockSize.BlockSize_4MB
    case 1:
    case 'BlockSize_64KB':
      return BlockSize.BlockSize_64KB
    case 2:
    case 'BlockSize_256KB':
      return BlockSize.BlockSize_256KB
    case 3:
    case 'BlockSize_1MB':
      return BlockSize.BlockSize_1MB
    case -1:
    case 'UNRECOGNIZED':
    default:
      return BlockSize.UNRECOGNIZED
  }
}

export function blockSizeToJSON(object: BlockSize): string {
  switch (object) {
    case BlockSize.BlockSize_4MB:
      return 'BlockSize_4MB'
    case BlockSize.BlockSize_64KB:
      return 'BlockSize_64KB'
    case BlockSize.BlockSize_256KB:
      return 'BlockSize_256KB'
    case BlockSize.BlockSize_1MB:
      return 'BlockSize_1MB'
    case BlockSize.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Config configures the lz4 compression transform. */
export interface Config {
  /**
   * BlockSize defines the maximum size of compressed blocks.
   * Defaults to 4MB.
   */
  blockSize: BlockSize
  /** BlockChecksum enables the block checksum feature. */
  blockChecksum: boolean
  /**
   * DisableChecksum disables all blocks or content checksum.
   * Default=false.
   */
  disableChecksum: boolean
  /**
   * CompressionLevel sets the compression level from 0-9.
   * The default value (0) is "FAST."
   */
  compressionLevel: number
}

function createBaseConfig(): Config {
  return {
    blockSize: 0,
    blockChecksum: false,
    disableChecksum: false,
    compressionLevel: 0,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.blockSize !== 0) {
      writer.uint32(8).int32(message.blockSize)
    }
    if (message.blockChecksum === true) {
      writer.uint32(16).bool(message.blockChecksum)
    }
    if (message.disableChecksum === true) {
      writer.uint32(24).bool(message.disableChecksum)
    }
    if (message.compressionLevel !== 0) {
      writer.uint32(32).uint32(message.compressionLevel)
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
          if (tag != 8) {
            break
          }

          message.blockSize = reader.int32() as any
          continue
        case 2:
          if (tag != 16) {
            break
          }

          message.blockChecksum = reader.bool()
          continue
        case 3:
          if (tag != 24) {
            break
          }

          message.disableChecksum = reader.bool()
          continue
        case 4:
          if (tag != 32) {
            break
          }

          message.compressionLevel = reader.uint32()
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
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>
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
      | Iterable<Uint8Array | Uint8Array[]>
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
    return {
      blockSize: isSet(object.blockSize)
        ? blockSizeFromJSON(object.blockSize)
        : 0,
      blockChecksum: isSet(object.blockChecksum)
        ? Boolean(object.blockChecksum)
        : false,
      disableChecksum: isSet(object.disableChecksum)
        ? Boolean(object.disableChecksum)
        : false,
      compressionLevel: isSet(object.compressionLevel)
        ? Number(object.compressionLevel)
        : 0,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.blockSize !== undefined &&
      (obj.blockSize = blockSizeToJSON(message.blockSize))
    message.blockChecksum !== undefined &&
      (obj.blockChecksum = message.blockChecksum)
    message.disableChecksum !== undefined &&
      (obj.disableChecksum = message.disableChecksum)
    message.compressionLevel !== undefined &&
      (obj.compressionLevel = Math.round(message.compressionLevel))
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.blockSize = object.blockSize ?? 0
    message.blockChecksum = object.blockChecksum ?? false
    message.disableChecksum = object.disableChecksum ?? false
    message.compressionLevel = object.compressionLevel ?? 0
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
