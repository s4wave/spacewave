/* eslint-disable */
import { Keypair } from "@go/github.com/aperturerobotics/identity/identity.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "forge.worker";

/**
 * Worker associates a set of keypairs with a worker name.
 * The name is used for API calls and command-line / UI tools.
 * The keypairs are load-balance assigned tasks to run by the scheduler.
 * Worker automatically starts controllers for all assigned objects.
 *
 * Graph links:
 * - identity/keypair-link: links to Keypairs representing the Worker.
 * Incoming links:
 * - forge/cluster-worker: links from Cluster to Worker.
 */
export interface Worker {
  /**
   * Name is the human readable worker name.
   * Example: "my-worker-1"
   * Must be a valid DNS label as defined in RFC 1123.
   */
  name: string;
}

/** WorkerCreateOp creates a Worker. */
export interface WorkerCreateOp {
  /** ObjectKey is the object key to create the Worker. */
  objectKey: string;
  /** Name is the worker name to create. */
  name: string;
  /**
   * Keypairs is the list of keypairs to add and/or link to the worker.
   * Must not have duplicates.
   * Optional.
   */
  keypairs: Keypair[];
}

function createBaseWorker(): Worker {
  return { name: "" };
}

export const Worker = {
  encode(message: Worker, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Worker {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseWorker();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Worker, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Worker | Worker[]> | Iterable<Worker | Worker[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Worker.encode(p).finish()];
        }
      } else {
        yield* [Worker.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Worker>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Worker> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Worker.decode(p)];
        }
      } else {
        yield* [Worker.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Worker {
    return { name: isSet(object.name) ? String(object.name) : "" };
  },

  toJSON(message: Worker): unknown {
    const obj: any = {};
    message.name !== undefined && (obj.name = message.name);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Worker>, I>>(object: I): Worker {
    const message = createBaseWorker();
    message.name = object.name ?? "";
    return message;
  },
};

function createBaseWorkerCreateOp(): WorkerCreateOp {
  return { objectKey: "", name: "", keypairs: [] };
}

export const WorkerCreateOp = {
  encode(message: WorkerCreateOp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKey !== "") {
      writer.uint32(10).string(message.objectKey);
    }
    if (message.name !== "") {
      writer.uint32(18).string(message.name);
    }
    for (const v of message.keypairs) {
      Keypair.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WorkerCreateOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseWorkerCreateOp();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.objectKey = reader.string();
          break;
        case 2:
          message.name = reader.string();
          break;
        case 3:
          message.keypairs.push(Keypair.decode(reader, reader.uint32()));
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<WorkerCreateOp, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<WorkerCreateOp | WorkerCreateOp[]> | Iterable<WorkerCreateOp | WorkerCreateOp[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WorkerCreateOp.encode(p).finish()];
        }
      } else {
        yield* [WorkerCreateOp.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WorkerCreateOp>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WorkerCreateOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WorkerCreateOp.decode(p)];
        }
      } else {
        yield* [WorkerCreateOp.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): WorkerCreateOp {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      name: isSet(object.name) ? String(object.name) : "",
      keypairs: Array.isArray(object?.keypairs) ? object.keypairs.map((e: any) => Keypair.fromJSON(e)) : [],
    };
  },

  toJSON(message: WorkerCreateOp): unknown {
    const obj: any = {};
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.name !== undefined && (obj.name = message.name);
    if (message.keypairs) {
      obj.keypairs = message.keypairs.map((e) => e ? Keypair.toJSON(e) : undefined);
    } else {
      obj.keypairs = [];
    }
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<WorkerCreateOp>, I>>(object: I): WorkerCreateOp {
    const message = createBaseWorkerCreateOp();
    message.objectKey = object.objectKey ?? "";
    message.name = object.name ?? "";
    message.keypairs = object.keypairs?.map((e) => Keypair.fromPartial(e)) || [];
    return message;
  },
};

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
