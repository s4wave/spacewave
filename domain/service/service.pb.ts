/* eslint-disable */
import { SignedMsg } from "@go/github.com/aperturerobotics/bifrost/peer/peer.pb.js";
import { Timestamp } from "@go/github.com/aperturerobotics/timestamp/timestamp.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { Entity } from "../../identity.pb.js";

export const protobufPackage = "identity.domain.service";

/** EntityLookupIdentifier is the identifier to search. */
export interface EntityLookupIdentifier {
  /** DomainId is the id of the domain to search. */
  domainId: string;
  /** EntityId is the id of the entity to search. */
  entityId: string;
}

/** LookupEntityReq is the signed body of LookupEntity. */
export interface LookupEntityReq {
  /** Identifier is the id to lookup. */
  identifier:
    | EntityLookupIdentifier
    | undefined;
  /** Timestamp is the timestamp of the request. */
  timestamp:
    | Timestamp
    | undefined;
  /** Nonce is a random one-time uint64. */
  nonce: Long;
}

/** LookupEntityResp is the response to the LookupEntity. */
export interface LookupEntityResp {
  /** Identifier is the echoed id of the request. */
  identifier:
    | EntityLookupIdentifier
    | undefined;
  /** LookupError contains any error looking up the entity. */
  lookupError: string;
  /** NotFound indicates if the error indicates a not found. */
  notFound: boolean;
  /** LookupEntity is the result of the lookup. */
  lookupEntity: Entity | undefined;
}

function createBaseEntityLookupIdentifier(): EntityLookupIdentifier {
  return { domainId: "", entityId: "" };
}

export const EntityLookupIdentifier = {
  encode(message: EntityLookupIdentifier, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.domainId !== "") {
      writer.uint32(10).string(message.domainId);
    }
    if (message.entityId !== "") {
      writer.uint32(18).string(message.entityId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EntityLookupIdentifier {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseEntityLookupIdentifier();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.domainId = reader.string();
          break;
        case 2:
          message.entityId = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EntityLookupIdentifier, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<EntityLookupIdentifier | EntityLookupIdentifier[]>
      | Iterable<EntityLookupIdentifier | EntityLookupIdentifier[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EntityLookupIdentifier.encode(p).finish()];
        }
      } else {
        yield* [EntityLookupIdentifier.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EntityLookupIdentifier>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<EntityLookupIdentifier> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EntityLookupIdentifier.decode(p)];
        }
      } else {
        yield* [EntityLookupIdentifier.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): EntityLookupIdentifier {
    return {
      domainId: isSet(object.domainId) ? String(object.domainId) : "",
      entityId: isSet(object.entityId) ? String(object.entityId) : "",
    };
  },

  toJSON(message: EntityLookupIdentifier): unknown {
    const obj: any = {};
    message.domainId !== undefined && (obj.domainId = message.domainId);
    message.entityId !== undefined && (obj.entityId = message.entityId);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<EntityLookupIdentifier>, I>>(object: I): EntityLookupIdentifier {
    const message = createBaseEntityLookupIdentifier();
    message.domainId = object.domainId ?? "";
    message.entityId = object.entityId ?? "";
    return message;
  },
};

function createBaseLookupEntityReq(): LookupEntityReq {
  return { identifier: undefined, timestamp: undefined, nonce: Long.UZERO };
}

export const LookupEntityReq = {
  encode(message: LookupEntityReq, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.identifier !== undefined) {
      EntityLookupIdentifier.encode(message.identifier, writer.uint32(10).fork()).ldelim();
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(18).fork()).ldelim();
    }
    if (!message.nonce.isZero()) {
      writer.uint32(24).uint64(message.nonce);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): LookupEntityReq {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLookupEntityReq();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.identifier = EntityLookupIdentifier.decode(reader, reader.uint32());
          break;
        case 2:
          message.timestamp = Timestamp.decode(reader, reader.uint32());
          break;
        case 3:
          message.nonce = reader.uint64() as Long;
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<LookupEntityReq, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<LookupEntityReq | LookupEntityReq[]> | Iterable<LookupEntityReq | LookupEntityReq[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LookupEntityReq.encode(p).finish()];
        }
      } else {
        yield* [LookupEntityReq.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, LookupEntityReq>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<LookupEntityReq> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LookupEntityReq.decode(p)];
        }
      } else {
        yield* [LookupEntityReq.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): LookupEntityReq {
    return {
      identifier: isSet(object.identifier) ? EntityLookupIdentifier.fromJSON(object.identifier) : undefined,
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
      nonce: isSet(object.nonce) ? Long.fromValue(object.nonce) : Long.UZERO,
    };
  },

  toJSON(message: LookupEntityReq): unknown {
    const obj: any = {};
    message.identifier !== undefined &&
      (obj.identifier = message.identifier ? EntityLookupIdentifier.toJSON(message.identifier) : undefined);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    message.nonce !== undefined && (obj.nonce = (message.nonce || Long.UZERO).toString());
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<LookupEntityReq>, I>>(object: I): LookupEntityReq {
    const message = createBaseLookupEntityReq();
    message.identifier = (object.identifier !== undefined && object.identifier !== null)
      ? EntityLookupIdentifier.fromPartial(object.identifier)
      : undefined;
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    message.nonce = (object.nonce !== undefined && object.nonce !== null) ? Long.fromValue(object.nonce) : Long.UZERO;
    return message;
  },
};

function createBaseLookupEntityResp(): LookupEntityResp {
  return { identifier: undefined, lookupError: "", notFound: false, lookupEntity: undefined };
}

export const LookupEntityResp = {
  encode(message: LookupEntityResp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.identifier !== undefined) {
      EntityLookupIdentifier.encode(message.identifier, writer.uint32(10).fork()).ldelim();
    }
    if (message.lookupError !== "") {
      writer.uint32(18).string(message.lookupError);
    }
    if (message.notFound === true) {
      writer.uint32(24).bool(message.notFound);
    }
    if (message.lookupEntity !== undefined) {
      Entity.encode(message.lookupEntity, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): LookupEntityResp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLookupEntityResp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.identifier = EntityLookupIdentifier.decode(reader, reader.uint32());
          break;
        case 2:
          message.lookupError = reader.string();
          break;
        case 3:
          message.notFound = reader.bool();
          break;
        case 4:
          message.lookupEntity = Entity.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<LookupEntityResp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<LookupEntityResp | LookupEntityResp[]> | Iterable<LookupEntityResp | LookupEntityResp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LookupEntityResp.encode(p).finish()];
        }
      } else {
        yield* [LookupEntityResp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, LookupEntityResp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<LookupEntityResp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [LookupEntityResp.decode(p)];
        }
      } else {
        yield* [LookupEntityResp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): LookupEntityResp {
    return {
      identifier: isSet(object.identifier) ? EntityLookupIdentifier.fromJSON(object.identifier) : undefined,
      lookupError: isSet(object.lookupError) ? String(object.lookupError) : "",
      notFound: isSet(object.notFound) ? Boolean(object.notFound) : false,
      lookupEntity: isSet(object.lookupEntity) ? Entity.fromJSON(object.lookupEntity) : undefined,
    };
  },

  toJSON(message: LookupEntityResp): unknown {
    const obj: any = {};
    message.identifier !== undefined &&
      (obj.identifier = message.identifier ? EntityLookupIdentifier.toJSON(message.identifier) : undefined);
    message.lookupError !== undefined && (obj.lookupError = message.lookupError);
    message.notFound !== undefined && (obj.notFound = message.notFound);
    message.lookupEntity !== undefined &&
      (obj.lookupEntity = message.lookupEntity ? Entity.toJSON(message.lookupEntity) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<LookupEntityResp>, I>>(object: I): LookupEntityResp {
    const message = createBaseLookupEntityResp();
    message.identifier = (object.identifier !== undefined && object.identifier !== null)
      ? EntityLookupIdentifier.fromPartial(object.identifier)
      : undefined;
    message.lookupError = object.lookupError ?? "";
    message.notFound = object.notFound ?? false;
    message.lookupEntity = (object.lookupEntity !== undefined && object.lookupEntity !== null)
      ? Entity.fromPartial(object.lookupEntity)
      : undefined;
    return message;
  },
};

/** IdentityDomain implements Entity lookup with a remote service. */
export interface IdentityDomain {
  /** LookupEntity requests the Entity corresponding to an entity_id. */
  LookupEntity(request: SignedMsg): Promise<LookupEntityResp>;
}

export class IdentityDomainClientImpl implements IdentityDomain {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "identity.domain.service.IdentityDomain";
    this.rpc = rpc;
    this.LookupEntity = this.LookupEntity.bind(this);
  }
  LookupEntity(request: SignedMsg): Promise<LookupEntityResp> {
    const data = SignedMsg.encode(request).finish();
    const promise = this.rpc.request(this.service, "LookupEntity", data);
    return promise.then((data) => LookupEntityResp.decode(new _m0.Reader(data)));
  }
}

/** IdentityDomain implements Entity lookup with a remote service. */
export type IdentityDomainDefinition = typeof IdentityDomainDefinition;
export const IdentityDomainDefinition = {
  name: "IdentityDomain",
  fullName: "identity.domain.service.IdentityDomain",
  methods: {
    /** LookupEntity requests the Entity corresponding to an entity_id. */
    lookupEntity: {
      name: "LookupEntity",
      requestType: SignedMsg,
      requestStream: false,
      responseType: LookupEntityResp,
      responseStream: false,
      options: {},
    },
  },
} as const;

interface Rpc {
  request(service: string, method: string, data: Uint8Array): Promise<Uint8Array>;
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
