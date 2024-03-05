/* eslint-disable */
import { Backoff } from "@go/github.com/aperturerobotics/util/backoff/backoff.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "devtool.web.entrypoint.plugin_host";

/** Config is the devtool web entrypoint plugin host controller config. */
export interface Config {
  /** VolumeId is the volume id to use for plugin storage (the devtool volume). */
  volumeId: string;
  /**
   * FetchBackoff is the backoff config for fetching plugin manifests.
   * If unset, defaults to reasonable defaults.
   */
  fetchBackoff:
    | Backoff
    | undefined;
  /**
   * ExecBackoff is the backoff config for executing plugin manifests.
   * If unset, defaults to reasonable defaults.
   */
  execBackoff: Backoff | undefined;
}

function createBaseConfig(): Config {
  return { volumeId: "", fetchBackoff: undefined, execBackoff: undefined };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.volumeId !== "") {
      writer.uint32(10).string(message.volumeId);
    }
    if (message.fetchBackoff !== undefined) {
      Backoff.encode(message.fetchBackoff, writer.uint32(18).fork()).ldelim();
    }
    if (message.execBackoff !== undefined) {
      Backoff.encode(message.execBackoff, writer.uint32(26).fork()).ldelim();
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
          if (tag !== 10) {
            break;
          }

          message.volumeId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.fetchBackoff = Backoff.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.execBackoff = Backoff.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
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
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config.encode(p).finish()];
        }
      } else {
        yield* [Config.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [Config.decode(p)];
        }
      } else {
        yield* [Config.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      volumeId: isSet(object.volumeId) ? globalThis.String(object.volumeId) : "",
      fetchBackoff: isSet(object.fetchBackoff) ? Backoff.fromJSON(object.fetchBackoff) : undefined,
      execBackoff: isSet(object.execBackoff) ? Backoff.fromJSON(object.execBackoff) : undefined,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    if (message.volumeId !== "") {
      obj.volumeId = message.volumeId;
    }
    if (message.fetchBackoff !== undefined) {
      obj.fetchBackoff = Backoff.toJSON(message.fetchBackoff);
    }
    if (message.execBackoff !== undefined) {
      obj.execBackoff = Backoff.toJSON(message.execBackoff);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.volumeId = object.volumeId ?? "";
    message.fetchBackoff = (object.fetchBackoff !== undefined && object.fetchBackoff !== null)
      ? Backoff.fromPartial(object.fetchBackoff)
      : undefined;
    message.execBackoff = (object.execBackoff !== undefined && object.execBackoff !== null)
      ? Backoff.fromPartial(object.execBackoff)
      : undefined;
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends Long ? string | number | Long : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
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
