/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'transform.chksum'

/** ChksumType is the checksum type enum. */
export enum ChksumType {
  /** ChksumType_UNKNOWN - ChksumType_UNKNOWN defaults to CRC64. */
  ChksumType_UNKNOWN = 0,
  /** ChksumType_CRC32 - ChksumType_CRC32 performs a appended 4 byte crc32 against the data. */
  ChksumType_CRC32 = 1,
  UNRECOGNIZED = -1,
}

export function chksumTypeFromJSON(object: any): ChksumType {
  switch (object) {
    case 0:
    case 'ChksumType_UNKNOWN':
      return ChksumType.ChksumType_UNKNOWN
    case 1:
    case 'ChksumType_CRC32':
      return ChksumType.ChksumType_CRC32
    case -1:
    case 'UNRECOGNIZED':
    default:
      return ChksumType.UNRECOGNIZED
  }
}

export function chksumTypeToJSON(object: ChksumType): string {
  switch (object) {
    case ChksumType.ChksumType_UNKNOWN:
      return 'ChksumType_UNKNOWN'
    case ChksumType.ChksumType_CRC32:
      return 'ChksumType_CRC32'
    case ChksumType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Config configures the snappy transform. */
export interface Config {
  /** ChksumType is the type of chksum to use. */
  chksumType: ChksumType
}

function createBaseConfig(): Config {
  return { chksumType: 0 }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.chksumType !== 0) {
      writer.uint32(8).int32(message.chksumType)
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
          if (tag !== 8) {
            break
          }

          message.chksumType = reader.int32() as any
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
      chksumType: isSet(object.chksumType)
        ? chksumTypeFromJSON(object.chksumType)
        : 0,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.chksumType !== undefined &&
      (obj.chksumType = chksumTypeToJSON(message.chksumType))
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.chksumType = object.chksumType ?? 0
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
