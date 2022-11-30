/* eslint-disable */
import { Timestamp } from "@go/github.com/aperturerobotics/timestamp/timestamp.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "kvtx.mqueue";

/** Config is additional optional configuration for the kv mqueue. */
export interface Config {
  /**
   * PollDur is the duration between polling checks.
   *
   * Default: don't poll, assume we are the only writer.
   * Wait will wait until Push is called on the same instance.
   * A Peek() check while waiting can be triggered by code.
   */
  pollDur: string;
}

/** MQQueueMeta is queue metadata. */
export interface MQQueueMeta {
  /** Head is the head position. */
  head: Long;
  /** Tail is the tail position. */
  tail: Long;
  /** Meta is any extra key/value metadata. */
  meta: { [key: string]: string };
}

export interface MQQueueMeta_MetaEntry {
  key: string;
  value: string;
}

/** MQMessageWrapper is the message wrapper used to store data. */
export interface MQMessageWrapper {
  /** Timestamp is the message timestamp. */
  timestamp:
    | Timestamp
    | undefined;
  /** Data is the message data. */
  data: Uint8Array;
}

function createBaseConfig(): Config {
  return { pollDur: "" };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pollDur !== "") {
      writer.uint32(10).string(message.pollDur);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.pollDur = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()];
        }
      } else {
        yield* [Config.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)];
        }
      } else {
        yield* [Config.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config {
    return { pollDur: isSet(object.pollDur) ? String(object.pollDur) : "" };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.pollDur !== undefined && (obj.pollDur = message.pollDur);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.pollDur = object.pollDur ?? "";
    return message;
  },
};

function createBaseMQQueueMeta(): MQQueueMeta {
  return { head: Long.UZERO, tail: Long.UZERO, meta: {} };
}

export const MQQueueMeta = {
  encode(message: MQQueueMeta, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (!message.head.isZero()) {
      writer.uint32(8).uint64(message.head);
    }
    if (!message.tail.isZero()) {
      writer.uint32(16).uint64(message.tail);
    }
    Object.entries(message.meta).forEach(([key, value]) => {
      MQQueueMeta_MetaEntry.encode({ key: key as any, value }, writer.uint32(26).fork()).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MQQueueMeta {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMQQueueMeta();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.head = reader.uint64() as Long;
          break;
        case 2:
          message.tail = reader.uint64() as Long;
          break;
        case 3:
          const entry3 = MQQueueMeta_MetaEntry.decode(reader, reader.uint32());
          if (entry3.value !== undefined) {
            message.meta[entry3.key] = entry3.value;
          }
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<MQQueueMeta, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<MQQueueMeta | MQQueueMeta[]> | Iterable<MQQueueMeta | MQQueueMeta[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MQQueueMeta.encode(p).finish()];
        }
      } else {
        yield* [MQQueueMeta.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MQQueueMeta>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<MQQueueMeta> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MQQueueMeta.decode(p)];
        }
      } else {
        yield* [MQQueueMeta.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): MQQueueMeta {
    return {
      head: isSet(object.head) ? Long.fromValue(object.head) : Long.UZERO,
      tail: isSet(object.tail) ? Long.fromValue(object.tail) : Long.UZERO,
      meta: isObject(object.meta)
        ? Object.entries(object.meta).reduce<{ [key: string]: string }>((acc, [key, value]) => {
          acc[key] = String(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: MQQueueMeta): unknown {
    const obj: any = {};
    message.head !== undefined && (obj.head = (message.head || Long.UZERO).toString());
    message.tail !== undefined && (obj.tail = (message.tail || Long.UZERO).toString());
    obj.meta = {};
    if (message.meta) {
      Object.entries(message.meta).forEach(([k, v]) => {
        obj.meta[k] = v;
      });
    }
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<MQQueueMeta>, I>>(object: I): MQQueueMeta {
    const message = createBaseMQQueueMeta();
    message.head = (object.head !== undefined && object.head !== null) ? Long.fromValue(object.head) : Long.UZERO;
    message.tail = (object.tail !== undefined && object.tail !== null) ? Long.fromValue(object.tail) : Long.UZERO;
    message.meta = Object.entries(object.meta ?? {}).reduce<{ [key: string]: string }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = String(value);
      }
      return acc;
    }, {});
    return message;
  },
};

function createBaseMQQueueMeta_MetaEntry(): MQQueueMeta_MetaEntry {
  return { key: "", value: "" };
}

export const MQQueueMeta_MetaEntry = {
  encode(message: MQQueueMeta_MetaEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MQQueueMeta_MetaEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMQQueueMeta_MetaEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string();
          break;
        case 2:
          message.value = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<MQQueueMeta_MetaEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<MQQueueMeta_MetaEntry | MQQueueMeta_MetaEntry[]>
      | Iterable<MQQueueMeta_MetaEntry | MQQueueMeta_MetaEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MQQueueMeta_MetaEntry.encode(p).finish()];
        }
      } else {
        yield* [MQQueueMeta_MetaEntry.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MQQueueMeta_MetaEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<MQQueueMeta_MetaEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MQQueueMeta_MetaEntry.decode(p)];
        }
      } else {
        yield* [MQQueueMeta_MetaEntry.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): MQQueueMeta_MetaEntry {
    return { key: isSet(object.key) ? String(object.key) : "", value: isSet(object.value) ? String(object.value) : "" };
  },

  toJSON(message: MQQueueMeta_MetaEntry): unknown {
    const obj: any = {};
    message.key !== undefined && (obj.key = message.key);
    message.value !== undefined && (obj.value = message.value);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<MQQueueMeta_MetaEntry>, I>>(object: I): MQQueueMeta_MetaEntry {
    const message = createBaseMQQueueMeta_MetaEntry();
    message.key = object.key ?? "";
    message.value = object.value ?? "";
    return message;
  },
};

function createBaseMQMessageWrapper(): MQMessageWrapper {
  return { timestamp: undefined, data: new Uint8Array() };
}

export const MQMessageWrapper = {
  encode(message: MQMessageWrapper, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(10).fork()).ldelim();
    }
    if (message.data.length !== 0) {
      writer.uint32(18).bytes(message.data);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MQMessageWrapper {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMQMessageWrapper();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.timestamp = Timestamp.decode(reader, reader.uint32());
          break;
        case 2:
          message.data = reader.bytes();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<MQMessageWrapper, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<MQMessageWrapper | MQMessageWrapper[]> | Iterable<MQMessageWrapper | MQMessageWrapper[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MQMessageWrapper.encode(p).finish()];
        }
      } else {
        yield* [MQMessageWrapper.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, MQMessageWrapper>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<MQMessageWrapper> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [MQMessageWrapper.decode(p)];
        }
      } else {
        yield* [MQMessageWrapper.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): MQMessageWrapper {
    return {
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
      data: isSet(object.data) ? bytesFromBase64(object.data) : new Uint8Array(),
    };
  },

  toJSON(message: MQMessageWrapper): unknown {
    const obj: any = {};
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    message.data !== undefined &&
      (obj.data = base64FromBytes(message.data !== undefined ? message.data : new Uint8Array()));
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<MQMessageWrapper>, I>>(object: I): MQMessageWrapper {
    const message = createBaseMQMessageWrapper();
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    message.data = object.data ?? new Uint8Array();
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
