/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'store.kvtx.ristretto'

/** Config configures the ristretto cache. */
export interface Config {
  /**
   * NumCounters is the number of 4-bit access counters to keep for admission and eviction.
   *
   * This should be 10x the number of items you expect to keep in the cache when full.
   *
   * For example, if you expect each item to have a cost of 1 and MaxCost is
   * 100, set NumCounters to 1,000. Or, if you use variable cost values but
   * expect the cache to hold around 10,000 items when full, set NumCounters to
   * 100,000. The important thing is the number of unique items in the full
   * cache, not necessarily the MaxCost value.
   *
   * If unset (zero) defaults to 100,000.
   */
  numCounters: Long
  /**
   * MaxCost is the maximum storage size in bytes of the cache.
   *
   * For example, if MaxCost is 1,000,000 (1MB) and the cache is full with 1,000
   * 1KB items, a new item (that's accepted) would cause 5 1KB items to be
   * evicted.
   *
   * If unset (zero) defaults to 1GB (1e9).
   */
  maxCost: Long
  /**
   * BufferItems is the size of the Get buffers.
   *
   * If for some reason you see Get performance decreasing with lots of
   * contention, try increasing this value in increments of 64. This is a
   * fine-tuning mechanism and you probably won't have to touch this.
   *
   * If unset (zero) defaults to 64.
   */
  bufferItems: number
  /**
   * TtlDur is the time to live duration.
   * If empty or zero has no ttl.
   * Example: 1m
   */
  ttlDur: string
}

function createBaseConfig(): Config {
  return {
    numCounters: Long.UZERO,
    maxCost: Long.UZERO,
    bufferItems: 0,
    ttlDur: '',
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.numCounters.equals(Long.UZERO)) {
      writer.uint32(8).uint64(message.numCounters)
    }
    if (!message.maxCost.equals(Long.UZERO)) {
      writer.uint32(16).uint64(message.maxCost)
    }
    if (message.bufferItems !== 0) {
      writer.uint32(24).uint32(message.bufferItems)
    }
    if (message.ttlDur !== '') {
      writer.uint32(34).string(message.ttlDur)
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

          message.numCounters = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.maxCost = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.bufferItems = reader.uint32()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.ttlDur = reader.string()
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
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      numCounters: isSet(object.numCounters)
        ? Long.fromValue(object.numCounters)
        : Long.UZERO,
      maxCost: isSet(object.maxCost)
        ? Long.fromValue(object.maxCost)
        : Long.UZERO,
      bufferItems: isSet(object.bufferItems)
        ? globalThis.Number(object.bufferItems)
        : 0,
      ttlDur: isSet(object.ttlDur) ? globalThis.String(object.ttlDur) : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (!message.numCounters.equals(Long.UZERO)) {
      obj.numCounters = (message.numCounters || Long.UZERO).toString()
    }
    if (!message.maxCost.equals(Long.UZERO)) {
      obj.maxCost = (message.maxCost || Long.UZERO).toString()
    }
    if (message.bufferItems !== 0) {
      obj.bufferItems = Math.round(message.bufferItems)
    }
    if (message.ttlDur !== '') {
      obj.ttlDur = message.ttlDur
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.numCounters =
      object.numCounters !== undefined && object.numCounters !== null
        ? Long.fromValue(object.numCounters)
        : Long.UZERO
    message.maxCost =
      object.maxCost !== undefined && object.maxCost !== null
        ? Long.fromValue(object.maxCost)
        : Long.UZERO
    message.bufferItems = object.bufferItems ?? 0
    message.ttlDur = object.ttlDur ?? ''
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
