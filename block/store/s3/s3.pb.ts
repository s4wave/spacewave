/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'block.store.s3'

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

function createBaseClientConfig(): ClientConfig {
  return { endpoint: '', credentials: undefined, disableSsl: false, region: '' }
}

export const ClientConfig = {
  encode(
    message: ClientConfig,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.endpoint !== '') {
      writer.uint32(10).string(message.endpoint)
    }
    if (message.credentials !== undefined) {
      Credentials.encode(message.credentials, writer.uint32(18).fork()).ldelim()
    }
    if (message.disableSsl === true) {
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
          if (tag != 10) {
            break
          }

          message.endpoint = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.credentials = Credentials.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag != 24) {
            break
          }

          message.disableSsl = reader.bool()
          continue
        case 4:
          if (tag != 34) {
            break
          }

          message.region = reader.string()
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
  // Transform<ClientConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClientConfig | ClientConfig[]>
      | Iterable<ClientConfig | ClientConfig[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClientConfig.encode(p).finish()]
        }
      } else {
        yield* [ClientConfig.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClientConfig>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ClientConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClientConfig.decode(p)]
        }
      } else {
        yield* [ClientConfig.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ClientConfig {
    return {
      endpoint: isSet(object.endpoint) ? String(object.endpoint) : '',
      credentials: isSet(object.credentials)
        ? Credentials.fromJSON(object.credentials)
        : undefined,
      disableSsl: isSet(object.disableSsl) ? Boolean(object.disableSsl) : false,
      region: isSet(object.region) ? String(object.region) : '',
    }
  },

  toJSON(message: ClientConfig): unknown {
    const obj: any = {}
    message.endpoint !== undefined && (obj.endpoint = message.endpoint)
    message.credentials !== undefined &&
      (obj.credentials = message.credentials
        ? Credentials.toJSON(message.credentials)
        : undefined)
    message.disableSsl !== undefined && (obj.disableSsl = message.disableSsl)
    message.region !== undefined && (obj.region = message.region)
    return obj
  },

  create<I extends Exact<DeepPartial<ClientConfig>, I>>(
    base?: I
  ): ClientConfig {
    return ClientConfig.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ClientConfig>, I>>(
    object: I
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
    writer: _m0.Writer = _m0.Writer.create()
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
          if (tag != 10) {
            break
          }

          message.accessKeyId = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.secretAccessKey = reader.string()
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.token = reader.string()
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
  // Transform<Credentials, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Credentials | Credentials[]>
      | Iterable<Credentials | Credentials[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Credentials.encode(p).finish()]
        }
      } else {
        yield* [Credentials.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Credentials>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Credentials> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Credentials.decode(p)]
        }
      } else {
        yield* [Credentials.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Credentials {
    return {
      accessKeyId: isSet(object.accessKeyId) ? String(object.accessKeyId) : '',
      secretAccessKey: isSet(object.secretAccessKey)
        ? String(object.secretAccessKey)
        : '',
      token: isSet(object.token) ? String(object.token) : '',
    }
  },

  toJSON(message: Credentials): unknown {
    const obj: any = {}
    message.accessKeyId !== undefined && (obj.accessKeyId = message.accessKeyId)
    message.secretAccessKey !== undefined &&
      (obj.secretAccessKey = message.secretAccessKey)
    message.token !== undefined && (obj.token = message.token)
    return obj
  },

  create<I extends Exact<DeepPartial<Credentials>, I>>(base?: I): Credentials {
    return Credentials.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Credentials>, I>>(
    object: I
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
