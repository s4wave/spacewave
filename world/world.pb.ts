/* eslint-disable */
import { ObjectRef } from '@go/github.com/aperturerobotics/hydra/bucket/bucket.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'identity.world'

/**
 * EntityUpdateOp updates an entity and links to other objects.
 * Operation verifies signatures on/for the Entity.
 * Adds/updates a Keypair object for each valid Keypair.
 * Adds links between the Entity and the Keypair.
 * Can only be applied as a world op.
 */
export interface EntityUpdateOp {
  /** EntityRef is the reference to the latest Entity object. */
  entityRef: ObjectRef | undefined
}

/** KeypairUpdateOp updates a Keypair. */
export interface KeypairUpdateOp {
  /** KeypairRef is the reference to the Keypair object. */
  keypairRef: ObjectRef | undefined
}

/** DomainInfoUpdateOp updates a DomainInfo. */
export interface DomainInfoUpdateOp {
  /** DomainInfoRef is the reference to the DomainInfo object. */
  domainInfoRef: ObjectRef | undefined
}

function createBaseEntityUpdateOp(): EntityUpdateOp {
  return { entityRef: undefined }
}

export const EntityUpdateOp = {
  encode(
    message: EntityUpdateOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.entityRef !== undefined) {
      ObjectRef.encode(message.entityRef, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EntityUpdateOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEntityUpdateOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.entityRef = ObjectRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EntityUpdateOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<EntityUpdateOp | EntityUpdateOp[]>
      | Iterable<EntityUpdateOp | EntityUpdateOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EntityUpdateOp.encode(p).finish()]
        }
      } else {
        yield* [EntityUpdateOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EntityUpdateOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<EntityUpdateOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EntityUpdateOp.decode(p)]
        }
      } else {
        yield* [EntityUpdateOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): EntityUpdateOp {
    return {
      entityRef: isSet(object.entityRef)
        ? ObjectRef.fromJSON(object.entityRef)
        : undefined,
    }
  },

  toJSON(message: EntityUpdateOp): unknown {
    const obj: any = {}
    message.entityRef !== undefined &&
      (obj.entityRef = message.entityRef
        ? ObjectRef.toJSON(message.entityRef)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<EntityUpdateOp>, I>>(
    object: I
  ): EntityUpdateOp {
    const message = createBaseEntityUpdateOp()
    message.entityRef =
      object.entityRef !== undefined && object.entityRef !== null
        ? ObjectRef.fromPartial(object.entityRef)
        : undefined
    return message
  },
}

function createBaseKeypairUpdateOp(): KeypairUpdateOp {
  return { keypairRef: undefined }
}

export const KeypairUpdateOp = {
  encode(
    message: KeypairUpdateOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.keypairRef !== undefined) {
      ObjectRef.encode(message.keypairRef, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): KeypairUpdateOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseKeypairUpdateOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.keypairRef = ObjectRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<KeypairUpdateOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<KeypairUpdateOp | KeypairUpdateOp[]>
      | Iterable<KeypairUpdateOp | KeypairUpdateOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeypairUpdateOp.encode(p).finish()]
        }
      } else {
        yield* [KeypairUpdateOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, KeypairUpdateOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<KeypairUpdateOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [KeypairUpdateOp.decode(p)]
        }
      } else {
        yield* [KeypairUpdateOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): KeypairUpdateOp {
    return {
      keypairRef: isSet(object.keypairRef)
        ? ObjectRef.fromJSON(object.keypairRef)
        : undefined,
    }
  },

  toJSON(message: KeypairUpdateOp): unknown {
    const obj: any = {}
    message.keypairRef !== undefined &&
      (obj.keypairRef = message.keypairRef
        ? ObjectRef.toJSON(message.keypairRef)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<KeypairUpdateOp>, I>>(
    object: I
  ): KeypairUpdateOp {
    const message = createBaseKeypairUpdateOp()
    message.keypairRef =
      object.keypairRef !== undefined && object.keypairRef !== null
        ? ObjectRef.fromPartial(object.keypairRef)
        : undefined
    return message
  },
}

function createBaseDomainInfoUpdateOp(): DomainInfoUpdateOp {
  return { domainInfoRef: undefined }
}

export const DomainInfoUpdateOp = {
  encode(
    message: DomainInfoUpdateOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.domainInfoRef !== undefined) {
      ObjectRef.encode(message.domainInfoRef, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DomainInfoUpdateOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDomainInfoUpdateOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.domainInfoRef = ObjectRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<DomainInfoUpdateOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DomainInfoUpdateOp | DomainInfoUpdateOp[]>
      | Iterable<DomainInfoUpdateOp | DomainInfoUpdateOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DomainInfoUpdateOp.encode(p).finish()]
        }
      } else {
        yield* [DomainInfoUpdateOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DomainInfoUpdateOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<DomainInfoUpdateOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DomainInfoUpdateOp.decode(p)]
        }
      } else {
        yield* [DomainInfoUpdateOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): DomainInfoUpdateOp {
    return {
      domainInfoRef: isSet(object.domainInfoRef)
        ? ObjectRef.fromJSON(object.domainInfoRef)
        : undefined,
    }
  },

  toJSON(message: DomainInfoUpdateOp): unknown {
    const obj: any = {}
    message.domainInfoRef !== undefined &&
      (obj.domainInfoRef = message.domainInfoRef
        ? ObjectRef.toJSON(message.domainInfoRef)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<DomainInfoUpdateOp>, I>>(
    object: I
  ): DomainInfoUpdateOp {
    const message = createBaseDomainInfoUpdateOp()
    message.domainInfoRef =
      object.domainInfoRef !== undefined && object.domainInfoRef !== null
        ? ObjectRef.fromPartial(object.domainInfoRef)
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
