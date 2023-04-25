/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "worker.controller";

/** Config is the Worker controller configuration. */
export interface Config {
  /** EngineId is the world engine id. */
  engineId: string;
  /**
   * ObjectKey is the Worker object to attach to.
   * If not exists, waits for it to exist.
   */
  objectKey: string;
  /**
   * PeerId sets the peer id to use for worker operations.
   * PeerId must match one of the Keypair attached to the Worker.
   * Looks up the private key via GetPeerWithID on the bus.
   * If unset, uses GetPeerWithID and DeriveKeypair with all Worker keys.
   */
  peerId: string;
  /**
   * AssignSelf sets the AssignSelf option on the task controller.
   * indicates we want to run executions on this worker.
   */
  assignSelf: boolean;
}

function createBaseConfig(): Config {
  return { engineId: "", objectKey: "", peerId: "", assignSelf: false };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.engineId !== "") {
      writer.uint32(10).string(message.engineId);
    }
    if (message.objectKey !== "") {
      writer.uint32(18).string(message.objectKey);
    }
    if (message.peerId !== "") {
      writer.uint32(26).string(message.peerId);
    }
    if (message.assignSelf === true) {
      writer.uint32(32).bool(message.assignSelf);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.engineId = reader.string();
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.objectKey = reader.string();
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.peerId = reader.string();
          continue;
        case 4:
          if (tag != 32) {
            break;
          }

          message.assignSelf = reader.bool();
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
    return {
      engineId: isSet(object.engineId) ? String(object.engineId) : "",
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : "",
      peerId: isSet(object.peerId) ? String(object.peerId) : "",
      assignSelf: isSet(object.assignSelf) ? Boolean(object.assignSelf) : false,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.engineId !== undefined && (obj.engineId = message.engineId);
    message.objectKey !== undefined && (obj.objectKey = message.objectKey);
    message.peerId !== undefined && (obj.peerId = message.peerId);
    message.assignSelf !== undefined && (obj.assignSelf = message.assignSelf);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.engineId = object.engineId ?? "";
    message.objectKey = object.objectKey ?? "";
    message.peerId = object.peerId ?? "";
    message.assignSelf = object.assignSelf ?? false;
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
