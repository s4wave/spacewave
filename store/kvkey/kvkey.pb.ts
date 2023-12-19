/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'store.kvkey'

/** Config is key/value key configuration. */
export interface Config {
  /**
   * Prefix is the prefix applied to all keys.
   * Default: h/
   */
  prefix: Uint8Array
  /**
   * BucketConfigPrefix is the prefix applied to bucket configs.
   * Default: bkt/c/
   */
  bucketConfigPrefix: Uint8Array
  /**
   * PeerPrivKey is the key to use for the peer private key.
   * Default: priv
   */
  peerPrivKey: Uint8Array
  /**
   * BlockPrefix is the prefix applied to block hashes.
   * Default: b/
   */
  blockPrefix: Uint8Array
  /**
   * ObjectStorePrefix is the prefix applied to object stores.
   * Default: objs/
   */
  objectStorePrefix: Uint8Array
  /**
   * MqueuePrefix contains the key to use for the message queues.
   * Default: mq/q/
   */
  mqueuePrefix: Uint8Array
  /**
   * MqueueMetaPrefix contains the key to use for the message queue metas.
   * Default: mq/m/
   */
  mqueueMetaPrefix: Uint8Array
  /**
   * BucketMqueuePrefix contains the mqueue id prefix to use for bucket reconcilers.
   * Default: bkt/
   */
  bucketMqueuePrefix: Uint8Array
}

function createBaseConfig(): Config {
  return {
    prefix: new Uint8Array(0),
    bucketConfigPrefix: new Uint8Array(0),
    peerPrivKey: new Uint8Array(0),
    blockPrefix: new Uint8Array(0),
    objectStorePrefix: new Uint8Array(0),
    mqueuePrefix: new Uint8Array(0),
    mqueueMetaPrefix: new Uint8Array(0),
    bucketMqueuePrefix: new Uint8Array(0),
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.prefix.length !== 0) {
      writer.uint32(10).bytes(message.prefix)
    }
    if (message.bucketConfigPrefix.length !== 0) {
      writer.uint32(18).bytes(message.bucketConfigPrefix)
    }
    if (message.peerPrivKey.length !== 0) {
      writer.uint32(26).bytes(message.peerPrivKey)
    }
    if (message.blockPrefix.length !== 0) {
      writer.uint32(42).bytes(message.blockPrefix)
    }
    if (message.objectStorePrefix.length !== 0) {
      writer.uint32(50).bytes(message.objectStorePrefix)
    }
    if (message.mqueuePrefix.length !== 0) {
      writer.uint32(58).bytes(message.mqueuePrefix)
    }
    if (message.mqueueMetaPrefix.length !== 0) {
      writer.uint32(66).bytes(message.mqueueMetaPrefix)
    }
    if (message.bucketMqueuePrefix.length !== 0) {
      writer.uint32(74).bytes(message.bucketMqueuePrefix)
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
          if (tag !== 10) {
            break
          }

          message.prefix = reader.bytes()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.bucketConfigPrefix = reader.bytes()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.peerPrivKey = reader.bytes()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.blockPrefix = reader.bytes()
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.objectStorePrefix = reader.bytes()
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.mqueuePrefix = reader.bytes()
          continue
        case 8:
          if (tag !== 66) {
            break
          }

          message.mqueueMetaPrefix = reader.bytes()
          continue
        case 9:
          if (tag !== 74) {
            break
          }

          message.bucketMqueuePrefix = reader.bytes()
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
      prefix: isSet(object.prefix)
        ? bytesFromBase64(object.prefix)
        : new Uint8Array(0),
      bucketConfigPrefix: isSet(object.bucketConfigPrefix)
        ? bytesFromBase64(object.bucketConfigPrefix)
        : new Uint8Array(0),
      peerPrivKey: isSet(object.peerPrivKey)
        ? bytesFromBase64(object.peerPrivKey)
        : new Uint8Array(0),
      blockPrefix: isSet(object.blockPrefix)
        ? bytesFromBase64(object.blockPrefix)
        : new Uint8Array(0),
      objectStorePrefix: isSet(object.objectStorePrefix)
        ? bytesFromBase64(object.objectStorePrefix)
        : new Uint8Array(0),
      mqueuePrefix: isSet(object.mqueuePrefix)
        ? bytesFromBase64(object.mqueuePrefix)
        : new Uint8Array(0),
      mqueueMetaPrefix: isSet(object.mqueueMetaPrefix)
        ? bytesFromBase64(object.mqueueMetaPrefix)
        : new Uint8Array(0),
      bucketMqueuePrefix: isSet(object.bucketMqueuePrefix)
        ? bytesFromBase64(object.bucketMqueuePrefix)
        : new Uint8Array(0),
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.prefix.length !== 0) {
      obj.prefix = base64FromBytes(message.prefix)
    }
    if (message.bucketConfigPrefix.length !== 0) {
      obj.bucketConfigPrefix = base64FromBytes(message.bucketConfigPrefix)
    }
    if (message.peerPrivKey.length !== 0) {
      obj.peerPrivKey = base64FromBytes(message.peerPrivKey)
    }
    if (message.blockPrefix.length !== 0) {
      obj.blockPrefix = base64FromBytes(message.blockPrefix)
    }
    if (message.objectStorePrefix.length !== 0) {
      obj.objectStorePrefix = base64FromBytes(message.objectStorePrefix)
    }
    if (message.mqueuePrefix.length !== 0) {
      obj.mqueuePrefix = base64FromBytes(message.mqueuePrefix)
    }
    if (message.mqueueMetaPrefix.length !== 0) {
      obj.mqueueMetaPrefix = base64FromBytes(message.mqueueMetaPrefix)
    }
    if (message.bucketMqueuePrefix.length !== 0) {
      obj.bucketMqueuePrefix = base64FromBytes(message.bucketMqueuePrefix)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.prefix = object.prefix ?? new Uint8Array(0)
    message.bucketConfigPrefix = object.bucketConfigPrefix ?? new Uint8Array(0)
    message.peerPrivKey = object.peerPrivKey ?? new Uint8Array(0)
    message.blockPrefix = object.blockPrefix ?? new Uint8Array(0)
    message.objectStorePrefix = object.objectStorePrefix ?? new Uint8Array(0)
    message.mqueuePrefix = object.mqueuePrefix ?? new Uint8Array(0)
    message.mqueueMetaPrefix = object.mqueueMetaPrefix ?? new Uint8Array(0)
    message.bucketMqueuePrefix = object.bucketMqueuePrefix ?? new Uint8Array(0)
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
