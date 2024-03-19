/* eslint-disable */
import {
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'block.store.s3'

/** Config configures the s3 block store controller. */
export interface Config {
  /** BlockStoreId is the block store id to use on the bus. */
  blockStoreId: string
  /** Client configures the s3 client. */
  client: ClientConfig | undefined
  /** BucketName is the s3 bucket name to use. */
  bucketName: string
  /**
   * ObjectPrefix is the prefix to use for object names.
   * Object name: {objectPrefix}{blockRefB58}
   */
  objectPrefix: string
  /** ReadOnly disables writing to the s3 store. */
  readOnly: boolean
  /**
   * ForceHashType forces writing the given hash type to the store.
   * If unset, accepts any hash type.
   */
  forceHashType: HashType
  /** BucketIds is a list of bucket ids to serve LookupBlockFromNetwork directives. */
  bucketIds: string[]
  /** SkipNotFound skips returning a value if the block was not found. */
  skipNotFound: boolean
  /** Verbose enables verbose logging of the block store. */
  verbose: boolean
}

/**
 * ClientConfig configures the s3 client.
 * Supports any s3-compatible object store.
 */
export interface ClientConfig {
  /** Endpoint is the endpoint to access the s3 api. */
  endpoint: string
  /** Credentials contains the authentication creds. */
  credentials: Credentials | undefined
  /**
   * DisableSsl disables using SSL to access the api.
   * If false, uses ssl.
   */
  disableSsl: boolean
  /**
   * Region is the name of the region to use.
   * Can be empty.
   */
  region: string
}

/** Credentials are credentials for a s3-compatible api. */
export interface Credentials {
  /** AccessKeyId is the authentication access key id. */
  accessKeyId: string
  /** SecretAccessKey is the secret access key corresponding to the access key id. */
  secretAccessKey: string
  /**
   * Token is the token to use.
   * Usually empty.
   */
  token: string
}

function createBaseConfig(): Config {
  return {
    blockStoreId: '',
    client: undefined,
    bucketName: '',
    objectPrefix: '',
    readOnly: false,
    forceHashType: 0,
    bucketIds: [],
    skipNotFound: false,
    verbose: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.blockStoreId !== '') {
      writer.uint32(10).string(message.blockStoreId)
    }
    if (message.client !== undefined) {
      ClientConfig.encode(message.client, writer.uint32(18).fork()).ldelim()
    }
    if (message.bucketName !== '') {
      writer.uint32(26).string(message.bucketName)
    }
    if (message.objectPrefix !== '') {
      writer.uint32(34).string(message.objectPrefix)
    }
    if (message.readOnly !== false) {
      writer.uint32(40).bool(message.readOnly)
    }
    if (message.forceHashType !== 0) {
      writer.uint32(48).int32(message.forceHashType)
    }
    for (const v of message.bucketIds) {
      writer.uint32(58).string(v!)
    }
    if (message.skipNotFound !== false) {
      writer.uint32(64).bool(message.skipNotFound)
    }
    if (message.verbose !== false) {
      writer.uint32(72).bool(message.verbose)
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

          message.blockStoreId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.client = ClientConfig.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.bucketName = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.objectPrefix = reader.string()
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.readOnly = reader.bool()
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.forceHashType = reader.int32() as any
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.bucketIds.push(reader.string())
          continue
        case 8:
          if (tag !== 64) {
            break
          }

          message.skipNotFound = reader.bool()
          continue
        case 9:
          if (tag !== 72) {
            break
          }

          message.verbose = reader.bool()
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
      blockStoreId: isSet(object.blockStoreId)
        ? globalThis.String(object.blockStoreId)
        : '',
      client: isSet(object.client)
        ? ClientConfig.fromJSON(object.client)
        : undefined,
      bucketName: isSet(object.bucketName)
        ? globalThis.String(object.bucketName)
        : '',
      objectPrefix: isSet(object.objectPrefix)
        ? globalThis.String(object.objectPrefix)
        : '',
      readOnly: isSet(object.readOnly)
        ? globalThis.Boolean(object.readOnly)
        : false,
      forceHashType: isSet(object.forceHashType)
        ? hashTypeFromJSON(object.forceHashType)
        : 0,
      bucketIds: globalThis.Array.isArray(object?.bucketIds)
        ? object.bucketIds.map((e: any) => globalThis.String(e))
        : [],
      skipNotFound: isSet(object.skipNotFound)
        ? globalThis.Boolean(object.skipNotFound)
        : false,
      verbose: isSet(object.verbose)
        ? globalThis.Boolean(object.verbose)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.blockStoreId !== '') {
      obj.blockStoreId = message.blockStoreId
    }
    if (message.client !== undefined) {
      obj.client = ClientConfig.toJSON(message.client)
    }
    if (message.bucketName !== '') {
      obj.bucketName = message.bucketName
    }
    if (message.objectPrefix !== '') {
      obj.objectPrefix = message.objectPrefix
    }
    if (message.readOnly !== false) {
      obj.readOnly = message.readOnly
    }
    if (message.forceHashType !== 0) {
      obj.forceHashType = hashTypeToJSON(message.forceHashType)
    }
    if (message.bucketIds?.length) {
      obj.bucketIds = message.bucketIds
    }
    if (message.skipNotFound !== false) {
      obj.skipNotFound = message.skipNotFound
    }
    if (message.verbose !== false) {
      obj.verbose = message.verbose
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.blockStoreId = object.blockStoreId ?? ''
    message.client =
      object.client !== undefined && object.client !== null
        ? ClientConfig.fromPartial(object.client)
        : undefined
    message.bucketName = object.bucketName ?? ''
    message.objectPrefix = object.objectPrefix ?? ''
    message.readOnly = object.readOnly ?? false
    message.forceHashType = object.forceHashType ?? 0
    message.bucketIds = object.bucketIds?.map((e) => e) || []
    message.skipNotFound = object.skipNotFound ?? false
    message.verbose = object.verbose ?? false
    return message
  },
}

function createBaseClientConfig(): ClientConfig {
  return { endpoint: '', credentials: undefined, disableSsl: false, region: '' }
}

export const ClientConfig = {
  encode(
    message: ClientConfig,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.endpoint !== '') {
      writer.uint32(10).string(message.endpoint)
    }
    if (message.credentials !== undefined) {
      Credentials.encode(message.credentials, writer.uint32(18).fork()).ldelim()
    }
    if (message.disableSsl !== false) {
      writer.uint32(24).bool(message.disableSsl)
    }
    if (message.region !== '') {
      writer.uint32(34).string(message.region)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ClientConfig {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClientConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.endpoint = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.credentials = Credentials.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.disableSsl = reader.bool()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.region = reader.string()
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
  // Transform<ClientConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClientConfig | ClientConfig[]>
      | Iterable<ClientConfig | ClientConfig[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ClientConfig.encode(p).finish()]
        }
      } else {
        yield* [ClientConfig.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClientConfig>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ClientConfig> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ClientConfig.decode(p)]
        }
      } else {
        yield* [ClientConfig.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ClientConfig {
    return {
      endpoint: isSet(object.endpoint)
        ? globalThis.String(object.endpoint)
        : '',
      credentials: isSet(object.credentials)
        ? Credentials.fromJSON(object.credentials)
        : undefined,
      disableSsl: isSet(object.disableSsl)
        ? globalThis.Boolean(object.disableSsl)
        : false,
      region: isSet(object.region) ? globalThis.String(object.region) : '',
    }
  },

  toJSON(message: ClientConfig): unknown {
    const obj: any = {}
    if (message.endpoint !== '') {
      obj.endpoint = message.endpoint
    }
    if (message.credentials !== undefined) {
      obj.credentials = Credentials.toJSON(message.credentials)
    }
    if (message.disableSsl !== false) {
      obj.disableSsl = message.disableSsl
    }
    if (message.region !== '') {
      obj.region = message.region
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ClientConfig>, I>>(
    base?: I,
  ): ClientConfig {
    return ClientConfig.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ClientConfig>, I>>(
    object: I,
  ): ClientConfig {
    const message = createBaseClientConfig()
    message.endpoint = object.endpoint ?? ''
    message.credentials =
      object.credentials !== undefined && object.credentials !== null
        ? Credentials.fromPartial(object.credentials)
        : undefined
    message.disableSsl = object.disableSsl ?? false
    message.region = object.region ?? ''
    return message
  },
}

function createBaseCredentials(): Credentials {
  return { accessKeyId: '', secretAccessKey: '', token: '' }
}

export const Credentials = {
  encode(
    message: Credentials,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.accessKeyId !== '') {
      writer.uint32(10).string(message.accessKeyId)
    }
    if (message.secretAccessKey !== '') {
      writer.uint32(18).string(message.secretAccessKey)
    }
    if (message.token !== '') {
      writer.uint32(26).string(message.token)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Credentials {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCredentials()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.accessKeyId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.secretAccessKey = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.token = reader.string()
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
  // Transform<Credentials, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Credentials | Credentials[]>
      | Iterable<Credentials | Credentials[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Credentials.encode(p).finish()]
        }
      } else {
        yield* [Credentials.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Credentials>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Credentials> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Credentials.decode(p)]
        }
      } else {
        yield* [Credentials.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Credentials {
    return {
      accessKeyId: isSet(object.accessKeyId)
        ? globalThis.String(object.accessKeyId)
        : '',
      secretAccessKey: isSet(object.secretAccessKey)
        ? globalThis.String(object.secretAccessKey)
        : '',
      token: isSet(object.token) ? globalThis.String(object.token) : '',
    }
  },

  toJSON(message: Credentials): unknown {
    const obj: any = {}
    if (message.accessKeyId !== '') {
      obj.accessKeyId = message.accessKeyId
    }
    if (message.secretAccessKey !== '') {
      obj.secretAccessKey = message.secretAccessKey
    }
    if (message.token !== '') {
      obj.token = message.token
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Credentials>, I>>(base?: I): Credentials {
    return Credentials.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Credentials>, I>>(
    object: I,
  ): Credentials {
    const message = createBaseCredentials()
    message.accessKeyId = object.accessKeyId ?? ''
    message.secretAccessKey = object.secretAccessKey ?? ''
    message.token = object.token ?? ''
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
