/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { ValueSet } from "../../target/target.pb.js";
import { Result } from "../../value/value.pb.js";

export const protobufPackage = "pass.tx";

/** TxType indicates the kind of transaction. */
export enum TxType {
  TxType_INVALID = 0,
  /**
   * TxType_START - TxType_START marks the pass as running.
   * Transitions to state RUNNING from PENDING.
   */
  TxType_START = 1,
  /**
   * TxType_CREATE_EXEC_SPECS - TxType_CREATE_EXEC_SPECS creates execution objects with specs.
   * Overwrites any already-existing executions with matching peer ids.
   * Can optionally clear the list of executions before creating.
   * Used to add / reset execution instances.
   */
  TxType_CREATE_EXEC_SPECS = 2,
  /**
   * TxType_UPDATE_EXEC_STATES - TxType_UPDATE_EXEC_STATES updates the execution states.
   * Searches for valid Execution objects linked to the Pass.
   * Updates the list with the found objects.
   * Transitions to CHECKING if terminal conditions are found.
   * If any assigned executions fail, the Pass will also fail.
   */
  TxType_UPDATE_EXEC_STATES = 3,
  /**
   * TxType_COMPLETE - TxType_COMPLETE sets the result of the execution.
   * If failed, can transition from any state.
   * If success, must transition from CHECKING state.
   * If success, all Execution states must be Successful.
   */
  TxType_COMPLETE = 4,
  UNRECOGNIZED = -1,
}

export function txTypeFromJSON(object: any): TxType {
  switch (object) {
    case 0:
    case "TxType_INVALID":
      return TxType.TxType_INVALID;
    case 1:
    case "TxType_START":
      return TxType.TxType_START;
    case 2:
    case "TxType_CREATE_EXEC_SPECS":
      return TxType.TxType_CREATE_EXEC_SPECS;
    case 3:
    case "TxType_UPDATE_EXEC_STATES":
      return TxType.TxType_UPDATE_EXEC_STATES;
    case 4:
    case "TxType_COMPLETE":
      return TxType.TxType_COMPLETE;
    case -1:
    case "UNRECOGNIZED":
    default:
      return TxType.UNRECOGNIZED;
  }
}

export function txTypeToJSON(object: TxType): string {
  switch (object) {
    case TxType.TxType_INVALID:
      return "TxType_INVALID";
    case TxType.TxType_START:
      return "TxType_START";
    case TxType.TxType_CREATE_EXEC_SPECS:
      return "TxType_CREATE_EXEC_SPECS";
    case TxType.TxType_UPDATE_EXEC_STATES:
      return "TxType_UPDATE_EXEC_STATES";
    case TxType.TxType_COMPLETE:
      return "TxType_COMPLETE";
    case TxType.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

/** Tx is the on-the-wire representation of a transaction. */
export interface Tx {
  /** TxType is the kind of transaction this is. */
  txType: TxType;
  /**
   * PassObjectKey is the Pass object ID this is associated with.
   * The Pass object must already exist.
   */
  passObjectKey: string;
  /**
   * TxStart contains the start transaction tx.
   * TxType_START
   */
  txStart:
    | TxStart
    | undefined;
  /**
   * TxCreateExecSpecs contains the create exec specs tx.
   * TxType_CREATE_EXEC_SPECS
   */
  txCreateExecSpecs:
    | TxCreateExecSpecs
    | undefined;
  /**
   * TxUpdateExecStates contains the update exec states tx.
   * TxType_EXEC_COMPLETE
   */
  txUpdateExecStates:
    | TxUpdateExecStates
    | undefined;
  /**
   * TxComplete contains the complete tx.
   * TxType_COMPLETE
   */
  txComplete: TxComplete | undefined;
}

/** ExecSpec contains a specification for creating an Execution. */
export interface ExecSpec {
  /**
   * PeerId is the identifier of the peer assigned to the execution.
   * Cannot be empty.
   */
  peerId: string;
}

/**
 * TxStart starts the execution of the pass, optionally creating Executions.
 *
 * Executes UpdateExecStates as a sub-transaction to scan for Execution objects.
 * Executes CreateExecSpecs as a nested transaction to create initial state.
 * TxType: TxType_START
 */
export interface TxStart {
  /** CreateExecSpecs is the nested create exec specs transaction. */
  createExecSpecs: TxCreateExecSpecs | undefined;
}

/**
 * TxCreateExecSpecs creates a set of Execution objects from the specs.
 * Overwrites any already-existing executions with matching peer ids.
 * TxType: TxType_CREATE_EXEC_SPECS
 */
export interface TxCreateExecSpecs {
  /** ExecSpecs contains specifications for execution objects to create. */
  execSpecs: ExecSpec[];
  /**
   * ClearExisting deletes all already-existing associated exec states.
   * If set and len(exec_specs) == 0, clears all execution instances.
   */
  clearExisting: boolean;
}

/**
 * TxUpdateExecStates updates the list of execution states for the Pass.
 * Updates the list with objects found via graph quad lookup.
 * Transitions to CHECKING if terminal conditions are found.
 * If any assigned executions fail, the Pass will also fail.
 * TxType: TxType_UPDATE_EXEC_STATES
 */
export interface TxUpdateExecStates {
}

/**
 * TxComplete completes the execution by setting the result.
 * If failed, may transition from any state.
 * If success, must be in the CHECKING state.
 * If success, all Exections must be in COMPLETE state and not failed.
 * TxType: TxType_COMPLETE
 */
export interface TxComplete {
  /** Result is information about the outcome of a completed pass. */
  result:
    | Result
    | undefined;
  /**
   * ValueSet is the set of outputs from the Pass.
   * Inputs must be empty.
   * Must match the outputs calculated from the Execution objects.
   * Must be empty if the result is not success.
   */
  valueSet: ValueSet | undefined;
}

function createBaseTx(): Tx {
  return {
    txType: 0,
    passObjectKey: "",
    txStart: undefined,
    txCreateExecSpecs: undefined,
    txUpdateExecStates: undefined,
    txComplete: undefined,
  };
}

export const Tx = {
  encode(message: Tx, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.txType !== 0) {
      writer.uint32(8).int32(message.txType);
    }
    if (message.passObjectKey !== "") {
      writer.uint32(18).string(message.passObjectKey);
    }
    if (message.txStart !== undefined) {
      TxStart.encode(message.txStart, writer.uint32(26).fork()).ldelim();
    }
    if (message.txCreateExecSpecs !== undefined) {
      TxCreateExecSpecs.encode(message.txCreateExecSpecs, writer.uint32(34).fork()).ldelim();
    }
    if (message.txUpdateExecStates !== undefined) {
      TxUpdateExecStates.encode(message.txUpdateExecStates, writer.uint32(42).fork()).ldelim();
    }
    if (message.txComplete !== undefined) {
      TxComplete.encode(message.txComplete, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Tx {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTx();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 8) {
            break;
          }

          message.txType = reader.int32() as any;
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.passObjectKey = reader.string();
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.txStart = TxStart.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag != 34) {
            break;
          }

          message.txCreateExecSpecs = TxCreateExecSpecs.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag != 42) {
            break;
          }

          message.txUpdateExecStates = TxUpdateExecStates.decode(reader, reader.uint32());
          continue;
        case 6:
          if (tag != 50) {
            break;
          }

          message.txComplete = TxComplete.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Tx, Uint8Array>
  async *encodeTransform(source: AsyncIterable<Tx | Tx[]> | Iterable<Tx | Tx[]>): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Tx.encode(p).finish()];
        }
      } else {
        yield* [Tx.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Tx>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Tx> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Tx.decode(p)];
        }
      } else {
        yield* [Tx.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Tx {
    return {
      txType: isSet(object.txType) ? txTypeFromJSON(object.txType) : 0,
      passObjectKey: isSet(object.passObjectKey) ? String(object.passObjectKey) : "",
      txStart: isSet(object.txStart) ? TxStart.fromJSON(object.txStart) : undefined,
      txCreateExecSpecs: isSet(object.txCreateExecSpecs)
        ? TxCreateExecSpecs.fromJSON(object.txCreateExecSpecs)
        : undefined,
      txUpdateExecStates: isSet(object.txUpdateExecStates)
        ? TxUpdateExecStates.fromJSON(object.txUpdateExecStates)
        : undefined,
      txComplete: isSet(object.txComplete) ? TxComplete.fromJSON(object.txComplete) : undefined,
    };
  },

  toJSON(message: Tx): unknown {
    const obj: any = {};
    message.txType !== undefined && (obj.txType = txTypeToJSON(message.txType));
    message.passObjectKey !== undefined && (obj.passObjectKey = message.passObjectKey);
    message.txStart !== undefined && (obj.txStart = message.txStart ? TxStart.toJSON(message.txStart) : undefined);
    message.txCreateExecSpecs !== undefined &&
      (obj.txCreateExecSpecs = message.txCreateExecSpecs
        ? TxCreateExecSpecs.toJSON(message.txCreateExecSpecs)
        : undefined);
    message.txUpdateExecStates !== undefined &&
      (obj.txUpdateExecStates = message.txUpdateExecStates
        ? TxUpdateExecStates.toJSON(message.txUpdateExecStates)
        : undefined);
    message.txComplete !== undefined &&
      (obj.txComplete = message.txComplete ? TxComplete.toJSON(message.txComplete) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<Tx>, I>>(base?: I): Tx {
    return Tx.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Tx>, I>>(object: I): Tx {
    const message = createBaseTx();
    message.txType = object.txType ?? 0;
    message.passObjectKey = object.passObjectKey ?? "";
    message.txStart = (object.txStart !== undefined && object.txStart !== null)
      ? TxStart.fromPartial(object.txStart)
      : undefined;
    message.txCreateExecSpecs = (object.txCreateExecSpecs !== undefined && object.txCreateExecSpecs !== null)
      ? TxCreateExecSpecs.fromPartial(object.txCreateExecSpecs)
      : undefined;
    message.txUpdateExecStates = (object.txUpdateExecStates !== undefined && object.txUpdateExecStates !== null)
      ? TxUpdateExecStates.fromPartial(object.txUpdateExecStates)
      : undefined;
    message.txComplete = (object.txComplete !== undefined && object.txComplete !== null)
      ? TxComplete.fromPartial(object.txComplete)
      : undefined;
    return message;
  },
};

function createBaseExecSpec(): ExecSpec {
  return { peerId: "" };
}

export const ExecSpec = {
  encode(message: ExecSpec, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.peerId !== "") {
      writer.uint32(10).string(message.peerId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ExecSpec {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseExecSpec();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.peerId = reader.string();
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ExecSpec, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<ExecSpec | ExecSpec[]> | Iterable<ExecSpec | ExecSpec[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExecSpec.encode(p).finish()];
        }
      } else {
        yield* [ExecSpec.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ExecSpec>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ExecSpec> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExecSpec.decode(p)];
        }
      } else {
        yield* [ExecSpec.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): ExecSpec {
    return { peerId: isSet(object.peerId) ? String(object.peerId) : "" };
  },

  toJSON(message: ExecSpec): unknown {
    const obj: any = {};
    message.peerId !== undefined && (obj.peerId = message.peerId);
    return obj;
  },

  create<I extends Exact<DeepPartial<ExecSpec>, I>>(base?: I): ExecSpec {
    return ExecSpec.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<ExecSpec>, I>>(object: I): ExecSpec {
    const message = createBaseExecSpec();
    message.peerId = object.peerId ?? "";
    return message;
  },
};

function createBaseTxStart(): TxStart {
  return { createExecSpecs: undefined };
}

export const TxStart = {
  encode(message: TxStart, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.createExecSpecs !== undefined) {
      TxCreateExecSpecs.encode(message.createExecSpecs, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxStart {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTxStart();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.createExecSpecs = TxCreateExecSpecs.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TxStart, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<TxStart | TxStart[]> | Iterable<TxStart | TxStart[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxStart.encode(p).finish()];
        }
      } else {
        yield* [TxStart.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxStart>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxStart> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxStart.decode(p)];
        }
      } else {
        yield* [TxStart.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): TxStart {
    return {
      createExecSpecs: isSet(object.createExecSpecs) ? TxCreateExecSpecs.fromJSON(object.createExecSpecs) : undefined,
    };
  },

  toJSON(message: TxStart): unknown {
    const obj: any = {};
    message.createExecSpecs !== undefined &&
      (obj.createExecSpecs = message.createExecSpecs ? TxCreateExecSpecs.toJSON(message.createExecSpecs) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<TxStart>, I>>(base?: I): TxStart {
    return TxStart.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<TxStart>, I>>(object: I): TxStart {
    const message = createBaseTxStart();
    message.createExecSpecs = (object.createExecSpecs !== undefined && object.createExecSpecs !== null)
      ? TxCreateExecSpecs.fromPartial(object.createExecSpecs)
      : undefined;
    return message;
  },
};

function createBaseTxCreateExecSpecs(): TxCreateExecSpecs {
  return { execSpecs: [], clearExisting: false };
}

export const TxCreateExecSpecs = {
  encode(message: TxCreateExecSpecs, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.execSpecs) {
      ExecSpec.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.clearExisting === true) {
      writer.uint32(16).bool(message.clearExisting);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxCreateExecSpecs {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTxCreateExecSpecs();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.execSpecs.push(ExecSpec.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag != 16) {
            break;
          }

          message.clearExisting = reader.bool();
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TxCreateExecSpecs, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<TxCreateExecSpecs | TxCreateExecSpecs[]> | Iterable<TxCreateExecSpecs | TxCreateExecSpecs[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxCreateExecSpecs.encode(p).finish()];
        }
      } else {
        yield* [TxCreateExecSpecs.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxCreateExecSpecs>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxCreateExecSpecs> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxCreateExecSpecs.decode(p)];
        }
      } else {
        yield* [TxCreateExecSpecs.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): TxCreateExecSpecs {
    return {
      execSpecs: Array.isArray(object?.execSpecs) ? object.execSpecs.map((e: any) => ExecSpec.fromJSON(e)) : [],
      clearExisting: isSet(object.clearExisting) ? Boolean(object.clearExisting) : false,
    };
  },

  toJSON(message: TxCreateExecSpecs): unknown {
    const obj: any = {};
    if (message.execSpecs) {
      obj.execSpecs = message.execSpecs.map((e) => e ? ExecSpec.toJSON(e) : undefined);
    } else {
      obj.execSpecs = [];
    }
    message.clearExisting !== undefined && (obj.clearExisting = message.clearExisting);
    return obj;
  },

  create<I extends Exact<DeepPartial<TxCreateExecSpecs>, I>>(base?: I): TxCreateExecSpecs {
    return TxCreateExecSpecs.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<TxCreateExecSpecs>, I>>(object: I): TxCreateExecSpecs {
    const message = createBaseTxCreateExecSpecs();
    message.execSpecs = object.execSpecs?.map((e) => ExecSpec.fromPartial(e)) || [];
    message.clearExisting = object.clearExisting ?? false;
    return message;
  },
};

function createBaseTxUpdateExecStates(): TxUpdateExecStates {
  return {};
}

export const TxUpdateExecStates = {
  encode(_: TxUpdateExecStates, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxUpdateExecStates {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTxUpdateExecStates();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TxUpdateExecStates, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxUpdateExecStates | TxUpdateExecStates[]>
      | Iterable<TxUpdateExecStates | TxUpdateExecStates[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxUpdateExecStates.encode(p).finish()];
        }
      } else {
        yield* [TxUpdateExecStates.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxUpdateExecStates>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxUpdateExecStates> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxUpdateExecStates.decode(p)];
        }
      } else {
        yield* [TxUpdateExecStates.decode(pkt)];
      }
    }
  },

  fromJSON(_: any): TxUpdateExecStates {
    return {};
  },

  toJSON(_: TxUpdateExecStates): unknown {
    const obj: any = {};
    return obj;
  },

  create<I extends Exact<DeepPartial<TxUpdateExecStates>, I>>(base?: I): TxUpdateExecStates {
    return TxUpdateExecStates.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<TxUpdateExecStates>, I>>(_: I): TxUpdateExecStates {
    const message = createBaseTxUpdateExecStates();
    return message;
  },
};

function createBaseTxComplete(): TxComplete {
  return { result: undefined, valueSet: undefined };
}

export const TxComplete = {
  encode(message: TxComplete, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.result !== undefined) {
      Result.encode(message.result, writer.uint32(10).fork()).ldelim();
    }
    if (message.valueSet !== undefined) {
      ValueSet.encode(message.valueSet, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxComplete {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTxComplete();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.result = Result.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.valueSet = ValueSet.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TxComplete, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<TxComplete | TxComplete[]> | Iterable<TxComplete | TxComplete[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxComplete.encode(p).finish()];
        }
      } else {
        yield* [TxComplete.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxComplete>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TxComplete> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxComplete.decode(p)];
        }
      } else {
        yield* [TxComplete.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): TxComplete {
    return {
      result: isSet(object.result) ? Result.fromJSON(object.result) : undefined,
      valueSet: isSet(object.valueSet) ? ValueSet.fromJSON(object.valueSet) : undefined,
    };
  },

  toJSON(message: TxComplete): unknown {
    const obj: any = {};
    message.result !== undefined && (obj.result = message.result ? Result.toJSON(message.result) : undefined);
    message.valueSet !== undefined && (obj.valueSet = message.valueSet ? ValueSet.toJSON(message.valueSet) : undefined);
    return obj;
  },

  create<I extends Exact<DeepPartial<TxComplete>, I>>(base?: I): TxComplete {
    return TxComplete.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<TxComplete>, I>>(object: I): TxComplete {
    const message = createBaseTxComplete();
    message.result = (object.result !== undefined && object.result !== null)
      ? Result.fromPartial(object.result)
      : undefined;
    message.valueSet = (object.valueSet !== undefined && object.valueSet !== null)
      ? ValueSet.fromPartial(object.valueSet)
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
