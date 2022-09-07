/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { BlockRef } from "../../hydra/block/block.pb.js";
import { Timestamp } from "../../timestamp/timestamp.pb.js";
import { ValueSet } from "../target/target.pb.js";
import { Result } from "../value/value.pb.js";

export const protobufPackage = "forge.execution";

/** State contains the possible execution states. */
export enum State {
  /** ExecutionState_UNKNOWN - ExecutionState_UNKNOWN is the unknown type. */
  ExecutionState_UNKNOWN = 0,
  /**
   * ExecutionState_PENDING - ExecutionState_PENDING is the state before the execution starts.
   * Transitions to RUNNING when the assigned peer acks exec start.
   */
  ExecutionState_PENDING = 1,
  /** ExecutionState_RUNNING - ExecutionState_RUNNING is the state when the execution is running. */
  ExecutionState_RUNNING = 2,
  /**
   * ExecutionState_COMPLETE - ExecutionState_COMPLETE is the terminal state of the execution.
   * This includes both success and failure termination states.
   */
  ExecutionState_COMPLETE = 3,
  UNRECOGNIZED = -1,
}

export function stateFromJSON(object: any): State {
  switch (object) {
    case 0:
    case "ExecutionState_UNKNOWN":
      return State.ExecutionState_UNKNOWN;
    case 1:
    case "ExecutionState_PENDING":
      return State.ExecutionState_PENDING;
    case 2:
    case "ExecutionState_RUNNING":
      return State.ExecutionState_RUNNING;
    case 3:
    case "ExecutionState_COMPLETE":
      return State.ExecutionState_COMPLETE;
    case -1:
    case "UNRECOGNIZED":
    default:
      return State.UNRECOGNIZED;
  }
}

export function stateToJSON(object: State): string {
  switch (object) {
    case State.ExecutionState_UNKNOWN:
      return "ExecutionState_UNKNOWN";
    case State.ExecutionState_PENDING:
      return "ExecutionState_PENDING";
    case State.ExecutionState_RUNNING:
      return "ExecutionState_RUNNING";
    case State.ExecutionState_COMPLETE:
      return "ExecutionState_COMPLETE";
    case State.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

/**
 * World graph links:
 *  - <parent> -> usually a Pass which created the Execution
 */
export interface Execution {
  /** ExecutionState is the current state of the execution. */
  executionState: State;
  /**
   * PeerId is the identifier of the peer assigned to the execution.
   * Can be empty.
   */
  peerId: string;
  /**
   * Timestamp is the time the parent object (usually Pass) was created.
   * Used as a reference timestamp to make all ops deterministic.
   * Must be set & is not updated.
   */
  timestamp:
    | Timestamp
    | undefined;
  /**
   * ValueSet is the set of inputs and outputs used in the execution.
   * Outputs are updated while the execution is in RUNNING state.
   */
  valueSet:
    | ValueSet
    | undefined;
  /** TargetRef is the block ref to the Target block. */
  targetRef:
    | BlockRef
    | undefined;
  /** Result is information about the outcome of a completed execution. */
  result: Result | undefined;
}

/** Spec contains information specified when creating a Execution. */
export interface Spec {
  /**
   * PeerId is the identifier of the peer assigned to the execution.
   * Can be empty.
   */
  peerId: string;
  /**
   * ValueSet is the set of inputs and outputs used in the execution.
   * Specified output values are used as initial values, and can be overridden.
   */
  valueSet:
    | ValueSet
    | undefined;
  /**
   * TargetRef is the target to run in the Execution.
   * Overrides "target" field if set.
   */
  targetRef: BlockRef | undefined;
}

function createBaseExecution(): Execution {
  return {
    executionState: 0,
    peerId: "",
    timestamp: undefined,
    valueSet: undefined,
    targetRef: undefined,
    result: undefined,
  };
}

export const Execution = {
  encode(message: Execution, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.executionState !== 0) {
      writer.uint32(8).int32(message.executionState);
    }
    if (message.peerId !== "") {
      writer.uint32(18).string(message.peerId);
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(26).fork()).ldelim();
    }
    if (message.valueSet !== undefined) {
      ValueSet.encode(message.valueSet, writer.uint32(34).fork()).ldelim();
    }
    if (message.targetRef !== undefined) {
      BlockRef.encode(message.targetRef, writer.uint32(42).fork()).ldelim();
    }
    if (message.result !== undefined) {
      Result.encode(message.result, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Execution {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseExecution();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.executionState = reader.int32() as any;
          break;
        case 2:
          message.peerId = reader.string();
          break;
        case 3:
          message.timestamp = Timestamp.decode(reader, reader.uint32());
          break;
        case 4:
          message.valueSet = ValueSet.decode(reader, reader.uint32());
          break;
        case 5:
          message.targetRef = BlockRef.decode(reader, reader.uint32());
          break;
        case 6:
          message.result = Result.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Execution, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Execution | Execution[]> | Iterable<Execution | Execution[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Execution.encode(p).finish()];
        }
      } else {
        yield* [Execution.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Execution>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Execution> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Execution.decode(p)];
        }
      } else {
        yield* [Execution.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Execution {
    return {
      executionState: isSet(object.executionState) ? stateFromJSON(object.executionState) : 0,
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      timestamp: isSet(object.timestamp) ? Timestamp.fromJSON(object.timestamp) : undefined,
      valueSet: isSet(object.valueSet) ? ValueSet.fromJSON(object.valueSet) : undefined,
      targetRef: isSet(object.targetRef) ? BlockRef.fromJSON(object.targetRef) : undefined,
      result: isSet(object.result) ? Result.fromJSON(object.result) : undefined,
    };
  },

  toJSON(message: Execution): unknown {
    const obj: any = {};
    message.executionState !== undefined && (obj.executionState = stateToJSON(message.executionState));
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.timestamp !== undefined &&
      (obj.timestamp = message.timestamp ? Timestamp.toJSON(message.timestamp) : undefined);
    message.valueSet !== undefined && (obj.valueSet = message.valueSet ? ValueSet.toJSON(message.valueSet) : undefined);
    message.targetRef !== undefined &&
      (obj.targetRef = message.targetRef ? BlockRef.toJSON(message.targetRef) : undefined);
    message.result !== undefined && (obj.result = message.result ? Result.toJSON(message.result) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Execution>, I>>(object: I): Execution {
    const message = createBaseExecution();
    message.executionState = object.executionState ?? 0;
    message.peerId = object.peerId ?? "";
    message.timestamp = (object.timestamp !== undefined && object.timestamp !== null)
      ? Timestamp.fromPartial(object.timestamp)
      : undefined;
    message.valueSet = (object.valueSet !== undefined && object.valueSet !== null)
      ? ValueSet.fromPartial(object.valueSet)
      : undefined;
    message.targetRef = (object.targetRef !== undefined && object.targetRef !== null)
      ? BlockRef.fromPartial(object.targetRef)
      : undefined;
    message.result = (object.result !== undefined && object.result !== null)
      ? Result.fromPartial(object.result)
      : undefined;
    return message;
  },
};

function createBaseSpec(): Spec {
  return { peerId: "", valueSet: undefined, targetRef: undefined };
}

export const Spec = {
  encode(message: Spec, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.peerId !== "") {
      writer.uint32(10).string(message.peerId);
    }
    if (message.valueSet !== undefined) {
      ValueSet.encode(message.valueSet, writer.uint32(18).fork()).ldelim();
    }
    if (message.targetRef !== undefined) {
      BlockRef.encode(message.targetRef, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Spec {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSpec();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.peerId = reader.string();
          break;
        case 2:
          message.valueSet = ValueSet.decode(reader, reader.uint32());
          break;
        case 3:
          message.targetRef = BlockRef.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Spec, Uint8Array>
  async *encodeTransform(source: AsyncIterable<Spec | Spec[]> | Iterable<Spec | Spec[]>): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Spec.encode(p).finish()];
        }
      } else {
        yield* [Spec.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Spec>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Spec> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Spec.decode(p)];
        }
      } else {
        yield* [Spec.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Spec {
    return {
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      valueSet: isSet(object.valueSet) ? ValueSet.fromJSON(object.valueSet) : undefined,
      targetRef: isSet(object.targetRef) ? BlockRef.fromJSON(object.targetRef) : undefined,
    };
  },

  toJSON(message: Spec): unknown {
    const obj: any = {};
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.valueSet !== undefined && (obj.valueSet = message.valueSet ? ValueSet.toJSON(message.valueSet) : undefined);
    message.targetRef !== undefined &&
      (obj.targetRef = message.targetRef ? BlockRef.toJSON(message.targetRef) : undefined);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<Spec>, I>>(object: I): Spec {
    const message = createBaseSpec();
    message.peerId = object.peerId ?? "";
    message.valueSet = (object.valueSet !== undefined && object.valueSet !== null)
      ? ValueSet.fromPartial(object.valueSet)
      : undefined;
    message.targetRef = (object.targetRef !== undefined && object.targetRef !== null)
      ? BlockRef.fromPartial(object.targetRef)
      : undefined;
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
