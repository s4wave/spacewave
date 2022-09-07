/* eslint-disable */
import { Signature } from "@go/github.com/aperturerobotics/bifrost/peer/peer.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "identity";

/** EntityChangeType is an entity change transaction type. */
export enum EntityChangeType {
  EntityChangeType_UNKNOWN = 0,
  EntityChangeType_REGISTER_KEYPAIR = 1,
  EntityChangeType_REMOVE_KEYPAIR = 2,
  UNRECOGNIZED = -1,
}

export function entityChangeTypeFromJSON(object: any): EntityChangeType {
  switch (object) {
    case 0:
    case "EntityChangeType_UNKNOWN":
      return EntityChangeType.EntityChangeType_UNKNOWN;
    case 1:
    case "EntityChangeType_REGISTER_KEYPAIR":
      return EntityChangeType.EntityChangeType_REGISTER_KEYPAIR;
    case 2:
    case "EntityChangeType_REMOVE_KEYPAIR":
      return EntityChangeType.EntityChangeType_REMOVE_KEYPAIR;
    case -1:
    case "UNRECOGNIZED":
    default:
      return EntityChangeType.UNRECOGNIZED;
  }
}

export function entityChangeTypeToJSON(object: EntityChangeType): string {
  switch (object) {
    case EntityChangeType.EntityChangeType_UNKNOWN:
      return "EntityChangeType_UNKNOWN";
    case EntityChangeType.EntityChangeType_REGISTER_KEYPAIR:
      return "EntityChangeType_REGISTER_KEYPAIR";
    case EntityChangeType.EntityChangeType_REMOVE_KEYPAIR:
      return "EntityChangeType_REMOVE_KEYPAIR";
    case EntityChangeType.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

/**
 * Entity is an individual user or system with a persistent identity.
 *
 * The root Entity object is not considered to be sensitive information.
 * For an Entity to be valid, all Keypairs must have valid signatures.
 */
export interface Entity {
  /**
   * EntityId is the user-specified entity identifier, akin to a username.
   * The entity id is not necessarily unique in all domains.
   * Must be a valid DNS label name as defined in RFC 1123.
   * Must be lowercase.
   */
  entityId: string;
  /**
   * EntityUuid is a domain-unique unique identifier, generated at account
   * registration time.
   *
   * Usually: UUIDv5(domain_uuid, entity_id)
   */
  entityUuid: string;
  /**
   * DomainId is the domain identifier (typically the domain name).
   * This domain controller controls this entity.
   * Must be a valid DNS subdomain name as defined in RFC 1123.
   * Must be lowercase.
   */
  domainId: string;
  /** Epoch is the change epoch for the entity, incremented when changes are made. */
  epoch: Long;
  /** EntityKeypairs contains marshalled EntityKeypair aliases of the Entity. */
  entityKeypairs: Uint8Array[];
  /**
   * KeypairSignatures contains the signatures for each Keypair.
   * The signature pub_key must match the peer_id of the Keypair.
   */
  keypairSignatures: Signature[];
}

/** EntityKeypair contains a binding between a Keypair and an Entity. */
export interface EntityKeypair {
  /**
   * EntityId is the entity_id field of the Entity.
   * Must match the entity_id specified in the Entity object.
   * If this is a Domain, this field will be empty.
   */
  entityId: string;
  /**
   * DomainId is the domain_id field of the Entity.
   * Must match the domain_id specified in the Entity object.
   */
  domainId: string;
  /** Keypair is the keypair to associate with the entity. */
  keypair: Keypair | undefined;
}

/** EntityRef is a reference to a entity on a domain. */
export interface EntityRef {
  /**
   * EntityId is the entity_id field of the Entity.
   * Must match the entity_id specified in the Entity object.
   */
  entityId: string;
  /**
   * DomainId is the domain_id field of the Entity.
   * Must match the domain_id specified in the Entity object.
   */
  domainId: string;
}

/** Keypair contains a peer ID (public key) and information to derive the key. */
export interface Keypair {
  /**
   * PeerId is the peer id of the keypair (derived from pubkey).
   * Must match the pub_key field.
   */
  peerId: string;
  /**
   * PubKey is the PEM-encoded public key with Bifrost keypem.
   * Must match the pub_key of the keypair signature on the Entity.
   */
  pubKey: string;
  /**
   * AuthMethodId is the authentication method to derive this key.
   * This is a black-box value: it is used to derive the key again later.
   */
  authMethodId: string;
  /**
   * AuthMethodParams is the encoded params object for the method.
   *
   * Params might include the CTAP2 records for binding, attestation.
   */
  authMethodParams: Uint8Array;
}

/**
 * PendingEntityChange is a ongoing change to a entity credential list.
 *
 * An additional transaction system will manage adding/removing/updating these
 * records, which exist to represent ongoing transactions to update an entity
 * record, for example, adding a new security key via a handshake with hardware.
 *
 * Specific change transaction types (Create, Update, Dismiss) are implemented
 * by the auth method (not in this system).
 */
export interface PendingEntityChange {
  /**
   * ChangePeerId is the peer id of the transactor submitting the change.
   *
   * This peer ID should be checked against incoming transactions. It should be
   * authenticated to be an ID with authority to change the record: usually
   * either a existing associated identity or a domain authority.
   */
  changePeerId: string;
  /** Epoch is the change epoch, incremented when changes are made. */
  epoch: Long;
  /** DomainIdentifier is the identifier of the related entity. */
  domainIdentifier: string;
  /** EntityChangeType is the type of this entity change. */
  entityChangeType: EntityChangeType;
  /** EntityChangeData is the inner data for the entity change. */
  entityChangeData: string;
}

/**
 * RegisterKeypair is used when adding a new keypair to a entity.
 *
 * EntityChangeType_REGISTER_KEYPAIR
 */
export interface RegisterKeypair {
  /**
   * The public key is derivable from the peer ID.
   * Only one Keypair with this public key / peer ID can be used.
   */
  registerPeerId: string;
  /** AuthMethodId is the authentication method to use. */
  authMethodId: string;
  /**
   * AuthMethodState is the encoded change state object for the method.
   *
   * State might include the CTAP2 challenge, for example.
   */
  authMethodState: Uint8Array;
}

/**
 * RemoveKeypair is used to remove a keypair by peer ID from the entity.
 *
 * EntityChangeType_REMOVE_KEYPAIR
 */
export interface RemoveKeypair {
  /** PeerId is the peer ID to remove from the existing keypairs. */
  peerId: string;
}

function createBaseEntity(): Entity {
  return { entityId: "", entityUuid: "", domainId: "", epoch: Long.UZERO, entityKeypairs: [], keypairSignatures: [] };
}

export const Entity = {
  encode(message: Entity, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.entityId !== "") {
      writer.uint32(10).string(message.entityId);
    }
    if (message.entityUuid !== "") {
      writer.uint32(18).string(message.entityUuid);
    }
    if (message.domainId !== "") {
      writer.uint32(26).string(message.domainId);
    }
    if (!message.epoch.isZero()) {
      writer.uint32(32).uint64(message.epoch);
    }
    for (const v of message.entityKeypairs) {
      writer.uint32(42).bytes(v!);
    }
    for (const v of message.keypairSignatures) {
      Signature.encode(v!, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Entity {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseEntity();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.entityId = reader.string();
          break;
        case 2:
          message.entityUuid = reader.string();
          break;
        case 3:
          message.domainId = reader.string();
          break;
        case 4:
          message.epoch = reader.uint64() as Long;
          break;
        case 5:
          message.entityKeypairs.push(reader.bytes());
          break;
        case 6:
          message.keypairSignatures.push(Signature.decode(reader, reader.uint32()));
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Entity, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Entity | Entity[]> | Iterable<Entity | Entity[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Entity.encode(p).finish()];
        }
      } else {
        yield* [Entity.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Entity>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Entity> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Entity.decode(p)];
        }
      } else {
        yield* [Entity.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Entity {
    return {
      entityId: isSet(object.entityId) ? String(object.entityId) : "",
      entityUuid: isSet(object.entityUuid) ? String(object.entityUuid) : "",
      domainId: isSet(object.domainId) ? String(object.domainId) : "",
      epoch: isSet(object.epoch) ? Long.fromValue(object.epoch) : Long.UZERO,
      entityKeypairs: Array.isArray(object?.entityKeypairs)
        ? object.entityKeypairs.map((e: any) => bytesFromBase64(e))
        : [],
      keypairSignatures: Array.isArray(object?.keypairSignatures)
        ? object.keypairSignatures.map((e: any) => Signature.fromJSON(e))
        : [],
    };
  },

  toJSON(message: Entity): unknown {
    const obj: any = {};
    message.entityId !== undefined && (obj.entityId = message.entityId);
    message.entityUuid !== undefined && (obj.entityUuid = message.entityUuid);
    message.domainId !== undefined && (obj.domainId = message.domainId);
    message.epoch !== undefined && (obj.epoch = (message.epoch || Long.UZERO).toString());
    if (message.entityKeypairs) {
      obj.entityKeypairs = message.entityKeypairs.map((e) => base64FromBytes(e !== undefined ? e : new Uint8Array()));
    } else {
      obj.entityKeypairs = [];
    }
    if (message.keypairSignatures) {
      obj.keypairSignatures = message.keypairSignatures.map((e) => e ? Signature.toJSON(e) : undefined);
    } else {
      obj.keypairSignatures = [];
    }
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Entity>, I>>(object: I): Entity {
    const message = createBaseEntity();
    message.entityId = object.entityId ?? "";
    message.entityUuid = object.entityUuid ?? "";
    message.domainId = object.domainId ?? "";
    message.epoch = (object.epoch !== undefined && object.epoch !== null) ? Long.fromValue(object.epoch) : Long.UZERO;
    message.entityKeypairs = object.entityKeypairs?.map((e) => e) || [];
    message.keypairSignatures = object.keypairSignatures?.map((e) => Signature.fromPartial(e)) || [];
    return message;
  },
};

function createBaseEntityKeypair(): EntityKeypair {
  return { entityId: "", domainId: "", keypair: undefined };
}

export const EntityKeypair = {
  encode(message: EntityKeypair, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.entityId !== "") {
      writer.uint32(10).string(message.entityId);
    }
    if (message.domainId !== "") {
      writer.uint32(18).string(message.domainId);
    }
    if (message.keypair !== undefined) {
      Keypair.encode(message.keypair, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EntityKeypair {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseEntityKeypair();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.entityId = reader.string();
          break;
        case 2:
          message.domainId = reader.string();
          break;
        case 3:
          message.keypair = Keypair.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EntityKeypair, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<EntityKeypair | EntityKeypair[]> | Iterable<EntityKeypair | EntityKeypair[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EntityKeypair.encode(p).finish()];
        }
      } else {
        yield* [EntityKeypair.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EntityKeypair>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<EntityKeypair> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EntityKeypair.decode(p)];
        }
      } else {
        yield* [EntityKeypair.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): EntityKeypair {
    return {
      entityId: isSet(object.entityId) ? String(object.entityId) : "",
      domainId: isSet(object.domainId) ? String(object.domainId) : "",
      keypair: isSet(object.keypair) ? Keypair.fromJSON(object.keypair) : undefined,
    };
  },

  toJSON(message: EntityKeypair): unknown {
    const obj: any = {};
    message.entityId !== undefined && (obj.entityId = message.entityId);
    message.domainId !== undefined && (obj.domainId = message.domainId);
    message.keypair !== undefined && (obj.keypair = message.keypair ? Keypair.toJSON(message.keypair) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<EntityKeypair>, I>>(object: I): EntityKeypair {
    const message = createBaseEntityKeypair();
    message.entityId = object.entityId ?? "";
    message.domainId = object.domainId ?? "";
    message.keypair = (object.keypair !== undefined && object.keypair !== null)
      ? Keypair.fromPartial(object.keypair)
      : undefined;
    return message;
  },
};

function createBaseEntityRef(): EntityRef {
  return { entityId: "", domainId: "" };
}

export const EntityRef = {
  encode(message: EntityRef, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.entityId !== "") {
      writer.uint32(10).string(message.entityId);
    }
    if (message.domainId !== "") {
      writer.uint32(18).string(message.domainId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EntityRef {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseEntityRef();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.entityId = reader.string();
          break;
        case 2:
          message.domainId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EntityRef, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<EntityRef | EntityRef[]> | Iterable<EntityRef | EntityRef[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EntityRef.encode(p).finish()];
        }
      } else {
        yield* [EntityRef.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EntityRef>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<EntityRef> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EntityRef.decode(p)];
        }
      } else {
        yield* [EntityRef.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): EntityRef {
    return {
      entityId: isSet(object.entityId) ? String(object.entityId) : "",
      domainId: isSet(object.domainId) ? String(object.domainId) : "",
    };
  },

  toJSON(message: EntityRef): unknown {
    const obj: any = {};
    message.entityId !== undefined && (obj.entityId = message.entityId);
    message.domainId !== undefined && (obj.domainId = message.domainId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<EntityRef>, I>>(object: I): EntityRef {
    const message = createBaseEntityRef();
    message.entityId = object.entityId ?? "";
    message.domainId = object.domainId ?? "";
    return message;
  },
};

function createBaseKeypair(): Keypair {
  return { peerId: "", pubKey: "", authMethodId: "", authMethodParams: new Uint8Array() };
}

export const Keypair = {
  encode(message: Keypair, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.peerId !== "") {
      writer.uint32(10).string(message.peerId);
    }
    if (message.pubKey !== "") {
      writer.uint32(18).string(message.pubKey);
    }
    if (message.authMethodId !== "") {
      writer.uint32(26).string(message.authMethodId);
    }
    if (message.authMethodParams.length !== 0) {
      writer.uint32(34).bytes(message.authMethodParams);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Keypair {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseKeypair();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.peerId = reader.string();
          break;
        case 2:
          message.pubKey = reader.string();
          break;
        case 3:
          message.authMethodId = reader.string();
          break;
        case 4:
          message.authMethodParams = reader.bytes();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Keypair, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Keypair | Keypair[]> | Iterable<Keypair | Keypair[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Keypair.encode(p).finish()];
        }
      } else {
        yield* [Keypair.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Keypair>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Keypair> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Keypair.decode(p)];
        }
      } else {
        yield* [Keypair.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Keypair {
    return {
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      pubKey: isSet(object.pubKey) ? String(object.pubKey) : "",
      authMethodId: isSet(object.authMethodId) ? String(object.authMethodId) : "",
      authMethodParams: isSet(object.authMethodParams) ? bytesFromBase64(object.authMethodParams) : new Uint8Array(),
    };
  },

  toJSON(message: Keypair): unknown {
    const obj: any = {};
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.pubKey !== undefined && (obj.pubKey = message.pubKey);
    message.authMethodId !== undefined && (obj.authMethodId = message.authMethodId);
    message.authMethodParams !== undefined &&
      (obj.authMethodParams = base64FromBytes(
        message.authMethodParams !== undefined ? message.authMethodParams : new Uint8Array(),
      ));
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Keypair>, I>>(object: I): Keypair {
    const message = createBaseKeypair();
    message.peerId = object.peerId ?? "";
    message.pubKey = object.pubKey ?? "";
    message.authMethodId = object.authMethodId ?? "";
    message.authMethodParams = object.authMethodParams ?? new Uint8Array();
    return message;
  },
};

function createBasePendingEntityChange(): PendingEntityChange {
  return { changePeerId: "", epoch: Long.UZERO, domainIdentifier: "", entityChangeType: 0, entityChangeData: "" };
}

export const PendingEntityChange = {
  encode(message: PendingEntityChange, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.changePeerId !== "") {
      writer.uint32(10).string(message.changePeerId);
    }
    if (!message.epoch.isZero()) {
      writer.uint32(16).uint64(message.epoch);
    }
    if (message.domainIdentifier !== "") {
      writer.uint32(26).string(message.domainIdentifier);
    }
    if (message.entityChangeType !== 0) {
      writer.uint32(32).int32(message.entityChangeType);
    }
    if (message.entityChangeData !== "") {
      writer.uint32(42).string(message.entityChangeData);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PendingEntityChange {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePendingEntityChange();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.changePeerId = reader.string();
          break;
        case 2:
          message.epoch = reader.uint64() as Long;
          break;
        case 3:
          message.domainIdentifier = reader.string();
          break;
        case 4:
          message.entityChangeType = reader.int32() as any;
          break;
        case 5:
          message.entityChangeData = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PendingEntityChange, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<PendingEntityChange | PendingEntityChange[]>
      | Iterable<PendingEntityChange | PendingEntityChange[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PendingEntityChange.encode(p).finish()];
        }
      } else {
        yield* [PendingEntityChange.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PendingEntityChange>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PendingEntityChange> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [PendingEntityChange.decode(p)];
        }
      } else {
        yield* [PendingEntityChange.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): PendingEntityChange {
    return {
      changePeerId: isSet(object.changePeerId) ? String(object.changePeerId) : "",
      epoch: isSet(object.epoch) ? Long.fromValue(object.epoch) : Long.UZERO,
      domainIdentifier: isSet(object.domainIdentifier) ? String(object.domainIdentifier) : "",
      entityChangeType: isSet(object.entityChangeType) ? entityChangeTypeFromJSON(object.entityChangeType) : 0,
      entityChangeData: isSet(object.entityChangeData) ? String(object.entityChangeData) : "",
    };
  },

  toJSON(message: PendingEntityChange): unknown {
    const obj: any = {};
    message.changePeerId !== undefined && (obj.changePeerId = message.changePeerId);
    message.epoch !== undefined && (obj.epoch = (message.epoch || Long.UZERO).toString());
    message.domainIdentifier !== undefined && (obj.domainIdentifier = message.domainIdentifier);
    message.entityChangeType !== undefined && (obj.entityChangeType = entityChangeTypeToJSON(message.entityChangeType));
    message.entityChangeData !== undefined && (obj.entityChangeData = message.entityChangeData);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<PendingEntityChange>, I>>(object: I): PendingEntityChange {
    const message = createBasePendingEntityChange();
    message.changePeerId = object.changePeerId ?? "";
    message.epoch = (object.epoch !== undefined && object.epoch !== null) ? Long.fromValue(object.epoch) : Long.UZERO;
    message.domainIdentifier = object.domainIdentifier ?? "";
    message.entityChangeType = object.entityChangeType ?? 0;
    message.entityChangeData = object.entityChangeData ?? "";
    return message;
  },
};

function createBaseRegisterKeypair(): RegisterKeypair {
  return { registerPeerId: "", authMethodId: "", authMethodState: new Uint8Array() };
}

export const RegisterKeypair = {
  encode(message: RegisterKeypair, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.registerPeerId !== "") {
      writer.uint32(10).string(message.registerPeerId);
    }
    if (message.authMethodId !== "") {
      writer.uint32(18).string(message.authMethodId);
    }
    if (message.authMethodState.length !== 0) {
      writer.uint32(26).bytes(message.authMethodState);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RegisterKeypair {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRegisterKeypair();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.registerPeerId = reader.string();
          break;
        case 2:
          message.authMethodId = reader.string();
          break;
        case 3:
          message.authMethodState = reader.bytes();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RegisterKeypair, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<RegisterKeypair | RegisterKeypair[]> | Iterable<RegisterKeypair | RegisterKeypair[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RegisterKeypair.encode(p).finish()];
        }
      } else {
        yield* [RegisterKeypair.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RegisterKeypair>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RegisterKeypair> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RegisterKeypair.decode(p)];
        }
      } else {
        yield* [RegisterKeypair.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): RegisterKeypair {
    return {
      registerPeerId: isSet(object.registerPeerId) ? String(object.registerPeerId) : "",
      authMethodId: isSet(object.authMethodId) ? String(object.authMethodId) : "",
      authMethodState: isSet(object.authMethodState) ? bytesFromBase64(object.authMethodState) : new Uint8Array(),
    };
  },

  toJSON(message: RegisterKeypair): unknown {
    const obj: any = {};
    message.registerPeerId !== undefined && (obj.registerPeerId = message.registerPeerId);
    message.authMethodId !== undefined && (obj.authMethodId = message.authMethodId);
    message.authMethodState !== undefined &&
      (obj.authMethodState = base64FromBytes(
        message.authMethodState !== undefined ? message.authMethodState : new Uint8Array(),
      ));
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<RegisterKeypair>, I>>(object: I): RegisterKeypair {
    const message = createBaseRegisterKeypair();
    message.registerPeerId = object.registerPeerId ?? "";
    message.authMethodId = object.authMethodId ?? "";
    message.authMethodState = object.authMethodState ?? new Uint8Array();
    return message;
  },
};

function createBaseRemoveKeypair(): RemoveKeypair {
  return { peerId: "" };
}

export const RemoveKeypair = {
  encode(message: RemoveKeypair, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.peerId !== "") {
      writer.uint32(10).string(message.peerId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RemoveKeypair {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRemoveKeypair();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.peerId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RemoveKeypair, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<RemoveKeypair | RemoveKeypair[]> | Iterable<RemoveKeypair | RemoveKeypair[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveKeypair.encode(p).finish()];
        }
      } else {
        yield* [RemoveKeypair.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveKeypair>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoveKeypair> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveKeypair.decode(p)];
        }
      } else {
        yield* [RemoveKeypair.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): RemoveKeypair {
    return { peerId: isSet(object.peerId) ? String(object.peerId) : "" };
  },

  toJSON(message: RemoveKeypair): unknown {
    const obj: any = {};
    message.peerId !== undefined && (obj.peerId = message.peerId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<RemoveKeypair>, I>>(object: I): RemoveKeypair {
    const message = createBaseRemoveKeypair();
    message.peerId = object.peerId ?? "";
    return message;
  },
};

declare var self: any | undefined;
declare var window: any | undefined;
declare var global: any | undefined;
var globalThis: any = (() => {
  if (typeof globalThis !== "undefined") {
    return globalThis;
  }
  if (typeof self !== "undefined") {
    return self;
  }
  if (typeof window !== "undefined") {
    return window;
  }
  if (typeof global !== "undefined") {
    return global;
  }
  throw "Unable to locate global object";
})();

function bytesFromBase64(b64: string): Uint8Array {
  if (globalThis.Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, "base64"));
  } else {
    const bin = globalThis.atob(b64);
    const arr = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i);
    }
    return arr;
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (globalThis.Buffer) {
    return globalThis.Buffer.from(arr).toString("base64");
  } else {
    const bin: string[] = [];
    arr.forEach((byte) => {
      bin.push(String.fromCharCode(byte));
    });
    return globalThis.btoa(bin.join(""));
  }
}

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
